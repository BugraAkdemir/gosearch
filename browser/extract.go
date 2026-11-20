package browser

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
)

// osStat is indirected for tests.
var osStat = os.Stat

// maxUnzipBytes caps total uncompressed bytes written by unzip so a malformed
// or hostile archive (zip bomb) cannot exhaust disk space. A real
// chrome-headless-shell archive extracts to ~200-350 MiB.
var maxUnzipBytes int64 = 1 << 30 // 1 GiB

// unzip extracts the zip archive at src into dst, creating parent
// directories. File modes are preserved so executables stay executable; zip
// entries carry Unix mode in the upper bits of ExternalAttrs. It refuses to
// follow entries escaping dst ("zip slip") and enforces maxUnzipBytes total
// extracted size.
func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = r.Close() }()

	var totalWritten int64
	for _, f := range r.File {
		name, ok := safeJoin(dst, f.Name)
		if !ok {
			return fmt.Errorf("entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(name, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(parentDir(name), 0o755); err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			_ = out.Close()
			return err
		}
		remaining := maxUnzipBytes - totalWritten
		if remaining < 0 {
			remaining = 0
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, remaining+1))
		totalWritten += n
		if copyErr != nil {
			_ = in.Close()
			_ = out.Close()
			return copyErr
		}
		if totalWritten > maxUnzipBytes {
			_ = in.Close()
			_ = out.Close()
			return fmt.Errorf("archive exceeds maximum uncompressed size (%d bytes)", maxUnzipBytes)
		}
		if err := in.Close(); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

// chmodExec marks path as executable (best-effort; a no-op on Windows).
func chmodExec(path string) error {
	if runtimeWindows() {
		return nil
	}
	return os.Chmod(path, 0o755)
}
