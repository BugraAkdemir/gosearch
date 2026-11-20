package browser

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Option configures an Engine. Options are applied in order.
type Option func(*engineConfig)

// engineConfig is the resolved configuration for one Engine.
type engineConfig struct {
	// executable forces a specific chromium-family binary path, bypassing
	// discovery entirely.
	executable string
	// allowDownload permits downloading chrome-headless-shell from Google's
	// official chrome-for-testing CDN when no local browser exists.
	allowDownload bool
	// cacheDir overrides the default OS user-cache location used for the
	// downloaded/embedded engine and its extraction (locked-down containers,
	// read-only HOME, etc.).
	cacheDir string
	// userAgent overrides the User-Agent the browser declares. Empty means
	// the normalized default (a standard desktop Chrome UA — see
	// normalizedUserAgent).
	userAgent string
	// headless is the resolved headless mode. The zero value must not force
	// headed, so headlessSeen records whether the caller expressed a choice.
	headless     bool
	headlessSeen bool
	// profileDir persists cookies/history across Engine instances when set
	// (e.g. solve a CAPTCHA once in headed mode, keep the session). When
	// empty, each Engine gets a throwaway temp profile.
	profileDir string
	// keepProfile marks a user-supplied profileDir as persistent (never
	// deleted by Close).
	keepProfile bool
	// onProgress, set via WithProgress, is invoked during an actual network
	// download (AllowDownload path only) with bytes-so-far and total size.
	onProgress func(downloaded, total int64)
	// markdown makes Engine.Fetch return GitHub-flavored Markdown instead of
	// plain text, matching gosearch.WithMarkdown. Off by default.
	markdown bool
	// maxConcurrency bounds how many Search/Fetch calls may run simultaneously
	// on one Engine. Values <= 1 serialize on a single shared tab (default).
	maxConcurrency int
	// timeout overrides the default per-operation timeout (30s Search / 45s
	// Fetch) when positive.
	timeout time.Duration
	// proxyRawURL routes Chromium traffic through --proxy-server when set.
	proxyRawURL string
	// maxResults caps Search results; <= 0 uses the default (30).
	maxResults int
	// allowedDomains/blockedDomains mirror gosearch's post-search domain policy.
	allowedDomains []string
	blockedDomains []string
	// waitSelector, when non-empty, makes Fetch wait for this CSS selector to
	// appear in the DOM after body is ready (useful for async SPAs).
	waitSelector string
	// waitDelay, when positive, adds a ctx-aware settle delay after DOM/selector
	// readiness before capturing outerHTML.
	waitDelay time.Duration
	// fallbackMinContent sets the minimum extracted rune count below which
	// FetchWithFallback considers an HTTP response an unrendered JS shell and
	// retries in the browser. Zero (when explicitly set) falls back only on
	// completely empty content.
	fallbackMinContent     int
	fallbackMinContentSeen bool
}

// WithHeadless controls whether the browser runs headlessly (default true,
// i.e. headless). WithHeadless(false) opens a visible window — useful once,
// with WithProfileDir, to manually clear an interactive challenge so the
// saved session carries over to later headless runs.
func WithHeadless(v bool) Option {
	return func(c *engineConfig) { c.headless = v; c.headlessSeen = true }
}

// WithProfileDir uses a persistent browser profile at path: cookies survive
// Close() and across runs. The directory is created if missing and is NOT
// deleted by Close.
func WithProfileDir(path string) Option {
	return func(c *engineConfig) { c.profileDir = path; c.keepProfile = true }
}

// WithUserAgent sets the exact User-Agent string the browser declares.
// By default chrome-headless-shell advertises "HeadlessChrome", which search
// engines treat as an automation-only build and answer differently; the
// default here is a standard desktop Chrome UA instead — the same
// realistic-identity policy the core HTTP client already applies. Nothing
// else about the browser is altered: webdriver stays on, fingerprints stay
// stock, CAPTCHAs are never solved.
func WithUserAgent(ua string) Option {
	return func(c *engineConfig) { c.userAgent = ua }
}

// WithExecutable bypasses all discovery and uses the given chromium-family
// executable (Chrome, Chromium, Edge, or chrome-headless-shell) as-is.
func WithExecutable(path string) Option {
	return func(c *engineConfig) { c.executable = path }
}

// AllowDownload permits New to download chrome-headless-shell — a small,
// official, automation-only build of Chromium published on Google's
// chrome-for-testing CDN — into the OS user cache directory the first time
// it is needed. Nothing is ever downloaded without this explicit opt-in.
func AllowDownload(v bool) Option {
	return func(c *engineConfig) { c.allowDownload = v }
}

// WithProgress registers a callback invoked periodically during an actual
// network download (the AllowDownload path only — never for discover(),
// findCachedBinary, the embedded archive, or an explicit WithExecutable)
// with bytes downloaded so far and the total size. total is 0 when the
// server's response carried no Content-Length, in which case downloaded
// still increases so callers can show an indeterminate "N MB downloaded"
// state instead of a percentage. Called synchronously, once per read chunk,
// from the same goroutine that called Install/New — it must not block or
// panic; a caller that needs the value on another goroutine should hand it
// off (e.g. update a mutex-guarded struct) rather than do slow work inline.
func WithProgress(fn func(downloaded, total int64)) Option {
	return func(c *engineConfig) { c.onProgress = fn }
}

