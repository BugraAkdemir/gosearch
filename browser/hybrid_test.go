package browser

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BugraAkdemir/gosearch"
)

func newTestStubEngine(t *testing.T, opts ...Option) *Engine {
	t.Helper()
	badBin := filepath.Join(t.TempDir(), "unused-stub-browser")
	all := append([]Option{WithExecutable(badBin)}, opts...)
	e, err := New(context.Background(), all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestFetchWithFallbackStaticSkipsBrowser(t *testing.T) {
	e := newTestStubEngine(t)
	browserCalled := false
	longText := strings.Repeat("This static page has plenty of readable content. ", 4)

	page, err := e.fetchWithFallback(
		context.Background(),
		"https://example.com/article",
		func(_ context.Context, rawURL string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return &gosearch.Page{URL: rawURL, Title: "Static", Content: longText}, nil
		},
		func(_ context.Context, _ string) (*gosearch.Page, error) {
			browserCalled = true
			return nil, errors.New("browser should not be invoked")
		},
	)
	if err != nil {
		t.Fatalf("fetchWithFallback: %v", err)
	}
	if browserCalled {
		t.Fatal("browser fallback was called for a rich static HTTP page")
	}
	if page.Title != "Static" {
		t.Errorf("page.Title = %q, want Static", page.Title)
	}
}

func TestFetchWithFallbackTriggersOnThinOrEmptyContent(t *testing.T) {
	e := newTestStubEngine(t)
	browserCalled := false

	page, err := e.fetchWithFallback(
		context.Background(),
		"https://spa.example.com/",
		func(_ context.Context, rawURL string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return &gosearch.Page{URL: rawURL, Title: "Shell", Content: "Loading..."}, nil
		},
		func(_ context.Context, rawURL string) (*gosearch.Page, error) {
			browserCalled = true
			return &gosearch.Page{URL: rawURL, Title: "Hydrated", Content: "Full rendered SPA article content."}, nil
		},
	)
	if err != nil {
		t.Fatalf("fetchWithFallback: %v", err)
	}
	if !browserCalled {
		t.Fatal("expected browser fallback on thin HTTP content")
	}
	if page.Title != "Hydrated" {
		t.Errorf("page.Title = %q, want Hydrated", page.Title)
	}
}

func TestFetchWithFallbackMinContentZeroOnlyFallsBackOnEmpty(t *testing.T) {
	e := newTestStubEngine(t, WithFallbackMinContent(0))
	browserCalled := false

	page, err := e.fetchWithFallback(
		context.Background(),
		"https://short.example.com/",
		func(_ context.Context, rawURL string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return &gosearch.Page{URL: rawURL, Title: "Short", Content: "Hi"}, nil
		},
		func(_ context.Context, _ string) (*gosearch.Page, error) {
			browserCalled = true
			return &gosearch.Page{Content: "browser"}, nil
		},
	)
	if err != nil {
		t.Fatalf("fetchWithFallback: %v", err)
	}
	if browserCalled {
		t.Fatal("WithFallbackMinContent(0) should NOT fall back on non-empty content")
	}
	if page.Content != "Hi" {
		t.Errorf("page.Content = %q, want Hi", page.Content)
	}
}

func TestFetchWithFallbackGracefulDegradationOnThinHTTPWhenBrowserFails(t *testing.T) {
	e := newTestStubEngine(t)
	page, err := e.fetchWithFallback(
		context.Background(),
		"https://short.example.com/",
		func(_ context.Context, rawURL string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return &gosearch.Page{URL: rawURL, Title: "Short", Content: "Small valid notice."}, nil
		},
		func(_ context.Context, _ string) (*gosearch.Page, error) {
			return nil, errors.New("browser boom")
		},
	)
	if err != nil {
		t.Fatalf("expected graceful degradation to thin HTTP page, got err: %v", err)
	}
	if page.Content != "Small valid notice." {
		t.Errorf("page.Content = %q", page.Content)
	}
}

func TestFetchWithFallbackTriggersOnBlockedAndJoinsDualFailure(t *testing.T) {
	e := newTestStubEngine(t)

	// HTTP blocked -> browser succeeds
	page, err := e.fetchWithFallback(
		context.Background(),
		"https://walled.example.com/",
		func(_ context.Context, _ string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return nil, fmt.Errorf("http: %w", gosearch.ErrChallenge)
		},
		func(_ context.Context, rawURL string) (*gosearch.Page, error) {
			return &gosearch.Page{URL: rawURL, Title: "Unlocked", Content: "Unlocked by JS"}, nil
		},
	)
	if err != nil {
		t.Fatalf("fetchWithFallback blocked->ok: %v", err)
	}
	if page.Title != "Unlocked" {
		t.Errorf("page.Title = %q, want Unlocked", page.Title)
	}

	// Both fail -> errors.Join preserves both sentinels
	browserErr := fmt.Errorf("browser: %w", gosearch.ErrBlocked)
	_, err = e.fetchWithFallback(
		context.Background(),
		"https://walled.example.com/",
		func(_ context.Context, _ string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return nil, fmt.Errorf("http: %w", gosearch.ErrChallenge)
		},
		func(_ context.Context, _ string) (*gosearch.Page, error) {
			return nil, browserErr
		},
	)
	if !errors.Is(err, gosearch.ErrChallenge) || !errors.Is(err, gosearch.ErrBlocked) {
		t.Errorf("dual failure error = %v, want both ErrChallenge and ErrBlocked joined", err)
	}
}

func TestFetchWithFallbackDoesNotFallbackOnNetworkErrorOrCancel(t *testing.T) {
	e := newTestStubEngine(t)
	netErr := errors.New("dial tcp: no such host")
	browserCalled := false

	_, err := e.fetchWithFallback(
		context.Background(),
		"https://missing.invalid/",
		func(_ context.Context, _ string, _ ...gosearch.Option) (*gosearch.Page, error) {
			return nil, netErr
		},
		func(_ context.Context, _ string) (*gosearch.Page, error) {
			browserCalled = true
			return nil, nil
		},
	)
	if !errors.Is(err, netErr) {
		t.Errorf("err = %v, want netErr", err)
	}
	if browserCalled {
		t.Fatal("browser fallback should not run on non-block network error")
	}

	// Closed engine returns ErrEngineClosed immediately
	_ = e.Close()
	if _, err := e.FetchWithFallback(context.Background(), "https://example.com/"); !errors.Is(err, ErrEngineClosed) {
		t.Errorf("closed FetchWithFallback = %v, want ErrEngineClosed", err)
	}
}

func TestSearchWithFallbackContract(t *testing.T) {
	e := newTestStubEngine(t)

	// 1. HTTP success -> browser never called
	browserCalled := false
	res, err := e.searchWithFallback(
		context.Background(),
		"golang",
		gosearch.DuckDuckGo,
		func(_ context.Context, _ string, _ gosearch.Engine, _ ...gosearch.Option) ([]gosearch.Result, error) {
			return []gosearch.Result{{Title: "Go", URL: "https://go.dev"}}, nil
		},
		func(_ context.Context, _ string) ([]gosearch.Result, error) {
			browserCalled = true
			return nil, nil
		},
	)
	if err != nil || len(res) != 1 || browserCalled {
		t.Fatalf("HTTP success case failed: res=%+v err=%v browserCalled=%v", res, err, browserCalled)
	}

	// 2. HTTP ErrNoResults -> MUST NOT trigger browser fallback
	browserCalled = false
	_, err = e.searchWithFallback(
		context.Background(),
		"xyzzy-nothing",
		gosearch.DuckDuckGo,
		func(_ context.Context, _ string, _ gosearch.Engine, _ ...gosearch.Option) ([]gosearch.Result, error) {
			return nil, fmt.Errorf("ddg: %w", gosearch.ErrNoResults)
		},
		func(_ context.Context, _ string) ([]gosearch.Result, error) {
			browserCalled = true
			return []gosearch.Result{{Title: "Should not run"}}, nil
		},
	)
	if !errors.Is(err, gosearch.ErrNoResults) || browserCalled {
		t.Fatalf("ErrNoResults must not trigger fallback: err=%v browserCalled=%v", err, browserCalled)
	}

	// 3. HTTP ErrBlocked -> falls back to browser
	browserCalled = false
	res, err = e.searchWithFallback(
		context.Background(),
		"golang",
		gosearch.Google,
		func(_ context.Context, _ string, _ gosearch.Engine, _ ...gosearch.Option) ([]gosearch.Result, error) {
			return nil, fmt.Errorf("google: %w", gosearch.ErrChallenge)
		},
		func(_ context.Context, _ string) ([]gosearch.Result, error) {
			browserCalled = true
			return []gosearch.Result{{Title: "Go (Browser)", URL: "https://go.dev"}}, nil
		},
	)
	if err != nil || !browserCalled || len(res) != 1 || res[0].Title != "Go (Browser)" {
		t.Fatalf("ErrChallenge fallback failed: res=%+v err=%v browserCalled=%v", res, err, browserCalled)
	}
}
