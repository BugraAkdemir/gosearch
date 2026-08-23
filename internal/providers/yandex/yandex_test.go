package yandex

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
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "yandex", name))
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
		t.Fatalf("got %d results, want 3 (pagination and clck links must be skipped)", len(results))
	}

	if got, want := results[0].URL, "https://www.facebook.com/"; got != want {
		t.Errorf("result[0].URL = %q, want %q", got, want)
	}
	if got, want := results[0].Title, "Facebook — log in or sign up"; got != want {
		t.Errorf("result[0].Title = %q, want %q", got, want)
	}
	if !strings.Contains(results[0].Snippet, "Log into Facebook to start sharing") {
		t.Errorf("result[0].Snippet = %q, want it to contain the snippet text", results[0].Snippet)
	}

	// Protocol-relative href must be promoted to https.
	if got, want := results[1].URL, "https://en.wikipedia.org/wiki/Facebook"; got != want {
		t.Errorf("result[1].URL = %q, want %q (protocol-relative promoted)", got, want)
	}
	if !strings.Contains(results[1].Snippet, "social networking service") {
		t.Errorf("result[1].Snippet = %q, want it to contain the snippet text", results[1].Snippet)
	}

	// Third result has no organic__text block; it must still come back.
	if results[2].Snippet != "" {
		t.Errorf("result[2].Snippet = %q, want empty", results[2].Snippet)
	}
	if got, want := results[2].URL, "https://www.facebook.com/login/"; got != want {
		t.Errorf("result[2].URL = %q, want %q", got, want)
	}
}

// TestCleanURL covers the href variants Yandex serves for organic results and
// the internal plumbing the parser must reject.
func TestCleanURL(t *testing.T) {
	cases := []struct {
		name, href, want string
	}{
		{"direct destination", "https://ex.com/page?a=1", "https://ex.com/page?a=1"},
		{"protocol-relative promoted", "//ex.com/page", "https://ex.com/page"},
		{"clck with url param decoded", "https://yandex.com/clck/redirect?url=https%3A%2F%2Fex.com%2F&t=1", "https://ex.com/"},
		{"clck without decodable target rejected", "https://yandex.com/clck/redirect?redircnt=1", ""},
		{"relative pagination rejected", "/search/?text=x&page=2", ""},
		{"yandex search page rejected", "https://yandex.com/search/?text=x&page=3", ""},
		{"captcha path rejected", "https://yandex.com/showcaptcha?cc=1", ""},
		{"passport rejected", "https://passport.yandex.ru/auth", ""},
		{"legit yandex-hosted page kept", "https://yandex.com/support/mail/", "https://yandex.com/support/mail/"},
		{"non-http scheme rejected", "javascript:void(0)", ""},
		{"empty rejected", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanURL(tc.href); got != tc.want {
				t.Errorf("cleanURL(%q) = %q, want %q", tc.href, got, tc.want)
			}
		})
	}
}

// TestParseDeduplicatesResults ensures two serp-items pointing at the same
// destination yield one result.
func TestParseDeduplicatesResults(t *testing.T) {
	page := `<html><body><ul>
	<li class="serp-item"><a class="organic__url" href="https://a.example/"><h2>First</h2></a></li>
	<li class="serp-item"><a class="organic__url" href="https://a.example/"><h2>First again</h2></a></li>
	</ul></body></html>`
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
// Yandex's actual markup. real_success.html must be a real response captured
// from a network Yandex trusts; this sandbox is 302'd to showcaptcha instead,
// so until that capture lands the test skips itself. Once present it asserts
// structural properties only (count, non-empty fields), since live prose can
// drift on re-capture. See plan.md's Phase 3 exit criteria and AGENTS.md
// Known Pitfalls.
func TestParseRealSuccessFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "yandex", "real_success.html")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no real success capture yet (%s): sandbox IP is redirected to showcaptcha; capture one from a trusted network and drop it there", path)
		}
		t.Fatalf("read fixture: %v", err)
	}

	doc, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) == 0 {
		t.Fatalf("real markup yielded 0 results — heuristic no longer matches Yandex's current organic markup; capture and inspect the page before touching parse()")
	}
	for i, r := range results {
		if r.Title == "" || r.URL == "" {
			t.Errorf("result[%d] has empty Title/URL: %+v", i, r)
		}
		if !strings.HasPrefix(r.URL, "http") || strings.Contains(r.URL, "yandex.com/clck") {
			t.Errorf("result[%d].URL = %q, want a non-clck http(s) destination", i, r.URL)
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
		// Yandex takes its query in the text parameter.
		if r.URL.Query().Get("text") == "" {
			t.Errorf("expected text query param, got %s", r.URL.RawQuery)
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

// TestSearchDetectsCaptchaRedirect exercises block detection through Search:
// Yandex signals a challenge by 302-ing to a showcaptcha URL (observed on the
// live engine), which the client follows; Detect must classify the final URL.
func TestSearchDetectsCaptchaRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/showcaptcha" {
			// Terminal captcha page (as the live engine serves it).
			_, _ = w.Write([]byte(`<html><body>captcha</body></html>`))
			return
		}
		http.Redirect(w, r, srv.URL+"/showcaptcha?cc=1&retpath=x", http.StatusFound)
	}))
	defer srv.Close()

	old := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = old }()

	_, err := Search(context.Background(), newTestClient(t), "facebook", 0)
	if !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Search behind showcaptcha redirect = %v, want ErrChallenge", err)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><ul id="search-result"></ul></body></html>`))
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
