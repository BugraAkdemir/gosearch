package browser

import (
	"os/exec"
	"runtime"
)

// discoverCandidates lists every executable name and absolute path this
// package probes, in priority order (stable channels first). Exported
// indirectly through the New error message so users can see exactly what was
// tried. Windows installs live under Program Files; macOS under /Applications;
// Linux relies on PATH plus the usual /usr locations.
func discoverCandidates() []string {
	names := []string{
		"google-chrome-stable", "google-chrome", "chromium", "chromium-browser",
		"microsoft-edge", "chrome-headless-shell",
	}
	switch runtime.GOOS {
	case "windows":
		return append(names,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		)
	case "darwin":
		return append(names,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		)
	default:
		return append(names,
			"/usr/bin/google-chrome-stable", "/usr/bin/google-chrome",
			"/usr/bin/chromium", "/usr/bin/chromium-browser",
			"/usr/bin/microsoft-edge", "/snap/bin/chromium",
		)
	}
}

// discover returns the first candidate that exists and is executable, or ""
// when none match. It never errors: absence of a browser is a normal state
// reported by New with full context.
func discover() string {
	for _, c := range discoverCandidates() {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		// Absolute paths are not resolved by LookPath on every platform when
		// they contain separators; stat them directly.
		if isExecutableFile(c) {
			return c
		}
	}
	return ""
}

// isExecutableFile reports whether path names an existing regular file with
// at least one execute bit set. On Windows, where the exec bit does not
// exist, existence alone is enough.
func isExecutableFile(path string) bool {
	info, err := osStat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
