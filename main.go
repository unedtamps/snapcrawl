package main

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/playwright-community/playwright-go"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"

	"webscraper/internal/db"
	"webscraper/internal/models"
	"webscraper/internal/openai"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/index.html
var indexHTML []byte

var (
	database *db.DB
	pw       *playwright.Playwright
)

func main() {
	// Load .env file (override existing vars)
	if err := godotenv.Overload(); err != nil {
		log.Println("⚠️  No .env file found or error loading it")
	}

	log.Println("🚀 Starting SnapCrawl v1.0...")

	// Initialize Playwright
	if err := initPlaywright(); err != nil {
		log.Fatalf("Failed to initialize Playwright: %v", err)
	}
	log.Println("✅ Playwright initialized")

	// Initialize Database
	var err error
	database, err = db.New("scraper.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	log.Println("✅ Database initialized")

	// Setup routes
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(120 * time.Second))

	// Static files
	r.Handle("/static/*", http.FileServer(http.FS(staticFS)))

	// Main page
	r.Get("/", serveIndex)

	// Projects API
	r.Post("/projects", handleCreateProject)
	r.Get("/projects", handleGetProjects)
	r.Get("/projects/{id}", handleGetProject)
	r.Put("/projects/{id}", handleUpdateProject)
	r.Delete("/projects/{id}", handleDeleteProject)

	// Scrape API
	r.Post("/projects/{id}/scrape", handleProjectScrape)

	// Data export
	r.Get("/projects/{id}/data", handleGetProjectData)
	r.Get("/projects/{id}/data.csv", handleExportCSV)

	// AI schema generation
	r.Post("/api/generate-schema", handleGenerateSchema)

	// Direct AI API (for testing)
	r.Post("/api/v2/ai/scrape", handleAIScrapeDirect)

	// API Interface config
	r.Get("/projects/{id}/api-config", handleGetAPIConfig)
	r.Put("/projects/{id}/api-config", handleSaveAPIConfig)

	// Public scrape endpoint
	r.Get("/api/public/{id}/scrape", handlePublicScrape)

	// LLM Providers API
	r.Get("/api/providers", handleGetProviders)
	r.Post("/api/providers", handleCreateProvider)
	r.Delete("/api/providers/{id}", handleDeleteProvider)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🌐 Server running on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

// initPlaywright initializes Playwright and installs browsers if needed
func initPlaywright() error {
	p, err := playwright.Run()
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "could not run driver") ||
			strings.Contains(errStr, "not installed") ||
			strings.Contains(errStr, "please install") ||
			strings.Contains(errStr, "driver") {
			log.Println("📦 Playwright driver not found, installing...")
			if installErr := playwright.Install(); installErr != nil {
				return fmt.Errorf("failed to install playwright: %w", installErr)
			}
			log.Println("✅ Playwright installed, starting...")
			p, err = playwright.Run()
			if err != nil {
				return fmt.Errorf("failed to run playwright after install: %w", err)
			}
		} else {
			return fmt.Errorf("failed to run playwright: %w", err)
		}
	}
	pw = p
	return nil
}

// serveIndex renders the main HTML page
func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

// ============ Project Handlers ============

