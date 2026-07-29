// Package htmlx holds small DOM helpers over golang.org/x/net/html that the
// providers and the readability extractor share: attribute lookup, class
// testing, text extraction, and tree traversal. Keeping them in one place
// avoids each parser reinventing node-walking (and getting the whitespace
// handling subtly wrong).
package htmlx

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Attr returns the value of n's attribute key, or "" if absent. Attribute names
// are matched case-insensitively (HTML attribute names are case-insensitive).
func Attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// HasClass reports whether n's class attribute contains the given class as a
// whitespace-separated token.
func HasClass(n *html.Node, class string) bool {
	if n == nil {
		return false
	}
	for _, f := range strings.Fields(Attr(n, "class")) {
		if f == class {
			return true
		}
	}
	return false
}

// Tag reports whether n is an element with the given atom (for example atom.A,
// atom.Div).
func Tag(n *html.Node, a atom.Atom) bool {
	return n != nil && n.Type == html.ElementNode && n.DataAtom == a
}

// Text returns the visible text of n's subtree: all descendant text nodes
// concatenated, with runs of whitespace collapsed to single spaces and the
// result trimmed. Content inside <script> and <style> is excluded.
func Text(n *html.Node) string {
	var b strings.Builder
	collectText(n, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

func collectText(n *html.Node, b *strings.Builder) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style) {
		return
	}
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		b.WriteByte(' ')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, b)
	}
}

// Walk invokes fn on n and every descendant, in preorder (parent before
// children).
func Walk(n *html.Node, fn func(*html.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}

// FindAll returns every node in n's subtree (including n) for which pred is
// true, in preorder.
func FindAll(n *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	Walk(n, func(node *html.Node) {
		if pred(node) {
			out = append(out, node)
		}
	})
	return out
}

// FindFirst returns the first node in n's subtree (including n) for which pred
// is true, in preorder, or nil.
func FindFirst(n *html.Node, pred func(*html.Node) bool) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != nil || node == nil {
			return
		}
		if pred(node) {
			found = node
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}
