// Package serrors holds gosearch's sentinel errors in an internal package.
//
// They live here, rather than in the public gosearch package, so that both the
// public package and internal packages (httpclient, providers) can return the
// exact same error values without creating an import cycle: the public package
// imports internal packages, so internal packages cannot import it back. The
// public gosearch package re-exports these under its own documented names.
package serrors

import "errors"

// These are the canonical sentinel error values. The public gosearch package
// re-exports them (ErrBlocked, ErrChallenge, ErrNoResults, ErrUnsupportedEngine)
// with full doc comments; see that package for usage semantics.
var (
	ErrBlocked           = errors.New("gosearch: request blocked by engine anti-bot system")
	ErrChallenge         = errors.New("gosearch: engine returned a captcha or javascript challenge")
	ErrNoResults         = errors.New("gosearch: no results found")
	ErrUnsupportedEngine = errors.New("gosearch: unsupported engine")
)
