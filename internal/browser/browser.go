package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/playwright-community/playwright-go"
)

// Manager holds the Playwright instance and provides browser helpers.
type Manager struct {
	pw *playwright.Playwright
}

// New creates a Manager. It installs Playwright if the driver is missing.
func New() (*Manager, error) {
	p, err := playwright.Run()
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "could not run driver") ||
			strings.Contains(errStr, "not installed") ||
			strings.Contains(errStr, "please install") ||
			strings.Contains(errStr, "driver") {
			log.Println("📦 Playwright driver not found, installing...")
			if installErr := playwright.Install(); installErr != nil {
				return nil, fmt.Errorf("failed to install playwright: %w", installErr)
			}
			log.Println("✅ Playwright installed, starting...")
			p, err = playwright.Run()
			if err != nil {
				return nil, fmt.Errorf("failed to run playwright after install: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to run playwright: %w", err)
		}
	}
	return &Manager{pw: p}, nil
}

// Playwright returns the underlying Playwright instance.
func (m *Manager) Playwright() *playwright.Playwright {
	return m.pw
}

// launchBrowser creates a headless browser, page, and returns cleanup funcs.
func (m *Manager) launchBrowser() (playwright.Browser, playwright.BrowserContext, playwright.Page, error) {
	browser, err := m.pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--disable-http2", "--disable-quic", "--no-sandbox",
			"--disable-setuid-sandbox", "--disable-dev-shm-usage",
		},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browserCtx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent:         playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		IgnoreHttpsErrors: playwright.Bool(true),
	})
	if err != nil {
		browser.Close()
		return nil, nil, nil, fmt.Errorf("failed to create context: %w", err)
	}

	page, err := browserCtx.NewPage()
	if err != nil {
		browserCtx.Close()
		browser.Close()
		return nil, nil, nil, fmt.Errorf("failed to create page: %w", err)
	}

	return browser, browserCtx, page, nil
}

// navigate opens targetURL and waits for the DOM to settle.
func (m *Manager) navigate(page playwright.Page, targetURL string) {
	_, err := page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		log.Printf("Navigation warning: %v", err)
	}
	time.Sleep(2 * time.Second)
}

// cleanDOMJS removes noise elements and strips unnecessary attributes.
const cleanDOMJS = `() => {
	const selectors = ['script', 'style', 'noscript', 'svg', 'canvas', 'video', 'audio', 'iframe', 'map', 'object', 'meta', 'link', 'nav', 'footer', 'header'];
	document.querySelectorAll(selectors.join(',')).forEach(el => el.remove());
	
	const elements = document.querySelectorAll('*');
	for (let i = 0; i < elements.length; i++) {
		const el = elements[i];
		const attrs = el.attributes;
		for (let j = attrs.length - 1; j >= 0; j--) {
			const name = attrs[j].name;
			if (!['href', 'src', 'alt', 'title'].includes(name)) {
				el.removeAttribute(name);
			}
		}
	}
	
	return document.body ? document.body.innerHTML : document.documentElement.innerHTML;
}`

// FetchMarkdown loads a page, cleans the DOM, and converts to Markdown.
func (m *Manager) FetchMarkdown(targetURL string) (string, error) {
	browser, browserCtx, page, err := m.launchBrowser()
	if err != nil {
		return "", err
	}
	defer browser.Close()
	defer browserCtx.Close()

	m.navigate(page, targetURL)

	rawContent, err := page.Evaluate(cleanDOMJS)

	var htmlContent string
	if err == nil {
		if str, ok := rawContent.(string); ok {
			htmlContent = str
		}
	}

	if htmlContent == "" {
		log.Printf("Falling back to raw page.Content()")
		htmlContent, _ = page.Content()
	}

	re := regexp.MustCompile(`\s+`)
	htmlContent = re.ReplaceAllString(htmlContent, " ")
	htmlContent = strings.TrimSpace(htmlContent)

	log.Printf("🧹 CleanedHTML size: %d bytes", len(htmlContent))

	markdown, err := md.ConvertString(htmlContent)
	if err != nil {
		log.Printf("⚠️  Markdown conversion failed, using cleaned HTML: %v", err)
		return htmlContent, nil
	}

	markdown = strings.TrimSpace(markdown)
	log.Printf("📝 Markdown size: %d bytes (%.0f%% reduction)", len(markdown), (1-float64(len(markdown))/float64(len(htmlContent)))*100)

	return markdown, nil
}

// cleanHTMLJS keeps more attributes (class, id, etc.) for selector generation.
const cleanHTMLJS = `() => {
	const selectors = ['script', 'style', 'noscript', 'svg', 'canvas', 'video', 'audio', 'iframe', 'map', 'object', 'meta', 'link'];
	document.querySelectorAll(selectors.join(',')).forEach(el => el.remove());
	
	const elements = document.querySelectorAll('*');
	for (let i = 0; i < elements.length; i++) {
		const el = elements[i];
		const attrs = el.attributes;
		for (let j = attrs.length - 1; j >= 0; j--) {
			const name = attrs[j].name;
			if (!['href', 'src', 'alt', 'title', 'class', 'id', 'name', 'type', 'value', 'data-*', 'aria-*', 'role', 'rel', 'target', 'colspan', 'rowspan'].includes(name)) {
				if (!name.startsWith('data-') && !name.startsWith('aria-')) {
					el.removeAttribute(name);
				}
			}
		}
	}
	
	return document.body ? document.body.innerHTML : document.documentElement.innerHTML;
}`

