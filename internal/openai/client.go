package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Client wraps OpenAI-compatible API
type Client struct {
	client     *openai.Client
	model      string
	baseURL    string
	maxRetries int
	isLocal    bool
}

// Config holds OpenAI configuration
type Config struct {
	APIKey       string
	BaseURL      string
	Model        string
	ProviderType string // "cloud" or "local"
}

// New creates a new OpenAI client
func New(cfg *Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("LLM Provider API Key is required")
	}

	openaiCfg := openai.DefaultConfig(cfg.APIKey)
	openaiCfg.BaseURL = cfg.BaseURL

	client := openai.NewClientWithConfig(openaiCfg)

	providerType := cfg.ProviderType
	if providerType == "" {
		providerType = "cloud"
	}

	return &Client{
		client:     client,
		model:      cfg.Model,
		baseURL:    cfg.BaseURL,
		maxRetries: 3,
		isLocal:    providerType == "local",
	}, nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-chat",
	}
}

// MustNew creates a new client or panics on error
func MustNew() *Client {
	cfg := DefaultConfig()
	client, err := New(cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to create OpenAI client: %v", err))
	}
	return client
}

// GetBaseURL returns the client's base URL
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// ExtractionRequest contains data for extraction
type ExtractionRequest struct {
	URL     string                 `json:"url"`
	Content string                 `json:"content"` // Markdown or HTML content of the page
	Schema  map[string]interface{} `json:"schema"`
	Prompt  string                 `json:"prompt,omitempty"`
}

// ExtractionResult contains the extracted data
type ExtractionResult struct {
	Data       map[string]interface{} `json:"data"`
	TokensUsed int                    `json:"tokens_used"`
	Duration   time.Duration          `json:"duration_ms"`
	Error      string                 `json:"error,omitempty"`
}

// SchemaResult contains a generated JSON schema
type SchemaResult struct {
	Schema     map[string]interface{} `json:"schema"`
	TokensUsed int                    `json:"tokens_used"`
	Error      string                 `json:"error,omitempty"`
}

// ExtractionConfigResult contains a generated extraction config
type ExtractionConfigResult struct {
	Config     map[string]interface{} `json:"config"`
	TokensUsed int                    `json:"tokens_used"`
	Error      string                 `json:"error,omitempty"`
}

// ExtractData performs AI extraction from HTML
func (c *Client) ExtractData(ctx context.Context, req *ExtractionRequest) (*ExtractionResult, error) {
	start := time.Now()

	// Use custom format if targeting local server
	if c.isLocal {
		return c.extractDataLocal(ctx, req, start)
	}

	maxLen := 15000 // safe cloud chunk limit
	chunks := splitContent(req.Content, maxLen)

	var finalData map[string]interface{}
	totalTokens := 0
	var lastErr error

	for i, chunkContent := range chunks {
		log.Printf("Processing cloud extraction chunk %d/%d...", i+1, len(chunks))

		prompt := buildExtractionPrompt(req.URL, req.Schema, req.Prompt, chunkContent)

		var resp openai.ChatCompletionResponse
		var chunkErr error

		for retry := 0; retry < c.maxRetries; retry++ {
			resp, chunkErr = c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       c.model,
				Messages:    prompt,
				MaxTokens:   4000,
				Temperature: 0.2,
			})

			if chunkErr == nil {
				break
			}
			time.Sleep(time.Duration(retry+1) * time.Second)
		}

		if chunkErr != nil {
			log.Printf("Chunk %d API request failed: %v", i+1, chunkErr)
			lastErr = chunkErr
			continue
		}

		if len(resp.Choices) == 0 {
			log.Printf("Chunk %d API returned no choices", i+1)
			lastErr = fmt.Errorf("API returned no choices")
			continue
		}

		totalTokens += resp.Usage.TotalTokens
		content := resp.Choices[0].Message.Content

		data, err := parseJSONResponse(content)
		if err != nil {
			log.Printf("Chunk %d JSON parse failed: %v", i+1, err)
			lastErr = err
			continue
		}

		finalData = mergeJSON(finalData, data)
	}

	if finalData == nil {
		return &ExtractionResult{
			Error:    fmt.Sprintf("Extraction failed. Last error: %v", lastErr),
			Duration: time.Since(start),
		}, nil
	}

	return &ExtractionResult{
		Data:       finalData,
		TokensUsed: totalTokens,
		Duration:   time.Since(start),
	}, nil
}

