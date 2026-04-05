package models

import (
	"encoding/json"
	"time"
)

// Project represents a scraping project
type Project struct {
	ID               string    `json:"id" db:"id"`
	Name             string    `json:"name" db:"name"`
	BaseURL          string    `json:"base_url" db:"base_url"`
	Schema           string    `json:"schema" db:"schema_json"`
	Prompt           string    `json:"prompt" db:"prompt"`
	Provider         string    `json:"provider" db:"provider"`
	APIEnabled       bool      `json:"api_enabled" db:"api_enabled"`
	ExtractionConfig string    `json:"extraction_config" db:"extraction_config"`
	Cookies          string    `json:"cookies" db:"cookies"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// APIParam defines a parameter for the project's public API
type APIParam struct {
	ID           int    `json:"id" db:"id"`
	ProjectID    string `json:"project_id" db:"project_id"`
	Name         string `json:"name" db:"name"`
	Type         string `json:"type" db:"type"` // "string" or "number"
	Required     bool   `json:"required" db:"required"`
	DefaultValue string `json:"default_value" db:"default_value"`
	Description  string `json:"description" db:"description"`
}

// APIConfig groups the API toggle + param list for the frontend
type APIConfig struct {
	Enabled bool       `json:"enabled"`
	Params  []APIParam `json:"params"`
}

// ScrapedData represents extracted data
type ScrapedData struct {
	ID        int       `json:"id" db:"id"`
	ProjectID string    `json:"project_id" db:"project_id"`
	URL       string    `json:"url" db:"url"`
	Data      string    `json:"data" db:"data_json"`
	Tokens    int       `json:"tokens_used" db:"tokens_used"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ScrapeRequest for triggering scrape
type ScrapeRequest struct {
	URL      string                 `json:"url"`
	Schema   map[string]interface{} `json:"schema"`
	Prompt   string                 `json:"prompt,omitempty"`
	Provider string                 `json:"provider,omitempty"`
}

// ScrapeResponse for returning results
type ScrapeResponse struct {
	URL        string      `json:"url"`
	Data       interface{} `json:"data"`
	TokensUsed int         `json:"tokens_used"`
	Duration   int         `json:"duration_ms"`
	Error      string      `json:"error,omitempty"`
}

// DataView for displaying scraped data
type DataView struct {
	ID        int             `json:"id"`
	URL       string          `json:"url"`
	Data      json.RawMessage `json:"data"`
	Tokens    int             `json:"tokens_used"`
	CreatedAt time.Time       `json:"created_at"`
}

// LLMProvider definitions for user-configured AI models
type LLMProvider struct {
	ID           int       `json:"id" db:"id"`
	Name         string    `json:"name" db:"name"`
	APIKey       string    `json:"api_key,omitempty" db:"api_key"`
	BaseURL      string    `json:"base_url" db:"base_url"`
	ModelName    string    `json:"model_name" db:"model_name"`
	ProviderType string    `json:"provider_type" db:"provider_type"` // "cloud" or "local"
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// ExtractionField defines a single field to extract from a page
type ExtractionField struct {
	Name      string `json:"name"`
	Selector  string `json:"selector"`
	Attribute string `json:"attribute"` // "text", "html", "href", "src", or any attribute name
}

// ExtractionConfig defines a selector-based extraction strategy
type ExtractionConfig struct {
	Container string            `json:"container"` // CSS selector for repeating container (empty = single page)
	Fields    []ExtractionField `json:"fields"`
}