func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, `{"error": "Name is required"}`, http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	if err := database.CreateProject(id, req.Name, "", "{}", "", "deepseek", 1000); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, `{"error": "Project name already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error": "Failed to create project"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "name": req.Name})
}

func handleGetProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := database.GetAllProjects()
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch projects"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := database.GetProject(id)
	if err != nil {
		http.Error(w, `{"error": "Project not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req models.Project
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Validate schema JSON
	if req.Schema != "" {
		if _, err := json.RawMessage(req.Schema).MarshalJSON(); err != nil {
			http.Error(w, `{"error": "Invalid JSON schema"}`, http.StatusBadRequest)
			return
		}
	}

	if err := database.UpdateProject(id, req.Name, req.BaseURL, req.Schema, req.Prompt, req.Provider, req.DelayMs); err != nil {
		http.Error(w, `{"error": "Failed to update project"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := database.DeleteProject(id); err != nil {
		http.Error(w, `{"error": "Failed to delete project"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ============ Scrape Handlers ============

func getAIClient(providerID string) (*openai.Client, error) {
	if providerID == "" {
		return nil, fmt.Errorf("provider is required")
	}
	p, err := database.GetLLMProviderByID(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider details: %w", err)
	}
	cfg := &openai.Config{
		APIKey:  p.APIKey,
		BaseURL: p.BaseURL,
		Model:   p.ModelName,
	}
	return openai.New(cfg)
}

func handleProjectScrape(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Get project config
	project, err := database.GetProject(id)
	if err != nil {
		http.Error(w, `{"error": "Project not found"}`, http.StatusNotFound)
		return
	}

	client, err := getAIClient(project.Provider)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "AI client initialization failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if project.BaseURL == "" {
		http.Error(w, `{"error": "Base URL not configured"}`, http.StatusBadRequest)
		return
	}

	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(project.Schema), &schema); err != nil {
		http.Error(w, `{"error": "Invalid schema in project config"}`, http.StatusInternalServerError)
		return
	}

	// Scrape using OpenAI
	start := time.Now()
	// Fetch page content as Markdown
	pageContent, fetchErr := fetchPageContent(project.BaseURL)
	if fetchErr != nil {
		log.Printf("Warning: failed to fetch page content: %v", fetchErr)
		pageContent = ""
	}

	result, err := client.ExtractData(r.Context(), &openai.ExtractionRequest{
		URL:     project.BaseURL,
		Content: pageContent,
		Schema:  schema,
		Prompt:  project.Prompt,
	})
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if result != nil && result.Error != "" {
		http.Error(w, fmt.Sprintf(`{"error": "Extraction failed: %s"}`, result.Error), http.StatusInternalServerError)
		return
	}

	// Save result to database
	if err := database.SaveScrapedData(id, project.BaseURL, result.Data, result.TokensUsed); err != nil {
		log.Printf("Failed to save scraped data: %v", err)
	}

	// Return response
	resp := models.ScrapeResponse{
		URL:        project.BaseURL,
		Data:       result.Data,
		TokensUsed: result.TokensUsed,
		Duration:   duration,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleAIScrapeDirect(w http.ResponseWriter, r *http.Request) {

	var req models.ScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, `{"error": "URL is required"}`, http.StatusBadRequest)
		return
	}

	// Fetch page content as Markdown
	pageContent, err := fetchPageContent(req.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch page: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	client, err := getAIClient(req.Provider)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "AI client initialization failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Use OpenAI to extract
	start := time.Now()
	result, err := client.ExtractData(r.Context(), &openai.ExtractionRequest{
		URL:     req.URL,
		Content: pageContent,
		Schema:  req.Schema,
		Prompt:  req.Prompt,
	})
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if result != nil && result.Error != "" {
		http.Error(w, fmt.Sprintf(`{"error": "Extraction failed: %s"}`, result.Error), http.StatusInternalServerError)
		return
	}

	resp := models.ScrapeResponse{
		URL:        req.URL,
		Data:       result.Data,
		TokensUsed: result.TokensUsed,
		Duration:   duration,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// fetchPageContent loads a page with Playwright, cleans the DOM, and converts to Markdown
func fetchPageContent(targetURL string) (string, error) {
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--disable-http2",
			"--disable-quic",
			"--no-sandbox",
			"--disable-setuid-sandbox",
			"--disable-dev-shm-usage",
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to launch browser: %w", err)
	}
	defer browser.Close()

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent:         playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		IgnoreHttpsErrors: playwright.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create context: %w", err)
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return "", fmt.Errorf("failed to create page: %w", err)
	}

	_, err = page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		// Non-fatal, might still get content
		log.Printf("Navigation warning: %v", err)
	}

	// Wait for page to settle
	time.Sleep(2 * time.Second)

	// Clean up DOM using JS to heavily reduce token payload
	rawContent, err := page.Evaluate(`() => {
		const selectors = ['script', 'style', 'noscript', 'svg', 'canvas', 'video', 'audio', 'iframe', 'map', 'object', 'meta', 'link', 'nav', 'footer', 'header'];
		document.querySelectorAll(selectors.join(',')).forEach(el => el.remove());
		
		const elements = document.querySelectorAll('*');
		for (let i = 0; i < elements.length; i++) {
			const el = elements[i];
			const attrs = el.attributes;
			for (let j = attrs.length - 1; j >= 0; j--) {
				const name = attrs[j].name;
				if (!['href', 'src', 'alt', 'title'].includes(name)) {
					el.removeAttribute(name);
				}
			}
		}
		
		return document.body ? document.body.innerHTML : document.documentElement.innerHTML;
	}`)

	var htmlContent string
	if err == nil {
		if str, ok := rawContent.(string); ok {
			htmlContent = str
		}
	}

	// Fallback to raw content if custom evaluation fails
	if htmlContent == "" {
		log.Printf("Falling back to raw page.Content()")
		htmlContent, _ = page.Content()
	}

	// Minify consecutive whitespace
	re := regexp.MustCompile(`\s+`)
	htmlContent = re.ReplaceAllString(htmlContent, " ")
	htmlContent = strings.TrimSpace(htmlContent)

	log.Printf("🧹 Cleaned HTML size: %d bytes", len(htmlContent))

	// Convert HTML to Markdown for better LLM comprehension and token efficiency
	markdown, err := md.ConvertString(htmlContent)
	if err != nil {
		log.Printf("⚠️ Markdown conversion failed, using cleaned HTML: %v", err)
		return htmlContent, nil
	}

	markdown = strings.TrimSpace(markdown)
	log.Printf("📝 Markdown size: %d bytes (%.0f%% reduction)", len(markdown), (1-float64(len(markdown))/float64(len(htmlContent)))*100)

	return markdown, nil
}
// ============ Schema Generation Handler ============

func handleGenerateSchema(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL      string `json:"url"`
		Prompt   string `json:"prompt"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" || req.Prompt == "" {
		http.Error(w, `{"error": "Both url and prompt are required"}`, http.StatusBadRequest)
		return
	}

	client, err := getAIClient(req.Provider)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "AI client initialization failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Fetch page content as Markdown
	pageContent, err := fetchPageContent(req.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch page: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Generate schema using AI
	result, err := client.GenerateSchema(r.Context(), req.URL, pageContent, req.Prompt)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if result.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Schema generation failed: " + result.Error})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ============ Data Export Handlers ============

func handleGetProjectData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := database.GetProjectData(id)
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch data"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func handleExportCSV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := database.GetProjectData(id)
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch data"}`, http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		http.Error(w, `{"error": "No data to export"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=data.csv")

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write header
	csvWriter.Write([]string{"ID", "URL", "Data", "Tokens Used", "Created At"})

	// Write rows
	for _, item := range data {
		var parsedData interface{}
		json.Unmarshal(item.Data, &parsedData)
		dataStr := string(item.Data)
		if parsedData != nil {
			if pretty, err := json.Marshal(parsedData); err == nil {
				dataStr = string(pretty)
			}
		}
		csvWriter.Write([]string{
			fmt.Sprintf("%d", item.ID),
			item.URL,
			dataStr,
			fmt.Sprintf("%d", item.Tokens),
			item.CreatedAt.Format(time.RFC3339),
		})
	}
}

// ============ API Interface Handlers ============

func handleGetAPIConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := database.GetProject(id)
	if err != nil {
		http.Error(w, `{"error": "Project not found"}`, http.StatusNotFound)
		return
	}

	params, err := database.GetAPIParams(id)
	if err != nil {
		params = []models.APIParam{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.APIConfig{
		Enabled: project.APIEnabled,
		Params:  params,
	})
}

func handleSaveAPIConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var config models.APIConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if err := database.SetAPIEnabled(id, config.Enabled); err != nil {
		http.Error(w, `{"error": "Failed to update API status"}`, http.StatusInternalServerError)
		return
	}

	if err := database.SaveAPIParams(id, config.Params); err != nil {
		http.Error(w, `{"error": "Failed to save API params"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handlePublicScrape(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// CORS headers for public API
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")

	// Get project
	project, err := database.GetProject(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Project not found"})
		return
	}

	client, err := getAIClient(project.Provider)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "AI service misconfigured for this project: " + err.Error()})
		return
	}

	if !project.APIEnabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Public API is not enabled for this project"})
		return
	}

	// Load param definitions
	apiParams, err := database.GetAPIParams(id)
	if err != nil {
		apiParams = []models.APIParam{}
	}

	// Validate incoming query params against definitions
	queryValues := r.URL.Query()
	validatedParams := url.Values{}
	var validationErrors []string

	for _, paramDef := range apiParams {
		val := queryValues.Get(paramDef.Name)
		if val == "" {
			if paramDef.Required {
				if paramDef.DefaultValue != "" {
					val = paramDef.DefaultValue
				} else {
					validationErrors = append(validationErrors, fmt.Sprintf("Missing required parameter: %s", paramDef.Name))
					continue
				}
			} else if paramDef.DefaultValue != "" {
				val = paramDef.DefaultValue
			} else {
				continue
			}
		}
		validatedParams.Set(paramDef.Name, val)
	}

	if len(validationErrors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "Validation failed",
			"errors": validationErrors,
		})
		return
	}

	// Build final URL
	finalURL := project.BaseURL
	if len(validatedParams) > 0 {
		parsed, err := url.Parse(finalURL)
		if err == nil {
			existing := parsed.Query()
			for k, v := range validatedParams {
				for _, val := range v {
					existing.Set(k, val)
				}
			}
			parsed.RawQuery = existing.Encode()
			finalURL = parsed.String()
		}
	}

	log.Printf("[PUBLIC API] Project %s → scraping %s", project.Name, finalURL)

	// Fetch page content
	pageContent, fetchErr := fetchPageContent(finalURL)
	if fetchErr != nil {
		log.Printf("Warning: failed to fetch page: %v", fetchErr)
		pageContent = ""
	}

	// Parse schema
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(project.Schema), &schema); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid schema in project config"})
		return
	}

	// Extract data
	start := time.Now()
	result, err := client.ExtractData(r.Context(), &openai.ExtractionRequest{
		URL:     finalURL,
		Content: pageContent,
		Schema:  schema,
		Prompt:  project.Prompt,
	})
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if result != nil && result.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": result.Error})
		return
	}

	// Save to DB
	database.SaveScrapedData(id, finalURL, result.Data, result.TokensUsed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.ScrapeResponse{
		URL:        finalURL,
		Data:       result.Data,
		TokensUsed: result.TokensUsed,
		Duration:   duration,
	})
}

// ============ Provider Handlers ============

func handleGetProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := database.GetLLMProviders()
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch providers"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

func handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req models.LLMProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.ModelName = strings.TrimSpace(req.ModelName)

	if req.Name == "" || req.APIKey == "" || req.BaseURL == "" || req.ModelName == "" {
		http.Error(w, `{"error": "All fields are required"}`, http.StatusBadRequest)
		return
	}

	if err := database.CreateLLMProvider(&req); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, `{"error": "Provider name already exists"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error": "Failed to create provider: `+err.Error()+`"}`, http.StatusInternalServerError)
		}
		return
	}

	// Remove API key from response
	req.APIKey = ""
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := database.DeleteLLMProvider(id); err != nil {
		http.Error(w, `{"error": "Failed to delete provider"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