// buildExtractionPrompt creates the chat messages for extraction
func buildExtractionPrompt(url string, schema map[string]interface{}, prompt, content string) []openai.ChatCompletionMessage {
	systemPrompt := `You are a data extraction specialist. Extract structured data from the page content according to the provided schema.

IMPORTANT: Return ONLY valid JSON matching this exact schema:
` + formatSchema(schema) + `

Rules:
1. Extract ALL items if schema specifies an array
2. Clean and normalize data (trim whitespace, remove extra formatting)
3. If data is not found, use null/empty values appropriate to the type
4. Do NOT include explanations, only the JSON object`

	userPrompt := fmt.Sprintf("URL: %s\n\n", url)
	if prompt != "" {
		userPrompt += fmt.Sprintf("Instructions: %s\n\n", prompt)
	}
	userPrompt += "Page Content Chunk:\n" + content

	return []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	}
}

type localLMRequest struct {
	Model        string  `json:"model"`
	SystemPrompt string  `json:"system_prompt"`
	Input        string  `json:"input"`
	Temperature  float64 `json:"temperature,omitempty"`
}

// localLMResponse handles various local LM response shapes
type localLMResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Output []struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	} `json:"output"`
	Response string `json:"response"`
	Message  string `json:"message"`
}

func (c *Client) extractDataLocal(ctx context.Context, req *ExtractionRequest, start time.Time) (*ExtractionResult, error) {
	systemPrompt := `You are a data extraction specialist. Extract structured data from the page content according to the provided schema.

IMPORTANT: Return ONLY valid JSON matching this exact schema:
` + formatSchema(req.Schema) + `

Rules:
1. Extract ALL items if schema specifies an array
2. Clean and normalize data (trim whitespace, remove extra formatting)
3. If data is not found, use null/empty values appropriate to the type
4. Do NOT include explanations, only the JSON object`

	maxLen := 15000 // chunk size for local LM
	chunks := splitContent(req.Content, maxLen)

	var finalData map[string]interface{}
	totalTokens := 0
	var lastErr error

	for i, chunkContent := range chunks {
		// Check if the parent context has been canceled before starting a new chunk
		select {
		case <-ctx.Done():
			return &ExtractionResult{
				Error:    "Extraction canceled by user",
				Duration: time.Since(start),
			}, ctx.Err()
		default:
		}

		log.Printf("Processing local extraction chunk %d/%d...", i+1, len(chunks))

		userPrompt := fmt.Sprintf("URL: %s\n\n", req.URL)
		if req.Prompt != "" {
			userPrompt += fmt.Sprintf("Instructions: %s\n\n", req.Prompt)
		}
		userPrompt += "Page Content Chunk:\n" + chunkContent

		payload := localLMRequest{
			Model:        c.model,
			SystemPrompt: systemPrompt,
			Input:        userPrompt,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal local request: %w", err)
		}

		endpoint := c.baseURL
		if !strings.HasSuffix(endpoint, "/api/v1/chat") {
			endpoint = strings.TrimRight(endpoint, "/") + "/api/v1/chat"
		}

		var content string
		var chunkErr error

		for retry := 0; retry < c.maxRetries; retry++ {
			// Use an independent per-chunk timeout so a slow chunk doesn't cascade-cancel others,
			// but derive it from the parent ctx so that if the user aborts, the chunk dies immediately.
			chunkCtx, chunkCancel := context.WithTimeout(ctx, 3*time.Minute)

			httpReq, err := http.NewRequestWithContext(chunkCtx, "POST", endpoint, bytes.NewReader(body))
			if err != nil {
				chunkCancel()
				return nil, err
			}
			httpReq.Header.Set("Content-Type", "application/json")
			apiKey := os.Getenv("OPENAI_API_KEY")
			if apiKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+apiKey)
			}

			client := &http.Client{} // timeout handled by chunkCtx
			resp, err := client.Do(httpReq)
			if err != nil {
				chunkCancel()
				chunkErr = err
				time.Sleep(time.Duration(retry+1) * time.Second)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				chunkCancel()
				chunkErr = fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
				time.Sleep(time.Duration(retry+1) * time.Second)
				continue
			}

			var lmResp localLMResponse
			err = json.NewDecoder(resp.Body).Decode(&lmResp)
			resp.Body.Close()
			chunkCancel()

			if err != nil {
				chunkErr = fmt.Errorf("failed to decode response: %w", err)
				time.Sleep(time.Duration(retry+1) * time.Second)
				continue
			}

			if len(lmResp.Choices) > 0 {
				content = lmResp.Choices[0].Message.Content
			} else if len(lmResp.Output) > 0 {
				for _, out := range lmResp.Output {
					if out.Type == "message" {
						content = out.Content
						break
					}
				}
				if content == "" {
					content = lmResp.Output[len(lmResp.Output)-1].Content
				}
			} else if lmResp.Response != "" {
				content = lmResp.Response
			} else if lmResp.Message != "" {
				content = lmResp.Message
			}

			chunkErr = nil
			break
		}

		if chunkErr != nil {
			log.Printf("Chunk %d failed: %v", i+1, chunkErr)
			lastErr = chunkErr
			continue // try next chunk
		}

		data, err := parseJSONResponse(content)
		if err != nil {
			log.Printf("Chunk %d JSON parse failed: %v", i+1, err)
			lastErr = err
			continue // try next chunk
		}

		finalData = mergeJSON(finalData, data)
	}

	if finalData == nil {
		return &ExtractionResult{
			Error:    fmt.Sprintf("Extraction failed. Last error: %v", lastErr),
			Duration: time.Since(start),
		}, nil
	}

	return &ExtractionResult{
		Data:       finalData,
		TokensUsed: totalTokens,
		Duration:   time.Since(start),
	}, nil
}

