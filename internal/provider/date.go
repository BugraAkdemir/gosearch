package provider

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/BugraAkdemir/gosearch/internal/htmlx"
)

// ExtractDate returns the freshness/publication date a search-result
// container carries, if any: a <time datetime="..."> attribute wins, then
// Bing's news_dt span text. It returns "" when the container carries neither —
// dates are best-effort metadata, never required, and engines differ wildly
// in whether they expose them on the no-JavaScript pages this library parses.
//
// The value passes through verbatim: ISO instants ("2026-08-20") and human
// relative stamps ("1 day ago") both occur. Whether the caller sees the value
// at all is decided one level up by the WithDates option, so callers who need
// timeless results keep getting them by default.
func ExtractDate(container *html.Node) string {
	if t := htmlx.FindFirst(container, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Time)
	}); t != nil {
		if dt := strings.TrimSpace(htmlx.Attr(t, "datetime")); dt != "" {
			return dt
		}
	}
	if s := htmlx.FindFirst(container, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Span) && htmlx.HasClass(n, "news_dt")
	}); s != nil {
		return strings.TrimSpace(htmlx.Text(s))
	}
	return ""
}
