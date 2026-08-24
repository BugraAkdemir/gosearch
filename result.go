// Package gosearch performs web search and page-content extraction without an
// API key. It works by fetching each search engine's public HTML result page
// (DuckDuckGo, Google, Yandex, Bing) and parsing it directly, plus a Fetch
// function that extracts the readable content of any URL.
//
// It is built for local-first / zero-dependency Go programs (for example an
// LLM agent's web-search tool) that cannot or will not depend on a hosted
// search API. The only third-party dependency is golang.org/x/net/html.
//
// # Reliability
//
// Search engines run anti-bot systems (image captchas, JavaScript challenges,
// IP-reputation blocks). gosearch behaves like an ordinary browser — realistic
// headers, a persistent cookie jar, self-imposed rate limiting, and the
// lowest-friction endpoint each engine offers — but it never attempts to solve
// a CAPTCHA, execute a JavaScript challenge, or spoof its identity to defeat a
// security control. When an engine blocks a request, Search returns a typed
// error (ErrBlocked or ErrChallenge) rather than an empty result, so callers
// can react (for example, fall back to another engine via WithFallback).
//
// Anti-bot strictness varies by engine and by network. DuckDuckGo is the most
// reliable (it is the only engine with an official no-JavaScript HTML
// endpoint); Google is moderate; Yandex is the most likely to block; Bing is
// the most tolerant of automated clients after DuckDuckGo. Requests from
// datacenter/cloud IPs are far more likely to be blocked than requests from a
// residential network.
package gosearch

// Result is a single web-search result returned by Search.
type Result struct {
	// Title is the result's display title (the clickable heading).
	Title string
	// URL is the destination link the result points to.
	URL string
	// Snippet is the short description/excerpt shown under the title. It may
	// be empty for some engines or result types.
	Snippet string
}

// Page is the extracted, readable content of a URL returned by Fetch. It holds
// the main article/body text with navigation, ads, scripts, and boilerplate
// removed — not the raw HTML of the page.
type Page struct {
	// URL is the page that was fetched. If the request followed redirects,
	// this is the final URL.
	URL string
	// Title is the page's title (from <title>, an <h1>, or article metadata,
	// whichever the extractor judged best). It may be empty.
	Title string
	// Content is the extracted main text of the page. It may be empty if the
	// extractor could not identify a main content region (for example, a page
	// whose content is rendered entirely by JavaScript).
	Content string
}
