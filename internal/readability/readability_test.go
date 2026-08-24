package readability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "readability", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestExtractArticle(t *testing.T) {
	art, err := Extract(fixture(t, "article.html"))
	if err != nil {
		t.Fatal(err)
	}

	// og:title must win over <title>.
	if art.Title != "The Real Article Title" {
		t.Errorf("Title = %q, want og:title 'The Real Article Title'", art.Title)
	}

	// Body prose must be present.
	for _, want := range []string{"quick brown fox", "second paragraph", "third paragraph"} {
		if !strings.Contains(art.Content, want) {
			t.Errorf("Content missing %q\n---\n%s", want, art.Content)
		}
	}

	// Noise must be gone: nav, ads, footer, related links.
	for _, bad := range []string{"Home", "Buy now", "Related 1", "Terms", "Twitter"} {
		if strings.Contains(art.Content, bad) {
			t.Errorf("Content should not contain noise %q\n---\n%s", bad, art.Content)
		}
	}
}

func TestExtractTitleFallbackToTitleTag(t *testing.T) {
	html := []byte(`<html><head><title>Only Title Tag</title></head>
		<body><article><p>Some reasonably long paragraph of prose, with commas, that clears the scoring threshold for content.</p></article></body></html>`)
	art, err := Extract(html)
	if err != nil {
		t.Fatal(err)
	}
	if art.Title != "Only Title Tag" {
		t.Errorf("Title = %q, want 'Only Title Tag'", art.Title)
	}
}

func TestExtractEmptyContentNotAnError(t *testing.T) {
	// A page with no real content (e.g. JS-rendered) yields empty Content, not
	// an error.
	html := []byte(`<html><head><title>Empty</title></head><body><div id="app"></div></body></html>`)
	art, err := Extract(html)
	if err != nil {
		t.Fatalf("Extract returned error for contentless page: %v", err)
	}
	if art.Content != "" {
		t.Errorf("Content = %q, want empty for contentless page", art.Content)
	}
	if art.Title != "Empty" {
		t.Errorf("Title = %q, want 'Empty'", art.Title)
	}
}

func TestExtractMalformedHTMLStillParses(t *testing.T) {
	// x/net/html is lenient; malformed markup should still not error.
	html := []byte(`<html><body><article><p>Unclosed paragraph with enough words, and commas, to score as content`)
	if _, err := Extract(html); err != nil {
		t.Errorf("Extract on malformed HTML errored: %v", err)
	}
}

// TestExtractSurvivesNoiseMarkerClassesOnRoot pins a live-found bug
// (en.wikipedia.org, 2026-08-24): modern pages put substrings like "header",
// "menu", or "nav" on the <html>/<body> elements themselves via feature-flag
// classes ("vector-feature-language-in-header-enabled"). Substring-matching
// those markers against the ROOT elements removed the entire tree and made
// every page yield empty content. The root must be untouchable.
func TestExtractSurvivesNoiseMarkerClassesOnRoot(t *testing.T) {
	page := []byte(`<html class="client-nojs vector-feature-language-in-header-enabled vector-feature-page-tools-pinned-disabled vector-feature-navigation-update-disabled">
<body class="skin-vector sticky-header-enabled menu-visible">
<nav class="sidebar"><a href="/">Home</a></nav>
<article>
<h2>Real content</h2>
<p>This paragraph is the genuine article body, long enough with commas to score as the main content of the page.</p>
</article>
</body></html>`)
	art, err := Extract(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(art.Content, "genuine article body") {
		t.Errorf("Content = %q, want the article prose (root classes must not nuke the tree)", art.Content)
	}
	if strings.Contains(art.Content, "Home") {
		t.Errorf("nav noise survived: %q", art.Content)
	}
}
