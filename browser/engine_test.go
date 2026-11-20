package browser

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/BugraAkdemir/gosearch"
)

func TestPageDiagnosticsNilAndCanceledSafe(t *testing.T) {
	if got := pageDiagnostics(context.Background(), nil, 200*time.Millisecond); got != "" {
		t.Errorf("pageDiagnostics(nil) = %q, want empty", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := pageDiagnostics(context.Background(), ctx, 200*time.Millisecond); got != "" {
		t.Errorf("pageDiagnostics(canceled tab) = %q, want empty", got)
	}
	if got := pageDiagnostics(ctx, context.Background(), 200*time.Millisecond); got != "" {
		t.Errorf("pageDiagnostics(canceled caller) = %q, want empty", got)
	}
	if got := diagTimeoutFor(30 * time.Second); got != 500*time.Millisecond {
		t.Errorf("diagTimeoutFor(30s) = %v, want 500ms", got)
	}
	if got := diagTimeoutFor(400 * time.Millisecond); got != 200*time.Millisecond {
		t.Errorf("diagTimeoutFor(400ms) = %v, want 200ms", got)
	}
	if got := diagTimeoutFor(50 * time.Millisecond); got != 100*time.Millisecond {
		t.Errorf("diagTimeoutFor(50ms) = %v, want 100ms floor", got)
	}
}

func TestInvalidExecutableDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	badBin := filepath.Join(t.TempDir(), "nonexistent-chromium-bin")
	e, err := New(ctx, WithExecutable(badBin))
	if err != nil {
		t.Fatalf("New(WithExecutable): %v", err)
	}
	defer func() { _ = e.Close() }()

	if _, err := e.Fetch(ctx, "https://example.com/"); err == nil {
		t.Fatal("Fetch with invalid executable succeeded, want error")
	}
	if _, err := e.Search(ctx, "golang"); err == nil {
		t.Fatal("Search with invalid executable succeeded, want error")
	}
}

func TestEngineClosedReturnsErrEngineClosed(t *testing.T) {
	ctx := context.Background()
	badBin := filepath.Join(t.TempDir(), "unused-bin")
	e, err := New(ctx, WithExecutable(badBin))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if _, err := e.Fetch(ctx, "https://example.com/"); !errors.Is(err, ErrEngineClosed) {
		t.Errorf("Fetch after Close = %v, want ErrEngineClosed", err)
	}
	if _, err := e.Search(ctx, "test"); !errors.Is(err, ErrEngineClosed) {
		t.Errorf("Search after Close = %v, want ErrEngineClosed", err)
	}
}

func TestCanceledContextShortCircuitsBeforeStart(t *testing.T) {
	badBin := filepath.Join(t.TempDir(), "unused-bin")
	e, err := New(context.Background(), WithExecutable(badBin))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = e.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := e.Fetch(ctx, "https://example.com/"); !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch(canceled) = %v, want context.Canceled", err)
	}
	if _, err := e.Search(ctx, "test"); !errors.Is(err, context.Canceled) {
		t.Errorf("Search(canceled) = %v, want context.Canceled", err)
	}
}

func TestClassifyRenderErrorPreservesSentinels(t *testing.T) {
	if err := classifyRenderError("fetch", context.Canceled, ""); !errors.Is(err, context.Canceled) || errors.Is(err, gosearch.ErrChallenge) {
		t.Errorf("context.Canceled misclassified: %v", err)
	}
	if err := classifyRenderError("fetch", ErrEngineClosed, ""); !errors.Is(err, ErrEngineClosed) || errors.Is(err, gosearch.ErrChallenge) {
		t.Errorf("ErrEngineClosed misclassified: %v", err)
	}
	deadlineErr := classifyRenderError("search", context.DeadlineExceeded, "https://www.google.com/sorry")
	if !errors.Is(deadlineErr, gosearch.ErrChallenge) {
		t.Errorf("deadlineErr = %v, want ErrChallenge", deadlineErr)
	}
	if !errors.Is(deadlineErr, context.DeadlineExceeded) {
		t.Errorf("deadlineErr = %v, want context.DeadlineExceeded preserved", deadlineErr)
	}
}

func TestIsBrowserDead(t *testing.T) {
	alive := context.Background()
	dead, cancel := context.WithCancel(context.Background())
	cancel()

	if !isBrowserDead(dead, alive, nil) {
		t.Error("expected canceled allocCtx to report dead")
	}
	if !isBrowserDead(alive, dead, nil) {
		t.Error("expected canceled rootTabCtx to report dead")
	}
	if !isBrowserDead(alive, alive, errors.New("websocket: close 1006 unexpected EOF")) {
		t.Error("expected websocket EOF error to report dead")
	}
	if isBrowserDead(alive, alive, context.DeadlineExceeded) {
		t.Error("expected plain page timeout on healthy browser NOT to report dead")
	}
}

func TestWithMaxConcurrencyClampsToAtLeastOne(t *testing.T) {
	cfg := &engineConfig{}
	WithMaxConcurrency(0)(cfg)
	if cfg.maxConcurrency != 1 {
		t.Errorf("maxConcurrency(0) = %d, want 1", cfg.maxConcurrency)
	}
	WithMaxConcurrency(-3)(cfg)
	if cfg.maxConcurrency != 1 {
		t.Errorf("maxConcurrency(-3) = %d, want 1", cfg.maxConcurrency)
	}
	WithMaxConcurrency(4)(cfg)
	if cfg.maxConcurrency != 4 {
		t.Errorf("maxConcurrency(4) = %d, want 4", cfg.maxConcurrency)
	}
}