// formatSchema converts schema to string representation
func formatSchema(schema map[string]interface{}) string {
	data, _ := json.MarshalIndent(schema, "", "  ")
	return string(data)
}

// parseJSONResponse extracts JSON from AI response (handles markdown code blocks)
func parseJSONResponse(content string) (map[string]interface{}, error) {
	// Find JSON block if it exists
	startIdx := strings.Index(content, "```json")
	if startIdx >= 0 {
		content = content[startIdx+7:]
		endIdx := strings.LastIndex(content, "```")
		if endIdx >= 0 {
			content = content[:endIdx]
		}
	} else {
		startIdx := strings.Index(content, "```")
		if startIdx >= 0 {
			content = content[startIdx+3:]
			endIdx := strings.LastIndex(content, "```")
			if endIdx >= 0 {
				content = content[:endIdx]
			}
		}
	}

	content = strings.TrimSpace(content)

	// Local/smaller LMs often output unquoted types (e.g. "field": string instead of "field": "string")
	// and add inline comments. We need to clean these up before parsing.

	// 1. Remove inline comments (// ...) but BE CAREFUL not to match http:// or https://
	// We only match // if it's preceded by whitespace
	commentRe := regexp.MustCompile(`(?m)\s+//.*$`)
	content = commentRe.ReplaceAllString(content, "")
	// Also strip full line comments
	fullLineCommentRe := regexp.MustCompile(`(?m)^\s*//.*$`)
	content = fullLineCommentRe.ReplaceAllString(content, "")

	// 2. Fix unquoted string/number/boolean values that aren't valid JSON
	// e.g. "title": string -> "title": "string"
	unquotedTypeRe := regexp.MustCompile(`:\s*(string|number|boolean|int|float)\b`)
	content = unquotedTypeRe.ReplaceAllString(content, `: "$1"`)

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		// Could try to extract from { to } as last fallback
		firstBrace := strings.Index(content, "{")
		lastBrace := strings.LastIndex(content, "}")
		if firstBrace >= 0 && lastBrace > firstBrace {
			fallback := content[firstBrace : lastBrace+1]
			if err2 := json.Unmarshal([]byte(fallback), &data); err2 == nil {
				return data, nil
			}
		}
		return nil, fmt.Errorf("failed to parse JSON response: %w\nContent was: %s", err, content)
	}
	return data, nil
}

