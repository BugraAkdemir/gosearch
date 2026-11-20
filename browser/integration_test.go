//go:build integration

// Integration tests hit the live web through a real browser and are excluded
// from `go test ./...` by the integration build tag (repo rule: unit tests
// stay deterministic and offline). Run with:
//
//	go test -race -tags integration ./...
package browser

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BugraAkdemir/gosearch"
)

// TestLiveSearchAndFetch drives a real engine against Google and example.com.
// It skips itself when no browser exists and downloads are not enabled, so it
// is safe to run anywhere:
//
//	SKIP when: no system browser + AllowDownload not passed
func TestLiveSearchAndFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	e, err := New(ctx)
	if err != nil {
		t.Skipf("no usable browser on this machine: %v", err)
	}
	defer func() { _ = e.Close() }()

	results, err := e.Search(ctx, "facebook")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("got 0 results")
	}
	for i, r := range results {
		if r.Title == "" || !strings.HasPrefix(r.URL, "http") {
			t.Errorf("result[%d] malformed: %+v", i, r)
		}
	}

	page, err := e.Fetch(ctx, "https://example.com/")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.Title == "" || len(page.Content) < 20 {
		t.Errorf("page incomplete: title=%q len(content)=%d", page.Title, len(page.Content))
	}
	if !strings.Contains(strings.ToLower(page.Content), "example domain") {
		t.Errorf("content missing expected text; got %.200q", page.Content)
	}

	eMD, err := New(ctx, WithMarkdown())
	if err != nil {
		t.Fatalf("New(WithMarkdown): %v", err)
	}
	defer func() { _ = eMD.Close() }()
	mdPage, err := eMD.Fetch(ctx, "https://example.com/")
	if err != nil {
		t.Fatalf("Fetch(WithMarkdown): %v", err)
	}
	if !strings.Contains(mdPage.Content, "# Example Domain") && !strings.Contains(strings.ToLower(mdPage.Content), "example domain") {
		t.Errorf("markdown content missing expected text; got %.200q", mdPage.Content)
	}
	var _ = gosearch.ErrChallenge // keep sentinel import documented for callers
}
