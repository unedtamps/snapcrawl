package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"webscraper/internal/browser"
	"webscraper/internal/db"
)

// Server holds all dependencies for HTTP handlers.
type Server struct {
	DB        *db.DB
	Browser   *browser.Manager
	StaticFS  http.FileSystem
	IndexHTML []byte
}

// NewRouter creates and configures the Chi router with all routes.
func (s *Server) NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Handle("/static/*", http.FileServer(s.StaticFS))

	s.register(r)
	return r
}

func (s *Server) register(r *chi.Mux) {
	r.Get("/", s.serveIndex)

	// Projects
	r.Post("/projects", s.handleCreateProject)
	r.Get("/projects", s.handleGetProjects)
	r.Get("/projects/{id}", s.handleGetProject)
	r.Put("/projects/{id}", s.handleUpdateProject)
	r.Delete("/projects/{id}", s.handleDeleteProject)

	// Scrape
	r.Post("/projects/{id}/scrape", s.handleProjectScrape)

	// Data export
	r.Get("/projects/{id}/data", s.handleGetProjectData)
	r.Get("/projects/{id}/data.csv", s.handleExportCSV)

	// Page preview
	r.Post("/api/preview-markdown", s.handlePreviewMarkdown)

	// Extraction config
	r.Post("/api/generate-extraction-config", s.handleGenerateExtractionConfig)
	r.Post("/api/test-extraction-config", s.handleTestExtractionConfig)

	// Direct AI API
	r.Post("/api/v2/ai/scrape", s.handleAIScrapeDirect)

	// API Interface config
	r.Get("/projects/{id}/api-config", s.handleGetAPIConfig)
	r.Put("/projects/{id}/api-config", s.handleSaveAPIConfig)

	// Public scrape - catch-all for dynamic path params
	r.Get("/api/public/{id}/scrape/*", s.handlePublicScrape)

	// LLM Providers
	r.Get("/api/providers", s.handleGetProviders)
	r.Post("/api/providers", s.handleCreateProvider)
	r.Delete("/api/providers/{id}", s.handleDeleteProvider)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.IndexHTML)
}
