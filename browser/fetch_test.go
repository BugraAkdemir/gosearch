package browser

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const sampleArticleHTML = `<!doctype html>
<html>
<head><title>Sample Guide</title></head>
<body>
  <nav><a href="/">Home</a> <a href="/about">About</a></nav>
  <article>
    <h1>Building Local Tools</h1>
    <p>Go makes it straightforward to build self-contained command line utilities, background services, and libraries with predictable memory footprints.</p>
    <h2>Key Steps</h2>
    <ul>
      <li>Keep dependencies small and auditable.</li>
      <li>Read more in <a href="https://example.com/docs">the docs</a>.</li>
    </ul>
    <pre><code>go test -race ./...</code></pre>
    <table>
      <tr><th>Metric</th><th>Value</th></tr>
      <tr><td>Deps</td><td>One</td></tr>
    </table>
  </article>
  <footer>Copyright Notice</footer>
</body>
</html>`

func TestExtractPagePlainVsMarkdown(t *testing.T) {
	plain, err := extractPage([]byte(sampleArticleHTML), "https://example.com/guide", false)
	if err != nil {
		t.Fatalf("extractPage plain: %v", err)
	}
	if plain.URL != "https://example.com/guide" {
		t.Errorf("plain.URL = %q", plain.URL)
	}
	if plain.Title != "Sample Guide" {
		t.Errorf("plain.Title = %q, want Sample Guide", plain.Title)
	}
	if !strings.Contains(plain.Content, "self-contained command line utilities") {
		t.Errorf("plain.Content missing main prose: %q", plain.Content)
	}
	if strings.Contains(plain.Content, "Copyright Notice") {
		t.Errorf("plain.Content leaked footer noise: %q", plain.Content)
	}
	if strings.Contains(plain.Content, "[the docs](https://example.com/docs)") {
		t.Errorf("plain mode unexpectedly emitted Markdown link: %q", plain.Content)
	}

	md, err := extractPage([]byte(sampleArticleHTML), "https://example.com/guide", true)
	if err != nil {
		t.Fatalf("extractPage markdown: %v", err)
	}
	for _, want := range []string{
		"# Building Local Tools",
		"## Key Steps",
		"- Keep dependencies small and auditable.",
		"[the docs](https://example.com/docs)",
		"```\ngo test -race ./...\n```",
		"| Metric | Value |",
	} {
		if !strings.Contains(md.Content, want) {
			t.Errorf("markdown Content missing %q;\ngot:\n%s", want, md.Content)
		}
	}
}

func TestTruncateHTMLSafeRespectsUTF8AndTagBoundaries(t *testing.T) {
	t.Run("under limit unchanged", func(t *testing.T) {
		in := "<p>Merhaba dünya</p>"
		got := truncateHTMLSafe(in, 1024)
		if string(got) != in {
			t.Errorf("got %q, want %q", got, in)
		}
	})

	t.Run("zero or negative returns nil", func(t *testing.T) {
		if got := truncateHTMLSafe("<p>abc</p>", 0); got != nil {
			t.Errorf("got %q, want nil", got)
		}
		if got := truncateHTMLSafe("<p>abc</p>", -5); got != nil {
			t.Errorf("got %q, want nil", got)
		}
	})

	t.Run("does not split multi-byte UTF-8 rune", func(t *testing.T) {
		// "ş" is 2 bytes (0xC5 0x9F), "€" is 3 bytes (0xE2 0x82 0xAC).
		in := "<p>a" + "ş" + "€" + "b</p>"
		for limit := 1; limit <= len(in); limit++ {
			got := truncateHTMLSafe(in, limit)
			if len(got) > limit {
				t.Errorf("limit=%d: len(got)=%d exceeds cap", limit, len(got))
			}
			if !utf8.Valid(got) {
				t.Errorf("limit=%d: invalid UTF-8 produced: %x", limit, got)
			}
		}
	})

	t.Run("keeps complete multi-byte rune flush at limit", func(t *testing.T) {
		// "<p>€" is 6 bytes: '<'(1) 'p'(1) '>'(1) '€'(3).
		in := "<p>€</p>"
		got := truncateHTMLSafe(in, 6)
		if string(got) != "<p>€" {
			t.Errorf("got %q, want %q", got, "<p>€")
		}
	})

	t.Run("backs up before unclosed tag", func(t *testing.T) {
		in := `<p>hello</p><div class="very-long-class-name">tail</div>`
		// Cut inside `<div class="very...`
		got := truncateHTMLSafe(in, len(`<p>hello</p><div class="ve`))
		if string(got) != `<p>hello</p>` {
			t.Errorf("got %q, want %q", got, `<p>hello</p>`)
		}
	})
}

func TestWithMarkdownOptionSetsConfig(t *testing.T) {
	cfg := &engineConfig{}
	if cfg.markdown {
		t.Fatal("default markdown should be false")
	}
	WithMarkdown()(cfg)
	if !cfg.markdown {
		t.Fatal("WithMarkdown() did not set cfg.markdown = true")
	}
}
