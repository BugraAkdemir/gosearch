// Package yandex implements web search against Yandex's server-rendered
// search result page (yandex.com/search/?text=...).
//
// This is the most fragile of the three providers: Yandex gates aggressively
// on geo/IP reputation (302 to showcaptcha even on first requests from
// datacenter IPs — see httpclient.Detect and AGENTS.md Known Pitfalls), and
// its result DOM rotates behind obfuscated class names. The parse heuristic
// below targets the documented organic-result shapes ("serp-item" containers,
// "organic__url" links, "organic__text" snippets), degrades to empty snippets
// rather than wrong results, and MUST be re-validated against a real captured
// success page before its output is trusted end-to-end — see plan.md Phase 3
// exit criteria and AGENTS.md Known Pitfalls.
package yandex

import (
	"context"
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

// Endpoint is Yandex's web search URL. It accepts the query as the text
// parameter and returns server-rendered organic-result markup. It is a var,
// not a const, so tests can point Search at a local httptest server.
var Endpoint = "https://yandex.com/search/"

// Search queries Yandex for query and returns up to maxResults results
// (maxResults <= 0 means no cap). It returns serrors.ErrChallenge/ErrBlocked
// if Yandex challenges or blocks the request, or serrors.ErrNoResults if the
// page parsed cleanly but contained no results.
func Search(ctx context.Context, client *httpclient.Client, query string, maxResults int) ([]provider.Result, error) {
	u := Endpoint + "?text=" + url.QueryEscape(query)
	resp, err := client.Get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("yandex: %w", err)
	}
	if err := httpclient.Detect(resp); err != nil {
		return nil, fmt.Errorf("yandex: %w", err)
	}

	doc, err := html.Parse(strings.NewReader(string(resp.Body)))
	if err != nil {
		return nil, fmt.Errorf("yandex: parse html: %w", err)
	}

	results := parse(doc, maxResults)
	if len(results) == 0 {
		return nil, fmt.Errorf("yandex: %w", serrors.ErrNoResults)
	}
	return results, nil
}

// parse extracts results from a parsed Yandex result document.
//
// The heuristic (best-effort — see the package comment): one result is one
// "serp-item" list item carrying an organic link, in essence
//
//	<li class="serp-item">
//	  <a class="organic__url" href="<destination or yandex clck wrap>">
//	    <h2 ...>Title</h2>
//	  </a>
//	  <div class="organic__text">Snippet text</div>
//	</li>
//
// We scope everything inside the serp-item so adjacent results cannot leak
// into each other. Destinations that stay on Yandex's own plumbing (wrapped
// /clck/ click-trackers without a decodable target, /search/ pagination,
// passport, captcha pages) are skipped, as are duplicates.
func parse(doc *html.Node, maxResults int) []provider.Result {
	items := htmlx.FindAll(doc, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Li) && htmlx.HasClass(n, "serp-item")
	})

	var out []provider.Result
	seen := map[string]bool{}
	for _, item := range items {
		link := htmlx.FindFirst(item, func(n *html.Node) bool {
			return htmlx.Tag(n, atom.A) && htmlx.HasClass(n, "organic__url")
		})
		if link == nil {
			continue
		}
		dest := cleanURL(htmlx.Attr(link, "href"))
		title := resultTitle(link)
		if dest == "" || title == "" || seen[dest] {
			continue
		}
		seen[dest] = true

		res := provider.Result{Title: title, URL: dest}
		if sn := htmlx.FindFirst(item, func(n *html.Node) bool {
			return htmlx.HasClass(n, "organic__text")
		}); sn != nil {
			res.Snippet = htmlx.Text(sn)
		}
		out = append(out, res)
		if maxResults > 0 && len(out) >= maxResults {
			break
		}
	}
	return out
}

// cleanURL normalizes an organic result href to the real destination URL, or
// "" when the href does not lead off Yandex's own plumbing: protocol-relative
// URLs are promoted to https, Yandex-hosted /clck/ click-tracker wraps are
// unwrapped only when their destination rides along in a query parameter,
// and Yandex search-pagination / passport / captcha paths are rejected.
func cleanURL(href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	ref, err := url.Parse(href)
	if err != nil || (ref.Scheme != "http" && ref.Scheme != "https") {
		return ""
	}

	host := strings.ToLower(ref.Hostname())
	yandexOwned := host == "yandex.com" || strings.HasSuffix(host, ".yandex.com") ||
		host == "yandex.ru" || strings.HasSuffix(host, ".yandex.ru") ||
		host == "ya.ru" || strings.HasSuffix(host, ".ya.ru")
	if !yandexOwned {
		return href
	}

	switch {
	case strings.HasPrefix(ref.Path, "/clck/"):
		// Click-tracker wrap: the destination occasionally travels in a
		// query parameter; when it doesn't, the real URL is unknowable
		// client-side, so the result is skipped rather than misreported.
		q := ref.Query()
		for _, k := range []string{"url", "u", "www"} {
			v := q.Get(k)
			if v == "" {
				continue
			}
			if inner, err := url.Parse(v); err == nil &&
				(inner.Scheme == "http" || inner.Scheme == "https") {
				return v
			}
			break
		}
		return ""
	case strings.HasPrefix(ref.Path, "/search"),
		strings.HasPrefix(ref.Path, "/showcaptcha"),
		strings.HasPrefix(ref.Path, "/passport"),
		strings.HasPrefix(host, "passport."),
		strings.HasPrefix(host, "captcha."):
		return ""
	default:
		// A legit page hosted on a Yandex domain (docs, mail, maps...).
		return href
	}
}

// resultTitle extracts a result's title from its organic link: from an h2
// inside the link when present (the common shape), else the link's own text.
func resultTitle(a *html.Node) string {
	if h := htmlx.FindFirst(a, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.H2)
	}); h != nil {
		return htmlx.Text(h)
	}
	return htmlx.Text(a)
}
