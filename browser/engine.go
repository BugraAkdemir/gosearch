package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Engine is one long-lived, lazily started browser instance shared across
// calls. By default a single process with a single serialized tab keeps
// steady-state memory at roughly one page's worth instead of paying full
// startup per request; pass WithMaxConcurrency(n) to run up to n isolated tabs
// in parallel inside the same process.
//
// Engine is safe for concurrent use. If the underlying Chromium process exits
// unexpectedly (for example OOM or external termination), the next call — or
// an automatic one-shot retry — restarts it transparently.
type Engine struct {
	mu            sync.Mutex
	allocCtx      context.Context
	tabCtx        context.Context // primary shared tab (also browser owner)
	stopTab       context.CancelFunc
	stopAllocator context.CancelFunc
	stopLifetime  context.CancelFunc
	started       bool
	closed        bool
	warmed        bool
	gen           uint64 // incremented on every start/restart

	closedCh       chan struct{}
	sem            chan struct{}
	maxConcurrency int

	profileDir  string
	executable  string
	userAgent   string
	runHeadless bool
	keepProfile bool
	markdown    bool
	closeOnce   sync.Once
}

// New resolves which executable to drive (explicit path > embedded archive >
// system discovery > cached download > opt-in download) and returns an Engine.
// The browser process starts lazily on first use, not in New, so constructing
// one never pays startup cost. Call Close when done.
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

	headless := !cfg.headlessSeen || cfg.headless
	var profile string
	if cfg.keepProfile {
		if err := os.MkdirAll(cfg.profileDir, 0o755); err != nil {
			return nil, fmt.Errorf("browser: create profile dir: %w", err)
		}
		profile = cfg.profileDir
	} else {
		p, err := os.MkdirTemp("", "gosearch-browser-")
		if err != nil {
			return nil, fmt.Errorf("browser: create profile dir: %w", err)
		}
		profile = p
	}

	maxConc := cfg.maxConcurrency
	if maxConc < 1 {
		maxConc = 1
	}

	return &Engine{
		executable:     exe,
		profileDir:     profile,
		userAgent:      normalizedUserAgent(cfg.userAgent),
		runHeadless:    headless,
		keepProfile:    cfg.keepProfile,
		markdown:       cfg.markdown,
		maxConcurrency: maxConc,
		sem:            make(chan struct{}, maxConc),
		closedCh:       make(chan struct{}),
	}, nil
}

// Executable reports the resolved chromium-family binary path. Valid before
// first use because resolution happens in New.
func (e *Engine) Executable() string { return e.executable }

// isClosed reports whether Close has been called.
func (e *Engine) isClosed() bool {
	e.mu.Lock()
	closed := e.closed
	e.mu.Unlock()
	return closed
}

// acquire claims one concurrency slot while honoring both caller ctx
// cancellation and Engine.Close.
func (e *Engine) acquire(ctx context.Context) error {
	if e.isClosed() {
		return ErrEngineClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case e.sem <- struct{}{}:
		if e.isClosed() {
			<-e.sem
			return ErrEngineClosed
		}
		return nil
	case <-e.closedCh:
		return ErrEngineClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) release() { <-e.sem }

// ensureStarted guarantees a live Chromium process is running and returns its
// allocator/root-tab contexts and generation number. If a previous browser
// process died unexpectedly, stale handles are torn down and a fresh process
// is launched transparently.
func (e *Engine) ensureStarted(ctx context.Context) (allocCtx, rootTabCtx context.Context, gen uint64, err error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, 0, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, nil, 0, ErrEngineClosed
	}
	if e.started && e.allocCtx != nil && e.allocCtx.Err() == nil && e.tabCtx != nil && e.tabCtx.Err() == nil {
		return e.allocCtx, e.tabCtx, e.gen, nil
	}

	// Clean up any dead prior generation before spawning a replacement.
	e.teardownLocked(false)

	lifetime, stopLifetime := context.WithCancel(context.Background())
	newAlloc, stopAllocator := chromedp.NewExecAllocator(lifetime,
		allocatorFlags(e.profileDir, e.executable, e.userAgent, e.runHeadless)...)
	newTab, stopTab := chromedp.NewContext(newAlloc)

	if err := chromedp.Run(newTab); err != nil {
		stopTab()
		stopAllocator()
		stopLifetime()
		return nil, nil, 0, fmt.Errorf("browser: start %s: %w", e.executable, err)
	}

	e.allocCtx = newAlloc
	e.tabCtx = newTab
	e.stopTab = stopTab
	e.stopAllocator = stopAllocator
	e.stopLifetime = stopLifetime
	e.started = true
	e.warmed = false
	e.gen++
	return e.allocCtx, e.tabCtx, e.gen, nil
}

