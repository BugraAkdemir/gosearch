package readability

import (
	"strings"
	"testing"
)

const mdFixture = `<html><head><title>T</title></head><body>
<article>
<h2>Kurulum</h2>
<p>Paketi kurmadan önce Go araç zincirinin sisteminizde kurulu olduğundan emin olmalısınız.</p>
<p>Ayrıntılar icin <a href="https://example.com/docs">resmi belgeler</a> sayfasina bakin; ayrica <strong>zorunlu</strong> ve <em>isteme bagli</em> adimlari atlamayin.</p>
<ul><li>Depoyu klonla</li>
<li>Bağımlılıkları indir</li>
<li>Testleri çalıştır</li></ul>
<pre><code>go test ./...</code></pre>
</article>
</body></html>`

// TestExtractMarkdown pins the opt-in markdown rendering: structure survives
// as headings, lists, fenced code, emphasis, and links instead of being
// flattened to bare text.
func TestExtractMarkdown(t *testing.T) {
	art, err := ExtractMarkdown([]byte(mdFixture))
	if err != nil {
		t.Fatal(err)
	}

	wantParts := []string{
		"## Kurulum",
		"[resmi belgeler](https://example.com/docs)",
		"**zorunlu**",
		"*isteme bagli*",
		"- Depoyu klonla\n- Bağımlılıkları indir\n- Testleri çalıştır",
		"```\ngo test ./...\n```",
	}
	for _, want := range wantParts {
		if !strings.Contains(art.Content, want) {
			t.Errorf("markdown content missing %q\n--- got ---\n%s", want, art.Content)
		}
	}
	if strings.Contains(art.Content, "<h2>") || strings.Contains(art.Content, "<p>") {
		t.Errorf("markdown content still carries raw tags:\n%s", art.Content)
	}
}

// TestExtractDefaultStaysPlainText pins backward compatibility: without the
// opt-in, Extract must keep returning plain text with no markdown syntax.
func TestExtractDefaultStaysPlainText(t *testing.T) {
	art, err := Extract([]byte(mdFixture))
	if err != nil {
		t.Fatal(err)
	}
	c := art.Content
	for _, marker := range []string{"```", "](http", "\n## ", "\n- "} {
		if strings.Contains(c, marker) {
			t.Errorf("default Extract leaked markdown marker %q:\n%s", marker, c)
		}
	}
}

func TestRenderTableAndOrder(t *testing.T) {
	page := `<html><body><article>
<h1>Baslik</h1>
<table><tr><th>Ad</th><th>Deger</th></tr><tr><td>timeout</td><td>15s</td></tr></table>
<p>Kapanis paragrafi burada yer aliyor ve tabloyu izleyerek sirayi koruyor.</p>
</article></body></html>`
	art, err := ExtractMarkdown([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Baslik\n\n| Ad | Deger |\n| --- | --- |\n| timeout | 15s |\n\nKapanis paragrafi burada yer aliyor ve tabloyu izleyerek sirayi koruyor."
	if art.Content != want {
		t.Errorf("content mismatch\n--- got ---\n%s\n--- want ---\n%s", art.Content, want)
	}
}
