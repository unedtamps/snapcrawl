package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"

	"webscraper/internal/extractor"
	"webscraper/internal/models"
	"webscraper/internal/openai"
)

func (s *Server) getAIClient(providerID string) (*openai.Client, error) {
	if providerID == "" {
		return nil, fmt.Errorf("provider is required")
	}

	if !regexp.MustCompile(`^\d+$`).MatchString(providerID) {
		providers, err := s.DB.GetLLMProviders()
		if err == nil && len(providers) > 0 {
			providerID = fmt.Sprintf("%d", providers[0].ID)
		} else {
			return nil, fmt.Errorf("please configure an AI Provider in the Settings menu first")
		}
	}

	p, err := s.DB.GetLLMProviderByID(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider details: %w", err)
	}
	cfg := &openai.Config{
		APIKey:       p.APIKey,
		BaseURL:      p.BaseURL,
		Model:        p.ModelName,
		ProviderType: p.ProviderType,
	}
	return openai.New(cfg)
}

func (s *Server) handleProjectScrape(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := s.DB.GetProject(id)
	if err != nil {
		http.Error(w, `{"error": "Project not found"}`, http.StatusNotFound)
		return
	}

	if project.BaseURL == "" {
		http.Error(w, `{"error": "Base URL not configured"}`, http.StatusBadRequest)
		return
	}

	start := time.Now()

	if project.ExtractionConfig != "" && project.ExtractionConfig != "{}" {
		s.scrapeWithExtractor(w, r, id, project, start)
		return
	}

	http.Error(w, `{"error": "No extraction config found. Please generate an extraction config for this project first."}`, http.StatusBadRequest)
}

func (s *Server) scrapeWithExtractor(w http.ResponseWriter, r *http.Request, id string, project *models.Project, start time.Time) {
	var config models.ExtractionConfig
	if err := json.Unmarshal([]byte(project.ExtractionConfig), &config); err != nil {
		http.Error(w, `{"error": "Invalid extraction config in project"}`, http.StatusInternalServerError)
		return
	}

	if err := extractor.ValidateConfig(config); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Invalid extraction config: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	page, cleanup, err := s.Browser.NewPageWithCookies(project.Cookies)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to launch browser: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	data, err := extractor.Extract(r.Context(), page, project.BaseURL, config)
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Extraction failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	resultMap := make(map[string]interface{})
	if config.Container != "" {
		resultMap["items"] = data
	} else if len(data) > 0 {
		resultMap = data[0]
	}

	if err := s.DB.SaveScrapedData(id, project.BaseURL, resultMap, 0); err != nil {
		log.Printf("Failed to save scraped data: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.ScrapeResponse{
		URL:        project.BaseURL,
		Data:       resultMap,
		TokensUsed: 0,
		Duration:   duration,
	})
}

func (s *Server) handleAIScrapeDirect(w http.ResponseWriter, r *http.Request) {
	var req models.ScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, `{"error": "URL is required"}`, http.StatusBadRequest)
		return
	}

	pageContent, err := s.Browser.FetchMarkdown(req.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch page: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	client, err := s.getAIClient(req.Provider)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "AI client initialization failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.ScrapeResponse{
		URL:        req.URL,
		Data:       result.Data,
		TokensUsed: result.TokensUsed,
		Duration:   duration,
	})
}

func (s *Server) handlePublicScrape(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")

	project, err := s.DB.GetProject(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Project not found"})
		return
	}

	if !project.APIEnabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Public API is not enabled for this project"})
		return
	}

	apiParams, err := s.DB.GetAPIParams(id)
	if err != nil {
		apiParams = []models.APIParam{}
	}

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

	start := time.Now()

	if project.ExtractionConfig != "" && project.ExtractionConfig != "{}" {
		s.publicScrapeWithExtractor(w, r, id, project, finalURL, start)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": "No extraction config found. Please generate an extraction config for this project first."})
}

func (s *Server) publicScrapeWithExtractor(w http.ResponseWriter, r *http.Request, id string, project *models.Project, finalURL string, start time.Time) {
	var config models.ExtractionConfig
	if err := json.Unmarshal([]byte(project.ExtractionConfig), &config); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid extraction config in project"})
		return
	}

	page, cleanup, err := s.Browser.NewPageWithCookies(project.Cookies)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to launch browser"})
		return
	}
	defer cleanup()

	data, err := extractor.Extract(r.Context(), page, finalURL, config)
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Extraction failed: " + err.Error()})
		return
	}

	resultMap := make(map[string]interface{})
	if config.Container != "" {
		resultMap["items"] = data
	} else if len(data) > 0 {
		resultMap = data[0]
	}

	s.DB.SaveScrapedData(id, finalURL, resultMap, 0)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.ScrapeResponse{
		URL:        finalURL,
		Data:       resultMap,
		TokensUsed: 0,
		Duration:   duration,
	})
}
