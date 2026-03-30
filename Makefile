BINARY_NAME=webscraper
BINARY_FOLDER=bin

.PHONY: all build clean run

all: build

build:
	@echo "Building binary..."
	go build -ldflags="-s -w" -o $(BINARY_FOLDER)/$(BINARY_NAME)
	@echo "✅ Build complete: $(BINARY_NAME)"

clean:
	@echo "Cleaning..."
	rm -f $(BINARY_FOLDER)/$(BINARY_NAME)
	rm -f scraper.db

run:
	@echo "Building binary..."
	go build -ldflags="-s -w" -o $(BINARY_FOLDER)/$(BINARY_NAME)
	@echo "Running..."
	./$(BINARY_FOLDER)/$(BINARY_NAME)
