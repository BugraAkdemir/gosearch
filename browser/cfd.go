package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cfdManifestURL is Google's official chrome-for-testing "last known good"
// endpoint. It lists stable builds of chrome-headless-shell — an automation-
// oriented Chromium without the browser UI — per platform.
const cfdManifestURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

// engineBinaryName is the executable produced by every chrome-headless-shell
// archive (with .exe on Windows appended by findEngineBinary).
const engineBinaryName = "chrome-headless-shell"

// playwrightHeadlessShellRevision pins the Microsoft Playwright build used
// as the linux/arm64 download source (see downloadPlaywrightHeadlessShellLinuxARM64).
// Google's chrome-for-testing service — the primary source for every other
// platform below — has never published an official linux/arm64 build
// (Raspberry Pi, ARM servers/VPS); Playwright separately builds and hosts
// one. There is no runtime-queryable "latest stable" manifest for it the way
// chrome-for-testing has (parseManifest below), so this is pinned by hand.
// To bump: check the "chromium-headless-shell" entry's "revision" field in
// https://github.com/microsoft/playwright/blob/main/packages/playwright-core/browsers.json
const playwrightHeadlessShellRevision = "1241" // Chrome for Testing 152.0.7977.54, as of 2026-08-26

// playwrightCDNBase is a var (not const) so tests can point it at a fake
// server; chrome-for-testing's cfdManifestURL has no equivalent test seam
// today because nothing exercises its download path outside integration
// tests — this one does (see playwright_download_test.go).
var playwrightCDNBase = "https://cdn.playwright.dev"

// playwrightHeadlessShellBinaryName is the executable name inside a
// Playwright chromium-headless-shell archive — "headless_shell", not
// "chrome-headless-shell" as chrome-for-testing names it.
const playwrightHeadlessShellBinaryName = "headless_shell"

// cfdManifest models only the fields this package consumes.
type cfdManifest struct {
	Channels struct {
		Stable struct {
			Version   string `json:"version"`
			Downloads struct {
				HeadlessShell []struct {
					Platform string `json:"platform"`
					URL      string `json:"url"`
				} `json:"chrome-headless-shell"`
			} `json:"downloads"`
		} `json:"Stable"`
	} `json:"channels"`
}

// CurrentPlatform names the chrome-for-testing platform slug for this host
// (linux64, mac-arm64, mac-x64, win64), or "" when unsupported.
func CurrentPlatform() string { return platformSlug() }

// StableEngineAsset returns the current stable headless-shell version string
// and its download URL for this host platform.
//
// For linux/amd64, darwin/arm64, darwin/amd64, and windows/amd64 it queries
// Google's chrome-for-testing manifest. For linux/arm64 (where chrome-for-testing
// publishes no build) it returns the pinned Microsoft Playwright CDN asset.
// Truly unsupported platforms return an error wrapping ErrUnsupportedPlatform.
func StableEngineAsset(ctx context.Context) (version, url string, err error) {
	if runtimeGOOS() == "linux" && runtimeGOARCH() == "arm64" {
		return playwrightHeadlessShellVersion(), playwrightHeadlessShellURL(), nil
	}
	if platformSlug() == "" {
		return "", "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, runtimeGOOS(), runtimeGOARCH())
	}
	data, err := fetchManifest(ctx)
	if err != nil {
		return "", "", err
	}
	return parseManifest(data)
}

func playwrightHeadlessShellVersion() string {
	return "playwright-" + playwrightHeadlessShellRevision
}

func playwrightHeadlessShellURL() string {
	return fmt.Sprintf("%s/builds/chromium/%s/chromium-headless-shell-linux-arm64.zip",
		playwrightCDNBase, playwrightHeadlessShellRevision)
}

