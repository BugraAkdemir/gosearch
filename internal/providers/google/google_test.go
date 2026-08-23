package google

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
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "google", name))
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
		t.Fatalf("got %d results, want 3 (pagination link must be skipped)", len(results))
	}

	// First result: redirect link must be decoded to the real destination.
	if got, want := results[0].URL, "https://www.facebook.com/"; got != want {
		t.Errorf("result[0].URL = %q, want %q (/url?q= decoded)", got, want)
	}
	if got, want := results[0].Title, "Facebook - log in or sign up"; got != want {
		t.Errorf("result[0].Title = %q, want %q", got, want)
	}
	if !strings.Contains(results[0].Snippet, "Log into Facebook to start sharing") {
		t.Errorf("result[0].Snippet = %q, want it to contain the snippet text", results[0].Snippet)
	}

	// Second result.
	if got, want := results[1].URL, "https://en.wikipedia.org/wiki/Facebook"; got != want {
		t.Errorf("result[1].URL = %q, want %q", got, want)
	}
	if !strings.Contains(results[1].Snippet, "social networking service") {
		t.Errorf("result[1].Snippet = %q, want it to contain the snippet text", results[1].Snippet)
	}

	// Third result has no snippet block; it must still be returned, snippet empty.
	if results[2].Snippet != "" {
		t.Errorf("result[2].Snippet = %q, want empty", results[2].Snippet)
	}
	if got, want := results[2].URL, "https://www.facebook.com/login/"; got != want {
		t.Errorf("result[2].URL = %q, want %q", got, want)
	}
}

// TestParseLegacyShape covers the older basic-HTML variant where the h3 wraps
// the link (h3.r > a) and the snippet is a span.st — the inverse nesting of
// the modern shape exercised by TestParseSuccessFixture.
func TestParseLegacyShape(t *testing.T) {
	page := `<html><body>
	<li class="g">
	  <h3 class="r"><a href="/url?q=https%3A%2F%2Fexample.org%2Fpage&amp;sa=U">Example Domain</a></h3>
	  <span class="st">This domain is for use in illustrative examples in documents.</span>
	</li>
	</body></html>`
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if got, want := results[0].URL, "https://example.org/page"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	if got, want := results[0].Title, "Example Domain"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if !strings.Contains(results[0].Snippet, "illustrative examples") {
		t.Errorf("Snippet = %q, want it to contain the st-span text", results[0].Snippet)
	}
}

// TestParseDeduplicatesResults ensures two redirect links pointing at the same
// destination yield one result (Google repeats links for sitelinks/media).
func TestParseDeduplicatesResults(t *testing.T) {
	page := `<html><body>
	<div class="g"><a href="/url?q=https%3A%2F%2Fa.example%2F&amp;sa=U"><h3>First</h3></a></div>
	<div class="g"><a href="/url?q=https%3A%2F%2Fa.example%2F&amp;sa=U"><h3>First again</h3></a></div>
	</body></html>`
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (duplicate destination deduplicated)", len(results))
	}
}

// TestParseRealSuccessFixture guards against silent parser breakage against
// Google's actual markup. real_success.html must be a real response captured
// from a network Google trusts (residential IP); this sandbox gets served a
// JS-challenge page instead, so until that capture lands the test skips
// itself. Once present it asserts structural properties only (count,
// non-empty fields), since live prose can drift on re-capture. See plan.md's
// Phase 2 exit criteria and AGENTS.md Known Pitfalls.
func TestParseRealSuccessFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "google", "real_success.html")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no real success capture yet (%s): sandbox IP is challenged by Google; capture one from a trusted network and drop it there", path)
		}
		t.Fatalf("read fixture: %v", err)
	}

	doc, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) == 0 {
		t.Fatalf("real markup yielded 0 results — heuristic no longer matches Google's current basic HTML; capture and inspect the page before touching detect.go or parse()")
	}
	for i, r := range results {
		if r.Title == "" || r.URL == "" {
			t.Errorf("result[%d] has empty Title/URL: %+v", i, r)
		}
		if !strings.HasPrefix(r.URL, "http") || strings.Contains(r.URL, "google.com/search") {
			t.Errorf("result[%d].URL = %q, want a non-Google http(s) destination", i, r.URL)
		}
	}
}

func TestParseMaxResults(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(string(fixture(t, "success.html"))))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 2)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestCleanURL(t *testing.T) {
	cases := []struct {
		name, href, want string
	}{
		{"encoded destination", "/url?q=https%3A%2F%2Fex.com%2Fa%3Fb%3D1%26c%3D2&sa=U", "https://ex.com/a?b=1&c=2"},
		{"absolute google.com/url form", "https://www.google.com/url?q=https%3A%2F%2Fex.com%2F&sa=U", "https://ex.com/"},
		{"missing q param", "/url?sa=U&ved=x", ""},
		{"not a /url redirect", "https://ex.com/plain", ""},
		{"non-http destination", "/url?q=%2Fsearch%3Fq%3Dx&sa=U", ""},
		{"wrapped google search pagination", "/url?q=https%3A%2F%2Fwww.google.com%2Fsearch%3Fq%3Dfacebook%26start%3D10&sa=U", ""},
		{"javascript destination", `/url?q=javascript%3Avoid(0)&sa=U`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanURL(tc.href); got != tc.want {
				t.Errorf("cleanURL(%q) = %q, want %q", tc.href, got, tc.want)
			}
		})
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
		_, _ = w.Write(fixture(t, "blocked.html")) // real captured challenge page, status 200
	}))
	defer srv.Close()

	old := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = old }()

	_, err := Search(context.Background(), newTestClient(t), "facebook", 0)
	// The captured page is Google's enablejs JS-challenge (Detect → ErrChallenge).
	if !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Search on challenge page = %v, want ErrChallenge", err)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="main"></div></body></html>`))
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
