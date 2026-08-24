// Package google implements web search against Google's classic non-JavaScript
// result page (the server-rendered markup served when the client does not run
// JavaScript).
//
// This provider is inherently best-effort: Google's result DOM is regionally
// A/B tested and can change without notice, and Google serves JS-challenge or
// "unusual traffic" pages to callers it distrusts (the shared
// httpclient.Detect reports those before this package ever parses). The parse
// heuristic below is therefore written against the documented/observed basic
// markup, degrades to empty snippets rather than wrong results, and MUST be
// re-validated against a real captured success page before its output is
// trusted end-to-end — see plan.md Phase 2 exit criteria and AGENTS.md Known
// Pitfalls.
package google

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

// Endpoint is Google's web search URL. It accepts the query as the q
// parameter and returns server-rendered result markup to non-JS clients. It
// is a var, not a const, so tests can point Search at a local httptest
// server.
var Endpoint = "https://www.google.com/search"

// Search queries Google for query and returns up to maxResults results
// (maxResults <= 0 means no cap). It returns serrors.ErrChallenge/ErrBlocked
// if Google challenges or blocks the request, or serrors.ErrNoResults if the
// page parsed cleanly but contained no results.
func Search(ctx context.Context, client *httpclient.Client, query string, maxResults int) ([]provider.Result, error) {
	u := Endpoint + "?q=" + url.QueryEscape(query)
	resp, err := client.Get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("google: %w", err)
	}
	if err := httpclient.Detect(resp); err != nil {
		return nil, fmt.Errorf("google: %w", err)
	}

	doc, err := html.Parse(strings.NewReader(string(resp.Body)))
	if err != nil {
		return nil, fmt.Errorf("google: parse html: %w", err)
	}

	results := parse(doc, maxResults)
	if len(results) == 0 {
		return nil, fmt.Errorf("google: %w", serrors.ErrNoResults)
	}
	return results, nil
}

// parse extracts results from a parsed Google result document.
//
// The heuristic (best-effort — see the package comment): every result carries
// its destination inside a redirect link of the form
//
//	<a href="/url?q=<encoded destination>&sa=U&ved=...">
//	  <h3 ...>Title</h3>            <!-- modern basic HTML: h3 inside the link -->
//	</a>
//	<div class="BNeawe s3v9rd ...">Snippet text</div>
//
// while the older basic variant wraps the other way around:
//
//	<h3 class="r"><a href="/url?q=...">Title</a></h3>
//	<span class="st">Snippet text</span>
//
// We anchor on a[href^=/url] links, take the title from an inner h3 (falling
// back to a wrapping h3, then to the link's own text), and look for the
// snippet inside the nearest result container (class g / xpd / Gx5Zad /
// MjjYud). Destinations that are themselves Google search pages (pagination,
// "more results") are skipped, as are duplicates.
func parse(doc *html.Node, maxResults int) []provider.Result {
	anchors := htmlx.FindAll(doc, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.A) && cleanURL(htmlx.Attr(n, "href")) != ""
	})

	var out []provider.Result
	seen := map[string]bool{}
	for _, a := range anchors {
		dest := cleanURL(htmlx.Attr(a, "href"))
		title := resultTitle(a)
		key := provider.NormalizeURL(dest)
		if title == "" || seen[key] {
			continue
		}
		seen[key] = true

		res := provider.Result{Title: title, URL: dest, Snippet: resultSnippet(a)}
		out = append(out, res)
		if maxResults > 0 && len(out) >= maxResults {
			break
		}
	}
	return out
}

// cleanURL decodes a Google result href into the real destination URL, or ""
// if the href is not a result redirect (/url?q=...) or its destination is not
// a usable http(s) page (including Google's own wrapped search-pagination
// links).
func cleanURL(href string) string {
	ref, err := url.Parse(href)
	if err != nil || ref.Path != "/url" {
		return ""
	}
	dest := ref.Query().Get("q")
	if dest == "" {
		return ""
	}
	u, err := url.Parse(dest)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if u.Host == "www.google.com" && strings.HasPrefix(u.Path, "/search") {
		return ""
	}
	return dest
}

// resultTitle extracts a result's title text from its redirect link: from an
// h3 inside the link (modern basic HTML), else from an h3 directly wrapping
// the link (legacy h3.r > a shape), else from the link's own text.
func resultTitle(a *html.Node) string {
	if h := htmlx.FindFirst(a, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.H3)
	}); h != nil {
		return htmlx.Text(h)
	}
	if htmlx.Tag(a.Parent, atom.H3) {
		return htmlx.Text(a.Parent)
	}
	return htmlx.Text(a)
}

// containerClasses are the class tokens observed on one-result containers
// across Google's basic-HTML variants. Any single match scopes the snippet
// lookup so it cannot leak between adjacent results. The list will need
// extending when Google rotates its obfuscated class names — that is expected
// and is why this provider is documented as best-effort.
var containerClasses = [...]string{"g", "xpd", "Gx5Zad", "MjjYud"}

// resultSnippet finds the snippet text belonging to the result anchored at a.
// It looks within the nearest result container for a snippet-classed block
// (BNeawe s3v9rd on modern basic HTML, span.st on the legacy shape) that is
// not the title link's own subtree. Best-effort: no container or no snippet
// block means an empty snippet, never a wrong one.
func resultSnippet(a *html.Node) string {
	container := enclosingResult(a)
	if container == nil {
		return ""
	}
	sn := htmlx.FindFirst(container, func(n *html.Node) bool {
		isSnippetTag := htmlx.Tag(n, atom.Div) || htmlx.Tag(n, atom.Span)
		hasSnippetClass := htmlx.HasClass(n, "s3v9rd") || htmlx.HasClass(n, "st")
		return isSnippetTag && hasSnippetClass && !contains(n, a)
	})
	if sn == nil {
		return ""
	}
	return htmlx.Text(sn)
}

// enclosingResult walks up from the title link to the nearest ancestor
// carrying a known result-container class (see containerClasses), bounded to a
// few levels so a page with none of those classes cannot cost a full-tree
// climb. Returns nil when no container class matches.
func enclosingResult(a *html.Node) *html.Node {
	for p, depth := a.Parent, 0; p != nil && depth < 12; p, depth = p.Parent, depth+1 {
		if p.Type != html.ElementNode {
			continue
		}
		for _, c := range containerClasses {
			if htmlx.HasClass(p, c) {
				return p
			}
		}
	}
	return nil
}

// contains reports whether anc is n or one of n's ancestors.
func contains(anc, n *html.Node) bool {
	for c := n; c != nil; c = c.Parent {
		if c == anc {
			return true
		}
	}
	return false
}