// CleanCache removes all browser engines previously downloaded or extracted
// into the gosearch browser cache directory (honoring WithCacheDir if passed).
// It never touches system-installed Chrome/Chromium/Edge binaries.
//
// Calling CleanCache when the cache directory does not exist is a safe no-op
// (returning nil without creating the directory). Dangerous paths (filesystem
// roots, HOME, the parent cache root, or TempDir) are rejected.
func CleanCache(opts ...Option) error {
	cfg := &engineConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	root, err := resolveCacheRootPath(cfg.cacheDir)
	if err != nil {
		return fmt.Errorf("browser: clean cache: %w", err)
	}
	if !isSafeCleanTarget(root) {
		return fmt.Errorf("browser: clean cache: refusing to clean unsafe path %q", root)
	}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("browser: clean cache: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("browser: clean cache: %q is not a directory", root)
	}

	// Default cache directory (<UserCacheDir>/gosearch/browser) is owned
	// exclusively by gosearch and can be removed wholesale. When the caller
	// supplied a custom WithCacheDir (e.g. a mounted container volume), clear
	// its entries while preserving the directory itself.
	if cfg.cacheDir == "" {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("browser: clean cache: %w", err)
		}
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("browser: clean cache: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return fmt.Errorf("browser: clean cache: %w", err)
		}
	}
	return nil
}

// Uninstall removes any browser engine previously installed by Install or
// AllowDownload from the local gosearch cache (equivalent to CleanCache). It
// never modifies or removes system-installed browsers.
func Uninstall(opts ...Option) error {
	return CleanCache(opts...)
}

// isSafeCleanTarget guards CleanCache against deleting broad system or user
// directories if a caller misconfigures WithCacheDir.
func isSafeCleanTarget(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	clean := filepath.Clean(dir)
	if clean == "." || clean == ".." || clean == string(filepath.Separator) {
		return false
	}
	// Reject filesystem roots on all platforms (e.g. "/" or "C:\").
	if filepath.Dir(clean) == clean {
		return false
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return false
	}
	forbidden := []string{filepath.Clean(os.TempDir())}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		forbidden = append(forbidden, filepath.Clean(home))
	}
	if cache, err := os.UserCacheDir(); err == nil && cache != "" {
		forbidden = append(forbidden, filepath.Clean(cache))
	}
	for _, f := range forbidden {
		if fAbs, err := filepath.Abs(f); err == nil && abs == fAbs {
			return false
		}
	}
	return true
}

// DownloadFile streams url into dstFile atomically (.part then rename).
func DownloadFile(ctx context.Context, url, dstFile string) error {
	return httpDownload(ctx, url, dstFile)
}

// fetchManifest performs the manifest GET shared by downloadEngine and the
// exported StableEngineAsset helper.
func fetchManifest(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfdManifestURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest: HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

// parseManifest extracts the stable chrome-headless-shell download URL for
// the current platform plus the version string it belongs to.
func parseManifest(data []byte) (version, url string, err error) {
	var m cfdManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", "", fmt.Errorf("parse manifest: %w", err)
	}
	version = m.Channels.Stable.Version
	slug := platformSlug()
	if slug == "" {
		return "", "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, runtimeGOOS(), runtimeGOARCH())
	}
	if version == "" {
		return "", "", fmt.Errorf("manifest missing stable version")
	}
	for _, d := range m.Channels.Stable.Downloads.HeadlessShell {
		if d.Platform == slug {
			return version, d.URL, nil
		}
	}
	return "", "", fmt.Errorf("no chrome-headless-shell asset for platform %q", slug)
}

