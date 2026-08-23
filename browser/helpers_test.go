package browser

import (
	"archive/zip"
	"os"
	"testing"
)

// setGOOSArch overrides the runtime seams for platform-slug tests.
func setGOOSArch(osName, arch string) {
	runtimeGOOSFn = func() string { return osName }
	runtimeGOARCHFn = func() string { return arch }
}

// restoreGOOSArch puts the real values back.
func restoreGOOSArch(realOS, realArch string) {
	runtimeGOOSFn = func() string { return realOS }
	runtimeGOARCHFn = func() string { return realArch }
}

// buildTestZip writes files (name → contents) as a zip archive, marking
// .sh-named and extensionless entries executable like chrome-for-testing's
// archives do.
func buildTestZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w := zip.NewWriter(f)
	for name, body := range files {
		hdr := &zip.FileHeader{Name: name}
		if filepathBase(name) == "chrome-headless-shell" {
			hdr.SetMode(0o755)
		} else {
			hdr.SetMode(0o644)
		}
		entry, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// filepathBase is a tiny indirection so the test file needs no extra import.
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == os.PathSeparator {
			return p[i+1:]
		}
	}
	return p
}
