package provider

import "testing"

// TestNormalizeURL pins the dedup-key rules: the key exists solely so that
// engines listing the same page under trivially different spellings collapse
// to one result. Displayed URLs are never rewritten — only the comparison key
// is normalized.
func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		same bool
	}{
		{
			name: "identical urls",
			a:    "https://example.com/a/b?q=1", b: "https://example.com/a/b?q=1",
			same: true,
		},
		{
			name: "host case is folded",
			a:    "https://EXample.COM/a", b: "https://example.com/a",
			same: true,
		},
		{
			name: "percent-encoded query value equals decoded form",
			a:    "https://www.mgm.gov.tr/tahmin/il-ve-ilceler.aspx?il=%C4%B0stanbul",
			b:    "https://www.mgm.gov.tr/tahmin/il-ve-ilceler.aspx?il=\xc4\xb0stanbul",
			same: true,
		},
		{
			name: "dotted vs undotted capital I folds together (observed live dup)",
			a:    "https://www.mgm.gov.tr/tahmin/il-ve-ilceler.aspx?il=Istanbul",
			b:    "https://www.mgm.gov.tr/tahmin/il-ve-ilceler.aspx?il=%C4%B0stanbul",
			same: true,
		},
		{
			name: "percent-encoded path segment equals decoded form",
			a:    "https://example.com/a%20b/c", b: "https://example.com/a b/c",
			same: true,
		},
		{
			name: "query parameter order is irrelevant",
			a:    "https://example.com/p?a=1&b=2", b: "https://example.com/p?b=2&a=1",
			same: true,
		},
		{
			name: "fragment is ignored",
			a:    "https://example.com/p#section", b: "https://example.com/p",
			same: true,
		},
		{
			name: "default port is ignored",
			a:    "https://example.com:443/p", b: "https://example.com/p",
			same: true,
		},
		{
			name: "different paths stay distinct",
			a:    "https://example.com/a", b: "https://example.com/b",
			same: false,
		},
		{
			name: "different hosts stay distinct",
			a:    "https://a.example.com/", b: "https://b.example.com/",
			same: false,
		},
		{
			name: "different schemes stay distinct",
			a:    "http://example.com/p", b: "https://example.com/p",
			same: false,
		},
		{
			name: "trailing slash is significant",
			a:    "https://example.com/dir/", b: "https://example.com/dir",
			same: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ka, kb := NormalizeURL(tc.a), NormalizeURL(tc.b)
			if (ka == kb) != tc.same {
				t.Errorf("NormalizeURL(%q)=%q vs NormalizeURL(%q)=%q: same=%v, want %v",
					tc.a, ka, tc.b, kb, ka == kb, tc.same)
			}
		})
	}
}
