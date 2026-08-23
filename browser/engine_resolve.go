package browser

import (
	"context"
	"fmt"
	"strings"

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

// resolveExecutable returns the path of the chromium-family executable to
// drive: an explicitly supplied path wins; then the embedded archive (when
// this binary was built with the gosearch_embed_engine build tag); then
// system discovery; then, if downloads are allowed, chrome-headless-shell.
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
	if cfg.allowDownload {
		p, err := downloadEngine(ctx, cfg.cacheDir)
		if err != nil {
			return "", fmt.Errorf("browser: download engine: %w", err)
		}
		return p, nil
	}
	return "", fmt.Errorf("%w: tried WithExecutable?=%v, embedded archive?=%v, system paths (%s)",
		ErrNoBrowserFound, cfg.executable != "", len(embeddedEngineZip) > 0,
		strings.Join(discoverCandidates(), ", "))
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
func allocatorFlags(profileDir, executable, userAgent string, headless bool) []chromedp.ExecAllocatorOption {
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