// FetchHTML loads a page and returns cleaned HTML with attributes preserved.
func (m *Manager) FetchHTML(targetURL string) (string, error) {
	browser, browserCtx, page, err := m.launchBrowser()
	if err != nil {
		return "", err
	}
	defer browser.Close()
	defer browserCtx.Close()

	m.navigate(page, targetURL)

	rawContent, err := page.Evaluate(cleanHTMLJS)

	var htmlContent string
	if err == nil {
		if str, ok := rawContent.(string); ok {
			htmlContent = str
		}
	}

	if htmlContent == "" {
		log.Printf("Falling back to raw page.Content()")
		htmlContent, _ = page.Content()
	}

	re := regexp.MustCompile(`\s+`)
	htmlContent = re.ReplaceAllString(htmlContent, " ")
	htmlContent = strings.TrimSpace(htmlContent)

	log.Printf("🧹 CleanedHTML size: %d bytes", len(htmlContent))

	return htmlContent, nil
}

// domTreeJS returns a compact indented DOM tree for LLM analysis.
const domTreeJS = `() => {
	const skip = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'SVG', 'CANVAS', 'VIDEO', 'AUDIO', 'IFRAME', 'MAP', 'OBJECT', 'META', 'LINK']);
	const keepAttrs = new Set(['href', 'src', 'alt', 'title', 'class', 'id', 'name', 'type', 'value', 'role', 'rel', 'target', 'colspan', 'rowspan', 'for', 'placeholder', 'action', 'method']);
	
	function buildTree(el, depth) {
		if (depth > 8) return [];
		const lines = [];
		const children = Array.from(el.children);
		
		for (const child of children) {
			const tag = child.tagName.toLowerCase();
			if (skip.has(child.tagName)) continue;
			
			const indent = '  '.repeat(depth);
			
			let attrs = '';
			for (const a of child.attributes) {
				if (keepAttrs.has(a.name) || a.name.startsWith('data-') || a.name.startsWith('aria-')) {
					if (a.name === 'class') {
						attrs += ' class="' + a.value.split(' ').slice(0, 3).join(' ') + '"';
					} else if (a.value.length < 80) {
						attrs += ' ' + a.name + '="' + a.value + '"';
					}
				}
			}
			
			const text = (child.innerText || '').trim();
			const shortText = text.length > 60 ? text.substring(0, 60) + '...' : text;
			
			if (child.children.length === 0 || shortText) {
				lines.push(indent + '<' + tag + attrs + '>' + (shortText ? ' ' + shortText : '') + '</' + tag + '>');
			} else {
				lines.push(indent + '<' + tag + attrs + '>');
				lines.push(...buildTree(child, depth + 1));
				lines.push(indent + '</' + tag + '>');
			}
		}
		return lines;
	}
	
	return buildTree(document.body, 0).join('\n');
}`

// FetchDOMTree loads a page and returns a compact text-based DOM tree.
func (m *Manager) FetchDOMTree(targetURL string) (string, error) {
	browser, browserCtx, page, err := m.launchBrowser()
	if err != nil {
		return "", err
	}
	defer browser.Close()
	defer browserCtx.Close()

	m.navigate(page, targetURL)

	result, err := page.Evaluate(domTreeJS)

	var treeContent string
	if err == nil {
		if str, ok := result.(string); ok {
			treeContent = str
		}
	}

	if treeContent == "" {
		log.Printf("Falling back to cleaned HTML for DOM tree")
		treeContent, _ = page.Content()
		re := regexp.MustCompile(`\s+`)
		treeContent = re.ReplaceAllString(treeContent, " ")
		if len(treeContent) > 12000 {
			treeContent = treeContent[:12000]
		}
	}

	log.Printf("🌳 DOM Tree size: %d bytes", len(treeContent))
	return treeContent, nil
}

// NewPage creates a fresh browser + page and returns them with a cleanup function.
// Callers should defer cleanup() to close browser and context.
func (m *Manager) NewPage() (playwright.Page, func(), error) {
	return m.NewPageWithCookies("")
}

// NewPageWithCookies creates a fresh browser + page with cookies set.
// cookiesJSON should be a JSON array of cookie objects:
// [{"name":"session","value":"abc123","domain":"example.com","path":"/"}]
func (m *Manager) NewPageWithCookies(cookiesJSON string) (playwright.Page, func(), error) {
	browser, err := m.pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--disable-http2", "--disable-quic", "--no-sandbox",
			"--disable-setuid-sandbox", "--disable-dev-shm-usage",
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browserCtx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent:         playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		IgnoreHttpsErrors: playwright.Bool(true),
	})
	if err != nil {
		browser.Close()
		return nil, nil, fmt.Errorf("failed to create context: %w", err)
	}

	if cookiesJSON != "" {
		var rawCookies []playwright.OptionalCookie
		if err := json.Unmarshal([]byte(cookiesJSON), &rawCookies); err != nil {
			browserCtx.Close()
			browser.Close()
			return nil, nil, fmt.Errorf("invalid cookies JSON: %w", err)
		}
		if err := browserCtx.AddCookies(rawCookies); err != nil {
			browserCtx.Close()
			browser.Close()
			return nil, nil, fmt.Errorf("failed to add cookies: %w", err)
		}
	}

	page, err := browserCtx.NewPage()
	if err != nil {
		browserCtx.Close()
		browser.Close()
		return nil, nil, fmt.Errorf("failed to create page: %w", err)
	}

	cleanup := func() {
		browserCtx.Close()
		browser.Close()
	}

	return page, cleanup, nil
}
