// Package bing implements web search against Microsoft Bing's plain-HTML
// result page.
//
// Measured 2026-08-24 from a flagged datacenter IP that Google and Yandex
// challenge or block outright: Bing served a clean HTTP 200 with ten organic
// results to a plain HTTP GET — no CAPTCHA, no JavaScript requirement. It is
// therefore the most automation-tolerant engine in gosearch after DuckDuckGo,
// and a strong fallback candidate.
//
// Two markup realities shape the parser:
//
//   - Titles live in h2 > a inside li.b_algo containers; snippets sit in
//     div.b_caption > p. This structure is comparatively stable but still
//     third-party-owned and may change without notice — best-effort like the
//     other providers.
//
//   - Title links are wrapped in Bing's click-tracker (/ck/a?...p=<token>).
//     Some responses carry the real destination in the u=a1<base64url> query
//     parameter (decoded directly); when absent, the destination is recovered
//     from the human-visible <cite> URL under the title, which Bing truncates
//     with "…" — that reconstruction is documented as best-effort and may lose
//     deep path segments on truncated entries.
package bing

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/BugraAkdemir/gosearch/internal/htmlx"
	"github.com/BugraAkdemir/gosearch/internal/httpclient"
	"github.com/BugraAkdemir/gosearch/internal/provider"
	"github.com/BugraAkdemir/gosearch/internal/serrors"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Endpoint is Bing's web search URL. It is a var so tests can point Search at
// a local httptest server.
var Endpoint = "https://www.bing.com/search"

// Search queries Bing for query and returns up to maxResults results
// (maxResults <= 0 means no cap). It returns serrors.ErrBlocked if Bing
// throttles or rejects the request, or serrors.ErrNoResults if the page parsed
// cleanly but contained no organic results.
func Search(ctx context.Context, client *httpclient.Client, query string, maxResults int) ([]provider.Result, error) {
	u := Endpoint + "?q=" + url.QueryEscape(query)
	resp, err := client.Get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("bing: %w", err)
	}
	if err := httpclient.Detect(resp); err != nil {
		return nil, fmt.Errorf("bing: %w", err)
	}

	doc, err := html.Parse(strings.NewReader(string(resp.Body)))
	if err != nil {
		return nil, fmt.Errorf("bing: parse html: %w", err)
	}

	results := parse(doc, maxResults)
	if len(results) == 0 {
		return nil, fmt.Errorf("bing: %w", serrors.ErrNoResults)
	}
	return results, nil
}

// parse extracts results from a parsed Bing result document: one li.b_algo per
// organic hit, title from h2 > a, snippet from the first <p> in the container,
// destination via cleanURL. Duplicates by cleaned URL are dropped.
func parse(doc *html.Node, maxResults int) []provider.Result {
	items := htmlx.FindAll(doc, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Li) && htmlx.HasClass(n, "b_algo")
	})

	var out []provider.Result
	seen := map[string]bool{}
	for _, item := range items {
		h2 := htmlx.FindFirst(item, func(n *html.Node) bool {
			return htmlx.Tag(n, atom.H2)
		})
		if h2 == nil {
			continue
		}
		link := htmlx.FindFirst(h2, func(n *html.Node) bool {
			return htmlx.Tag(n, atom.A)
		})
		if link == nil {
			continue
		}

		title := strings.TrimSpace(htmlx.Text(link))
		dest := cleanURL(htmlx.Attr(link, "href"), citeURL(item))
		key := provider.NormalizeURL(dest)
		if dest == "" || title == "" || seen[key] {
			continue
		}
		seen[key] = true

		res := provider.Result{Title: title, URL: dest}
		if sn := htmlx.FindFirst(item, func(n *html.Node) bool {
			return htmlx.Tag(n, atom.P)
		}); sn != nil {
			res.Snippet = strings.TrimSpace(htmlx.Text(sn))
		}
		res.Date = provider.ExtractDate(item)
		out = append(out, res)
		if maxResults > 0 && len(out) >= maxResults {
			break
		}
	}
	return out
}

// citeURL returns the trimmed text of the result's visible citation element
// (class b_attribution > cite), or "" when absent.
func citeURL(item *html.Node) string {
	cite := htmlx.FindFirst(item, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Cite)
	})
	if cite == nil {
		return ""
	}
	return strings.TrimSpace(htmlx.Text(cite))
}

// cleanURL recovers the real destination for a Bing result. Priority:
//  1. u=a1<base64url> query parameter embedded in the tracker link (exact).
//  2. The visible citation string, e.g. "example.com › path › page" or a full
//     https URL, rebuilt into a navigable address (best-effort: Bing truncates
//     long paths with "…", losing tail segments).
//
// Returns "" when neither yields a usable http(s) address.
func cleanURL(trackerHref, cite string) string {
	if dest := decodeTrackerParam(trackerHref); dest != "" {
		return dest
	}
	return fromCite(cite)
}

// decodeTrackerParam unwraps Bing's /ck/a click-tracker href: the real
// destination travels in the u parameter as "a1" + base64url where "/"
// characters were replaced by "!". Returns "" when absent or undecodable.
func decodeTrackerParam(trackerHref string) string {
	ref, err := url.Parse(trackerHref)
	if err != nil {
		return ""
	}
	tok := ref.Query().Get("u")
	if !strings.HasPrefix(tok, "a1") {
		return ""
	}
	body := strings.ReplaceAll(strings.TrimPrefix(tok, "a1"), "!", "/")
	raw, err := base64.URLEncoding.DecodeString(body +
		strings.Repeat("=", (4-len(body)%4)%4))
	if err != nil {
		return ""
	}
	dest := string(raw)
	u, err := url.Parse(dest)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return dest
}

// fromCite rebuilds a navigable URL from Bing's visible citation text.
// Accepts either a full URL or the segmented display form
// "host › dir › page"; missing schemes are promoted to https. Trailing
// truncation markers ("…", "...") are stripped — the result may point at a
// parent directory of the original target, which beats having no URL at all
// and is documented as best-effort.
func fromCite(cite string) string {
	cite = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(cite), ".… "))
	if cite == "" {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(cite, "›") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	host := parts[0]
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	out := host
	for _, p := range parts[1:] {
		out = strings.TrimRight(out, "/") + "/" + strings.Trim(p, "/")
	}
	u, err := url.Parse(out)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return out
}