const schemaGenerationSystemPrompt = `You are an expert JSON SCHEMA DESIGNER. Your ONLY job is to output a JSON schema describing the STRUCTURE of the data the user wants to extract.
DO NOT extract the actual data from the page! Do NOT output real data values like actual titles or URLs.
You must output ONLY an abstract JSON schema representation with type strings (e.g., "string", "number", "boolean", "array").

Rules:
1. Output ONLY a valid JSON object representing the schema structure. No text before or after.
2. DO NOT output actual data from the text. The values must ONLY be the literal strings "string", "number", or "boolean".
3. ALL keys and values MUST be wrapped in double quotes.
4. For lists of items, use: {"type": "array", "items": {"field1": "string", "field2": "number"}}
5. For nested objects, nest the schema accordingly.
6. Keep field names short, descriptive, and snake_case.
7. Do NOT include any comments (no // or /*) anywhere in the JSON.
8. Do NOT include explanations, only the JSON object.

Example output for "I want product names and prices":
{"products": {"type": "array", "items": {"name": "string", "price": "string"}}}`

// GenerateSchema uses AI to generate a JSON extraction schema from page content and user description
func (c *Client) GenerateSchema(ctx context.Context, url, content, userPrompt string) (*SchemaResult, error) {
	// Use local format if targeting local server
	if c.isLocal {
		return c.generateSchemaLocal(ctx, url, content, userPrompt)
	}

	userMsg := fmt.Sprintf("URL: %s\n\nUser wants: %s\n\nPage Content:\n%s", url, userPrompt, content)

	var resp openai.ChatCompletionResponse
	var err error

	for i := 0; i < c.maxRetries; i++ {
		resp, err = c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: schemaGenerationSystemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			MaxTokens:   2000,
			Temperature: 0.3,
		})

		if err == nil {
			break
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("schema generation failed after %d retries: %w", c.maxRetries, err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API returned no choices")
	}

	responseContent := resp.Choices[0].Message.Content
	schema, err := parseJSONResponse(responseContent)
	if err != nil {
		return &SchemaResult{
			Error: fmt.Sprintf("Failed to parse generated schema: %v", err),
		}, nil
	}

	return &SchemaResult{
		Schema:     schema,
		TokensUsed: resp.Usage.TotalTokens,
	}, nil
}

