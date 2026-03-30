# SnapCrawl AI

Simple AI-powered web scraper. Single binary, zero build step.

## Features

- Direct LLM extraction from any webpage
- Define JSON schema, AI extracts matching data
- Synchronous scraping (immediate results)
- Export to JSON and CSV
- Simple project-based workflow

## Tech Stack

- Go + Chi router
- Playwright for browser automation
- OpenAI-compatible API (DeepSeek, GLM, MiniMax, etc.)
- SQLite for data storage
- Embedded HTML/JS (no build)

## Quick Start

### Build

```bash
make build
```

### Run

```bash
./bin/webscraper
```

Server starts at http://localhost:8080

### Configure

1. Create a project (enter name)
2. Select project, configure:
   - Base URL (the page to scrape)
   - JSON Schema (structure of data you want)
   - Optional: AI instructions
   - Provider: DeepSeek / GLM / MiniMax / Custom
   - Delay between requests (ms)
3. Click **Save**
4. Click **Test** to test on one page
5. Click **Scrape** to extract data

### Environment Variables

Create `.env` file:

```bash
OPENAI_API_KEY=sk-your-api-key
# Optional:
# OPENAI_BASE_URL=https://api.deepseek.com/v1
# OPENAI_MODEL=deepseek-chat
# PORT=8080
```

## API Endpoints

- `GET /` - Main UI
- `POST /projects` - Create project
- `GET /projects` - List all projects
- `GET /projects/{id}` - Get project config
- `PUT /projects/{id}` - Update project config
- `DELETE /projects/{id}` - Delete project
- `POST /projects/{id}/scrape` - Scrape the project's base URL
- `GET /projects/{id}/data` - Get all scraped data (JSON)
- `GET /projects/{id}/data.csv` - Export as CSV
- `POST /api/v2/ai/scrape` - Direct AI scrape (no project)

## Data Model

```json
{
  "products": {
    "type": "array",
    "items": {
      "name": "string",
      "price": "number",
      "description": "string"
    }
  }
}
```

The AI will extract structured data matching this schema.

## Example cURL

Create project:
```bash
curl -X POST http://localhost:8080/projects \
  -H "Content-Type: application/json" \
  -d '{"name":"My Store"}'
```

Update config:
```bash
curl -X PUT http://localhost:8080/projects/{id} \
  -H "Content-Type: application/json" \
  -d '{
    "base_url": "https://example.com/products",
    "schema": {
      "products": {
        "type": "array",
        "items": {
          "name": "string",
          "price": "number"
        }
      }
    },
    "provider": "deepseek"
  }'
```

Scrape:
```bash
curl -X POST http://localhost:8080/projects/{id}/scrape
```

Get data:
```bash
curl http://localhost:8080/projects/{id}/data
```

Export CSV:
```bash
curl http://localhost:8080/projects/{id}/data.csv -o data.csv
```

## Supported Providers

- **DeepSeek** (default) - `OPENAI_BASE_URL=https://api.deepseek.com/v1`
- **GLM** (Z.ai) - `OPENAI_BASE_URL=https://open.bigmodel.cn/api/paas/v4`
- **MiniMax** - `OPENAI_BASE_URL=https://api.minimax.chat/v1`
- **OpenAI** - default base URL
- **Custom** - any OpenAI-compatible API

Just set `OPENAI_BASE_URL` and `OPENAI_MODEL` in your `.env`.

## How It Works

1. Scraper loads the page with Playwright (headless Chrome)
2. Gets full HTML content
3. Sends HTML + schema to LLM
4. AI extracts structured data matching schema
5. Results stored in SQLite, returned to UI

## Requirements

- Go 1.20+
- Internet connection (for API calls)
- Chromium (auto-downloaded on first run)

## Project Structure

```
snapcrawl/
├── main.go              # Application entry point
├── internal/
│   ├── db/              # Database layer
│   ├── models/          # Data models
│   └── openai/          # OpenAI client wrapper
├── templates/
│   └── index.html       # Main UI page
├── static/
│   └── app.js           # Frontend JavaScript
├── schema.sql           # Database schema
├── scraper.db           # SQLite database (created automatically)
├── Makefile
├── go.mod
└── README.md
```

## License

MIT
