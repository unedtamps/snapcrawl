package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"webscraper/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

// Simplified schema for v1.0
const schemaSQL = `
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    schema_json TEXT NOT NULL,
    prompt TEXT,
    provider TEXT DEFAULT 'deepseek',
    delay_ms INTEGER DEFAULT 1000,
    api_enabled INTEGER DEFAULT 0,
    extraction_config TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS scraped_data (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    url TEXT NOT NULL,
    data_json TEXT NOT NULL,
    tokens_used INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS api_params (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT DEFAULT 'string',
    required INTEGER DEFAULT 0,
    default_value TEXT DEFAULT '',
    description TEXT DEFAULT '',
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS llm_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    api_key TEXT NOT NULL,
    base_url TEXT NOT NULL,
    model_name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// DB wraps sql.DB with our custom methods
type DB struct {
	*sql.DB
}

// New creates and initializes a new database connection
func New(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{sqlDB}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

// migrate runs the schema migration
func (db *DB) migrate() error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return err
	}

	// Add api_enabled column to existing databases (idempotent)
	db.Exec("ALTER TABLE projects ADD COLUMN api_enabled INTEGER DEFAULT 0")

	// Add extraction_config column to existing databases (idempotent)
	db.Exec("ALTER TABLE projects ADD COLUMN extraction_config TEXT DEFAULT ''")

	return nil
}

// ── Projects ──

// CreateProject creates a new project
func (db *DB) CreateProject(id, name, baseURL, schema, prompt, provider string, delayMs int) error {
	_, err := db.Exec(
		"INSERT INTO projects (id, name, base_url, schema_json, prompt, provider, delay_ms, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, name, baseURL, schema, prompt, provider, delayMs, time.Now(), time.Now(),
	)
	return err
}

// GetProject retrieves a project by ID
func (db *DB) GetProject(id string) (*models.Project, error) {
	var p models.Project
	err := db.QueryRow(
		"SELECT id, name, base_url, schema_json, prompt, provider, delay_ms, api_enabled, extraction_config, created_at, updated_at FROM projects WHERE id = ?",
		id,
	).Scan(&p.ID, &p.Name, &p.BaseURL, &p.Schema, &p.Prompt, &p.Provider, &p.DelayMs, &p.APIEnabled, &p.ExtractionConfig, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetAllProjects retrieves all projects
func (db *DB) GetAllProjects() ([]models.Project, error) {
	rows, err := db.Query("SELECT id, name, base_url, schema_json, prompt, provider, delay_ms, api_enabled, extraction_config, created_at, updated_at FROM projects ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.Schema, &p.Prompt, &p.Provider, &p.DelayMs, &p.APIEnabled, &p.ExtractionConfig, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProject updates an existing project
func (db *DB) UpdateProject(id, name, baseURL, schema, prompt, provider string, delayMs int) error {
	_, err := db.Exec(
		"UPDATE projects SET name = ?, base_url = ?, schema_json = ?, prompt = ?, provider = ?, delay_ms = ?, updated_at = ? WHERE id = ?",
		name, baseURL, schema, prompt, provider, delayMs, time.Now(), id,
	)
	return err
}

// UpdateExtractionConfig updates only the extraction config for a project
func (db *DB) UpdateExtractionConfig(projectID, extractionConfig string) error {
	_, err := db.Exec(
		"UPDATE projects SET extraction_config = ?, updated_at = ? WHERE id = ?",
		extractionConfig, time.Now(), projectID,
	)
	return err
}

// DeleteProject removes a project (cascade deletes data)
func (db *DB) DeleteProject(projectID string) error {
	_, err := db.Exec("DELETE FROM projects WHERE id = ?", projectID)
	return err
}

// ProjectNameExists checks if a project name already exists
func (db *DB) ProjectNameExists(name string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM projects WHERE name = ?", name).Scan(&count)
	return count > 0, err
}

// ── Scraped Data ──

// SaveScrapedData saves scraped results
func (db *DB) SaveScrapedData(projectID, url string, data interface{}, tokensUsed int) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	_, err = db.Exec(
		"INSERT INTO scraped_data (project_id, url, data_json, tokens_used, created_at) VALUES (?, ?, ?, ?, ?)",
		projectID, url, string(dataJSON), tokensUsed, time.Now(),
	)
	return err
}

// GetProjectData retrieves all scraped data for a project
func (db *DB) GetProjectData(projectID string) ([]models.DataView, error) {
	rows, err := db.Query(
		"SELECT id, url, data_json, tokens_used, created_at FROM scraped_data WHERE project_id = ? ORDER BY created_at DESC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []models.DataView
	for rows.Next() {
		var d models.DataView
		if err := rows.Scan(&d.ID, &d.URL, &d.Data, &d.Tokens, &d.CreatedAt); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

// GetProjectLatestData gets the most recent scrape for a project
func (db *DB) GetProjectLatestData(projectID string) (*models.DataView, error) {
	var d models.DataView
	err := db.QueryRow(
		"SELECT id, url, data_json, tokens_used, created_at FROM scraped_data WHERE project_id = ? ORDER BY created_at DESC LIMIT 1",
		projectID,
	).Scan(&d.ID, &d.URL, &d.Data, &d.Tokens, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ── API Config ──

// SetAPIEnabled toggles the public API for a project
func (db *DB) SetAPIEnabled(projectID string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	_, err := db.Exec("UPDATE projects SET api_enabled = ?, updated_at = ? WHERE id = ?", val, time.Now(), projectID)
	return err
}

// SaveAPIParams replaces all API params for a project
func (db *DB) SaveAPIParams(projectID string, params []models.APIParam) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing params
	_, err = tx.Exec("DELETE FROM api_params WHERE project_id = ?", projectID)
	if err != nil {
		return err
	}

	// Insert new params
	for _, p := range params {
		_, err = tx.Exec(
			"INSERT INTO api_params (project_id, name, type, required, default_value, description) VALUES (?, ?, ?, ?, ?, ?)",
			projectID, p.Name, p.Type, p.Required, p.DefaultValue, p.Description,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAPIParams retrieves all API params for a project
func (db *DB) GetAPIParams(projectID string) ([]models.APIParam, error) {
	rows, err := db.Query(
		"SELECT id, project_id, name, type, required, default_value, description FROM api_params WHERE project_id = ? ORDER BY id ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var params []models.APIParam
	for rows.Next() {
		var p models.APIParam
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Type, &p.Required, &p.DefaultValue, &p.Description); err != nil {
			return nil, err
		}
		params = append(params, p)
	}
	return params, rows.Err()
}

// ── LLM Providers ──

func (db *DB) CreateLLMProvider(p *models.LLMProvider) error {
	query := `INSERT INTO llm_providers (name, api_key, base_url, model_name) VALUES (?, ?, ?, ?)`
	res, err := db.Exec(query, p.Name, p.APIKey, p.BaseURL, p.ModelName)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		p.ID = int(id)
	}
	return nil
}

func (db *DB) GetLLMProviders() ([]models.LLMProvider, error) {
	query := `SELECT id, name, base_url, model_name, created_at FROM llm_providers ORDER BY created_at DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	providers := make([]models.LLMProvider, 0)
	for rows.Next() {
		var p models.LLMProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.ModelName, &p.CreatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, nil
}

func (db *DB) GetLLMProviderByID(id string) (*models.LLMProvider, error) {
	query := `SELECT id, name, api_key, base_url, model_name, created_at FROM llm_providers WHERE id = ?`
	row := db.QueryRow(query, id)

	var p models.LLMProvider
	err := row.Scan(&p.ID, &p.Name, &p.APIKey, &p.BaseURL, &p.ModelName, &p.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("provider not found")
		}
		return nil, err
	}
	return &p, nil
}

func (db *DB) DeleteLLMProvider(id string) error {
	query := `DELETE FROM llm_providers WHERE id = ?`
	_, err := db.Exec(query, id)
	return err
}
