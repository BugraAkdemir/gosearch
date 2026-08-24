package gosearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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

// TestSearchDatesOptIn pins the WithDates contract: providers may surface a
// result's date, but it reaches the caller only when explicitly requested —
// callers who need timeless results (historical queries, stable snapshots)
// never see dates by accident.
func TestSearchDatesOptIn(t *testing.T) {
	withDispatch(t, func(_ context.Context, _ Engine, _ *httpclient.Client, _ string, _ int) ([]provider.Result, error) {
		return []provider.Result{{
			Title: "t", URL: "https://x", Snippet: "s", Date: "2026-08-20",
		}}, nil
	})

	defaultResults, err := Search(context.Background(), "q", DuckDuckGo)
	if err != nil {
		t.Fatal(err)
	}
	if defaultResults[0].Date != "" {
		t.Errorf("default Date = %q, want empty without WithDates", defaultResults[0].Date)
	}

	dated, err := Search(context.Background(), "q", DuckDuckGo, WithDates())
	if err != nil {
		t.Fatal(err)
	}
	if dated[0].Date != "2026-08-20" {
		t.Errorf("WithDates() Date = %q, want %q", dated[0].Date, "2026-08-20")
	}
}

// stubResults dispatches a fixed result set for filter testing.
func stubResults(t *testing.T, rs []provider.Result) {
	t.Helper()
	withDispatch(t, func(_ context.Context, _ Engine, _ *httpclient.Client, _ string, _ int) ([]provider.Result, error) {
		return rs, nil
	})
}

func sampleResults() []provider.Result {
	return []provider.Result{
		{Title: "t1", URL: "https://good.example.com/a"},
		{Title: "t2", URL: "https://spam.example.net/b"},
		{Title: "t3", URL: "https://www.spam.example.net/"},
		{Title: "t4", URL: "https://notspam.example.net/c"},
		{Title: "t5", URL: "https://other.org/d"},
	}
}

func hostsOf(rs []Result) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		u, _ := url.Parse(r.URL)
		out = append(out, u.Hostname())
	}
	return out
}

// TestSearchDomainFilters pins the allow/block policy: blocked domains match
// their subdomains but not longer host names that merely end with the same
// text; an allowlist keeps only suffix matches; deny is applied before allow;
// and without either list nothing is filtered.
func TestSearchDomainFilters(t *testing.T) {
	stubResults(t, sampleResults())

	unfiltered, err := Search(context.Background(), "q", DuckDuckGo)
	if err != nil || len(unfiltered) != 5 {
		t.Fatalf("unfiltered = %d results (err %v), want 5", len(unfiltered), err)
	}

	denied, err := Search(context.Background(), "q", DuckDuckGo,
		WithBlockedDomains("spam.example.net"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"good.example.com", "notspam.example.net", "other.org"}
	if got := hostsOf(denied); !reflect.DeepEqual(got, want) {
		t.Errorf("deny kept %v, want %v (subdomains die, lookalike 'notspam' survives)", got, want)
	}

	allowed, err := Search(context.Background(), "q", DuckDuckGo,
		WithAllowedDomains("example.net"))
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"spam.example.net", "www.spam.example.net", "notspam.example.net"}
	if got := hostsOf(allowed); !reflect.DeepEqual(got, want) {
		t.Errorf("allow kept %v, want %v", got, want)
	}

	both, err := Search(context.Background(), "q", DuckDuckGo,
		WithAllowedDomains("example.net"),
		WithBlockedDomains("notspam.example.net"))
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"spam.example.net", "www.spam.example.net"}
	if got := hostsOf(both); !reflect.DeepEqual(got, want) {
		t.Errorf("deny+allow kept %v, want %v (deny wins over allow)", got, want)
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

// TestFetchMarkdownOption pins the WithMarkdown contract: opting in renders
// Page.Content as Markdown (structure preserved), while the default call keeps
// returning plain text.
func TestFetchMarkdownOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>MD</title></head><body>
			<article>
			<h2>Kurulum</h2>
			<p>This is the real body prose, with commas, long enough to score as the main content of the page.</p>
			<ul><li>first step</li><li>second step</li></ul>
			</article></body></html>`))
	}))
	defer srv.Close()

	mdPage, err := Fetch(context.Background(), srv.URL, WithMarkdown())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Kurulum", "- first step\n- second step"} {
		if !strings.Contains(mdPage.Content, want) {
			t.Errorf("markdown Content missing %q; got:\n%s", want, mdPage.Content)
		}
	}

	plainPage, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plainPage.Content, "## Kurulum") ||
		strings.Contains(plainPage.Content, "- first step") {
		t.Errorf("default Fetch leaked markdown:\n%s", plainPage.Content)
	}
}
