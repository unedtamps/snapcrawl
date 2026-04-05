package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"webscraper/internal/browser"
	"webscraper/internal/db"
	"webscraper/internal/server"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/index.html
var indexHTML []byte

func main() {
	if err := godotenv.Overload(); err != nil {
		log.Println("⚠️  No .env file found or error loading it")
	}

	log.Println("🚀 Starting SnapCrawl v1.0...")

	bm, err := browser.New()
	if err != nil {
		log.Fatalf("Failed to initialize Playwright: %v", err)
	}
	log.Println("✅ Playwright initialized")

	dbPath := "scraper.db"
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		os.MkdirAll(dataDir, 0755)
		dbPath = dataDir + "/scraper.db"
	}
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	log.Println("✅ Database initialized")

	srv := &server.Server{
		DB:        database,
		Browser:   bm,
		StaticFS:  http.FS(staticFS),
		IndexHTML: indexHTML,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🌐 Server running on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, srv.NewRouter()))
}
