package duckduckgo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/BugraAkdemir/gosearch/internal/httpclient"
	"github.com/BugraAkdemir/gosearch/internal/serrors"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "duckduckgo", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestParseSuccessFixture(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(string(fixture(t, "success.html"))))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First result: redirect link must be decoded to the real destination.
	if got, want := results[0].URL, "https://www.facebook.com/"; got != want {
		t.Errorf("result[0].URL = %q, want %q (uddg decoded)", got, want)
	}
	if got, want := results[0].Title, "Facebook - log in or sign up"; got != want {
		t.Errorf("result[0].Title = %q, want %q", got, want)
	}
	if !strings.Contains(results[0].Snippet, "Log into Facebook") {
		t.Errorf("result[0].Snippet = %q, want it to contain the snippet text", results[0].Snippet)
	}

	// Second result: direct URL passes through unchanged.
	if got, want := results[1].URL, "https://en.wikipedia.org/wiki/Facebook"; got != want {
		t.Errorf("result[1].URL = %q, want %q", got, want)
	}

	// Third result has no snippet; it must still be returned, snippet empty.
	if results[2].Snippet != "" {
		t.Errorf("result[2].Snippet = %q, want empty", results[2].Snippet)
	}
	if got, want := results[2].URL, "https://www.facebook.com/login/"; got != want {
		t.Errorf("result[2].URL = %q, want %q", got, want)
	}
}

// TestParseRealSuccessFixture guards against silent parser breakage against
// DuckDuckGo's actual markup. Unlike TestParseSuccessFixture (a small
// synthetic page), real_success.html is a real response captured 2026-08-23
// from a residential IP for the query "facebook" (see AGENTS.md's Known
// Pitfalls — Session 1 had only ever validated parse() against synthetic
// markup). It only asserts structural properties (count, non-empty fields,
// decoded URLs), not exact snippet text, since a live capture's prose can
// drift on re-capture even though the markup shape stays stable.
func TestParseRealSuccessFixture(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(string(fixture(t, "real_success.html"))))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
	}
	for i, r := range results {
		if r.Title == "" {
			t.Errorf("result[%d].Title is empty", i)
		}
		if r.URL == "" {
			t.Errorf("result[%d].URL is empty", i)
		}
		if strings.Contains(r.URL, "duckduckgo.com/l/") {
			t.Errorf("result[%d].URL = %q, want uddg redirect decoded to real destination", i, r.URL)
		}
	}
	if got, want := results[0].URL, "https://www.facebook.com/"; got != want {
		t.Errorf("result[0].URL = %q, want %q", got, want)
	}
}

func TestParseMaxResults(t *testing.T) {
	doc, _ := html.Parse(strings.NewReader(string(fixture(t, "success.html"))))
	if got := parse(doc, 2); len(got) != 2 {
		t.Errorf("parse(maxResults=2) returned %d, want 2", len(got))
	}
}

// TestParseDedupsEncodingVariants pins canonical-URL dedup: one page listed
// under different spellings of the same query value must yield one result,
// keeping the first-seen original spelling in Result.URL.
func TestParseDedupsEncodingVariants(t *testing.T) {
	page := `<html><body>
<div class="result"><h2 class="result__title"><a class="result__a" href="https://ex.com/p?il=%C4%B0stanbul">Bir</a></h2></div>
<div class="result"><h2 class="result__title"><a class="result__a" href="https://ex.com/p?il=Istanbul">İki</a></h2></div>
</body></html>`
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (encoding variants are one page)", len(results))
	}
	if got, want := results[0].URL, "https://ex.com/p?il=%C4%B0stanbul"; got != want {
		t.Errorf("kept URL = %q, want first-seen original %q", got, want)
	}
}

func TestCleanURL(t *testing.T) {
	cases := map[string]string{
		"//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fx&rut=z": "https://example.com/x",
		"https://direct.example.com/page":                              "https://direct.example.com/page",
		"//cdn.example.com/rel":                                        "https://cdn.example.com/rel",
		"":                                                             "",
	}
	for in, want := range cases {
		if got := cleanURL(in); got != want {
			t.Errorf("cleanURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func newTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSearchEndToEndSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			t.Errorf("expected q query param, got %s", r.URL.RawQuery)
		}
		_, _ = w.Write(fixture(t, "success.html"))
	}))
	defer srv.Close()

	old := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = old }()

	results, err := Search(context.Background(), newTestClient(t), "facebook", 0)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
}

func TestSearchDetectsBlockedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "blocked.html")) // real captured captcha page, status 200
	}))
	defer srv.Close()

	old := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = old }()

	_, err := Search(context.Background(), newTestClient(t), "facebook", 0)
	if !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Search on captcha page = %v, want ErrChallenge", err)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="links" class="results"></div></body></html>`))
	}))
	defer srv.Close()

	old := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = old }()

	_, err := Search(context.Background(), newTestClient(t), "asdkjhaskdjh", 0)
	if !errors.Is(err, serrors.ErrNoResults) {
		t.Fatalf("Search on empty page = %v, want ErrNoResults", err)
	}
}