// WithMarkdown makes Engine.Fetch return extracted main content as
// GitHub-flavored Markdown in Page.Content (headings, lists, code fences,
// links, emphasis, tables) instead of plain text, mirroring
// gosearch.WithMarkdown. Off by default so existing callers see plain text.
// Ignored by Engine.Search.
func WithMarkdown() Option {
	return func(c *engineConfig) { c.markdown = true }
}

// WithMaxConcurrency sets the maximum number of Search/Fetch calls that may
// execute simultaneously on one Engine. Values <= 1 (the default) serialize
// calls on a single reused tab, keeping steady-state memory at roughly one
// page's worth. Values > 1 open an isolated tab per concurrent call inside the
// same browser process, sharing cookies and profile state while preventing
// tabs from clobbering each other.
func WithMaxConcurrency(n int) Option {
	return func(c *engineConfig) {
		if n < 1 {
			n = 1
		}
		c.maxConcurrency = n
	}
}

// WithTimeout overrides the per-call timeout for Engine.Search (default 30s)
// and Engine.Fetch (default 45s). A non-positive duration resets to the
// per-operation defaults.
func WithTimeout(d time.Duration) Option {
	return func(c *engineConfig) {
		if d <= 0 {
			c.timeout = 0
			return
		}
		c.timeout = d
	}
}

// WithProxy routes all browser traffic through the given proxy URL (for
// example "http://host:port", "https://host:port", or "socks5://host:port").
// Invalid proxy URLs cause New to return an error immediately. Like core
// gosearch.WithProxy, this is for legitimate network egress, not identity
// rotation.
func WithProxy(rawURL string) Option {
	return func(c *engineConfig) { c.proxyRawURL = strings.TrimSpace(rawURL) }
}

// WithMaxResults caps the number of results returned by Engine.Search. A
// non-positive value resets to the default cap (30). Ignored by Engine.Fetch.
func WithMaxResults(n int) Option {
	return func(c *engineConfig) {
		if n <= 0 {
			c.maxResults = 0
			return
		}
		c.maxResults = n
	}
}

// WithBlockedDomains drops Engine.Search results whose host matches one of the
// given domains or any subdomain of one, mirroring gosearch.WithBlockedDomains.
// Combined with WithAllowedDomains, deny is applied first. Ignored by Fetch.
func WithBlockedDomains(domains ...string) Option {
	return func(c *engineConfig) { c.blockedDomains = append(c.blockedDomains, domains...) }
}

// WithAllowedDomains keeps only Engine.Search results whose host matches one
// of the given domains or any subdomain of one, mirroring
// gosearch.WithAllowedDomains. An empty list means no allowlisting. Ignored by
// Fetch.
func WithAllowedDomains(domains ...string) Option {
	return func(c *engineConfig) { c.allowedDomains = append(c.allowedDomains, domains...) }
}

// WithWaitSelector instructs Engine.Fetch to wait for the given CSS selector
// to appear in the DOM (after body is ready) before capturing the rendered
// HTML — useful for client-rendered SPAs (React, Vue, Next.js) that mount
// article content asynchronously after initial load. Empty disables the extra
// wait (the default). Ignored by Engine.Search.
func WithWaitSelector(selector string) Option {
	return func(c *engineConfig) { c.waitSelector = strings.TrimSpace(selector) }
}

// WithWaitDelay instructs Engine.Fetch to wait d after DOM readiness (and any
// WithWaitSelector) before capturing HTML, letting late hydration or XHR calls
// settle. Context cancellation cuts the wait short immediately. Non-positive
// values disable the delay (the default). Ignored by Engine.Search.
func WithWaitDelay(d time.Duration) Option {
	return func(c *engineConfig) {
		if d <= 0 {
			c.waitDelay = 0
			return
		}
		c.waitDelay = d
	}
}

// WithFallbackMinContent configures the minimum rune count of trimmed text
// that FetchWithFallback accepts from a plain-HTTP gosearch.Fetch before
// treating the page as an unrendered JS shell and falling back to the browser.
// Default is 100 runes; pass 0 to fall back only when HTTP extraction yields
// completely empty content (or ErrBlocked/ErrChallenge).
func WithFallbackMinContent(chars int) Option {
	return func(c *engineConfig) {
		if chars < 0 {
			chars = 0
		}
		c.fallbackMinContent = chars
		c.fallbackMinContentSeen = true
	}
}

// validateProxyURL verifies that rawURL is a supported Chromium proxy URL when
// non-empty.
func validateProxyURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("browser: invalid proxy url %q: %w", rawURL, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks4", "socks5", "socks5h":
	default:
		return fmt.Errorf("browser: invalid proxy url %q: unsupported scheme %q", rawURL, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("browser: invalid proxy url %q: missing host", rawURL)
	}
	return nil
}

