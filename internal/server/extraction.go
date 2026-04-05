package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"webscraper/internal/extractor"
	"webscraper/internal/models"
)

func (s *Server) handleGenerateExtractionConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL         string  `json:"url"`
		Prompt      string  `json:"prompt"`
		Provider    string  `json:"provider"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" || req.Prompt == "" {
		http.Error(w, `{"error": "Both url and prompt are required"}`, http.StatusBadRequest)
		return
	}

	if req.Temperature <= 0 {
		req.Temperature = 0.2
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2000
	}

	client, err := s.getAIClient(req.Provider)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "AI client initialization failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	pageContent, err := s.Browser.FetchHTML(req.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch page: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	result, err := client.GenerateExtractionConfig(r.Context(), req.URL, pageContent, req.Prompt, req.Temperature, req.MaxTokens)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if result.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": result.Error})
		return
	}

	if cfg, ok := result.Config["fields"]; ok {
		if fields, ok := cfg.([]interface{}); ok {
			for _, f := range fields {
				if fm, ok := f.(map[string]interface{}); ok {
					if sel, ok := fm["selector"].(string); ok {
						fm["selector"] = regexp.MustCompile(`::\w+`).ReplaceAllString(sel, "")
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleTestExtractionConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string                  `json:"url"`
		Config models.ExtractionConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, `{"error": "URL is required"}`, http.StatusBadRequest)
		return
	}

	if err := extractor.ValidateConfig(req.Config); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Invalid extraction config: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	page, cleanup, err := s.Browser.NewPage()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to launch browser: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	start := time.Now()
	data, err := extractor.Extract(r.Context(), page, req.URL, req.Config)
	duration := int(time.Since(start).Milliseconds())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Extraction failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":        data,
		"count":       len(data),
		"duration_ms": duration,
	})
}
