package gosearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BugraAkdemir/gosearch/internal/httpclient"
	"github.com/BugraAkdemir/gosearch/internal/provider"
	"github.com/BugraAkdemir/gosearch/internal/serrors"
)

// withDispatch temporarily replaces the provider dispatch for a test.
func withDispatch(t *testing.T, fn func(context.Context, Engine, *httpclient.Client, string, int) ([]provider.Result, error)) {
	t.Helper()
	old := dispatch
	dispatch = fn
	t.Cleanup(func() { dispatch = old })
}

func TestSearchUnsupportedEngine(t *testing.T) {
	_, err := Search(context.Background(), "q", Engine(99))
	if !errors.Is(err, ErrUnsupportedEngine) {
		t.Fatalf("err = %v, want ErrUnsupportedEngine", err)
	}
}

func TestSearchFallbackAdvancesOnBlock(t *testing.T) {
	var tried []Engine
	withDispatch(t, func(_ context.Context, e Engine, _ *httpclient.Client, _ string, _ int) ([]provider.Result, error) {
		tried = append(tried, e)
		if e == Google {
			return nil, fmt.Errorf("%w: fake", serrors.ErrChallenge)
		}
		return []provider.Result{{Title: "ok", URL: "https://x", Snippet: "s"}}, nil
	})

	results, err := Search(context.Background(), "q", Google, WithFallback(DuckDuckGo))
	if err != nil {
		t.Fatalf("err = %v, want success via fallback", err)
	}
	if len(results) != 1 || results[0].Title != "ok" {
		t.Fatalf("results = %+v", results)
	}
	if len(tried) != 2 || tried[0] != Google || tried[1] != DuckDuckGo {
		t.Fatalf("tried = %v, want [Google DuckDuckGo]", tried)
	}
}

func TestSearchAllBlockedReturnsJoinedError(t *testing.T) {
	withDispatch(t, func(_ context.Context, e Engine, _ *httpclient.Client, _ string, _ int) ([]provider.Result, error) {
		return nil, fmt.Errorf("%w: %s down", serrors.ErrBlocked, e)
	})

	_, err := Search(context.Background(), "q", DuckDuckGo, WithFallback(Google, Yandex))
	if err == nil {
		t.Fatal("want error when all engines blocked")
	}
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("errors.Is(err, ErrBlocked) = false; err = %v", err)
	}
	// The joined error should mention each engine that failed.
	for _, name := range []string{"duckduckgo", "google", "yandex"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("joined error missing %q: %v", name, err)
		}
	}
}

func TestSearchNoResultsDoesNotFallback(t *testing.T) {
	var tried []Engine
	withDispatch(t, func(_ context.Context, e Engine, _ *httpclient.Client, _ string, _ int) ([]provider.Result, error) {
		tried = append(tried, e)
		return nil, fmt.Errorf("%w", serrors.ErrNoResults)
	})

	_, err := Search(context.Background(), "q", DuckDuckGo, WithFallback(Google))
	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("err = %v, want ErrNoResults", err)
	}
	if len(tried) != 1 {
		t.Fatalf("tried %v engines, want only the primary (no fallback on ErrNoResults)", tried)
	}
}

func TestFetchExtractsContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>My Page</title></head>
			<body><nav><a href="/">Home</a></nav>
			<article><p>This is the real body prose, with commas, long enough to score as the main content of the page.</p></article>
			</body></html>`))
	}))
	defer srv.Close()

	page, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "My Page" {
		t.Errorf("Title = %q, want 'My Page'", page.Title)
	}
	if !strings.Contains(page.Content, "real body prose") {
		t.Errorf("Content = %q, want it to contain the body prose", page.Content)
	}
	if strings.Contains(page.Content, "Home") {
		t.Errorf("Content should not contain nav text: %q", page.Content)
	}
}

func TestFetchDetectsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked on HTTP 403", err)
	}
}
