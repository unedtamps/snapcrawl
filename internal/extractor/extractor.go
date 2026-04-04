package extractor

import (
	"context"
	"fmt"
	"strings"

	"github.com/playwright-community/playwright-go"
	"webscraper/internal/models"
)

// Extract runs a selector-based extraction config against a URL using Playwright
func Extract(ctx context.Context, page playwright.Page, targetURL string, config models.ExtractionConfig) ([]map[string]interface{}, error) {
	_, err := page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	// Wait for network to settle
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateDomcontentloaded,
	})

	var results []map[string]interface{}

	if config.Container != "" {
		containers, err := page.QuerySelectorAll(config.Container)
		if err != nil {
			return nil, fmt.Errorf("failed to query container '%s': %w", config.Container, err)
		}

		for _, container := range containers {
			item := make(map[string]interface{})
			for _, field := range config.Fields {
				val, err := extractField(container, field)
				if err != nil {
					return nil, fmt.Errorf("field '%s': %w", field.Name, err)
				}
				item[field.Name] = val
			}
			results = append(results, item)
		}
	} else {
		item := make(map[string]interface{})
		for _, field := range config.Fields {
			val, err := extractFieldFromPage(page, field)
			if err != nil {
				return nil, fmt.Errorf("field '%s': %w", field.Name, err)
			}
			item[field.Name] = val
		}
		results = append(results, item)
	}

	return results, nil
}

func extractField(container playwright.ElementHandle, field models.ExtractionField) (interface{}, error) {
	if field.Selector == "" {
		return "", nil
	}

	el, err := container.QuerySelector(field.Selector)
	if err != nil {
		return "", fmt.Errorf("selector '%s': %w", field.Selector, err)
	}
	if el == nil {
		return "", nil
	}

	return extractValue(el, field.Attribute)
}

func extractFieldFromPage(page playwright.Page, field models.ExtractionField) (interface{}, error) {
	if field.Selector == "" {
		return "", nil
	}

	el, err := page.QuerySelector(field.Selector)
	if err != nil {
		return "", fmt.Errorf("selector '%s': %w", field.Selector, err)
	}
	if el == nil {
		return "", nil
	}

	return extractValue(el, field.Attribute)
}

func extractValue(el playwright.ElementHandle, attribute string) (interface{}, error) {
	switch strings.ToLower(attribute) {
	case "text", "":
		text, err := el.TextContent()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(text), nil
	case "html", "innerhtml":
		html, err := el.InnerHTML()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(html), nil
	case "href":
		val, err := el.GetAttribute("href")
		if err != nil {
			return "", err
		}
		return val, nil
	case "src":
		val, err := el.GetAttribute("src")
		if err != nil {
			return "", err
		}
		return val, nil
	default:
		val, err := el.GetAttribute(attribute)
		if err != nil {
			return "", err
		}
		return val, nil
	}
}

// ValidateConfig checks if the extraction config is valid
func ValidateConfig(config models.ExtractionConfig) error {
	if len(config.Fields) == 0 {
		return fmt.Errorf("at least one field is required")
	}
	for i, f := range config.Fields {
		if f.Name == "" {
			return fmt.Errorf("field %d: name is required", i)
		}
		if f.Selector == "" {
			return fmt.Errorf("field '%s': selector is required", f.Name)
		}
	}
	return nil
}
