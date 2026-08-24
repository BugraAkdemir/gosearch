package gosearch

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestEngineString(t *testing.T) {
	cases := map[Engine]string{
		DuckDuckGo: "duckduckgo",
		Google:     "google",
		Yandex:     "yandex",
		Bing:       "bing",
		Engine(99): "unknown",
	}
	for e, want := range cases {
		if got := e.String(); got != want {
			t.Errorf("Engine(%d).String() = %q, want %q", int(e), got, want)
		}
	}
}

func TestEngineValid(t *testing.T) {
	for _, e := range []Engine{DuckDuckGo, Google, Yandex, Bing} {
		if !e.valid() {
			t.Errorf("%v.valid() = false, want true", e)
		}
	}
	if Engine(99).valid() {
		t.Error("Engine(99).valid() = true, want false")
	}
}

func TestDefaultConfig(t *testing.T) {
	c := apply(nil)
	if c.timeout != 15*time.Second {
		t.Errorf("default timeout = %v, want 15s", c.timeout)
	}
	if c.maxResults != 0 {
		t.Errorf("default maxResults = %d, want 0", c.maxResults)
	}
	if c.retries != 2 {
		t.Errorf("default retries = %d, want 2", c.retries)
	}
}

func TestWithRetriesOption(t *testing.T) {
	if got := apply([]Option{WithRetries(5)}).retries; got != 5 {
		t.Errorf("WithRetries(5) = %d, want 5", got)
	}
	// Zero/negative disables retrying entirely (sentinel -1).
	for _, n := range []int{0, -3} {
		if got := apply([]Option{WithRetries(n)}).retries; got != -1 {
			t.Errorf("WithRetries(%d) = %d, want -1 (disabled)", n, got)
		}
	}
}

func TestOptionsApply(t *testing.T) {
	client := &http.Client{}
	c := apply([]Option{
		nil, // must be tolerated
		WithTimeout(5 * time.Second),
		WithUserAgent("test-agent"),
		WithProxy("http://proxy:8080"),
		WithHeader("X-Test", "1"),
		WithHeader("X-Test", "2"), // Set overrides
		WithCookies(&http.Cookie{Name: "a", Value: "b"}),
		WithFallback(Google, Yandex),
		WithMaxResults(10),
		WithHTTPClient(client),
	})

	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.timeout)
	}
	if c.userAgent != "test-agent" {
		t.Errorf("userAgent = %q, want test-agent", c.userAgent)
	}
	if c.proxyRawURL != "http://proxy:8080" {
		t.Errorf("proxyRawURL = %q", c.proxyRawURL)
	}
	if got := c.extraHeaders.Get("X-Test"); got != "2" {
		t.Errorf("X-Test header = %q, want 2 (later Set wins)", got)
	}
	if len(c.cookies) != 1 || c.cookies[0].Name != "a" {
		t.Errorf("cookies = %+v", c.cookies)
	}
	if len(c.fallback) != 2 || c.fallback[0] != Google || c.fallback[1] != Yandex {
		t.Errorf("fallback = %v", c.fallback)
	}
	if c.maxResults != 10 {
		t.Errorf("maxResults = %d, want 10", c.maxResults)
	}
	if c.httpClient != client {
		t.Error("httpClient not set to supplied client")
	}
}

func TestWithTimeoutNonPositiveResets(t *testing.T) {
	c := apply([]Option{WithTimeout(-1)})
	if c.timeout != 15*time.Second {
		t.Errorf("timeout after WithTimeout(-1) = %v, want default 15s", c.timeout)
	}
}

func TestWithMaxResultsNegativeClampedToZero(t *testing.T) {
	c := apply([]Option{WithMaxResults(-5)})
	if c.maxResults != 0 {
		t.Errorf("maxResults after WithMaxResults(-5) = %d, want 0", c.maxResults)
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	errs := []error{ErrBlocked, ErrChallenge, ErrNoResults, ErrUnsupportedEngine}
	for i := range errs {
		for j := range errs {
			if i != j && errors.Is(errs[i], errs[j]) {
				t.Errorf("errors.Is(%v, %v) = true, want distinct sentinels", errs[i], errs[j])
			}
		}
	}
	// Wrapping must preserve errors.Is through %w.
	wrapped := fmt.Errorf("%w: extra context", ErrBlocked)
	if !errors.Is(wrapped, ErrBlocked) {
		t.Error("errors.Is(wrapped, ErrBlocked) = false, want true through %w")
	}
	if errors.Is(wrapped, ErrChallenge) {
		t.Error("wrapped ErrBlocked matched ErrChallenge")
	}
}
