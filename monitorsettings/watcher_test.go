package monitorsettings

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jackc/pgx/v5"

	shared "go.ntppool.org/common/monitorsettings"
)

type fakeQuerier struct {
	mu    sync.Mutex
	value string
	err   error
	calls atomic.Int64
	// gate, when non-nil, blocks GetSystemSetting until it is closed.
	gate chan struct{}
}

func (f *fakeQuerier) GetSystemSetting(ctx context.Context, key string) ([]byte, error) {
	f.calls.Add(1)
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.value), nil
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
	if got.IntervalTesting.Duration != shared.DefaultIntervalTesting {
		t.Errorf("IntervalTesting: got %v, want default %v", got.IntervalTesting.Duration, shared.DefaultIntervalTesting)
	}
	if got.BatchSize != shared.DefaultBatchSize {
		t.Errorf("BatchSize: got %d, want default %d", got.BatchSize, shared.DefaultBatchSize)
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
	if got.IntervalTesting.Duration != shared.DefaultIntervalTesting {
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
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		q := &fakeQuerier{value: `{"interval_testing":"30m"}`}
		w, err := NewWatcher(ctx, q, discardLogger())
		if err != nil {
			t.Fatalf("NewWatcher: %v", err)
		}
		baseline := q.calls.Load()

		// Hold the query open so all callers overlap with the first
		// in-flight refresh. synctest.Wait returns once every goroutine
		// is parked: one in the gated query, the rest in singleflight.
		gate := make(chan struct{})
		q.mu.Lock()
		q.gate = gate
		q.mu.Unlock()

		const n = 50
		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				if err := w.Refresh(ctx); err != nil {
					t.Errorf("Refresh: %v", err)
				}
			})
		}
		synctest.Wait()
		close(gate)
		wg.Wait()

		if delta := q.calls.Load() - baseline; delta != 1 {
			t.Errorf("underlying calls: got %d, want 1 for %d concurrent refreshes", delta, n)
		}
	})
}

// problemLog records the "monitor settings problem" log lines.
type problemLog struct {
	mu     sync.Mutex
	levels []slog.Level
}

func (p *problemLog) Enabled(context.Context, slog.Level) bool { return true }
func (p *problemLog) WithAttrs([]slog.Attr) slog.Handler       { return p }
func (p *problemLog) WithGroup(string) slog.Handler            { return p }

func (p *problemLog) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "monitor settings problem" {
		p.mu.Lock()
		p.levels = append(p.levels, r.Level)
		p.mu.Unlock()
	}
	return nil
}

// take returns the levels logged since the last call.
func (p *problemLog) take() []slog.Level {
	p.mu.Lock()
	defer p.mu.Unlock()
	l := p.levels
	p.levels = nil
	return l
}

// The watcher logs the Check findings when it loads the settings and when
// they change, not on every refresh.
func TestWatcherLogsProblems(t *testing.T) {
	ctx := t.Context()
	logged := &problemLog{}

	q := &fakeQuerier{value: validSettings}
	w, err := NewWatcher(ctx, q, slog.New(logged))
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	steps := []struct {
		name string
		raw  []byte
		want []slog.Level
	}{
		{"valid settings", nil, nil},
		{"trial_every under the minimum", withKey(t, "trial_every", `"30m"`), []slog.Level{slog.LevelError}},
		{"unchanged", nil, nil},
		{"selector key missing", withKey(t, "review_every", ""), []slog.Level{slog.LevelWarn}},
		{"fixed", []byte(validSettings), nil},
	}
	for i, st := range steps {
		if i > 0 {
			if st.raw != nil {
				q.set(string(st.raw))
			}
			if err := w.Refresh(ctx); err != nil {
				t.Fatalf("%s: Refresh: %v", st.name, err)
			}
		}
		if got := logged.take(); !slices.Equal(got, st.want) {
			t.Errorf("%s: logged %v, want %v", st.name, got, st.want)
		}
	}
}
