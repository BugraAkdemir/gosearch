// Package browser is the OPTIONAL real-browser engine for gosearch: it
// drives an unmodified Chromium-family browser (Chrome, Edge, Chromium, or
// Google's chrome-headless-shell) over CDP to run searches and extract page
// content where plain HTTP cannot — most notably pages that only render or
// unlock behind JavaScript.
//
// It is a separate Go module on purpose: depending on it pulls in chromedp
// and, potentially, a ~100–300 MB browser runtime. The core gosearch module
// stays dependency-light; nothing here is imported unless you ask for it.
//
// Honest limitations (the line this project will not cross): the browser is
// driven UNMODIFIED — no stealth patches, no navigator.webdriver masking, no
// fingerprint spoofing. That means it clears JavaScript-gated pages but does
// NOT defeat IP-reputation blocks or interactive CAPTCHAs, and automated-
// browser signals remain detectable. When an engine still refuses to serve
// results, this package reports ErrChallenge/ErrBlocked-wrapped errors like
// the rest of gosearch instead of pretending otherwise.
package browser

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Engine is one long-lived, lazily started browser instance shared across
// calls: a single process with a single tab keeps steady-state memory at
// roughly one page's worth instead of paying full startup per request.
type Engine struct {
	mu            sync.Mutex
	ctx           context.Context // chromedp tab context, set on first start
	stop          context.CancelFunc
	stopAllocator context.CancelFunc
	profileDir    string
	executable    string
	startOnce     sync.Once
	startErr      error
	closeOnce     sync.Once
}

// New resolves which executable to drive (explicit path > embedded archive >
// system discovery > opt-in download) and returns an Engine. The browser
// process starts lazily on first use, not in New, so constructing one never
// pays startup cost. Call Close when done.
func New(ctx context.Context, opts ...Option) (*Engine, error) {
	cfg := &engineConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	exe, err := resolveExecutable(ctx, cfg)
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "gosearch-browser-")
	if err != nil {
		return nil, fmt.Errorf("browser: create profile dir: %w", err)
	}
	return &Engine{executable: exe, profileDir: profile}, nil
}

// Executable reports the resolved chromium-family binary path. Valid before
// first use because resolution happens in New.
func (e *Engine) Executable() string { return e.executable }

// start launches the browser exactly once. The internal lifetime is
// deliberately detached from the caller's New-context so an Engine can
// outlive it; Close ends everything.
func (e *Engine) start(parent context.Context) error {
	e.startOnce.Do(func() {
		lifetime, stopLifetime := context.WithCancel(context.WithoutCancel(parent))
		e.stop = stopLifetime

		allocCtx, stopAllocator := chromedp.NewExecAllocator(lifetime, allocatorFlags(e.profileDir)...)
		e.stopAllocator = stopAllocator
		tabCtx, _ := chromedp.NewContext(allocCtx)
		if err := chromedp.Run(tabCtx); err != nil {
			stopAllocator()
			stopLifetime()
			e.startErr = fmt.Errorf("browser: start %s: %w", e.executable, err)
			return
		}
		e.mu.Lock()
		e.ctx = tabCtx
		e.mu.Unlock()
	})
	return e.startErr
}

// run executes actions on the shared tab under the given per-call timeout.
func (e *Engine) run(ctx context.Context, timeout time.Duration, actions ...chromedp.Action) error {
	if err := e.start(ctx); err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(e.ctx, timeout)
	defer cancel()
	return chromedp.Run(callCtx, actions...)
}

// Close shuts the browser down and removes the temporary profile directory.
// It is idempotent; calling methods on a closed Engine surfaces the original
// startup error or a canceled-context error from chromedp.
func (e *Engine) Close() error {
	e.closeOnce.Do(func() {
		if e.stopAllocator != nil {
			e.stopAllocator()
		}
		if e.stop != nil {
			e.stop()
		}
		_ = os.RemoveAll(e.profileDir)
	})
	return nil
}
