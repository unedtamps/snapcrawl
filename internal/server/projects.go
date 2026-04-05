package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"webscraper/internal/models"
)

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
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
	if err := s.DB.CreateProject(id, req.Name, "", "{}", "", "deepseek"); err != nil {
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

func (s *Server) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.DB.GetAllProjects()
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch projects"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := s.DB.GetProject(id)
	if err != nil {
		http.Error(w, `{"error": "Project not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req models.Project
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.ExtractionConfig != "" {
		var ec models.ExtractionConfig
		if err := json.Unmarshal([]byte(req.ExtractionConfig), &ec); err != nil {
			http.Error(w, `{"error": "Invalid extraction config JSON"}`, http.StatusBadRequest)
			return
		}
	}

	if err := s.DB.UpdateProject(id, req.Name, req.BaseURL, req.Schema, req.Prompt, req.Provider); err != nil {
		http.Error(w, `{"error": "Failed to update project"}`, http.StatusInternalServerError)
		return
	}

	s.DB.UpdateExtractionConfig(id, req.ExtractionConfig)
	s.DB.UpdateCookies(id, req.Cookies)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.DB.DeleteProject(id); err != nil {
		http.Error(w, `{"error": "Failed to delete project"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
