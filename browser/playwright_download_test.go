package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakePlaywrightZip returns a valid chromium-headless-shell-linux-arm64.zip
// payload, matching the archive layout Playwright actually ships: the
// binary is named "headless_shell" (no "chrome-" prefix), nested one
// directory deep.
func fakePlaywrightZip(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "src.zip")
	buildTestZip(t, zipPath, map[string][]byte{
		"chrome-linux/headless_shell": []byte("#!/bin/sh\nexit 0\n"),
		"chrome-linux/LICENSE":        []byte("fake license"),
	})
	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestDownloadPlaywrightHeadlessShellLinuxARM64 exercises the linux/arm64
// download path end to end against a fake CDN, and confirms the extracted
// "headless_shell" binary is renamed to the package-wide canonical
// engineBinaryName so findEngineBinary (unmodified) still locates it.
func TestDownloadPlaywrightHeadlessShellLinuxARM64(t *testing.T) {
	payload := fakePlaywrightZip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	realBase := playwrightCDNBase
	playwrightCDNBase = srv.URL
	defer func() { playwrightCDNBase = realBase }()

	cacheDir := t.TempDir()
	p, err := downloadPlaywrightHeadlessShellLinuxARM64(context.Background(), cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != engineBinaryName+exeSuffix() {
		t.Errorf("downloaded binary path = %q, want basename %q (renamed from headless_shell)", p, engineBinaryName+exeSuffix())
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("renamed binary mode = %v, want execute bits preserved", info.Mode())
		}
	}
	// findEngineBinary — unmodified, chrome-for-testing-oriented lookup —
	// must find it under the new name with no awareness of Playwright.
	if bin := findEngineBinary(cacheDir); bin == "" {
		t.Error("findEngineBinary found nothing after the arm64 download+rename")
	}

	// A second call must short-circuit to the cached binary, not re-download.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("second call re-hit the network instead of reusing the cached binary")
		w.WriteHeader(http.StatusInternalServerError)
	})
	p2, err := downloadPlaywrightHeadlessShellLinuxARM64(context.Background(), cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if p2 != p {
		t.Errorf("second call path = %q, want the same cached path %q", p2, p)
	}
}

// TestDownloadEngineDispatchesToPlaywrightOnLinuxARM64 confirms downloadEngine
// itself routes linux/arm64 to the Playwright path instead of the
// chrome-for-testing manifest flow (which has no linux/arm64 asset at all).
func TestDownloadEngineDispatchesToPlaywrightOnLinuxARM64(t *testing.T) {
	payload := fakePlaywrightZip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	realBase := playwrightCDNBase
	playwrightCDNBase = srv.URL
	defer func() { playwrightCDNBase = realBase }()

	gotOS, gotArch := runtime.GOOS, runtime.GOARCH
	defer restoreGOOSArch(gotOS, gotArch)
	setGOOSArch("linux", "arm64")

	cacheDir := t.TempDir()
	p, err := downloadEngine(context.Background(), cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != engineBinaryName+exeSuffix() {
		t.Errorf("downloadEngine(linux/arm64) path = %q, want basename %q", p, engineBinaryName+exeSuffix())
	}
}

// TestDownloadEngineTrulyUnsupportedPlatformWrapsSentinel confirms a
// combination with no download source at all (neither chrome-for-testing
// nor the arm64 special case) still errors clearly and matches
// ErrUnsupportedPlatform via errors.Is, so callers can build an actionable
// hint without parsing the message text.
func TestDownloadEngineTrulyUnsupportedPlatformWrapsSentinel(t *testing.T) {
	gotOS, gotArch := runtime.GOOS, runtime.GOARCH
	defer restoreGOOSArch(gotOS, gotArch)
	setGOOSArch("linux", "386")

	_, err := downloadEngine(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("want an error for linux/386, got nil")
	}
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("err = %v, want errors.Is match against ErrUnsupportedPlatform", err)
	}
}
