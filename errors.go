package gosearch

import "errors"

// Sentinel errors returned by Search and Fetch. Callers should test for these
// with errors.Is, never by string-matching an error message. Providers wrap
// these with additional context using fmt.Errorf("%w: ..."), so errors.Is
// continues to work through wrapping and through the errors.Join that the
// fallback chain uses.
var (
	// ErrBlocked means the engine refused the request because its anti-bot
	// system flagged it (for example an HTTP 429, an IP-reputation block, or a
	// redirect to a "you look like a bot" page). This is not a bug in the
	// query; it typically means the current IP/network is distrusted. Callers
	// using WithFallback will advance to the next engine on this error.
	ErrBlocked = errors.New("gosearch: request blocked by engine anti-bot system")

	// ErrChallenge means the engine served an interactive challenge (an image
	// CAPTCHA or a JavaScript anti-bot challenge) instead of results. It is a
	// specific, common form of being blocked. gosearch never attempts to solve
	// such challenges by design. Callers using WithFallback will advance to the
	// next engine on this error.
	ErrChallenge = errors.New("gosearch: engine returned a captcha or javascript challenge")

	// ErrNoResults means the request succeeded and was parsed successfully, but
	// the engine genuinely returned no results for the query. This is a valid
	// answer, not a failure: WithFallback does NOT advance to another engine on
	// this error, because a different engine is no more likely to have results
	// for a query that legitimately has none.
	ErrNoResults = errors.New("gosearch: no results found")

	// ErrUnsupportedEngine means Search was called with an Engine value that is
	// not one of the defined constants (DuckDuckGo, Google, Yandex).
	ErrUnsupportedEngine = errors.New("gosearch: unsupported engine")
)
