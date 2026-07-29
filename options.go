package gosearch

import (
	"net/http"
	"time"
)

// Option configures a Search or Fetch call. Options are applied in order.
//
// Most options (timeout, proxy, headers, cookies, custom client) apply to both
// Search and Fetch. A few are meaningful only for Search — WithFallback and
// WithMaxResults — and are ignored by Fetch; each such option documents this.
type Option func(*config)

// config is the resolved, internal configuration for a single Search or Fetch
// call. It is populated from defaults plus the caller's Options, then handed to
// the internal HTTP client and provider.
type config struct {
	// timeout bounds the whole operation (per request; fallback engines each
	// get their own timeout).
	timeout time.Duration
	// userAgent overrides the default browser User-Agent when non-empty.
	userAgent string
	// proxyRawURL, when non-empty, routes requests through this proxy.
	proxyRawURL string
	// extraHeaders are added to every request (overriding defaults on conflict).
	extraHeaders http.Header
	// cookies are seeded into the client's cookie jar before the first request.
	cookies []*http.Cookie
	// httpClient, when non-nil, is used as-is instead of the library building
	// its own client. This is an advanced escape hatch (see WithHTTPClient).
	httpClient *http.Client
	// fallback is the ordered list of engines to try if the primary is blocked.
	// Search-only.
	fallback []Engine
	// maxResults caps the number of results returned; 0 means no cap.
	// Search-only.
	maxResults int
}

// defaultConfig returns the baseline configuration used before any Option is
// applied.
func defaultConfig() *config {
	return &config{
		timeout:      15 * time.Second,
		extraHeaders: http.Header{},
	}
}

// apply returns a config built from the defaults with all opts applied in order.
func apply(opts []Option) *config {
	c := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// WithTimeout sets the timeout for the operation. A non-positive duration
// resets to the default. The default is 15 seconds.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		if d <= 0 {
			c.timeout = 15 * time.Second
			return
		}
		c.timeout = d
	}
}

// WithUserAgent overrides the default browser User-Agent header. Supplying a
// realistic browser User-Agent is part of how this library avoids looking like
// a bot; override only if you have a specific reason.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// WithProxy routes all requests through the given proxy URL (for example
// "http://user:pass@host:port" or "socks5://host:port"). This is a legitimate
// way to use your own network egress; it is not used to rotate identities to
// defeat anti-bot systems.
func WithProxy(rawURL string) Option {
	return func(c *config) { c.proxyRawURL = rawURL }
}

// WithHeader adds or overrides a single request header. Call it multiple times
// to set multiple headers.
func WithHeader(key, value string) Option {
	return func(c *config) {
		if c.extraHeaders == nil {
			c.extraHeaders = http.Header{}
		}
		c.extraHeaders.Set(key, value)
	}
}

// WithCookies seeds cookies into the client's cookie jar before the first
// request. This lets you reuse a session (for example cookies exported from
// your own logged-in browser) so requests look like a returning visitor.
func WithCookies(cookies ...*http.Cookie) Option {
	return func(c *config) { c.cookies = append(c.cookies, cookies...) }
}

// WithHTTPClient uses the supplied *http.Client as-is instead of the client the
// library would otherwise build. This is an advanced escape hatch: it bypasses
// the library's default headers, cookie jar, and rate limiting, so you take
// responsibility for configuring those yourself. Timeout, proxy, and header
// options are not applied on top of a client supplied this way.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.httpClient = client }
}

// WithFallback sets the ordered list of engines to try if the primary engine
// returns ErrBlocked or ErrChallenge. Engines are tried in exactly the order
// given; the first that succeeds wins. Fallback is not triggered by
// ErrNoResults (an empty result set is a valid answer). This option is
// Search-only and is ignored by Fetch.
func WithFallback(engines ...Engine) Option {
	return func(c *config) { c.fallback = append(c.fallback, engines...) }
}

// WithMaxResults caps the number of results Search returns. Zero (the default)
// means no cap. This option is Search-only and is ignored by Fetch.
func WithMaxResults(n int) Option {
	return func(c *config) {
		if n < 0 {
			n = 0
		}
		c.maxResults = n
	}
}
