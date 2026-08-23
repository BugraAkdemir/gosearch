package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/BugraAkdemir/gosearch"
)

// fetchTimeout bounds one rendered page load and extraction.
const fetchTimeout = 45 * time.Second

// Fetch retrieves url in the shared tab, waits for JavaScript to render it,
// and extracts the readable main content into a gosearch.Page — a drop-in
// swap for gosearch.Fetch for callers whose target pages need JS.
//
// The extractor prefers <article>/<main>, falls back to the text-heaviest
// container heuristic, strips script/style/nav/header/footer/aside/form
// noise, and caps output at ~20k characters. It is deliberately simpler than
// the core module's DOM-based readability extractor: rendered innerText has
// already discarded most chrome, so heavy scoring buys little here.
func (e *Engine) Fetch(ctx context.Context, rawURL string) (*gosearch.Page, error) {
	if !validHTTPOrHTTPS(rawURL) {
		return nil, fmt.Errorf("browser: fetch %q: only http(s) URLs are supported", rawURL)
	}
	var raw string
	err := e.run(ctx, fetchTimeout,
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(fetchExtractJS, &raw),
	)
	if err != nil {
		return nil, classifyRenderError("fetch", err, e.pageDiagnostics(ctx))
	}

	var page struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return nil, fmt.Errorf("browser: decode render output: %w", err)
	}
	final := page.URL
	if final == "" {
		final = rawURL
	}
	return &gosearch.Page{URL: final, Title: page.Title, Content: page.Content}, nil
}

// validHTTPOrHTTPS reports whether rawURL parses to an http(s) URL.
func validHTTPOrHTTPS(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}
