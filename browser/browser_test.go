package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverCandidatesNonEmpty(t *testing.T) {
	if len(discoverCandidates()) == 0 {
		t.Fatal("discoverCandidates() empty for every OS")
	}
}

// TestPlatformSlug pins the slug mapping so a typo cannot silently point
// downloads at a nonexistent chrome-for-testing asset.
func TestPlatformSlug(t *testing.T) {
	cases := map[[2]string]string{
		{"linux", "amd64"}:   "linux64",
		{"darwin", "arm64"}:  "mac-arm64",
		{"darwin", "amd64"}:  "mac-x64",
		{"windows", "amd64"}: "win64",
		{"linux", "386"}:     "", // unsupported → caller must error clearly
	}
	for key, want := range cases {
		gotOS, gotArch := runtime.GOOS, runtime.GOARCH
		defer func() { restoreGOOSArch(gotOS, gotArch) }()
		setGOOSArch(key[0], key[1])
		if got := platformSlug(); got != want {
			t.Errorf("platformSlug(%s/%s) = %q, want %q", key[0], key[1], got, want)
		}
	}
}

func TestParseManifestPicksStableHeadlessShell(t *testing.T) {
	const manifest = `{
	  "channels": {
	    "Stable": {
	      "version": "131.0.6778.204",
	      "downloads": {
	        "chrome": [{"platform": "linux64", "url": "https://example.com/chrome.zip"}],
	        "chrome-headless-shell": [
	          {"platform": "win64", "url": "https://example.com/chs-win64.zip"},
	          {"platform": "linux64", "url": "https://example.com/chs-linux64.zip"}
	        ]
	      }
	    }
	  }
	}`
	version, url, err := parseManifest([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	if version != "131.0.6778.204" {
		t.Errorf("version = %q", version)
	}
	if url != "https://example.com/chs-linux64.zip" {
		t.Errorf("url = %q, want the linux64 headless-shell asset", url)
	}
}

func TestSafeJoinContainsTraversal(t *testing.T) {
	// Design: traversal attempts are NEUTRALIZED into the destination
	// directory rather than rejected, so a hostile archive entry "../evil"
	// lands at dst/evil and can never escape dst.
	p, ok := safeJoin("/cache/dir", "../evil")
	if !ok {
		t.Fatal("safeJoin rejected an entry; sanitization is the intended mitigation")
	}
	if !strings.HasPrefix(p, "/cache/dir") || strings.Contains(p, "..") {
		t.Errorf("safeJoin(../evil) = %q, want a path contained in /cache/dir", p)
	}
	// A clean nested entry must survive unchanged.
	want := filepath.Join("/cache/dir", "sub", "file.bin")
	if got, ok := safeJoin("/cache/dir", "sub/file.bin"); !ok || got != want {
		t.Errorf("safeJoin(sub/file.bin) = %q (%v), want %q", got, ok, want)
	}
}

// TestFindCachedBinaryFindsAPriorDownloadWithoutNetwork pins the actual bug
// this function exists to fix: IsInstalled()-style callers never set
// AllowDownload, so before findCachedBinary existed, resolveExecutable had
// no way to see a binary a PRIOR Install() call had already downloaded —
// only discover()'s system paths. Reported live: browser.Install()
// succeeded on a Raspberry Pi (linux/arm64, via the Playwright fallback),
// yet the very next IsInstalled() check still reported false.
func TestFindCachedBinaryFindsAPriorDownloadWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	if p := findCachedBinary(dir); p != "" {
		t.Fatalf("findCachedBinary(empty cache) = %q, want \"\"", p)
	}

	// Mirrors what downloadEngine's Playwright branch leaves behind:
	// engineCacheRoot/<prefixed-dir>/<canonical binary name>.
	nested := filepath.Join(dir, "playwright-1241", "chrome-linux")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(nested, engineBinaryName+exeSuffix())
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findCachedBinary(dir)
	if got != binPath {
		t.Errorf("findCachedBinary(populated cache) = %q, want %q", got, binPath)
	}
}

func TestUnzipPreservesExecutableBit(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "engine.zip")
	buildTestZip(t, zipPath, map[string][]byte{
		"chrome-headless-shell-linux64/chrome-headless-shell": []byte("#!/bin/sh\nexit 0\n"),
		"chrome-headless-shell-linux64/LICENSE":               []byte("fake license"),
	})
	dst := filepath.Join(dir, "out")
	if err := unzip(zipPath, dst); err != nil {
		t.Fatal(err)
	}
	bin := findEngineBinary(dst)
	if bin == "" {
		t.Fatal("findEngineBinary found nothing after extraction")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(bin)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("extracted binary mode = %v, want execute bits preserved", info.Mode())
		}
	}
}
