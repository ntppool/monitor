package monitorsettings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Querier is the minimum DB surface the Watcher needs. Both ntpdb.Queries
// and ntpdb.QuerierTx satisfy it.
type Querier interface {
	GetSystemSetting(ctx context.Context, key string) (string, error)
}

// SettingsKey is the system_settings row key that holds the JSON payload.
const SettingsKey = "monitors"

// RefreshInterval is the nominal cadence for background re-reads of the
// system_settings row. The actual sleep is jittered up to RefreshJitter.
const (
	RefreshInterval = time.Hour
	RefreshJitter   = 10 * time.Minute
)

// Watcher caches MonitorSettings and refreshes them from system_settings
// on a background ticker. Concurrent forced refreshes are coalesced via
// singleflight.
type Watcher struct {
	q   Querier
	log *slog.Logger

	mu       sync.RWMutex
	current  MonitorSettings
	loadedAt time.Time

	sf singleflight.Group
}

// NewWatcher loads settings synchronously once, starts a background refresh
// goroutine, and returns the watcher. Returns an error only if the initial
// load fails in a way that isn't recoverable by falling back to defaults.
func NewWatcher(ctx context.Context, q Querier, log *slog.Logger) (*Watcher, error) {
	if log == nil {
		log = slog.Default()
	}
	w := &Watcher{q: q, log: log}
	if err := w.refresh(ctx, true); err != nil {
		return nil, fmt.Errorf("initial monitor settings load: %w", err)
	}
	go w.run(ctx)
	return w, nil
}

// Current returns a snapshot of the cached settings (with defaults applied).
func (w *Watcher) Current() MonitorSettings {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.current
}

// LoadedAt returns the time of the most recent successful refresh.
func (w *Watcher) LoadedAt() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.loadedAt
}

// Refresh forces an immediate re-read. Concurrent callers share a single
// DB query via singleflight.
func (w *Watcher) Refresh(ctx context.Context) error {
	_, err, _ := w.sf.Do("refresh", func() (any, error) {
		return nil, w.refresh(ctx, false)
	})
	return err
}

func (w *Watcher) refresh(ctx context.Context, initial bool) error {
	raw, err := w.q.GetSystemSetting(ctx, SettingsKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var parsed MonitorSettings
	if raw != "" {
		if jerr := json.Unmarshal([]byte(raw), &parsed); jerr != nil {
			w.log.WarnContext(
				ctx, "could not parse monitor settings; using defaults",
				slog.String("err", jerr.Error()),
				slog.String("raw", raw),
			)
			parsed = MonitorSettings{}
		}
	}
	fresh := parsed.WithDefaults()

	w.mu.Lock()
	prev := w.current
	w.current = fresh
	w.loadedAt = time.Now()
	w.mu.Unlock()

	switch {
	case initial:
		w.logSettings(ctx, slog.LevelInfo, "monitor settings loaded", fresh)
	case fresh != prev:
		w.logChange(ctx, prev, fresh)
	}
	return nil
}

func (w *Watcher) logSettings(ctx context.Context, level slog.Level, msg string, s MonitorSettings) {
	maxSamples := MaxSamplesInWindow(s.IntervalTesting.Duration)
	thresholdActive := RequiredCountForActive(s.IntervalTesting.Duration)
	thresholdTesting := RequiredCountForTesting()

	w.log.Log(
		ctx, level, msg,
		slog.Duration("interval_active", s.IntervalActive.Duration),
		slog.Duration("interval_testing", s.IntervalTesting.Duration),
		slog.Duration("interval_all", s.IntervalAll.Duration),
		slog.Int("batch_size", int(s.BatchSize)),
		slog.Int64("max_samples_24h", maxSamples),
		slog.Int64("min_count_for_active", thresholdActive),
		slog.Int64("min_count_for_testing", thresholdTesting),
	)

	if maxSamples < HistoricalMinForActive {
		w.log.WarnContext(
			ctx,
			"interval_testing too long for historical 32-sample threshold; using degraded dynamic threshold",
			slog.Duration("interval_testing", s.IntervalTesting.Duration),
			slog.Int64("max_samples_24h", maxSamples),
			slog.Int64("dynamic_threshold", thresholdActive),
		)
	}
}

func (w *Watcher) logChange(ctx context.Context, prev, fresh MonitorSettings) {
	w.log.InfoContext(
		ctx, "monitor settings changed",
		slog.Duration("prev_interval_active", prev.IntervalActive.Duration),
		slog.Duration("new_interval_active", fresh.IntervalActive.Duration),
		slog.Duration("prev_interval_testing", prev.IntervalTesting.Duration),
		slog.Duration("new_interval_testing", fresh.IntervalTesting.Duration),
		slog.Duration("prev_interval_all", prev.IntervalAll.Duration),
		slog.Duration("new_interval_all", fresh.IntervalAll.Duration),
		slog.Int("prev_batch_size", int(prev.BatchSize)),
		slog.Int("new_batch_size", int(fresh.BatchSize)),
	)
	w.logSettings(ctx, slog.LevelInfo, "monitor settings (post-change)", fresh)
}

func (w *Watcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(nextDelay()):
		}
		if err := w.Refresh(ctx); err != nil {
			w.log.WarnContext(
				ctx, "monitor settings refresh failed",
				slog.String("err", err.Error()),
			)
		}
	}
}

func nextDelay() time.Duration {
	if RefreshJitter <= 0 {
		return RefreshInterval
	}
	return RefreshInterval + time.Duration(rand.Int64N(int64(RefreshJitter)))
}
