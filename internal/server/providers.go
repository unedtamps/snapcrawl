package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"webscraper/internal/models"
)

func (s *Server) handleGetAPIConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := s.DB.GetProject(id)
	if err != nil {
		http.Error(w, `{"error": "Project not found"}`, http.StatusNotFound)
		return
	}

	params, err := s.DB.GetAPIParams(id)
	if err != nil {
		params = []models.APIParam{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.APIConfig{
		Enabled: project.APIEnabled,
		Params:  params,
	})
}

func (s *Server) handleSaveAPIConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var config models.APIConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if err := s.DB.SetAPIEnabled(id, config.Enabled); err != nil {
		http.Error(w, `{"error": "Failed to update API status"}`, http.StatusInternalServerError)
		return
	}

	if err := s.DB.SaveAPIParams(id, config.Params); err != nil {
		http.Error(w, `{"error": "Failed to save API params"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.DB.GetLLMProviders()
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch providers"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
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

	if err := s.DB.CreateLLMProvider(&req); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, `{"error": "Provider name already exists"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error": "Failed to create provider: `+err.Error()+`"}`, http.StatusInternalServerError)
		}
		return
	}

	req.APIKey = ""
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.DB.DeleteLLMProvider(id); err != nil {
		http.Error(w, `{"error": "Failed to delete provider"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
