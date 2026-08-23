package httpclient

import (
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetSetsBrowserHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "ok" {
		t.Fatalf("unexpected response: %d %q", resp.StatusCode, resp.Body)
	}
	if ua := got.Get("User-Agent"); !strings.Contains(ua, "Chrome/") {
		t.Errorf("User-Agent = %q, want a Chrome UA", ua)
	}
	for _, h := range []string{"Accept", "Accept-Language", "Sec-Fetch-Mode", "sec-ch-ua"} {
		if got.Get(h) == "" {
			t.Errorf("missing default header %q", h)
		}
	}
	// We must not set Accept-Encoding ourselves. Go's transport transparently
	// adds "gzip" and decodes it; if we'd set it (e.g. to include br) Go would
	// hand back an undecodable body. So the server may see "gzip" (from Go) but
	// never a br/deflate combo we'd have injected.
	if ae := got.Get("Accept-Encoding"); ae != "" && ae != "gzip" {
		t.Errorf("Accept-Encoding = %q, want unset or Go's transparent gzip", ae)
	}
}

func TestGetTransparentlyDecodesGzip(t *testing.T) {
	const plain = "<html><body>merhaba dünya</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only gzip if the client advertised it (Go's transport does).
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = w.Write([]byte(plain))
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(plain))
		_ = gz.Close()
	}))
	defer srv.Close()

	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != plain {
		t.Errorf("body = %q, want transparently-decoded %q", resp.Body, plain)
	}
}

func TestGetUserAgentAndExtraHeaderOverride(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	c, err := New(Config{
		UserAgent:    "custom-ua",
		ExtraHeaders: http.Header{"X-Custom": {"yes"}, "Accept-Language": {"tr-TR"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if got.Get("User-Agent") != "custom-ua" {
		t.Errorf("User-Agent = %q, want custom-ua", got.Get("User-Agent"))
	}
	if got.Get("X-Custom") != "yes" {
		t.Errorf("X-Custom = %q, want yes", got.Get("X-Custom"))
	}
	if got.Get("Accept-Language") != "tr-TR" {
		t.Errorf("Accept-Language = %q, want extra header to override default", got.Get("Accept-Language"))
	}
}

func TestRateLimitSpacesRequestsToSameHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	c, err := New(Config{MinInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.Get(ctx, srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	// 3 requests, 2 gaps of >=100ms => at least ~200ms total.
	if elapsed := time.Since(start); elapsed < 180*time.Millisecond {
		t.Errorf("3 requests took %v, want >= ~200ms from rate limiting", elapsed)
	}
}

func TestRateLimitRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	c, err := New(Config{MinInterval: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// First request primes the last-request time for the host.
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	// Second request would wait ~10s; a short-deadline ctx must cut it off fast.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.Get(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if time.Since(start) > time.Second {
		t.Errorf("Get did not honor ctx cancellation promptly (took %v)", time.Since(start))
	}
}

func TestBodyIsCapped(t *testing.T) {
	// Serve more than maxBodyBytes; Get must return at most maxBodyBytes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := strings.Repeat("a", 1<<20) // 1 MiB
		for i := 0; i < 10; i++ {           // 10 MiB total
			_, _ = w.Write([]byte(chunk))
		}
	}))
	defer srv.Close()

	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) > maxBodyBytes {
		t.Errorf("body len = %d, want <= maxBodyBytes (%d)", len(resp.Body), maxBodyBytes)
	}
}