// downloadEngine downloads (or reuses) chrome-headless-shell in the OS user
// cache directory and returns the path of its executable. onProgress, if
// non-nil, is invoked after every chunk written during an actual network
// download (never on a cache hit) with bytes-so-far and the total size (0 if
// the server omitted Content-Length).
func downloadEngine(ctx context.Context, cacheDirOverride string, onProgress func(downloaded, total int64)) (string, error) {
	if runtimeGOOS() == "linux" && runtimeGOARCH() == "arm64" {
		return downloadPlaywrightHeadlessShellLinuxARM64(ctx, cacheDirOverride, onProgress)
	}
	slug := platformSlug()
	if slug == "" {
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, runtimeGOOS(), runtimeGOARCH())
	}
	root, err := engineCacheRoot(cacheDirOverride)
	if err != nil {
		return "", err
	}
	if p := findEngineBinary(root); p != "" {
		return p, nil
	}

	data, err := fetchManifest(ctx)
	if err != nil {
		return "", err
	}
	version, zipURL, err := parseManifest(data)
	if err != nil {
		return "", err
	}

	dst := filepath.Join(root, version)
	if p := findEngineBinary(dst); p != "" {
		return p, nil // another process/call already materialized this version
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}

	zipPath, cleanupZip, err := tempDownloadPath(root, "gosearch-cfd-*.zip")
	if err != nil {
		return "", err
	}
	defer cleanupZip()
	if err := httpDownloadProgress(ctx, zipURL, zipPath, onProgress); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dst); err != nil {
		return "", err
	}

	p := findEngineBinary(dst)
	if p == "" {
		return "", fmt.Errorf("archive extracted but no %s binary found", engineBinaryName)
	}
	if err := chmodExec(p); err != nil {
		return "", err
	}
	return p, nil
}

// downloadPlaywrightHeadlessShellLinuxARM64 downloads chromium-headless-shell
// from Microsoft's Playwright CDN — the linux/arm64 substitute for
// chrome-for-testing, which publishes no build for this platform at all (see
// playwrightHeadlessShellRevision). The extracted binary is renamed from
// Playwright's "headless_shell" to the package-wide canonical
// engineBinaryName so findEngineBinary, and everything built on top of it,
// stays a single-source-of-truth lookup with no awareness of a second
// download provider.
func downloadPlaywrightHeadlessShellLinuxARM64(ctx context.Context, cacheDirOverride string, onProgress func(downloaded, total int64)) (string, error) {
	root, err := engineCacheRoot(cacheDirOverride)
	if err != nil {
		return "", err
	}
	// Prefixed distinctly from chrome-for-testing's version-string
	// subdirectories (e.g. "152.0.7977.54") so the two sources can never
	// collide in the same cache root.
	dst := filepath.Join(root, playwrightHeadlessShellVersion())
	if p := findEngineBinary(dst); p != "" {
		return p, nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}

	url := playwrightHeadlessShellURL()
	zipPath, cleanupZip, err := tempDownloadPath(root, "gosearch-playwright-*.zip")
	if err != nil {
		return "", err
	}
	defer cleanupZip()
	if err := httpDownloadProgress(ctx, url, zipPath, onProgress); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dst); err != nil {
		return "", err
	}

	p := findPlaywrightHeadlessShellBinary(dst)
	if p == "" {
		return "", fmt.Errorf("archive extracted but no %s binary found", playwrightHeadlessShellBinaryName)
	}
	canonical := filepath.Join(filepath.Dir(p), engineBinaryName+exeSuffix())
	if err := os.Rename(p, canonical); err != nil {
		return "", fmt.Errorf("normalize playwright binary name: %w", err)
	}
	if err := chmodExec(canonical); err != nil {
		return "", err
	}
	return canonical, nil
}