// resetGeneration tears down the browser only if it is still on gen, so
// concurrent callers witnessing the same crash do not reset a newly restarted
// browser a second time.
func (e *Engine) resetGeneration(gen uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || !e.started || e.gen != gen {
		return
	}
	e.teardownLocked(false)
}

// teardownLocked shuts down the active browser handles and clears lifecycle
// state. Caller must hold e.mu. When graceful is true, Browser.close is sent
// first so Chromium flushes profile files cleanly before exit.
func (e *Engine) teardownLocked(graceful bool) {
	tabCtx := e.tabCtx
	stopTab := e.stopTab
	stopAllocator := e.stopAllocator
	stopLifetime := e.stopLifetime

	e.allocCtx = nil
	e.tabCtx = nil
	e.stopTab = nil
	e.stopAllocator = nil
	e.stopLifetime = nil
	e.started = false
	e.warmed = false

	shutdownHandles(tabCtx, stopTab, stopAllocator, stopLifetime, graceful)
}

// shutdownHandles stops a browser generation synchronously so Chromium exits
// before callers inspect or delete profile directories.
func shutdownHandles(tabCtx context.Context, stopTab, stopAllocator, stopLifetime context.CancelFunc, graceful bool) {
	if graceful && tabCtx != nil && tabCtx.Err() == nil {
		cancelCtx, cancelTimeout := context.WithTimeout(tabCtx, 3*time.Second)
		_ = chromedp.Cancel(cancelCtx)
		cancelTimeout()
	}
	if stopTab != nil {
		stopTab()
	}
	if stopAllocator != nil {
		stopAllocator()
	}
	if stopLifetime != nil {
		stopLifetime()
	}
}

// run executes actions under the given per-call timeout while honoring
// concurrency limits, caller context cancellation, and automatic one-shot
// crash recovery. On error it also returns best-effort tab diagnostics
// captured before releasing the tab/slot.
func (e *Engine) run(ctx context.Context, timeout time.Duration, actions ...chromedp.Action) (string, error) {
	if err := e.acquire(ctx); err != nil {
		return "", err
	}
	defer e.release()

	for attempt := range 2 {
		allocCtx, rootTabCtx, gen, err := e.ensureStarted(ctx)
		if err != nil {
			return "", err
		}

		workTab := rootTabCtx
		var closeWorkTab context.CancelFunc
		if e.maxConcurrency > 1 {
			workTab, closeWorkTab = chromedp.NewContext(rootTabCtx)
		}

		callCtx, cancelCall := context.WithTimeout(workTab, timeout)
		stopAfter := context.AfterFunc(ctx, cancelCall)

		runErr := chromedp.Run(callCtx, actions...)

		stopAfter()
		cancelCall()

		if runErr == nil {
			if closeWorkTab != nil {
				closeWorkTab()
			}
			return "", nil
		}

		// Caller canceled or hit caller's own deadline: surface immediately
		// without blaming a crash or wasting time on diagnostics.
		if ctx.Err() != nil {
			if closeWorkTab != nil {
				closeWorkTab()
			}
			return "", ctx.Err()
		}

		if e.isClosed() {
			if closeWorkTab != nil {
				closeWorkTab()
			}
			return "", ErrEngineClosed
		}

		// Browser died mid-call: tear down the dead generation and retry once
		// on a fresh Chromium process.
		if attempt == 0 && isBrowserDead(allocCtx, rootTabCtx, runErr) {
			if closeWorkTab != nil {
				closeWorkTab()
			}
			e.resetGeneration(gen)
			continue
		}

		diag := pageDiagnostics(workTab)
		if closeWorkTab != nil {
			closeWorkTab()
		}
		return diag, runErr
	}
	return "", fmt.Errorf("browser: operation failed after restart")
}