// resolveExecutable returns the path of the chromium-family executable to
// drive: an explicitly supplied path wins; then the embedded archive (when
// this binary was built with the gosearch_embed_engine build tag); then
// system discovery; then a previously-downloaded engine already sitting in
// the cache (no network needed — this is what makes IsInstalled-style calls,
// which never set AllowDownload, correctly report "yes" right after a
// successful Install instead of only ever seeing discover()'s system paths);
// then, if downloads are allowed, chrome-headless-shell.
func resolveExecutable(ctx context.Context, cfg *engineConfig) (string, error) {
	if cfg.executable != "" {
		return cfg.executable, nil
	}
	if len(embeddedEngineZip) > 0 {
		p, err := ensureEmbedded(ctx, cfg.cacheDir)
		if err != nil {
			return "", fmt.Errorf("browser: embedded engine: %w", err)
		}
		return p, nil
	}
	if p := discover(); p != "" {
		return p, nil
	}
	if p := findCachedBinary(cfg.cacheDir); p != "" {
		return p, nil
	}
	if cfg.allowDownload {
		p, err := downloadEngine(ctx, cfg.cacheDir, cfg.onProgress)
		if err != nil {
			return "", fmt.Errorf("browser: download engine: %w", err)
		}
		return p, nil
	}
	return "", fmt.Errorf("%w: tried WithExecutable?=%v, embedded archive?=%v, system paths (%s)",
		ErrNoBrowserFound, cfg.executable != "", len(embeddedEngineZip) > 0,
		strings.Join(discoverCandidates(), ", "))
}

// findCachedBinary reports whether an engine from an earlier download
// (chrome-for-testing OR the linux/arm64 Playwright fallback — both land
// under engineCacheRoot and both end up named engineBinaryName after
// downloadEngine's rename step) already sits in the cache, without making
// any network call or creating directories on disk.
func findCachedBinary(cacheDirOverride string) string {
	root, err := resolveCacheRootPath(cacheDirOverride)
	if err != nil {
		return ""
	}
	return findEngineBinary(root)
}

// normalizedUserAgent returns the User-Agent the browser declares. chrome-
// headless-shell advertises "HeadlessChrome" by default, which engines treat
// as an automation-only build and answer differently; the default here is a
// standard desktop Chrome UA — the same realistic-identity policy the core
// HTTP client applies. An explicit non-empty ua wins verbatim. Nothing else
// about the browser is altered (webdriver stays on, fingerprints stay stock).
func normalizedUserAgent(ua string) string {
	if ua != "" {
		return ua
	}
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"
}

// allocatorFlags keeps per-instance consumption low and behavior stable:
// no GPU process, no images (search pages are text), a small window, a
// private profile directory, and a normal browser identity string. The
// resolved executable is pinned explicitly so chromedp launches OUR binary
// instead of probing PATH itself. The browser itself is NEVER patched or
// disguised beyond the declared UA — see the package comment for the project
// line on anti-bot behavior.
func allocatorFlags(profileDir, executable, userAgent, proxyRawURL string, headless bool) []chromedp.ExecAllocatorOption {
	base := chromedp.DefaultExecAllocatorOptions[:]
	extra := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(executable),
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("user-agent", normalizedUserAgent(userAgent)),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.WindowSize(1280, 800),
		chromedp.UserDataDir(profileDir),
	}
	if proxyRawURL != "" {
		extra = append(extra, chromedp.ProxyServer(proxyRawURL))
	}
	return append(base, extra...)
}

// platformSlug maps runtime.GOOS/GOARCH onto chrome-for-testing download
// platform names. Unsupported combinations return "" and the caller reports
// a clear error instead of guessing a URL.
func platformSlug() string {
	switch {
	case runtimeGOOS() == "linux" && runtimeGOARCH() == "amd64":
		return "linux64"
	case runtimeGOOS() == "darwin" && runtimeGOARCH() == "arm64":
		return "mac-arm64"
	case runtimeGOOS() == "darwin" && runtimeGOARCH() == "amd64":
		return "mac-x64"
	case runtimeGOOS() == "windows" && runtimeGOARCH() == "amd64":
		return "win64"
	default:
		return ""
	}
}

// WithCacheDir overrides where the downloaded or embedded engine is stored
// and extracted (default: the OS user cache directory, e.g.
// ~/.cache/gosearch/browser on Linux). Useful when the default location is
// read-only, as in some locked-down containers.
func WithCacheDir(path string) Option {
	return func(c *engineConfig) { c.cacheDir = path }
}

// Install performs the same discovery-or-download resolution as New without
// constructing an Engine: use it during deployment or image building
// (Dockerfile RUN step, setup script) so the one-time engine download is not
// paid by a live user request later. It is safe to call repeatedly — an
// already-present executable or cached engine short-circuits to nil.
func Install(ctx context.Context, opts ...Option) error {
	cfg := &engineConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	if cfg.executable != "" && isExecutableFile(cfg.executable) {
		return nil
	}
	_, err := resolveExecutable(ctx, cfg)
	return err
}