// findPlaywrightHeadlessShellBinary walks root for Playwright's
// "headless_shell" executable name — the pre-rename counterpart of
// findEngineBinary, used only during extraction of the arm64 archive.
func findPlaywrightHeadlessShellBinary(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipAll //nolint:nilerr // best-effort scan
		}
		if !d.IsDir() && d.Name() == playwrightHeadlessShellBinaryName {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ensureEmbedded materializes the archive compiled into the binary (build
// tag gosearch_embed_engine) into the cache directory exactly once, then
// returns its executable path.
func ensureEmbedded(ctx context.Context, cacheDirOverride string) (string, error) {
	_ = ctx
	root, err := engineCacheRoot(cacheDirOverride)
	if err != nil {
		return "", err
	}
	if p := findEngineBinary(filepath.Join(root, "embedded")); p != "" {
		return p, nil
	}
	dst := filepath.Join(root, "embedded")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	zipPath, cleanupZip, err := tempDownloadPath(dst, "embedded-*.zip")
	if err != nil {
		return "", err
	}
	defer cleanupZip()
	if err := os.WriteFile(zipPath, embeddedEngineZip, 0o600); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dst); err != nil {
		return "", err
	}
	p := findEngineBinary(dst)
	if p == "" {
		// Support embedded Playwright linux/arm64 archives whose binary is
		// named "headless_shell" before canonical rename.
		if pw := findPlaywrightHeadlessShellBinary(dst); pw != "" {
			canonical := filepath.Join(filepath.Dir(pw), engineBinaryName+exeSuffix())
			if err := os.Rename(pw, canonical); err != nil {
				return "", fmt.Errorf("normalize embedded playwright binary name: %w", err)
			}
			p = canonical
		}
	}
	if p == "" {
		return "", fmt.Errorf("embedded archive contains no %s binary", engineBinaryName)
	}
	if err := chmodExec(p); err != nil {
		return "", err
	}
	return p, nil
}

// tempDownloadPath allocates a unique temporary file path inside dir (never in
// shared /tmp with predictable names) and returns a cleanup function that
// removes both the archive and any leftover .part file.
func tempDownloadPath(dir, pattern string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, err
	}
	path = f.Name()
	_ = f.Close()
	cleanup = func() {
		_ = os.Remove(path)
		_ = os.Remove(path + ".part")
	}
	return path, cleanup, nil
}

// resolveCacheRootPath computes the browser cache directory path without
// creating it on disk.
func resolveCacheRootPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(base, "gosearch", "browser"), nil
}

// engineCacheRoot returns the engine storage directory — an explicit
// override when set (WithCacheDir), else <user-cache>/gosearch/browser —
// creating it if needed.
func engineCacheRoot(override string) (string, error) {
	dir, err := resolveCacheRootPath(override)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// findEngineBinary walks root depth-first for the chrome-headless-shell
// executable and returns its path, or "" when absent. Archives nest one or
// two directories deep depending on platform.
func findEngineBinary(root string) string {
	name := engineBinaryName + exeSuffix()
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipAll //nolint:nilerr // best-effort scan
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// httpDownload streams url into dstFile with a bounded read window.
func httpDownload(ctx context.Context, url, dstFile string) error {
	return httpDownloadProgress(ctx, url, dstFile, nil)
}

// downloadChunkSize is the read-buffer size for httpDownloadProgress's
// manual copy loop — needed (instead of io.Copy) so onProgress can fire
// after each chunk. 256KB matches the chunk size an already-proven
// progress-reporting download loop elsewhere in this codebase's sibling
// project (memo's internal/modelstore.Store.doDownload) uses.
const downloadChunkSize = 256 * 1024

// httpDownloadProgress is httpDownload with an optional progress callback,
// invoked after every chunk written with (downloaded, total) — total is 0
// when the response carries no Content-Length. onProgress may be nil, in
// which case this is byte-for-byte equivalent to the old io.Copy-based
// httpDownload (same atomic .part-then-rename contract).
func httpDownloadProgress(ctx context.Context, url, dstFile string, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	var total int64
	if res.ContentLength > 0 {
		total = res.ContentLength
	}

	tmp := dstFile + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var downloaded int64
	buf := make([]byte, downloadChunkSize)
	for {
		select {
		case <-ctx.Done():
			_ = os.Remove(tmp)
			return ctx.Err()
		default:
		}
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				_ = os.Remove(tmp)
				return writeErr
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = os.Remove(tmp)
			return readErr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dstFile)
}

// exeSuffix returns ".exe" on Windows, "" elsewhere.
func exeSuffix() string {
	if runtimeWindows() {
		return ".exe"
	}
	return ""
}

// safeJoin joins base and name, refusing traversal outside base.
func safeJoin(base, name string) (string, bool) {
	clean := filepath.Clean("/" + strings.ReplaceAll(name, "\\", "/"))
	full := filepath.Join(base, clean)
	if full != base && !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

// parentDir returns the directory portion of path.
func parentDir(path string) string { return filepath.Dir(path) }

// runtimeWindows reports whether the host OS is Windows (test seam).
func runtimeWindows() bool { return runtimeGOOS() == "windows" }
