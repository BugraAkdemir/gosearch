// Package duckduckgo implements web search against html.duckduckgo.com, the
// no-JavaScript HTML endpoint. It is the most reliable provider in gosearch
// because that endpoint is explicitly designed to work without JavaScript.
//
// Note: the endpoint can serve an image-captcha page with HTTP 200 when it
// distrusts the caller; the shared httpclient.Detect handles that, so this
// package assumes it is parsing a genuine result page.
package duckduckgo

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/BugraAkdemir/gosearch/internal/htmlx"
	"github.com/BugraAkdemir/gosearch/internal/httpclient"
	"github.com/BugraAkdemir/gosearch/internal/provider"
	"github.com/BugraAkdemir/gosearch/internal/serrors"
)

// endpoint is DuckDuckGo's no-JS HTML search endpoint. It accepts the query as
// the q parameter and returns server-rendered result markup. It is a var, not a
// const, so tests can point Search at a local httptest server.
var Endpoint = "https://html.duckduckgo.com/html/"

// Search queries DuckDuckGo for query and returns up to maxResults results
// (maxResults <= 0 means no cap). It returns serrors.ErrChallenge/ErrBlocked if
// the endpoint blocks the request, or serrors.ErrNoResults if the page parsed
// cleanly but contained no results.
func Search(ctx context.Context, client *httpclient.Client, query string, maxResults int) ([]provider.Result, error) {
	u := Endpoint + "?q=" + url.QueryEscape(query)
	resp, err := client.Get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: %w", err)
	}
	if err := httpclient.Detect(resp); err != nil {
		return nil, fmt.Errorf("duckduckgo: %w", err)
	}

	doc, err := html.Parse(strings.NewReader(string(resp.Body)))
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: parse html: %w", err)
	}

	results := parse(doc, maxResults)
	if len(results) == 0 {
		return nil, fmt.Errorf("duckduckgo: %w", serrors.ErrNoResults)
	}
	return results, nil
}

// parse extracts results from a parsed DuckDuckGo HTML result document.
//
// The markup for one result is, in essence:
//
//	<div class="result ... web-result">
//	  <h2 class="result__title">
//	    <a class="result__a" href="//duckduckgo.com/l/?uddg=<encoded-url>&...">Title</a>
//	  </h2>
//	  <a class="result__snippet" href="...">Snippet text</a>
//	</div>
//
// We anchor on the title link (a.result__a): each one is a result. The snippet
// is the nearest a.result__snippet within the same enclosing result container.
func parse(doc *html.Node, maxResults int) []provider.Result {
	titleLinks := htmlx.FindAll(doc, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.A) && htmlx.HasClass(n, "result__a")
	})

	var out []provider.Result
	for _, a := range titleLinks {
		title := htmlx.Text(a)
		href := cleanURL(htmlx.Attr(a, "href"))
		if title == "" || href == "" {
			continue
		}
		res := provider.Result{Title: title, URL: href}
		if container := enclosingResult(a); container != nil {
			if snip := htmlx.FindFirst(container, func(n *html.Node) bool {
				return htmlx.Tag(n, atom.A) && htmlx.HasClass(n, "result__snippet")
			}); snip != nil {
				res.Snippet = htmlx.Text(snip)
			}
		}
		out = append(out, res)
		if maxResults > 0 && len(out) >= maxResults {
			break
		}
	}
	return out
}

// enclosingResult walks up from a title link to the nearest ancestor that is a
// result container (a div carrying the "result" or "result__body" class), so
// the snippet lookup is scoped to this result and cannot leak into the next.
func enclosingResult(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode &&
			(htmlx.HasClass(p, "result") || htmlx.HasClass(p, "result__body")) {
			return p
		}
	}
	return nil
}

// cleanURL normalizes a DuckDuckGo result href. DuckDuckGo usually wraps the
// destination in a redirect link of the form
// "//duckduckgo.com/l/?uddg=<url-encoded destination>&rut=..."; we extract and
// decode the uddg parameter to return the real destination. Protocol-relative
// URLs ("//host/...") are promoted to https. Direct http(s) URLs pass through.
func cleanURL(href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if strings.Contains(u.Host, "duckduckgo.com") && strings.HasPrefix(u.Path, "/l/") {
		if dest := u.Query().Get("uddg"); dest != "" {
			// uddg is already the destination URL (url.Values decoded it once).
			return dest
		}
	}
	return u.String()
}
