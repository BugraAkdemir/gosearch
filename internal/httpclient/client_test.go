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

// fastClient builds a client with retry backoff and rate-limit spacing small
// enough that retry tests do not slow the suite.
func fastClient(t *testing.T, maxRetries int) *Client {
	t.Helper()
	c, err := New(Config{
		MaxRetries:   maxRetries,
		RetryBackoff: time.Millisecond,
		MinInterval:  time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGetRetriesTransientStatusThenSucceeds(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		if count <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resp, err := fastClient(t, 2).Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get after transient 503s: %v", err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "ok" {
		t.Fatalf("unexpected response: %d %q", resp.StatusCode, resp.Body)
	}
	if count != 3 {
		t.Errorf("server hit %d times, want 3 (initial + 2 retries)", count)
	}
}

func TestGetGivesUpAfterMaxRetries(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := fastClient(t, 2).Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Get on persistent 502 = nil error, want error")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error = %v, want it to mention HTTP 502", err)
	}
	if count != 3 {
		t.Errorf("server hit %d times, want exactly 3", count)
	}
}

// TestGetDoesNotRetryBlocks pins the policy that anti-bot rejections are
// never retried: a block is deterministic for the caller's IP reputation, so
// the first answer must be final (fallback engines exist for this case).
func TestGetDoesNotRetryBlocks(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	resp, err := fastClient(t, 2).Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get on 403: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 passed through untouched", resp.StatusCode)
	}
	if count != 1 {
		t.Errorf("server hit %d times, want exactly 1 (no retries on blocks)", count)
	}
}

func TestGetNegativeMaxRetriesDisablesRetry(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := fastClient(t, -1).Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Get with retries disabled = nil error, want error")
	}
	if count != 1 {
		t.Errorf("server hit %d times, want exactly 1", count)
	}
}

func TestBackoffForDoublesAndCaps(t *testing.T) {
	base := 100 * time.Millisecond
	cases := []struct {
		n    int
		want time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{10, maxBackoff}, // capped
	}
	for _, tc := range cases {
		if got := backoffFor(base, tc.n); got != tc.want {
			t.Errorf("backoffFor(base, %d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}
