package monitorsettings

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fakeQuerier struct {
	mu    sync.Mutex
	value string
	err   error
	calls atomic.Int64
}

func (f *fakeQuerier) GetSystemSetting(ctx context.Context, key string) (string, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

func (f *fakeQuerier) set(v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.value = v
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWatcherInitialLoad_Empty(t *testing.T) {
	ctx := t.Context()

	q := &fakeQuerier{} // empty value, no error → defaults
	w, err := NewWatcher(ctx, q, discardLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	got := w.Current()
	if got.IntervalTesting.Duration != DefaultIntervalTesting {
		t.Errorf("IntervalTesting: got %v, want default %v", got.IntervalTesting.Duration, DefaultIntervalTesting)
	}
	if got.BatchSize != DefaultBatchSize {
		t.Errorf("BatchSize: got %d, want default %d", got.BatchSize, DefaultBatchSize)
	}
}

func TestWatcherInitialLoad_NoRows(t *testing.T) {
	ctx := t.Context()

	q := &fakeQuerier{err: pgx.ErrNoRows}
	w, err := NewWatcher(ctx, q, discardLogger())
	if err != nil {
		t.Fatalf("NewWatcher should tolerate ErrNoRows: %v", err)
	}
	got := w.Current()
	if got.IntervalTesting.Duration != DefaultIntervalTesting {
		t.Errorf("expected defaults, got %+v", got)
	}
}

func TestWatcherParsesJSON(t *testing.T) {
	ctx := t.Context()

	q := &fakeQuerier{value: `{"interval_active":"7m","interval_testing":"25m","interval_all":"40s","batch_size":60}`}
	w, err := NewWatcher(ctx, q, discardLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	got := w.Current()
	if got.IntervalActive.Duration != 7*time.Minute {
		t.Errorf("IntervalActive: got %v, want 7m", got.IntervalActive.Duration)
	}
	if got.IntervalTesting.Duration != 25*time.Minute {
		t.Errorf("IntervalTesting: got %v, want 25m", got.IntervalTesting.Duration)
	}
	if got.IntervalAll.Duration != 40*time.Second {
		t.Errorf("IntervalAll: got %v, want 40s", got.IntervalAll.Duration)
	}
	if got.BatchSize != 60 {
		t.Errorf("BatchSize: got %d, want 60", got.BatchSize)
	}
}

func TestWatcherRefreshDetectsChange(t *testing.T) {
	ctx := t.Context()

	q := &fakeQuerier{value: `{"interval_testing":"30m"}`}
	w, err := NewWatcher(ctx, q, discardLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if w.Current().IntervalTesting.Duration != 30*time.Minute {
		t.Fatalf("initial IntervalTesting wrong")
	}

	q.set(`{"interval_testing":"20m"}`)
	if err := w.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := w.Current().IntervalTesting.Duration; got != 20*time.Minute {
		t.Errorf("after refresh IntervalTesting: got %v, want 20m", got)
	}
}

func TestWatcherRefreshCoalesces(t *testing.T) {
	ctx := t.Context()

	q := &fakeQuerier{value: `{"interval_testing":"30m"}`}
	w, err := NewWatcher(ctx, q, discardLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	baseline := q.calls.Load()

	// Fire many concurrent forced refreshes; singleflight should
	// coalesce at least some of them. We don't assert a tight bound
	// because singleflight only deduplicates calls that overlap with an
	// in-flight call, and with an instant fake DB the window is small.
	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.Refresh(ctx)
		}()
	}
	wg.Wait()

	delta := q.calls.Load() - baseline
	if delta == 0 {
		t.Errorf("expected at least one underlying call after concurrent refreshes")
	}
	if delta >= n {
		t.Errorf("expected singleflight to coalesce some calls; saw %d/%d", delta, n)
	}
}
