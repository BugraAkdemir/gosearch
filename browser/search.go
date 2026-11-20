package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/BugraAkdemir/gosearch"
	"github.com/BugraAkdemir/gosearch/internal/provider"
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
	diag, err := e.run(ctx, e.effectiveTimeout(searchTimeout),
		chromedp.Navigate(googleSearchURL(query)),
		chromedp.ActionFunc(dismissConsentIfNeeded),
		chromedp.WaitVisible("h3", chromedp.ByQuery),
		chromedp.Evaluate(searchExtractJS, &raw),
	)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("browser: search: %w", err)
		}
		return nil, classifyRenderError("search", err, diag)
	}

	var raws []renderResult
	if err := json.Unmarshal([]byte(raw), &raws); err != nil {
		return nil, fmt.Errorf("browser: decode render output: %w", err)
	}

	organic := normalizeSearchResults(raws)
	if len(organic) == 0 {
		return nil, fmt.Errorf("browser: %w: page rendered but no organic results matched the heuristic",
			gosearch.ErrNoResults)
	}
	return applySearchPolicy(organic, e.maxResults, e.allowedDomains, e.blockedDomains), nil
}

// normalizeSearchResults filters raw DOM candidates down to usable,
// canonically deduplicated organic results (matching core providers' use of
// internal/provider.NormalizeURL).
func normalizeSearchResults(raws []renderResult) []gosearch.Result {
	out := make([]gosearch.Result, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, r := range raws {
		u := strings.TrimSpace(r.URL)
		title := strings.TrimSpace(r.Title)
		if u == "" || title == "" || !usableDestination(u) {
			continue
		}
		key := provider.NormalizeURL(u)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, gosearch.Result{
			Title:   title,
			URL:     u,
			Snippet: strings.TrimSpace(r.Snippet),
		})
	}
	return out
}

// applySearchPolicy enforces domain allow/deny filters (deny first, then
// allow — identical to core gosearch.applyDomainPolicy) and caps output at
// maxResults (falling back to maxSearchResults when maxResults <= 0).
func applySearchPolicy(in []gosearch.Result, maxResults int, allowedDomains, blockedDomains []string) []gosearch.Result {
	capLimit := maxResults
	if capLimit <= 0 {
		capLimit = maxSearchResults
	}
	filtered := applyDomainPolicy(in, allowedDomains, blockedDomains)
	if len(filtered) > capLimit {
		return filtered[:capLimit]
	}
	return filtered
}

// applyDomainPolicy enforces suffix-host domain filtering identical to core
// gosearch: deny first, then allowlist; results without a parsable host fail
// an active allowlist.
func applyDomainPolicy(rs []gosearch.Result, allowedDomains, blockedDomains []string) []gosearch.Result {
	if len(allowedDomains) == 0 && len(blockedDomains) == 0 {
		return rs
	}
	lower := func(ds []string) []string {
		out := make([]string, len(ds))
		for i, d := range ds {
			out[i] = strings.ToLower(strings.Trim(d, "."))
		}
		return out
	}
	denied := lower(blockedDomains)
	allowed := lower(allowedDomains)

	matches := func(host, domain string) bool {
		return domain != "" && (host == domain || strings.HasSuffix(host, "."+domain))
	}
	hostOf := func(rawURL string) string {
		u, err := url.Parse(rawURL)
		if err != nil {
			return ""
		}
		return strings.ToLower(u.Hostname())
	}

	out := make([]gosearch.Result, 0, len(rs))
	for _, r := range rs {
		host := hostOf(r.URL)
		blocked := false
		for _, d := range denied {
			if host != "" && matches(host, d) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		if len(allowed) > 0 {
			ok := false
			for _, d := range allowed {
				if host != "" && matches(host, d) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, r)
	}
	return out
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
// level failures, context cancellation, and closed-engine errors surface as-is.
// The landed-on URL and page title are appended when reachable so a failing
// run reports WHERE the engine ended up instead of a bare timeout.
func classifyRenderError(op string, err error, diag string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrEngineClosed) {
		return fmt.Errorf("browser: %s: %w", op, err)
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "h3 not found") {
		msg := "no result markup appeared (likely consent wall or captcha)"
		if diag != "" {
			msg += "; landed on: " + diag
		}
		return fmt.Errorf("browser: %s: %s: %w: %w", op, msg, gosearch.ErrChallenge, err)
	}
	return fmt.Errorf("browser: %s: %w", op, err)
}

// pageDiagnostics best-effort captures where tabCtx actually is (final URL +
// document title) for error reporting. Never panics or fails the caller: a
// nil, canceled, or dead tab context simply yields an empty string.
func pageDiagnostics(tabCtx context.Context) string {
	if tabCtx == nil || tabCtx.Err() != nil {
		return ""
	}
	var raw string
	dctx, cancel := context.WithTimeout(tabCtx, 3*time.Second)
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