// isBrowserDead reports whether runErr indicates the underlying Chromium
// process or CDP connection died (as opposed to a page load / selector timeout
// on a healthy browser).
func isBrowserDead(allocCtx, rootTabCtx context.Context, runErr error) bool {
	if allocCtx != nil && allocCtx.Err() != nil {
		return true
	}
	if rootTabCtx != nil && rootTabCtx.Err() != nil {
		return true
	}
	if runErr == nil {
		return false
	}
	msg := strings.ToLower(runErr.Error())
	for _, marker := range []string{
		"target closed",
		"session closed",
		"websocket",
		"connection reset",
		"broken pipe",
		"eof",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// warmUp navigates Google's homepage once per browser generation so the
// session picks up ordinary first-visit cookies before the first search — the
// natural human flow (open site, then search). Best-effort: failures are
// ignored and never retried; the search itself reports any real problem.
func (e *Engine) warmUp(ctx context.Context) {
	if e.isClosed() || ctx.Err() != nil {
		return
	}
	e.mu.Lock()
	alreadyWarmed := e.warmed && e.started && e.tabCtx != nil && e.tabCtx.Err() == nil
	e.mu.Unlock()
	if alreadyWarmed {
		return
	}

	if err := e.acquire(ctx); err != nil {
		return
	}
	defer e.release()

	_, rootTabCtx, gen, err := e.ensureStarted(ctx)
	if err != nil {
		return // Search's own error path reports startup failures
	}

	e.mu.Lock()
	if e.warmed && e.gen == gen {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	workTab := rootTabCtx
	var closeWorkTab context.CancelFunc
	if e.maxConcurrency > 1 {
		workTab, closeWorkTab = chromedp.NewContext(rootTabCtx)
		defer closeWorkTab()
	}

	dctx, cancel := context.WithTimeout(workTab, 15*time.Second)
	stopAfter := context.AfterFunc(ctx, cancel)
	_ = chromedp.Run(dctx,
		chromedp.Navigate("https://www.google.com/"),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
	stopAfter()
	cancel()

	e.mu.Lock()
	if e.gen == gen && !e.closed {
		e.warmed = true
	}
	e.mu.Unlock()
}

// Close shuts the browser down cleanly and waits for Chromium to exit before
// returning. Throwaway profile directories are removed; user-supplied
// persistent ones (WithProfileDir) are kept — that is the whole point of them.
// Idempotent.
func (e *Engine) Close() error {
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.closed = true
		close(e.closedCh)
		tabCtx := e.tabCtx
		stopTab := e.stopTab
		stopAllocator := e.stopAllocator
		stopLifetime := e.stopLifetime
		e.allocCtx = nil
		e.tabCtx = nil
		e.stopTab = nil
		e.stopAllocator = nil
		e.stopLifetime = nil
		e.started = false
		e.warmed = false
		e.mu.Unlock()

		shutdownHandles(tabCtx, stopTab, stopAllocator, stopLifetime, true)

		if !e.keepProfile && e.profileDir != "" {
			removeDirWithRetry(e.profileDir)
		}
	})
	return nil
}

// removeDirWithRetry deletes dir, retrying briefly if Chromium helper
// subprocesses are still closing profile files immediately after exit.
func removeDirWithRetry(dir string) {
	for i := range 5 {
		err := os.RemoveAll(dir)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return
		}
		if i < 4 {
			time.Sleep(25 * time.Millisecond)
		}
	}
}
