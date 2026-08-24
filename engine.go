package gosearch

// Engine identifies which search engine Search should query. Pass one of the
// exported constants (DuckDuckGo, Google, Yandex, Bing) as the primary
// engine, and optionally more via WithFallback.
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
	// anti-bot gating, so it is the most likely of these engines to return
	// ErrBlocked or ErrChallenge.
	Yandex

	// Bing queries Microsoft Bing Search. Bing's plain-HTML endpoint is the
	// least aggressive of the four against automated clients — it served
	// clean organic results to a flagged datacenter IP during testing where
	// Google and Yandex challenged or blocked. Titles arrive wrapped in
	// Bing's click-tracker; the real destination is recovered from the
	// result's visible citation URL (best-effort, see the provider docs).
	Bing
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
	case Bing:
		return "bing"
	default:
		return "unknown"
	}
}

// valid reports whether e is one of the defined engine constants.
func (e Engine) valid() bool {
	return e == DuckDuckGo || e == Google || e == Yandex || e == Bing
}
