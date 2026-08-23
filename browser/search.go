package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/BugraAkdemir/gosearch"
)

// searchTimeout bounds one rendered search: navigation plus JS settle time.
const searchTimeout = 30 * time.Second

// maxSearchResults caps parsed results; the rendered page carries far more
// anchors than organic results and dedup/filtering runs before this cap.
const maxSearchResults = 30

// Search renders Google for query in the shared tab and extracts results
// from the DOM AFTER JavaScript has run — the case plain HTTP cannot handle.
//
// The heuristic is best-effort exactly like the core providers: every anchor
// containing an <h3> is treated as a result candidate, titles come from the
// h3, snippets from the surrounding result container's text with the title
// line removed. Non-http(s) destinations, Google-internal plumbing (search
// pagination, accounts, support, /url wrappers) and duplicates are skipped.
// When no h3 ever appears (consent wall, captcha, unusual layout), Search
// returns an error wrapping gosearch.ErrChallenge rather than empty results,
// so callers can distinguish "no answers" from "engine refused".
func (e *Engine) Search(ctx context.Context, query string) ([]gosearch.Result, error) {
	e.warmUp(ctx)

	// Navigate, clear Google's cookie-consent interstitial when it appears
	// (a normal visitor clicks "Accept"; so do we), then wait for results.
	// No hl/num overrides: default locale and result count look like an
	// ordinary visit, which is exactly the point.
	var raw string
	err := e.run(ctx, searchTimeout,
		chromedp.Navigate(googleSearchURL(query)),
		chromedp.ActionFunc(dismissConsentIfNeeded),
		chromedp.WaitVisible("h3", chromedp.ByQuery),
		chromedp.Evaluate(searchExtractJS, &raw),
	)
	if err != nil {
		diag := e.pageDiagnostics(ctx)
		return nil, classifyRenderError("search", err, diag)
	}

	var raws []renderResult
	if err := json.Unmarshal([]byte(raw), &raws); err != nil {
		return nil, fmt.Errorf("browser: decode render output: %w", err)
	}

	out := make([]gosearch.Result, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, r := range raws {
		u := strings.TrimSpace(r.URL)
		title := strings.TrimSpace(r.Title)
		if u == "" || title == "" || seen[u] || !usableDestination(u) {
			continue
		}
		seen[u] = true
		out = append(out, gosearch.Result{
			Title:   title,
			URL:     u,
			Snippet: strings.TrimSpace(r.Snippet),
		})
		if len(out) >= maxSearchResults {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("browser: %w: page rendered but no organic results matched the heuristic",
			gosearch.ErrNoResults)
	}
	return out, nil
}

// renderResult is one candidate extracted by searchExtractJS.
type renderResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// usableDestination filters rendered hrefs down to real result pages.
// Rendered Google links are absolute http(s); internal plumbing lives on
// google.* hosts under known path prefixes.
func usableDestination(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if !strings.Contains(host, "google.") {
		return true
	}
	for _, p := range []string{"/search", "/url", "/imgres", "/travel", "/maps"} {
		if strings.HasPrefix(u.Path, p) {
			return false
		}
	}
	switch host {
	case "accounts.google.com", "support.google.com", "policies.google.com":
		return false
	}
	return true
}

// googleSearchURL builds the desktop search URL the engine navigates to.
func googleSearchURL(query string) string {
	return "https://www.google.com/search?q=" + url.QueryEscape(query)
}

// classifyRenderError maps chromedp failures onto gosearch's sentinel
// vocabulary: a missing h3 after a successful navigation almost always means
// a consent wall, captcha, or challenge — ErrChallenge — while transport-
// level failures surface as-is. The landed-on URL and page title are appended
// when reachable so a failing run reports WHERE the engine ended up instead
// of a bare timeout.
func classifyRenderError(op string, err error, diag string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "h3 not found") {
		msg := "no result markup appeared (likely consent wall or captcha)"
		if diag != "" {
			msg += "; landed on: " + diag
		}
		return fmt.Errorf("browser: %s: %s: %w", op, msg, gosearch.ErrChallenge)
	}
	return fmt.Errorf("browser: %s: %w", op, err)
}

// pageDiagnostics best-effort captures where the tab actually is (final URL
// + document title) for error reporting. Never fails the caller: any problem
// yields an empty string.
func (e *Engine) pageDiagnostics(_ context.Context) string {
	var raw string
	dctx, cancel := context.WithTimeout(e.ctx, 3*time.Second)
	defer cancel()
	if err := chromedp.Run(dctx, chromedp.Evaluate(pageStateJS, &raw)); err != nil || raw == "" {
		return ""
	}
	var st struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(raw), &st) != nil {
		return ""
	}
	switch {
	case st.URL != "" && st.Title != "":
		return st.URL + " (" + st.Title + ")"
	case st.URL != "":
		return st.URL
	default:
		return ""
	}
}
