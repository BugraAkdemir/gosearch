package bing

import (
	"context"
	"encoding/base64"
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
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "bing", name))
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
		t.Fatalf("got %d results, want 3 (cite-less result must be skipped)", len(results))
	}

	// First result: segmented citation rebuilt into a navigable URL.
	if got, want := results[0].URL, "https://www.mgm.gov.tr/tahmin/il-ve-ilceler.aspx"; got != want {
		t.Errorf("result[0].URL = %q, want %q (cite reconstructed)", got, want)
	}
	if !strings.Contains(results[0].Title, "Meteoroloji") {
		t.Errorf("result[0].Title = %q", results[0].Title)
	}
	if !strings.Contains(results[0].Snippet, "Genel Müdürlüğü") {
		t.Errorf("result[0].Snippet = %q, want caption text", results[0].Snippet)
	}

	// Second result: scheme-less host promoted to https.
	if got, want := results[1].URL, "https://havadurumu15gunluk.xyz/istanbul"; got != want {
		t.Errorf("result[1].URL = %q, want %q", got, want)
	}

	// Third result: truncated cite loses its tail but keeps the parent path.
	if got, want := results[2].URL, "https://www.accuweather.com/tr/tr-istanbul-weather"; got != want {
		t.Errorf("result[2].URL = %q, want truncation-stripped parent path", got)
	}
}

func TestDecodeTrackerParam(t *testing.T) {
	dest := "https://en.wikipedia.org/wiki/Facebook"
	tok := "a1" + strings.ReplaceAll(base64.URLEncoding.EncodeToString([]byte(dest)), "/", "!")
	href := "https://www.bing.com/ck/a?!&&p=opaque&u=" + tok + "&ntb=1"

	if got := decodeTrackerParam(href); got != dest {
		t.Errorf("decodeTrackerParam = %q, want %q", got, dest)
	}
	if got := decodeTrackerParam("https://www.bing.com/ck/a?!&&p=opaque"); got != "" {
		t.Errorf("decodeTrackerParam without u param = %q, want empty", got)
	}
}

// TestFromCiteTable pins the citation-reconstruction rules.
func TestFromCiteTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://ex.com/a/b", "https://ex.com/a/b"},
		{"ex.com › a › b", "https://ex.com/a/b"},
		{"https://ex.com › deep › path…", "https://ex.com/deep/path"},
		{"www.ex.com", "https://www.ex.com"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := fromCite(tc.in); got != tc.want {
			t.Errorf("fromCite(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestParseRealSuccessFixture guards against silent parser breakage against
// Bing's actual markup. real_success.html must be a real response captured
// from any network — Bing served clean results even to this project's flagged
// datacenter IP during the 2026-08-24 probe, so capturing one is easy. Until
// the file lands the test skips itself. See plan.md and AGENTS.md Known
// Pitfalls.
func TestParseRealSuccessFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "bing", "real_success.html")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no real success capture yet (%s): run `curl -sA <chrome UA> 'https://www.bing.com/search?q=test' -o testdata/bing/real_success.html` from any network", path)
		}
		t.Fatalf("read fixture: %v", err)
	}

	doc, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) == 0 {
		t.Fatal("real markup yielded 0 results — heuristic no longer matches Bing's organic markup; inspect the capture before touching parse()")
	}
	for i, r := range results {
		if r.Title == "" || r.URL == "" || !strings.HasPrefix(r.URL, "http") {
			t.Errorf("result[%d] incomplete: %+v", i, r)
		}
	}
}

func TestParseMaxResults(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(string(fixture(t, "success.html"))))
	if err != nil {
		t.Fatal(err)
	}
	if got := parse(doc, 2); len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
}

// TestParseDedupsEncodingVariants pins the canonical-URL dedup: engines list
// one page under several spellings (percent-encoded vs literal non-ASCII,
// dotted vs undotted capital I — observed live on Bing), and callers must see
// one result. The first-seen original spelling is what surfaces in Result.URL.
func TestParseDedupsEncodingVariants(t *testing.T) {
	page := `<!doctype html><html><body><ol id="b_results">
<li class="b_algo">
  <h2><a href="https://www.bing.com/ck/a?!&&amp;&amp;p=tok1">Bir</a></h2>
  <div class="b_attribution"><cite>https://ex.com/p?il=%C4%B0stanbul</cite></div>
</li>
<li class="b_algo">
  <h2><a href="https://www.bing.com/ck/a?!&&amp;&amp;p=tok2">İki</a></h2>
  <div class="b_attribution"><cite>https://ex.com/p?il=Istanbul</cite></div>
</li>
</ol></body></html>`
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

func newTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestParseExtractsDate pins date extraction: Bing stamps fresh results with
// a news_dt span inside the caption; generic pages may carry a <time
// datetime>. Both must land in Result.Date; absence stays "".
func TestParseExtractsDate(t *testing.T) {
	page := `<!doctype html><html><body><ol id="b_results">
<li class="b_algo">
  <h2><a href="https://www.bing.com/ck/a?!&&amp;&amp;p=tok1">Taze</a></h2>
  <div class="b_attribution"><cite>https://ex.com/a</cite></div>
  <div class="b_caption"><p><span class="news_dt">1 day ago</span> · Guncel icerik metni burada uzayarak devam ediyor.</p></div>
</li>
<li class="b_algo">
  <h2><a href="https://www.bing.com/ck/a?!&&amp;&amp;p=tok2">Zamanli</a></h2>
  <div class="b_attribution"><cite>https://ex.com/b</cite></div>
  <time datetime="2026-08-20">Aug 20</time>
</li>
</ol></body></html>`
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	results := parse(doc, 0)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if got, want := results[0].Date, "1 day ago"; got != want {
		t.Errorf("news_dt Date = %q, want %q", got, want)
	}
	if got, want := results[1].Date, "2026-08-20"; got != want {
		t.Errorf("<time datetime> Date = %q, want %q", got, want)
	}
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

	results, err := Search(context.Background(), newTestClient(t), "istanbul hava durumu", 0)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
}

// TestSearchDetectsBlockedPage exercises the anti-bot contract: a 403 from
// Bing must surface as ErrBlocked through Search, not as an empty result.
func TestSearchDetectsBlockedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	old := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = old }()

	_, err := Search(context.Background(), newTestClient(t), "facebook", 0)
	if !errors.Is(err, serrors.ErrBlocked) {
		t.Fatalf("Search on 403 = %v, want ErrBlocked", err)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><ol id="b_results"></ol></body></html>`))
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
