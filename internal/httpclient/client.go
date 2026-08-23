// Package httpclient provides the shared HTTP client every gosearch provider
// uses. It centralizes the "look like an ordinary browser, not a bot" behavior
// — a realistic header set, a persistent cookie jar, and self-imposed per-host
// rate limiting — plus a single block-detection helper (see detect.go) so
// anti-bot handling is not reimplemented in each provider.
//
// It never attempts to defeat a security control: no CAPTCHA solving, no
// JavaScript-challenge execution, no fingerprint spoofing. It only makes an
// automated request resemble a normal one and reports honestly when an engine
// blocks it.
package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// maxBodyBytes caps how much of a response body is read into memory, so a
// hostile or huge page cannot exhaust memory. Search result pages and typical
// articles are far smaller than this.
const maxBodyBytes = 8 << 20 // 8 MiB

// Config configures a Client. All fields are optional; the zero value yields a
// sensible default client.
type Config struct {
	// Timeout bounds each individual request. Zero means 15 seconds.
	Timeout time.Duration
	// UserAgent overrides the default browser User-Agent when non-empty.
	UserAgent string
	// ProxyRawURL, when non-empty, routes requests through this proxy.
	ProxyRawURL string
	// ExtraHeaders are applied to every request, overriding defaults on
	// conflict.
	ExtraHeaders http.Header
	// Cookies are seeded into the cookie jar (grouped by their Domain) before
	// the first request.
	Cookies []*http.Cookie
	// HTTPClient, when non-nil, is used as-is. The caller is then responsible
	// for its transport, jar, and timeout; Timeout/ProxyRawURL are ignored and
	// only ExtraHeaders/UserAgent are still applied per request. Rate limiting
	// still applies.
	HTTPClient *http.Client
	// MinInterval is the minimum spacing between requests to the same host,
	// enforced across all requests made through this Client. Zero means 1
	// second. Meaningful only when a Client is reused across multiple requests.
	MinInterval time.Duration
	// MaxRetries is how many times a request that fails TRANSIENTLY —
	// transport error or HTTP 408/500/502/503/504 — is retried before Get
	// gives up. Zero means the default of 2; a negative value disables
	// retries entirely. Blocks and challenges (403/429, captcha pages) are
	// deliberately never retried: they are deterministic for the caller's
	// IP reputation, and hammering an engine that already flagged you only
	// makes things worse — use gosearch.WithFallback for those.
	MaxRetries int
	// RetryBackoff is the delay before the first retry, doubling on each
	// further attempt (capped at 5 seconds). Zero means 500 milliseconds.
	RetryBackoff time.Duration
}

// Client is a reusable HTTP client with browser-like defaults, a cookie jar,
// and per-host rate limiting. It is safe for concurrent use.
type Client struct {
	http         *http.Client
	userAgent    string
	extra        http.Header
	minInterval  time.Duration
	maxRetries   int
	retryBackoff time.Duration

	mu       sync.Mutex
	lastHost map[string]time.Time
}

// defaultUserAgent is a current, realistic desktop Chrome User-Agent. Engines
// use the User-Agent (among many signals) to distinguish browsers from bots; a
// plausible one is part of behaving like a normal visitor.
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// New builds a Client from cfg. It returns an error only if the proxy URL or
// cookie jar cannot be constructed.
func New(cfg Config) (*Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	minInterval := cfg.MinInterval
	if minInterval <= 0 {
		minInterval = time.Second
	}
	maxRetries := cfg.MaxRetries
	retryBackoff := cfg.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = 500 * time.Millisecond
	}

	hc := cfg.HTTPClient
	if hc == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("httpclient: new cookie jar: %w", err)
		}
		transport := &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		}
		if cfg.ProxyRawURL != "" {
			pu, err := url.Parse(cfg.ProxyRawURL)
			if err != nil {
				return nil, fmt.Errorf("httpclient: parse proxy url: %w", err)
			}
			transport.Proxy = http.ProxyURL(pu)
		}
		hc = &http.Client{Timeout: timeout, Jar: jar, Transport: transport}
	}

	// Seed cookies into the jar, grouped by domain.
	if hc.Jar != nil && len(cfg.Cookies) > 0 {
		byDomain := map[string][]*http.Cookie{}
		for _, ck := range cfg.Cookies {
			host := strings.TrimPrefix(ck.Domain, ".")
			if host == "" {
				continue // cannot place a cookie without a domain
			}
			byDomain[host] = append(byDomain[host], ck)
		}
		for host, cks := range byDomain {
			if u, err := url.Parse("https://" + host + "/"); err == nil {
				hc.Jar.SetCookies(u, cks)
			}
		}
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}

	return &Client{
		http:         hc,
		userAgent:    ua,
		extra:        cfg.ExtraHeaders,
		minInterval:  minInterval,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		lastHost:     map[string]time.Time{},
	}, nil
}

