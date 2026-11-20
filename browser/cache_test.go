package browser

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStableEngineAssetSupportsLinuxARM64AndErrorsOnUnsupported(t *testing.T) {
	realOS, realArch := runtime.GOOS, runtime.GOARCH
	defer restoreGOOSArch(realOS, realArch)

	setGOOSArch("linux", "arm64")
	ver, assetURL, err := StableEngineAsset(context.Background())
	if err != nil {
		t.Fatalf("StableEngineAsset(linux/arm64) failed: %v", err)
	}
	if ver != playwrightHeadlessShellVersion() {
		t.Errorf("version = %q, want %q", ver, playwrightHeadlessShellVersion())
	}
	if !strings.Contains(assetURL, "chromium-headless-shell-linux-arm64.zip") {
		t.Errorf("assetURL = %q, want playwright arm64 zip URL", assetURL)
	}

	setGOOSArch("linux", "386")
	_, _, err = StableEngineAsset(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("StableEngineAsset(linux/386) err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestCleanCacheAndUninstall(t *testing.T) {
	t.Run("missing dir is a no-op and not created", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist-cache")
		if err := CleanCache(WithCacheDir(missing)); err != nil {
			t.Fatalf("CleanCache(missing): %v", err)
		}
		if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("CleanCache created missing dir (stat err=%v)", err)
		}
	})

	t.Run("clears cached binaries inside custom cacheDir while preserving dir", func(t *testing.T) {
		cacheDir := t.TempDir()
		nested := filepath.Join(cacheDir, "152.0.7977.54", "chrome-headless-shell-linux64")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(nested, engineBinaryName+exeSuffix())
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := findCachedBinary(cacheDir); got != bin {
			t.Fatalf("findCachedBinary before clean = %q, want %q", got, bin)
		}

		if err := Uninstall(WithCacheDir(cacheDir)); err != nil {
			t.Fatalf("Uninstall: %v", err)
		}
		if got := findCachedBinary(cacheDir); got != "" {
			t.Errorf("findCachedBinary after Uninstall = %q, want empty", got)
		}
		if info, err := os.Stat(cacheDir); err != nil || !info.IsDir() {
			t.Errorf("custom cacheDir should still exist after CleanCache, err=%v", err)
		}
	})

	t.Run("refuses dangerous target paths", func(t *testing.T) {
		dangerous := []string{
			string(filepath.Separator),
			".",
			"..",
			os.TempDir(),
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			dangerous = append(dangerous, home)
		}
		if userCache, err := os.UserCacheDir(); err == nil && userCache != "" {
			dangerous = append(dangerous, userCache)
		}
		for _, p := range dangerous {
			if err := CleanCache(WithCacheDir(p)); err == nil {
				t.Errorf("CleanCache(%q) succeeded, want safety refusal error", p)
			}
		}
	})
}

func TestFindCachedBinaryDoesNotCreateMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-created")
	if got := findCachedBinary(missing); got != "" {
		t.Fatalf("findCachedBinary(missing) = %q, want empty", got)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("findCachedBinary created missing dir on disk (stat err=%v)", err)
	}
}

func TestUnzipEnforcesMaxUnzipBytes(t *testing.T) {
	origMax := maxUnzipBytes
	maxUnzipBytes = 1024 // 1 KiB cap for test speed
	defer func() { maxUnzipBytes = origMax }()

	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bomb.zip")
	buildTestZip(t, zipPath, map[string][]byte{
		"big.bin": bytes.Repeat([]byte("A"), 2048),
	})

	err := unzip(zipPath, filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "maximum uncompressed size") {
		t.Fatalf("unzip oversize archive err = %v, want max uncompressed size error", err)
	}
}
