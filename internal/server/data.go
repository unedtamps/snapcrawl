package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetProjectData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := s.DB.GetProjectData(id)
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch data"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := s.DB.GetProjectData(id)
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

	csvWriter.Write([]string{"ID", "URL", "Data", "Tokens Used", "Created At"})

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
			item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

func (s *Server) handlePreviewMarkdown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"markdown": pageContent,
		"size":     fmt.Sprintf("%d bytes", len(pageContent)),
	})
}
