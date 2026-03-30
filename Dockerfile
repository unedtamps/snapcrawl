# ── Stage 1: Build ──
FROM golang:1.25-bookworm AS builder

WORKDIR /src

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/snapcrawl .

# ── Stage 2: Runtime ──
FROM mcr.microsoft.com/playwright:v1.52.0-noble

# Install Go SQLite runtime dependency
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/snapcrawl .

# Install Playwright browsers (Chromium only)
RUN npx playwright install chromium

# Persistent data volume
VOLUME /app/data

ENV PORT=8080
ENV DATA_DIR=/app/data

EXPOSE 8080

CMD ["./snapcrawl"]
