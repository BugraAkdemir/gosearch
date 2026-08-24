// Package provider holds the shared type that search-engine providers return.
//
// It lives in an internal package (rather than the public gosearch package) so
// providers can be written without importing gosearch, avoiding an import cycle
// (gosearch imports the providers). The public gosearch package converts these
// into its own documented Result type.
package provider

import (
	"net/url"
	"sort"
	"strings"
	"unicode"
)

// Result is one parsed search result, produced by a provider's Search function.
// It mirrors the public gosearch.Result field-for-field; gosearch copies these
// across so the public type's documentation and identity stay in the public
// package.
type Result struct {
	Title   string
	URL     string
	Snippet string

	// Date is the result's freshness stamp as the engine rendered it (for
	// example "2026-08-20" or "1 day ago"), extracted best-effort from the
	// result container. Engines often omit it entirely; "" is normal. The
	// root package strips this unless the caller opted in via WithDates.
	Date string
}

// NormalizeURL returns a comparison key for URL-based deduplication. Search
// engines routinely list one page under several spellings — percent-encoded
// vs literal non-ASCII query values, dotted vs undotted capital I (a live
// Bing duplicate: ?il=Istanbul vs ?il=%C4%B0stanbul), differing parameter
// order — and callers should see one result, not near-identical twins.
//
// The key folds what provably does not change the resource: scheme and host
// case, default ports, fragments, percent-encoding, and Unicode simple case
// (so İ and I collapse; deliberate — engines treat them as interchangeable).
// Everything else stays significant: path and trailing slash, query presence,
// http vs https. The returned key is NEVER a display URL — providers keep the
// original string in Result.URL.
func NormalizeURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if p := u.Port(); p != "" &&
		(scheme != "http" || p != "80") && (scheme != "https" || p != "443") {
		host += ":" + p
	}

	key := scheme + "://" + fold(host) + fold(u.Path)
	if q := u.Query(); len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for i, k := range keys {
			for _, v := range q[k] {
				if i > 0 || b.Len() > 0 {
					b.WriteByte('&')
				}
				b.WriteString(url.QueryEscape(fold(k)))
				b.WriteByte('=')
				b.WriteString(url.QueryEscape(fold(v)))
			}
		}
		key += "?" + b.String()
	}
	return key
}

// fold applies Unicode simple lowercasing to every rune. Simple mapping is
// the point: 'İ' (U+0130) lowers to plain 'i', so the live-observed
// Istanbul/%C4%B0stanbul duplicate merges while ordinary text is untouched.
func fold(s string) string {
	return strings.Map(unicode.ToLower, s)
}