// Response is a fetched HTTP response with its body already read (bounded by
// maxBodyBytes). FinalURL reflects the URL after any redirects.
type Response struct {
	StatusCode int
	FinalURL   string
	Header     http.Header
	Body       []byte
}

// Get fetches rawURL with the client's browser-like headers, honoring ctx and
// the per-host rate limit. It reads and returns the (bounded) body. It does not
// perform block detection — call Detect on the returned Response for that.
//
// Transient failures — transport errors (other than ctx cancellation) and
// HTTP 408/500/502/503/504 — are retried with exponential backoff up to the
// configured MaxRetries; every attempt still passes through rateLimit. A
// request that ends on a transient status returns an error rather than a
// Response, so a server-error page can never reach a parser. Blocks
// (403/429) and challenge pages are never retried.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	attempts := c.maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := range attempts {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoffFor(c.retryBackoff, attempt)); err != nil {
				return nil, lastErr // ctx done while backing off; keep original failure
			}
		}
		resp, err := c.getOnce(ctx, rawURL)
		switch {
		case err == nil:
			if !transientStatus(resp.StatusCode) {
				return resp, nil
			}
			lastErr = fmt.Errorf("httpclient: get %q: HTTP %d", rawURL, resp.StatusCode)
		case ctx.Err() != nil:
			// The caller gave up; retrying would only spin uselessly.
			return nil, fmt.Errorf("httpclient: get %q: %w", rawURL, err)
		default:
			lastErr = fmt.Errorf("httpclient: get %q: %w", rawURL, err)
		}
	}
	if lastErr == nil { // unreachable when attempts >= 1, but be explicit
		lastErr = fmt.Errorf("httpclient: get %q: all attempts failed", rawURL)
	}
	return nil, lastErr
}

// getOnce performs exactly one rate-limited request and reads its body.
func (c *Client) getOnce(ctx context.Context, rawURL string) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("httpclient: parse url %q: %w", rawURL, err)
	}
	if err := c.rateLimit(ctx, u.Host); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("httpclient: new request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpclient: get %q: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("httpclient: read body: %w", err)
	}

	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return &Response{
		StatusCode: resp.StatusCode,
		FinalURL:   finalURL,
		Header:     resp.Header,
		Body:       body,
	}, nil
}

// transientStatus reports whether an HTTP status code is worth retrying:
// request timeout (408) and the server-side gateway/upstream failures
// (500/502/503/504). Everything else is either an answer or a block.
func transientStatus(code int) bool {
	return code == 408 || code == 500 || code == 502 || code == 503 || code == 504
}

// backoffFor returns the wait before retry number n (1-based): the configured
// base doubled per attempt, capped at maxBackoff.
const maxBackoff = 5 * time.Second

func backoffFor(base time.Duration, n int) time.Duration {
	d := base
	for i := 1; i < n && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// sleepCtx waits for d or until ctx is done, whichever comes first. It
// returns ctx.Err() when the context ended first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// rateLimit blocks until at least minInterval has elapsed since the last
// request to host, or until ctx is done. It records the (post-wait) time as the
// new last-request time for host.
func (c *Client) rateLimit(ctx context.Context, host string) error {
	c.mu.Lock()
	last, ok := c.lastHost[host]
	now := time.Now()
	var wait time.Duration
	if ok {
		if elapsed := now.Sub(last); elapsed < c.minInterval {
			wait = c.minInterval - elapsed
		}
	}
	// Reserve the slot now (optimistically) so concurrent callers to the same
	// host serialize instead of all reading the same stale timestamp.
	c.lastHost[host] = now.Add(wait)
	c.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// setHeaders applies the default browser-like header set, then the caller's
// ExtraHeaders (which override defaults).
//
// Note: Accept-Encoding is deliberately NOT set. Go's HTTP transport
// transparently adds gzip and decodes it only when it, not the caller, set the
// header. Setting Accept-Encoding here (for example to include br) would make
// Go hand back a compressed body we would then fail to decode. Fingerprint
// realism is not worth returning undecodable bytes to every provider.
func (c *Client) setHeaders(req *http.Request) {
	h := req.Header
	h.Set("User-Agent", c.userAgent)
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,"+
		"image/avif,image/webp,image/apng,*/*;q=0.8")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Upgrade-Insecure-Requests", "1")
	h.Set("Sec-Fetch-Dest", "document")
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Site", "none")
	h.Set("Sec-Fetch-User", "?1")
	h.Set("sec-ch-ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua-platform", `"Windows"`)

	for key, vals := range c.extra {
		h.Del(key)
		for _, v := range vals {
			h.Add(key, v)
		}
	}
}
