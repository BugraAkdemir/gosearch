package browser

import "errors"

// ErrNoBrowserFound is returned by New when no usable Chromium-family
// executable was discovered on the system, none was supplied via
// WithExecutable, and downloads are not enabled. The error text names every
// location that was probed so the fix (install Chrome/Chromium/Edge, pass a
// path, or enable downloads) is obvious.
var ErrNoBrowserFound = errors.New("browser: no chromium-family executable found")

// ErrDownloadDisabled is returned when the caller explicitly opted OUT of
// downloading (AllowDownload(false)) but no system browser was found either,
// or when an embed-mode build was requested but the engine archive is absent.
var ErrDownloadDisabled = errors.New("browser: download disabled and no executable available")
