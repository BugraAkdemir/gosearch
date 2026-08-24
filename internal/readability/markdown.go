package readability

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/BugraAkdemir/gosearch/internal/htmlx"
)

// headingLevels maps heading atoms to their markdown level so rendering never
// relies on atom enum ordering.
var headingLevels = map[atom.Atom]int{
	atom.H1: 1, atom.H2: 2, atom.H3: 3, atom.H4: 4, atom.H5: 5, atom.H6: 6,
}

// ExtractMarkdown is Extract with the winning container rendered as
// GitHub-flavored Markdown instead of flattened text: headings survive as
// # levels, lists keep their bullets, code blocks come back fenced, links stay
// [text](href), and emphasis survives as **bold** / *italic*. Structure is
// exactly what makes extracted pages useful to LLM consumers; callers wanting
// bare text use Extract.
//
// The same limitations as Extract apply, plus deliberate simplifications:
// nested inline formatting flattens (a link inside bold renders its inner
// text), <br> becomes a space, blockquotes collapse to one "> "-prefixed
// line, and table cells carry inline text only.
func ExtractMarkdown(htmlBytes []byte) (*Article, error) {
	doc, err := html.Parse(strings.NewReader(string(htmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("readability: parse html: %w", err)
	}

	title := extractTitle(doc)
	removeNoise(doc)

	best := topCandidate(doc)
	content := ""
	if best != nil {
		content = renderMarkdown(best)
	}
	return &Article{Title: title, Content: content}, nil
}

// renderMarkdown emits the container's block-level descendants in document
// order as Markdown blocks joined by blank lines. Wrapper elements without
// block meaning are recursed into; when the container holds no recognizable
// blocks the fallback is its whole flattened text, mirroring Extract.
func renderMarkdown(container *html.Node) string {
	var blocks []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch {
			case headingLevels[c.DataAtom] > 0:
				blocks = append(blocks,
					strings.Repeat("#", headingLevels[c.DataAtom])+" "+collapseText(c))
			case c.DataAtom == atom.P:
				if t := inlineMD(c); t != "" {
					blocks = append(blocks, t)
				}
			case c.DataAtom == atom.Pre:
				blocks = append(blocks, fencedCode(c))
			case c.DataAtom == atom.Ul, c.DataAtom == atom.Ol:
				blocks = append(blocks, markdownList(c))
			case c.DataAtom == atom.Blockquote:
				if t := inlineMD(c); t != "" {
					blocks = append(blocks, "> "+t)
				}
			case c.DataAtom == atom.Table:
				blocks = append(blocks, markdownTable(c))
			default:
				walk(c)
			}
		}
	}
	walk(container)
	if len(blocks) == 0 {
		return htmlx.Text(container)
	}
	return strings.Join(blocks, "\n\n")
}

// inlineMD renders an element's mixed content as one normalized inline
// Markdown run: links, emphasis, and inline code keep their syntax, everything
// else contributes text with whitespace collapsed to single spaces.
func inlineMD(n *html.Node) string {
	var b strings.Builder
	writeInline(&b, n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func writeInline(b *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			b.WriteString(c.Data)
		case html.ElementNode:
			switch c.DataAtom {
			case atom.A:
				if txt := collapseText(c); txt != "" {
					if href := htmlx.Attr(c, "href"); href != "" {
						b.WriteString("[" + txt + "](" + href + ")")
					} else {
						b.WriteString(txt)
					}
				}
			case atom.Strong, atom.B:
				if txt := collapseText(c); txt != "" {
					b.WriteString("**" + txt + "**")
				}
			case atom.Em, atom.I:
				if txt := collapseText(c); txt != "" {
					b.WriteString("*" + txt + "*")
				}
			case atom.Code:
				if txt := collapseText(c); txt != "" {
					b.WriteString("`" + txt + "`")
				}
			case atom.Br:
				b.WriteString(" ")
			default:
				writeInline(b, c)
			}
		}
	}
}

// collapseText flattens an element to plain text with whitespace collapsed.
func collapseText(n *html.Node) string {
	return strings.Join(strings.Fields(htmlx.Text(n)), " ")
}

// fencedCode returns the raw text of a <pre> as a fenced code block. Whitespace
// inside code is preserved verbatim — collapsing it would corrupt the sample.
func fencedCode(pre *html.Node) string {
	return "```\n" + strings.Trim(htmlx.Text(pre), "\n") + "\n```"
}

// markdownList renders ul/ol items ("- " or "N. "). Only direct li children
// count, so nested lists flatten into their parent item's inline text rather
// than double-emitting.
func markdownList(list *html.Node) string {
	var lines []string
	i := 0
	for c := list.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.DataAtom != atom.Li {
			continue
		}
		i++
		if list.DataAtom == atom.Ol {
			lines = append(lines, fmt.Sprintf("%d. %s", i, inlineMD(c)))
		} else {
			lines = append(lines, "- "+inlineMD(c))
		}
	}
	return strings.Join(lines, "\n")
}

// markdownTable renders rows as pipe tables, treating the first row as the
// header and emitting the standard separator row under it.
func markdownTable(table *html.Node) string {
	rows := htmlx.FindAll(table, func(n *html.Node) bool {
		return htmlx.Tag(n, atom.Tr)
	})
	if len(rows) == 0 {
		return ""
	}
	var out []string
	headerCells := 0
	for ri, row := range rows {
		var cells []string
		for c := row.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode &&
				(c.DataAtom == atom.Td || c.DataAtom == atom.Th) {
				cells = append(cells, inlineMD(c))
			}
		}
		if len(cells) == 0 {
			continue
		}
		if ri == 0 {
			headerCells = len(cells)
		}
		out = append(out, "| "+strings.Join(cells, " | ")+" |")
		if ri == 0 {
			out = append(out, "|"+strings.Repeat(" --- |", headerCells))
		}
	}
	return strings.Join(out, "\n")
}