func (c *Client) generateSchemaLocal(ctx context.Context, url, content, userPrompt string) (*SchemaResult, error) {
	// For small local LMs, feeding the entire page content often causes them to ignore
	// the system prompt and perform actual data extraction, leading to token truncation.
	// We truncate the content so it only sees enough to deduce the structure.
	maxContentLen := 3000
	if len(content) > maxContentLen {
		content = content[:maxContentLen] + "\n\n...[CONTENT TRUNCATED FOR SCHEMA GENERATION]..."
	}

	userMsg := fmt.Sprintf("URL: %s\n\nUser wants: %s\n\nPage Content Snapshot (Truncated):\n%s\n\nCRITICAL REMINDER: Output ONLY the empty JSON schema structure. DO NOT extract actual data! Use the literal word \"string\" or \"number\" as the values.", url, userPrompt, content)

	endpoint := c.baseURL
	if !strings.HasSuffix(endpoint, "/api/v1/chat") {
		endpoint = strings.TrimRight(endpoint, "/") + "/api/v1/chat"
	}

	var schema map[string]interface{}
	var lastErr error

	// We'll retry on both network errors AND JSON parsing errors
	for i := 0; i < c.maxRetries; i++ {
		payload := localLMRequest{
			Model:        c.model,
			SystemPrompt: schemaGenerationSystemPrompt,
			Input:        userMsg,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal local request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		var lmResp localLMResponse
		err = json.NewDecoder(resp.Body).Decode(&lmResp)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to decode response: %w", err)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		var responseContent string
		if len(lmResp.Choices) > 0 {
			responseContent = lmResp.Choices[0].Message.Content
		} else if len(lmResp.Output) > 0 {
			for _, out := range lmResp.Output {
				if out.Type == "message" {
					responseContent = out.Content
					break
				}
			}
			if responseContent == "" {
				responseContent = lmResp.Output[len(lmResp.Output)-1].Content
			}
		} else if lmResp.Response != "" {
			responseContent = lmResp.Response
		} else if lmResp.Message != "" {
			responseContent = lmResp.Message
		}

		// Try parsing the returned content
		schema, err = parseJSONResponse(responseContent)
		if err == nil {
			// Success! Return immediately
			return &SchemaResult{
				Schema:     schema,
				TokensUsed: 0,
			}, nil
		}

		// Parsing failed. Save the error and append it to the prompt for the next retry iteration
		lastErr = err
		userMsg += fmt.Sprintf("\n\n---\nWARNING: Your previous response was INVALID JSON or contained ACTUAL DATA. The JSON parser returned this error:\n%v\n\nPlease remember to NEVER extract real data, only output the schema structure using \"string\" or \"number\" as values. Fix the syntax error and return ONLY the completely valid JSON object.", err)

		time.Sleep(time.Duration(i+1) * time.Second)
	}

	// If we exhausted retries and still failed to parse
	return &SchemaResult{
		Error: fmt.Sprintf("Failed to generate valid schema after %d attempts. Last error: %v", c.maxRetries, lastErr),
	}, nil
}

const extractionConfigSystemPrompt = `You are an expert CSS selector and web scraping specialist. Your task is to analyze the provided HTML and generate a JSON extraction config that uses CSS selectors to extract the data the user wants.

Rules:
1. Output ONLY a valid JSON object. No text before or after.
2. The JSON must have this exact structure:
   {
     "container": "CSS selector for repeating items (empty string if single page)",
     "fields": [
       {"name": "field_name", "selector": "CSS selector relative to container", "attribute": "text|html|href|src|attribute_name"}
     ]
   }
3. Use "container" when there are multiple repeating items (like product cards, list items, table rows).
4. Use empty string "" for "container" when extracting from a single page (like a detail page).
5. For "attribute", use: "text" for text content, "href" for links, "src" for images, or any HTML attribute name.
6. Keep field names short, descriptive, and snake_case.
7. Use precise CSS selectors that uniquely target the desired elements.
8. Do NOT include any comments or explanations.
9. Do NOT extract actual data values - only define the selectors.

Example for product listing:
{"container":".product-card","fields":[{"name":"title","selector":"h2.product-title","attribute":"text"},{"name":"price","selector":".price","attribute":"text"},{"name":"image","selector":"img","attribute":"src"},{"name":"link","selector":"a","attribute":"href"}]}

Example for single page:
{"container":"","fields":[{"name":"title","selector":"h1","attribute":"text"},{"name":"author","selector":".author-name","attribute":"text"},{"name":"date","selector":"time","attribute":"text"}]}`

// GenerateExtractionConfig uses AI to generate a selector-based extraction config from HTML
func (c *Client) GenerateExtractionConfig(ctx context.Context, url, html, userPrompt string, temperature float64, maxTokens int) (*ExtractionConfigResult, error) {
	if c.isLocal {
		return c.generateExtractionConfigLocal(ctx, url, html, userPrompt, temperature, maxTokens)
	}

	userMsg := fmt.Sprintf("URL: %s\n\nUser wants: %s\n\nHTML:\n%s", url, userPrompt, html)

	var resp openai.ChatCompletionResponse
	var err error

	for i := 0; i < c.maxRetries; i++ {
		resp, err = c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: extractionConfigSystemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			MaxTokens:   maxTokens,
			Temperature: float32(temperature),
		})

		if err == nil {
			break
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("extraction config generation failed after %d retries: %w", c.maxRetries, err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API returned no choices")
	}

	responseContent := resp.Choices[0].Message.Content
	config, err := parseJSONResponse(responseContent)
	if err != nil {
		return &ExtractionConfigResult{
			Error: fmt.Sprintf("Failed to parse generated config: %v", err),
		}, nil
	}

	return &ExtractionConfigResult{
		Config:     config,
		TokensUsed: resp.Usage.TotalTokens,
	}, nil
}

func (c *Client) generateExtractionConfigLocal(ctx context.Context, url, html, userPrompt string, temperature float64, maxTokens int) (*ExtractionConfigResult, error) {
	maxLen := 15000
	if len(html) > maxLen {
		html = html[:maxLen] + "\n\n...[CONTENT TRUNCATED]..."
	}

	userMsg := fmt.Sprintf("URL: %s\n\nUser wants: %s\n\nHTML:\n%s", url, userPrompt, html)

	endpoint := c.baseURL
	if !strings.HasSuffix(endpoint, "/api/v1/chat") {
		endpoint = strings.TrimRight(endpoint, "/") + "/api/v1/chat"
	}

	var config map[string]interface{}
	var lastErr error

	for i := 0; i < c.maxRetries; i++ {
		payload := localLMRequest{
			Model:        c.model,
			SystemPrompt: extractionConfigSystemPrompt,
			Input:        userMsg,
			Temperature:  temperature,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal local request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		var lmResp localLMResponse
		err = json.NewDecoder(resp.Body).Decode(&lmResp)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to decode response: %w", err)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		var responseContent string
		if len(lmResp.Choices) > 0 {
			responseContent = lmResp.Choices[0].Message.Content
		} else if len(lmResp.Output) > 0 {
			for _, out := range lmResp.Output {
				if out.Type == "message" {
					responseContent = out.Content
					break
				}
			}
			if responseContent == "" {
				responseContent = lmResp.Output[len(lmResp.Output)-1].Content
			}
		} else if lmResp.Response != "" {
			responseContent = lmResp.Response
		} else if lmResp.Message != "" {
			responseContent = lmResp.Message
		}

		config, err = parseJSONResponse(responseContent)
		if err == nil {
			return &ExtractionConfigResult{
				Config:     config,
				TokensUsed: 0,
			}, nil
		}

		lastErr = err
		userMsg += fmt.Sprintf("\n\n---\nWARNING: Your previous response was INVALID JSON. Parser error: %v\n\nFix the syntax and return ONLY the valid JSON object.", err)
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	return &ExtractionConfigResult{
		Error: fmt.Sprintf("Failed to generate valid extraction config after %d attempts. Last error: %v", c.maxRetries, lastErr),
	}, nil
}

// splitContent breaks a large string into chunks of roughly maxLen characters.
// It prefers splitting on double newlines (\n\n) to preserve paragraphs.
func splitContent(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}

	var chunks []string
	paragraphs := strings.Split(content, "\n\n")
	var currentChunk strings.Builder

	for _, p := range paragraphs {
		if currentChunk.Len()+len(p)+2 > maxLen && currentChunk.Len() > 0 {
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
		}

		if len(p) > maxLen {
			// A single paragraph is larger than maxLen. Force split it.
			if currentChunk.Len() > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
			}
			for len(p) > maxLen {
				chunks = append(chunks, p[:maxLen])
				p = p[maxLen:]
			}
			currentChunk.WriteString(p)
		} else {
			if currentChunk.Len() > 0 {
				currentChunk.WriteString("\n\n")
			}
			currentChunk.WriteString(p)
		}
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

// mergeJSON recursively merges two JSON structures.
// Arrays are concatenated. Maps are recursively merged. Primitives are retained.
func mergeJSON(dest, src map[string]interface{}) map[string]interface{} {
	if dest == nil {
		return src
	}
	if src == nil {
		return dest
	}

	for k, vSrc := range src {
		vDest, exists := dest[k]
		if !exists {
			dest[k] = vSrc
			continue
		}

		// If both are arrays, concatenate
		if arrSrc, okSrc := vSrc.([]interface{}); okSrc {
			if arrDest, okDest := vDest.([]interface{}); okDest {
				dest[k] = append(arrDest, arrSrc...)
				continue
			}
		}

		// If both are maps, recursively merge
		if mapSrc, okSrc := vSrc.(map[string]interface{}); okSrc {
			if mapDest, okDest := vDest.(map[string]interface{}); okDest {
				dest[k] = mergeJSON(mapDest, mapSrc)
				continue
			}
		}

		// For primitives, if dest is empty/null, take src
		if vDest == "" || vDest == nil {
			dest[k] = vSrc
		}
	}
	return dest
}
