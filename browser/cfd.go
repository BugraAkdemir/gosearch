package browser

import (
	"context"
	"encoding/json"
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
// tests — this one does (see cfd_arm64_test.go).
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

// StableEngineAsset returns the current stable chrome-headless-shell version
// and its download URL for this platform.
func StableEngineAsset(ctx context.Context) (version, url string, err error) {
	data, err := fetchManifest(ctx)
	if err != nil {
		return "", "", err
	}
	return parseManifest(data)
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
	if version == "" || slug == "" {
		return "", "", fmt.Errorf("manifest missing stable version or unsupported platform %s/%s", runtimeGOOS(), runtimeGOARCH())
	}
	for _, d := range m.Channels.Stable.Downloads.HeadlessShell {
		if d.Platform == slug {
			return version, d.URL, nil
		}
	}
	return "", "", fmt.Errorf("no chrome-headless-shell asset for platform %q", slug)
}

// downloadEngine downloads (or reuses) chrome-headless-shell in the OS user
// cache directory and returns the path of its executable.
func downloadEngine(ctx context.Context, cacheDirOverride string) (string, error) {
	if runtimeGOOS() == "linux" && runtimeGOARCH() == "arm64" {
		return downloadPlaywrightHeadlessShellLinuxARM64(ctx, cacheDirOverride)
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

	zipPath := filepath.Join(os.TempDir(), "gosearch-chrome-headless-shell-"+version+".zip")
	if err := httpDownload(ctx, zipURL, zipPath); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dst); err != nil {
		return "", err
	}
	_ = os.Remove(zipPath)

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
func downloadPlaywrightHeadlessShellLinuxARM64(ctx context.Context, cacheDirOverride string) (string, error) {
	root, err := engineCacheRoot(cacheDirOverride)
	if err != nil {
		return "", err
	}
	// Prefixed distinctly from chrome-for-testing's version-string
	// subdirectories (e.g. "152.0.7977.54") so the two sources can never
	// collide in the same cache root.
	dst := filepath.Join(root, "playwright-"+playwrightHeadlessShellRevision)
	if p := findEngineBinary(dst); p != "" {
		return p, nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/builds/chromium/%s/chromium-headless-shell-linux-arm64.zip", playwrightCDNBase, playwrightHeadlessShellRevision)
	zipPath := filepath.Join(os.TempDir(), "gosearch-chrome-headless-shell-playwright-"+playwrightHeadlessShellRevision+".zip")
	if err := httpDownload(ctx, url, zipPath); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dst); err != nil {
		return "", err
	}
	_ = os.Remove(zipPath)

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
	zipPath := filepath.Join(dst, "engine.zip")
	if err := os.WriteFile(zipPath, embeddedEngineZip, 0o644); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dst); err != nil {
		return "", err
	}
	p := findEngineBinary(dst)
	if p == "" {
		return "", fmt.Errorf("embedded archive contains no %s binary", engineBinaryName)
	}
	if err := chmodExec(p); err != nil {
		return "", err
	}
	return p, nil
}

// engineCacheRoot returns the engine storage directory — an explicit
// override when set (WithCacheDir), else <user-cache>/gosearch/browser —
// creating it if needed.
func engineCacheRoot(override string) (string, error) {
	if override != "" {
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", err
		}
		return override, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	dir := filepath.Join(base, "gosearch", "browser")
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
	tmp := dstFile + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, res.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
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
