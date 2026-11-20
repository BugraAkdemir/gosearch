package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/chromedp/chromedp"

	"github.com/BugraAkdemir/gosearch"
	"github.com/BugraAkdemir/gosearch/internal/readability"
)

// fetchTimeout bounds one rendered page load and extraction.
const fetchTimeout = 45 * time.Second

// maxHTMLBytes caps the rendered HTML passed into readability, mirroring
// internal/httpclient's maxBodyBytes (8 MiB) so an unbounded infinite-scroll
// DOM cannot exhaust memory.
const maxHTMLBytes = 8 << 20 // 8 MiB

// Fetch retrieves url in the browser, waits for JavaScript to render it, and
// extracts the readable main content into a gosearch.Page — a drop-in swap for
// gosearch.Fetch for callers whose target pages need JS.
//
// Extraction delegates to the core module's DOM-based readability package
// (shared across both modules so plain-text and WithMarkdown output stay
// identical between HTTP and browser fetches). Captured HTML is capped at
// 8 MiB on a UTF-8 and tag-safe boundary.
func (e *Engine) Fetch(ctx context.Context, rawURL string) (*gosearch.Page, error) {
	if !validHTTPOrHTTPS(rawURL) {
		return nil, fmt.Errorf("browser: fetch %q: only http(s) URLs are supported", rawURL)
	}
	var raw string
	err := e.run(ctx, fetchTimeout,
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(fetchHTMLJS, &raw),
	)
	if err != nil {
		return nil, classifyRenderError("fetch", err, e.pageDiagnostics(ctx))
	}

	var rendered struct {
		URL  string `json:"url"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(raw), &rendered); err != nil {
		return nil, fmt.Errorf("browser: decode render output: %w", err)
	}
	finalURL := rendered.URL
	if finalURL == "" {
		finalURL = rawURL
	}
	htmlBytes := truncateHTMLSafe(rendered.HTML, maxHTMLBytes)
	return extractPage(htmlBytes, finalURL, e.markdown)
}

// extractPage runs the core readability extractor (plain text or Markdown) on
// rendered HTML bytes and wraps the result into a gosearch.Page.
func extractPage(htmlBytes []byte, finalURL string, markdown bool) (*gosearch.Page, error) {
	var (
		art *readability.Article
		err error
	)
	if markdown {
		art, err = readability.ExtractMarkdown(htmlBytes)
	} else {
		art, err = readability.Extract(htmlBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("browser: extract %q: %w", finalURL, err)
	}
	return &gosearch.Page{URL: finalURL, Title: art.Title, Content: art.Content}, nil
}

// truncateHTMLSafe caps raw to at most maxBytes bytes without splitting a
// multi-byte UTF-8 rune or leaving an unclosed opening/closing HTML tag at the
// tail. When raw already fits within maxBytes it is returned as-is.
func truncateHTMLSafe(raw string, maxBytes int) []byte {
	if maxBytes <= 0 {
		return nil
	}
	if len(raw) <= maxBytes {
		return []byte(raw)
	}
	b := []byte(raw[:maxBytes])

	// Trim any incomplete trailing UTF-8 sequence without disturbing a
	// complete multi-byte rune sitting flush against the boundary.
	for n := 1; n <= utf8.UTFMax && n <= len(b); n++ {
		if utf8.RuneStart(b[len(b)-n]) {
			if !utf8.FullRune(b[len(b)-n:]) {
				b = b[:len(b)-n]
			}
			break
		}
	}

	// If the cut landed mid-tag ("<div class=..."), back up to before '<' so
	// the HTML parser sees well-bounded markup.
	if lastLT := bytes.LastIndexByte(b, '<'); lastLT >= 0 {
		if bytes.IndexByte(b[lastLT:], '>') < 0 {
			b = b[:lastLT]
		}
	}
	return b
}

// validHTTPOrHTTPS reports whether rawURL parses to an http(s) URL.
func validHTTPOrHTTPS(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}
