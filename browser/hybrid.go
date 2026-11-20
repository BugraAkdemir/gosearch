package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/BugraAkdemir/gosearch"
)

// defaultFallbackMinContent is the default rune threshold below which
// FetchWithFallback treats a plain-HTTP extraction as a likely unrendered JS
// shell and falls back to the real browser.
const defaultFallbackMinContent = 100

type (
	httpFetchFunc     func(ctx context.Context, rawURL string, opts ...gosearch.Option) (*gosearch.Page, error)
	browserFetchFunc  func(ctx context.Context, rawURL string) (*gosearch.Page, error)
	httpSearchFunc    func(ctx context.Context, query string, engine gosearch.Engine, opts ...gosearch.Option) ([]gosearch.Result, error)
	browserSearchFunc func(ctx context.Context, query string) ([]gosearch.Result, error)
)

// FetchWithFallback tries the fast, zero-browser gosearch.Fetch first and
// falls back to Engine.Fetch only when needed:
//
//   - Plain HTTP returns gosearch.ErrBlocked or gosearch.ErrChallenge.
//   - Plain HTTP succeeds, but the extracted content looks like an unrendered
//     JS shell (empty, or fewer trimmed runes than WithFallbackMinContent —
//     default 100).
//
// When plain HTTP already extracts sufficient content, Chromium is never even
// started (zero browser RAM/startup cost on static pages). Engine-level
// WithMarkdown, WithProxy, and WithTimeout are automatically forwarded to the
// HTTP attempt ahead of caller-supplied opts so explicit opts still win.
//
// Caller context cancellation and invalid URLs never trigger fallback. If the
// HTTP attempt returned thin (non-empty) content and the browser fallback
// fails, the thin HTTP page is returned as graceful degradation; if HTTP was
// blocked/empty and the browser also fails, errors.Join of both errors is
// returned (or the browser error when HTTP had empty content).
func (e *Engine) FetchWithFallback(ctx context.Context, rawURL string, opts ...gosearch.Option) (*gosearch.Page, error) {
	return e.fetchWithFallback(ctx, rawURL, gosearch.Fetch, e.Fetch, opts...)
}

func (e *Engine) fetchWithFallback(
	ctx context.Context,
	rawURL string,
	httpFetch httpFetchFunc,
	browserFetch browserFetchFunc,
	opts ...gosearch.Option,
) (*gosearch.Page, error) {
	if e.isClosed() {
		return nil, fmt.Errorf("browser: fetch fallback: %w", ErrEngineClosed)
	}
	if !validHTTPOrHTTPS(rawURL) {
		return nil, fmt.Errorf("browser: fetch %q: only http(s) URLs are supported", rawURL)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("browser: fetch fallback: %w", err)
	}

	coreOpts := e.buildCoreFetchOpts(opts)
	httpPage, httpErr := httpFetch(ctx, rawURL, coreOpts...)
	if httpErr == nil && httpPage != nil {
		trimmedLen := utf8.RuneCountInString(strings.TrimSpace(httpPage.Content))
		if !needsContentFallback(trimmedLen, e.fallbackMinContent) {
			return httpPage, nil
		}
		if ctx.Err() != nil {
			return httpPage, nil
		}
		bPage, bErr := browserFetch(ctx, rawURL)
		if bErr == nil && bPage != nil {
			if strings.TrimSpace(bPage.Content) != "" || trimmedLen == 0 {
				return bPage, nil
			}
			return httpPage, nil
		}
		if trimmedLen > 0 && ctx.Err() == nil {
			return httpPage, nil
		}
		return nil, bErr
	}

	if ctx.Err() != nil {
		return nil, httpErr
	}
	if !errors.Is(httpErr, gosearch.ErrBlocked) && !errors.Is(httpErr, gosearch.ErrChallenge) {
		return nil, httpErr
	}

	bPage, bErr := browserFetch(ctx, rawURL)
	if bErr == nil {
		return bPage, nil
	}
	return nil, errors.Join(httpErr, bErr)
}

// SearchWithFallback tries plain-HTTP gosearch.Search against primary first
// and falls back to Engine.Search ONLY when plain HTTP returns
// gosearch.ErrBlocked or gosearch.ErrChallenge.
//
// Matching core gosearch's fallback contract, gosearch.ErrNoResults is a valid
// empty answer and does NOT trigger browser fallback. Engine-level proxy,
// timeout, maxResults, and domain filters are forwarded ahead of caller opts.
func (e *Engine) SearchWithFallback(ctx context.Context, query string, primary gosearch.Engine, opts ...gosearch.Option) ([]gosearch.Result, error) {
	return e.searchWithFallback(ctx, query, primary, gosearch.Search, e.Search, opts...)
}

func (e *Engine) searchWithFallback(
	ctx context.Context,
	query string,
	primary gosearch.Engine,
	httpSearch httpSearchFunc,
	browserSearch browserSearchFunc,
	opts ...gosearch.Option,
) ([]gosearch.Result, error) {
	if e.isClosed() {
		return nil, fmt.Errorf("browser: search fallback: %w", ErrEngineClosed)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("browser: search fallback: %w", err)
	}

	coreOpts := e.buildCoreSearchOpts(opts)
	results, httpErr := httpSearch(ctx, query, primary, coreOpts...)
	if httpErr == nil {
		return results, nil
	}
	if ctx.Err() != nil {
		return nil, httpErr
	}
	if !errors.Is(httpErr, gosearch.ErrBlocked) && !errors.Is(httpErr, gosearch.ErrChallenge) {
		return nil, httpErr
	}

	bResults, bErr := browserSearch(ctx, query)
	if bErr == nil {
		return bResults, nil
	}
	return nil, errors.Join(httpErr, bErr)
}

func needsContentFallback(trimmedRunes, minContent int) bool {
	if minContent <= 0 {
		return trimmedRunes == 0
	}
	return trimmedRunes < minContent
}

func (e *Engine) buildCoreFetchOpts(extra []gosearch.Option) []gosearch.Option {
	var out []gosearch.Option
	if e.markdown {
		out = append(out, gosearch.WithMarkdown())
	}
	if e.proxyRawURL != "" {
		out = append(out, gosearch.WithProxy(e.proxyRawURL))
	}
	if e.timeout > 0 {
		out = append(out, gosearch.WithTimeout(e.timeout))
	}
	return append(out, extra...)
}

func (e *Engine) buildCoreSearchOpts(extra []gosearch.Option) []gosearch.Option {
	var out []gosearch.Option
	if e.proxyRawURL != "" {
		out = append(out, gosearch.WithProxy(e.proxyRawURL))
	}
	if e.timeout > 0 {
		out = append(out, gosearch.WithTimeout(e.timeout))
	}
	if e.maxResults > 0 {
		out = append(out, gosearch.WithMaxResults(e.maxResults))
	}
	if len(e.allowedDomains) > 0 {
		out = append(out, gosearch.WithAllowedDomains(e.allowedDomains...))
	}
	if len(e.blockedDomains) > 0 {
		out = append(out, gosearch.WithBlockedDomains(e.blockedDomains...))
	}
	return append(out, extra...)
}
