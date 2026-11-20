package browser

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateProxyURL(t *testing.T) {
	valid := []string{
		"",
		"http://127.0.0.1:8080",
		"https://proxy.example.com:8443",
		"socks5://127.0.0.1:1080",
		"socks5h://user:pass@proxy.local:1080",
		"socks4://127.0.0.1:1080",
	}
	for _, raw := range valid {
		if err := validateProxyURL(raw); err != nil {
			t.Errorf("validateProxyURL(%q) unexpected error: %v", raw, err)
		}
	}

	invalid := []string{
		"ftp://proxy.example.com:21",
		"127.0.0.1:8080",
		"http://",
		"://bad-url",
	}
	for _, raw := range invalid {
		if err := validateProxyURL(raw); err == nil {
			t.Errorf("validateProxyURL(%q) = nil, want error", raw)
		}
	}
}

func TestNewRejectsInvalidProxyBeforeStarting(t *testing.T) {
	badBin := filepath.Join(t.TempDir(), "fake-browser")
	_, err := New(context.Background(), WithExecutable(badBin), WithProxy("ftp://not-supported:21"))
	if err == nil {
		t.Fatal("New with unsupported proxy scheme succeeded, want error")
	}
}

func TestWithTimeoutOptionAndEffectiveTimeout(t *testing.T) {
	badBin := filepath.Join(t.TempDir(), "fake-browser")
	e, err := New(context.Background(), WithExecutable(badBin))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	if got := e.effectiveTimeout(fetchTimeout); got != fetchTimeout {
		t.Errorf("default effectiveTimeout(fetch) = %v, want %v", got, fetchTimeout)
	}

	eCustom, err := New(context.Background(), WithExecutable(badBin), WithTimeout(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eCustom.Close() }()
	if got := eCustom.effectiveTimeout(fetchTimeout); got != 12*time.Second {
		t.Errorf("custom effectiveTimeout = %v, want 12s", got)
	}

	// Non-positive resets back to defaults.
	cfg := &engineConfig{timeout: 10 * time.Second}
	WithTimeout(0)(cfg)
	if cfg.timeout != 0 {
		t.Errorf("WithTimeout(0) left timeout=%v, want 0", cfg.timeout)
	}
	WithTimeout(-5 * time.Second)(cfg)
	if cfg.timeout != 0 {
		t.Errorf("WithTimeout(-5s) left timeout=%v, want 0", cfg.timeout)
	}
}

func TestNormalizeAndApplySearchPolicy(t *testing.T) {
	raws := []renderResult{
		{Title: "Spam", URL: "https://sub.spam.example/a", Snippet: "s1"},
		{Title: "Good 1", URL: "https://docs.golang.org/pkg?a=1&b=2", Snippet: "g1"},
		// Canonical duplicate of Good 1 (different host case, fragment, param order).
		{Title: "Good 1 Dup", URL: "https://DOCS.GOLANG.ORG/pkg?b=2&a=1#section", Snippet: "dup"},
		{Title: "NotSpam", URL: "https://notspam.example/ok", Snippet: "ns"},
		{Title: "Good 2", URL: "https://golang.org/doc", Snippet: "g2"},
		{Title: "Other", URL: "https://other.net/page", Snippet: "o"},
		{Title: "Google Internal", URL: "https://www.google.com/search?q=x", Snippet: "internal"},
	}

	organic := normalizeSearchResults(raws)
	if len(organic) != 5 {
		t.Fatalf("normalizeSearchResults len = %d, want 5 (dedup + internal filter): %+v", len(organic), organic)
	}

	// Deny removes sub.spam.example, preserves notspam.example; cap applies AFTER deny.
	denied := applySearchPolicy(organic, 2, nil, []string{"spam.example"})
	if len(denied) != 2 {
		t.Fatalf("denied+capped len = %d, want 2: %+v", len(denied), denied)
	}
	if denied[0].Title != "Good 1" || denied[1].Title != "NotSpam" {
		t.Errorf("unexpected denied results: %+v", denied)
	}

	// Allow + Deny combined: deny wins over allow for subdomains.
	combo := applySearchPolicy(organic, 0, []string{"golang.org"}, []string{"docs.golang.org"})
	if len(combo) != 1 || combo[0].Title != "Good 2" {
		t.Errorf("allow+deny combo = %+v, want only Good 2", combo)
	}
}

func TestWaitOptionsAndSleepActionCancel(t *testing.T) {
	cfg := &engineConfig{}
	WithWaitSelector("  article.main  ")(cfg)
	if cfg.waitSelector != "article.main" {
		t.Errorf("waitSelector = %q, want article.main", cfg.waitSelector)
	}
	WithWaitDelay(250 * time.Millisecond)(cfg)
	if cfg.waitDelay != 250*time.Millisecond {
		t.Errorf("waitDelay = %v, want 250ms", cfg.waitDelay)
	}
	WithWaitDelay(-1)(cfg)
	if cfg.waitDelay != 0 {
		t.Errorf("WithWaitDelay(-1) left %v, want 0", cfg.waitDelay)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := sleepAction(5 * time.Second).Do(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("sleepAction(canceled) err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("sleepAction did not return promptly on cancel: %v", time.Since(start))
	}
}
