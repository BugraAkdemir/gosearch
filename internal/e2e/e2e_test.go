package e2e

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/BugraAkdemir/gosearch"
	"github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo"
	"github.com/BugraAkdemir/gosearch/internal/providers/google"
	"github.com/BugraAkdemir/gosearch/internal/providers/yandex"
)

// fixture reads a testdata file for the given engine.
func fixture(t *testing.T, engine, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", engine, name))
	if err != nil {
		t.Fatalf("read fixture %s/%s: %v", engine, name, err)
	}
	return b
}

// pointEndpoints redirects every provider's Endpoint at the given URL and
// restores the original when the test ends. The e2e package exists precisely
// because these vars are exported: only here can the full public-API →
// dispatch → real-provider chain run against local servers.
func pointEndpoints(t *testing.T, ddg, goog, yan string) {
	t.Helper()
	oldDDG, oldGoogle, oldYandex := duckduckgo.Endpoint, google.Endpoint, yandex.Endpoint
	duckduckgo.Endpoint, google.Endpoint, yandex.Endpoint = ddg, goog, yan
	t.Cleanup(func() {
		duckduckgo.Endpoint, google.Endpoint, yandex.Endpoint = oldDDG, oldGoogle, oldYandex
	})
}

// staticServer serves body with 200 on every request.
func staticServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestFallbackEndToEnd confirms the documented chain through the REAL
// providers: Google (primary) serves its captured challenge page → the chain
// advances to DuckDuckGo, which succeeds → Yandex is never consulted.
func TestFallbackEndToEnd(t *testing.T) {
	var yandexHits int
	ddgSrv := staticServer(t, fixture(t, "duckduckgo", "success.html"))
	googleSrv := staticServer(t, fixture(t, "google", "blocked.html")) // real capture: enablejs challenge
	yandexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		yandexHits++
		_, _ = w.Write([]byte("should not be reached"))
	}))
	t.Cleanup(yandexSrv.Close)

	pointEndpoints(t, ddgSrv.URL, googleSrv.URL, yandexSrv.URL)

	results, err := gosearch.Search(context.Background(), "facebook",
		gosearch.Google,
		gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
	)
	if err != nil {
		t.Fatalf("Search with working fallback: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("got 0 results from fallback engine, want >0")
	}
	if results[0].Title == "" || results[0].URL == "" {
		t.Errorf("results[0] incomplete: %+v", results[0])
	}
	if yandexHits != 0 {
		t.Errorf("yandex hit %d times after an earlier engine succeeded, want 0 (first success wins)", yandexHits)
	}
}

// TestAllBlockedJoinedError confirms that when every engine in the chain is
// blocked/challenged, Search returns the errors.Join of each engine's error —
// so both sentinel families remain discoverable via errors.Is.
func TestAllBlockedJoinedError(t *testing.T) {
	// Google serves HTTP 403 → ErrBlocked; DDG serves its real captcha page →
	// ErrChallenge; Yandex redirects to showcaptcha → ErrChallenge.
	srv403 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv403.Close)

	ddgSrv := staticServer(t, fixture(t, "duckduckgo", "blocked.html"))
	var yanHits int
	yandexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		yanHits++
		if r.URL.Path == "/showcaptcha" {
			_, _ = w.Write([]byte("captcha page"))
			return
		}
		http.Redirect(w, r, yandexRedirectTarget(r), http.StatusFound)
	}))
	t.Cleanup(yandexSrv.Close)

	pointEndpoints(t, ddgSrv.URL, srv403.URL, yandexSrv.URL)

	_, err := gosearch.Search(context.Background(), "facebook",
		gosearch.Google,
		gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
	)
	if err == nil {
		t.Fatal("Search with all engines blocked = nil error, want joined error")
	}
	if !errors.Is(err, gosearch.ErrBlocked) {
		t.Errorf("joined error lost ErrBlocked (google 403): %v", err)
	}
	if !errors.Is(err, gosearch.ErrChallenge) {
		t.Errorf("joined error lost ErrChallenge (ddg/yandex): %v", err)
	}
	if yanHits == 0 {
		t.Error("yandex never tried; every engine in the chain must be attempted")
	}
}

// yandexRedirectTarget sends the client to this server's own /showcaptcha so
// no live request leaves the test process.
func yandexRedirectTarget(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/showcaptcha?cc=1"
}
