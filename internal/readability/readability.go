// Package readability extracts the main readable content of an HTML page —
// title and body text — with navigation, ads, scripts, and boilerplate removed.
// It powers gosearch.Fetch.
//
// The algorithm is a deliberately small, dependency-free approximation of the
// well-known "readability" approach: strip known-noise elements, score
// block containers by how much paragraph text they hold (penalizing
// link-heavy blocks), pick the top-scoring container, and return its text. It
// is not a full readability implementation and does not run JavaScript, so a
// page whose content is rendered client-side may yield little or no text — that
// is a documented limitation, not a bug.
package readability

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/BugraAkdemir/gosearch/internal/htmlx"
)

// Article is the extracted content of a page.
type Article struct {
	// Title is the page's best-guess title (og:title, <title>, or first <h1>).
	Title string
	// Content is the extracted main body text, paragraphs joined by blank lines.
	Content string
}

// noiseTags are elements that never hold main article content and are removed
// wholesale before scoring.
var noiseTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Nav: true, atom.Header: true,
	atom.Footer: true, atom.Aside: true, atom.Form: true, atom.Noscript: true,
	atom.Iframe: true, atom.Svg: true, atom.Button: true,
}

// noiseIDClass are substrings that, when found in an element's id or class,
// mark it as boilerplate (menus, sidebars, comments, ads) to be removed.
var noiseIDClass = []string{
	"nav", "footer", "header", "sidebar", "comment", "menu", "ad-", "ads",
	"advert", "promo", "social", "share", "related", "cookie", "banner",
}

// Extract parses HTML and returns its title and main body text. It returns an
// error only if the HTML cannot be parsed at all; a page with no discernible
// main content yields an Article with empty Content (not an error).
func Extract(htmlBytes []byte) (*Article, error) {
	doc, err := html.Parse(strings.NewReader(string(htmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("readability: parse html: %w", err)
	}

	title := extractTitle(doc)
	removeNoise(doc)

	best := topCandidate(doc)
	content := ""
	if best != nil {
		content = extractContent(best)
	}
	return &Article{Title: title, Content: content}, nil
}

// extractTitle prefers the og:title meta, then <title>, then the first <h1>.
func extractTitle(doc *html.Node) string {
	if og := htmlx.FindFirst(doc, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Meta) &&
			strings.EqualFold(htmlx.Attr(n, "property"), "og:title")
	}); og != nil {
		if c := strings.TrimSpace(htmlx.Attr(og, "content")); c != "" {
			return c
		}
	}
	if t := htmlx.FindFirst(doc, func(n *html.Node) bool { return htmlx.Tag(n, atom.Title) }); t != nil {
		if txt := htmlx.Text(t); txt != "" {
			return txt
		}
	}
	if h1 := htmlx.FindFirst(doc, func(n *html.Node) bool { return htmlx.Tag(n, atom.H1) }); h1 != nil {
		return htmlx.Text(h1)
	}
	return ""
}

// removeNoise detaches noise elements from the tree in place so they do not
// contribute to scoring or content extraction. The document root (<html>) and
// <body> are exempt: modern pages hang feature-flag classes on them
// ("vector-feature-language-in-header-enabled", "sticky-header-enabled") whose
// substrings match noise markers, and removing either element would erase the
// whole page — a real failure observed against en.wikipedia.org.
func removeNoise(doc *html.Node) {
	var toRemove []*html.Node
	htmlx.Walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		if n.DataAtom == atom.Html || n.DataAtom == atom.Body {
			return
		}
		if noiseTags[n.DataAtom] || hasNoiseIDClass(n) {
			toRemove = append(toRemove, n)
		}
	})
	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
}

func hasNoiseIDClass(n *html.Node) bool {
	id := strings.ToLower(htmlx.Attr(n, "id"))
	class := strings.ToLower(htmlx.Attr(n, "class"))
	for _, marker := range noiseIDClass {
		if strings.Contains(id, marker) || strings.Contains(class, marker) {
			return true
		}
	}
	return false
}

// topCandidate scores every block container by the paragraph text it holds
// (penalizing link-dense blocks) and returns the highest scorer.
func topCandidate(doc *html.Node) *html.Node {
	scores := map[*html.Node]float64{}
	paragraphs := htmlx.FindAll(doc, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.P) || htmlx.Tag(n, atom.Pre) || htmlx.Tag(n, atom.Td)
	})
	for _, p := range paragraphs {
		text := htmlx.Text(p)
		if len(text) < 25 {
			continue
		}
		score := 1.0
		score += float64(strings.Count(text, ",")) // comma-rich text ~ prose
		score += min(float64(len(text))/100.0, 3.0)
		score *= 1.0 - linkDensity(p)

		// Credit the paragraph's parent and, at half weight, its grandparent —
		// the container that holds the most prose wins.
		if p.Parent != nil {
			scores[p.Parent] += score
			if p.Parent.Parent != nil {
				scores[p.Parent.Parent] += score / 2
			}
		}
	}

	var best *html.Node
	var bestScore float64
	for node, s := range scores {
		if s > bestScore {
			best, bestScore = node, s
		}
	}
	return best
}

// linkDensity returns the fraction (0..1) of n's text that sits inside anchors;
// a high value means the block is navigation/link list rather than prose.
func linkDensity(n *html.Node) float64 {
	total := len(htmlx.Text(n))
	if total == 0 {
		return 0
	}
	linkLen := 0
	for _, a := range htmlx.FindAll(n, func(node *html.Node) bool { return htmlx.Tag(node, atom.A) }) {
		linkLen += len(htmlx.Text(a))
	}
	d := float64(linkLen) / float64(total)
	if d > 1 {
		d = 1
	}
	return d
}

// extractContent returns the readable text of the winning container: its
// paragraph-like descendants joined by blank lines, or the container's whole
// text if it has no such descendants.
func extractContent(container *html.Node) string {
	blocks := htmlx.FindAll(container, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.P) || htmlx.Tag(n, atom.Pre) ||
			htmlx.Tag(n, atom.H2) || htmlx.Tag(n, atom.H3) ||
			htmlx.Tag(n, atom.Li)
	})
	var parts []string
	for _, b := range blocks {
		if txt := htmlx.Text(b); txt != "" {
			parts = append(parts, txt)
		}
	}
	if len(parts) == 0 {
		return htmlx.Text(container)
	}
	return strings.Join(parts, "\n\n")
}
