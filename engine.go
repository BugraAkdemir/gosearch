package gosearch

// Engine identifies which search engine Search should query. Pass one of the
// exported constants (DuckDuckGo, Google, Yandex) as the primary engine, and
// optionally more via WithFallback.
type Engine int

const (
	// DuckDuckGo queries html.duckduckgo.com, DuckDuckGo's no-JavaScript HTML
	// endpoint. It is the most reliable engine for this library because it is
	// explicitly designed to work without JavaScript.
	DuckDuckGo Engine = iota

	// Google queries Google Search. Google has no official no-JavaScript
	// endpoint and its result markup is regionally A/B tested, so parsing is
	// best-effort and more likely to break or be blocked than DuckDuckGo.
	Google

	// Yandex queries Yandex Search. Yandex applies aggressive, geo/IP-based
	// anti-bot gating, so it is the most likely of the three engines to return
	// ErrBlocked or ErrChallenge.
	Yandex
)

// String returns the engine's human-readable name, suitable for logs and error
// messages.
func (e Engine) String() string {
	switch e {
	case DuckDuckGo:
		return "duckduckgo"
	case Google:
		return "google"
	case Yandex:
		return "yandex"
	default:
		return "unknown"
	}
}

// valid reports whether e is one of the defined engine constants.
func (e Engine) valid() bool {
	return e == DuckDuckGo || e == Google || e == Yandex
}
