package gosearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/BugraAkdemir/gosearch/internal/httpclient"
	"github.com/BugraAkdemir/gosearch/internal/provider"
	"github.com/BugraAkdemir/gosearch/internal/providers/bing"
	"github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo"
	"github.com/BugraAkdemir/gosearch/internal/providers/google"
	"github.com/BugraAkdemir/gosearch/internal/providers/yandex"
	"github.com/BugraAkdemir/gosearch/internal/readability"
	"github.com/BugraAkdemir/gosearch/internal/serrors"
)

// Search queries the given engine for query and returns the parsed results.
//
// If WithFallback supplied additional engines, they are tried, in order, only
// when the current engine returns ErrBlocked or ErrChallenge; the first engine
// that succeeds wins. Fallback is NOT triggered by ErrNoResults (an empty
// result set is a valid answer) nor by other errors (network failures, etc.),
// which are returned immediately. If every engine in the chain is
// blocked/challenged, Search returns the errors.Join of each engine's error, so
// errors.Is(err, ErrBlocked) / errors.Is(err, ErrChallenge) still report true.
//
// The same underlying HTTP client (with its cookie jar and rate limiter) is
// reused across the fallback chain.
//
// All four engines are implemented. The DuckDuckGo parser is validated
// against a real captured success page; the Google and Yandex parsers are
// best-effort heuristics written against documented markup until a real
// capture lands — see plan.md's exit criteria and AGENTS.md Known Pitfalls.
// Bing served clean organic results even to a flagged datacenter IP during
// the 2026-08-24 probe, but its parser is likewise best-effort until a real
// capture lands.
func Search(ctx context.Context, query string, engine Engine, opts ...Option) ([]Result, error) {
	if !engine.valid() {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedEngine, int(engine))
	}
	cfg := apply(opts)

	client, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	engines := append([]Engine{engine}, cfg.fallback...)
	var errs []error
	for _, e := range engines {
		results, err := dispatch(ctx, e, client, query, cfg.maxResults)
		if err == nil {
			return toResults(results), nil
		}
		errs = append(errs, err)
		// Only a block/challenge is worth trying the next engine for.
		if errors.Is(err, serrors.ErrBlocked) || errors.Is(err, serrors.ErrChallenge) {
			continue
		}
		// Any other error (no results, network) is final.
		return nil, err
	}
	return nil, errors.Join(errs...)
}

// dispatch routes a search to the provider package for e. It is a package var
// so tests can substitute fake providers to exercise the fallback chain without
// hitting the network.
var dispatch = func(ctx context.Context, e Engine, client *httpclient.Client, query string, maxResults int) ([]provider.Result, error) {
	switch e {
	case DuckDuckGo:
		return duckduckgo.Search(ctx, client, query, maxResults)
	case Google:
		return google.Search(ctx, client, query, maxResults)
	case Yandex:
		return yandex.Search(ctx, client, query, maxResults)
	case Bing:
		return bing.Search(ctx, client, query, maxResults)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedEngine, int(e))
	}
}

// Fetch retrieves url and extracts its main readable content into a Page. It
// does not run JavaScript, so pages whose content is rendered client-side may
// yield an empty Page.Content. If the server responds with an anti-bot page
// (for example HTTP 403/429), Fetch returns ErrBlocked/ErrChallenge.
//
// With Markdown opted in (WithMarkdown), Page.Content carries GitHub-flavored
// Markdown with headings, lists, code fences, links, and emphasis preserved;
// the default remains plain text.
//
// Search-only options (WithFallback, WithMaxResults) are ignored by Fetch.
func Fetch(ctx context.Context, url string, opts ...Option) (*Page, error) {
	cfg := apply(opts)
	client, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("gosearch: fetch %q: %w", url, err)
	}
	if err := httpclient.Detect(resp); err != nil {
		return nil, fmt.Errorf("gosearch: fetch %q: %w", url, err)
	}

	var art *readability.Article
	if cfg.markdown {
		art, err = readability.ExtractMarkdown(resp.Body)
	} else {
		art, err = readability.Extract(resp.Body)
	}
	if err != nil {
		return nil, fmt.Errorf("gosearch: fetch %q: %w", url, err)
	}
	return &Page{URL: resp.FinalURL, Title: art.Title, Content: art.Content}, nil
}

// newHTTPClient builds the shared HTTP client from a resolved config.
func newHTTPClient(cfg *config) (*httpclient.Client, error) {
	return httpclient.New(httpclient.Config{
		Timeout:      cfg.timeout,
		UserAgent:    cfg.userAgent,
		ProxyRawURL:  cfg.proxyRawURL,
		ExtraHeaders: cfg.extraHeaders,
		Cookies:      cfg.cookies,
		HTTPClient:   cfg.httpClient,
		MaxRetries:   cfg.retries,
	})
}

// toResults converts internal provider results to the public Result type.
func toResults(in []provider.Result) []Result {
	out := make([]Result, len(in))
	for i, r := range in {
		out[i] = Result{Title: r.Title, URL: r.URL, Snippet: r.Snippet}
	}
	return out
}
