FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/snapcrawl .

FROM mcr.microsoft.com/playwright:v1.52.0-noble

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/snapcrawl .

RUN npx playwright install chromium

VOLUME /app/data

ENV PORT=8080
ENV DATA_DIR=/app/data

EXPOSE 8080

CMD ["./snapcrawl"]
