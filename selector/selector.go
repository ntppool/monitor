package selector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.ntppool.org/common/database"
	"go.ntppool.org/common/database/pgdb"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
)

// ParentConfig provides configuration from the parent command
type ParentConfig interface {
	GetConfigFile() string
}

// Cmd provides the command structure for CLI integration
type Cmd struct {
	Server   ServerCmd   `cmd:"server" help:"run continously"`
	Run      OnceCmd     `cmd:"once" help:"run once"`
	Simulate SimulateCmd `cmd:"simulate" help:"simulate monitor selection for a server"`
}

type (
	ServerCmd struct {
		MetricsPort int `default:"9000" help:"Metrics server port" flag:"metrics-port"`
	}
	OnceCmd struct {
		ServerID    *int64 `arg:"" optional:"" help:"Server ID to process (if not specified, processes all servers)"`
		MetricsPort int    `default:"9000" help:"Metrics server port" flag:"metrics-port"`
	}
	SimulateCmd struct {
		ServerID int64 `arg:"" help:"Server ID to simulate selection for"`
		Verbose  bool  `flag:"verbose" short:"v" help:"Enable verbose debug logging"`
	}
)

func (cmd ServerCmd) Run(ctx context.Context, parent ParentConfig) error {
	return run(ctx, parent.GetConfigFile(), true, cmd.MetricsPort, nil)
}

func (cmd OnceCmd) Run(ctx context.Context, parent ParentConfig) error {
	return run(ctx, parent.GetConfigFile(), false, cmd.MetricsPort, cmd.ServerID)
}

func (cmd SimulateCmd) Run(ctx context.Context, parent ParentConfig) error {
	log := logger.FromContext(ctx)

	// Set debug level if verbose is enabled
	if cmd.Verbose {
		// Create a new logger with debug level
		debugHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		log = slog.New(debugHandler)
		ctx = logger.NewContext(ctx, log)
	}

	log.InfoContext(ctx, "starting selector simulation",
		"serverID", cmd.ServerID,
		"verbose", cmd.Verbose)

	// Open database connection
	dbconn, err := ntpdb.OpenDB(ctx, parent.GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer dbconn.Close()

	// Create selector instance without metrics (nil)
	sl, err := NewSelector(ctx, dbconn, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create selector: %w", err)
	}

	// Run simulation within read-only transaction
	db := ntpdb.New(dbconn)

	var outcome reviewOutcome
	err = database.WithReadOnlyTransaction(ctx, db, func(ctx context.Context, txDB ntpdb.QuerierTx) error {
		log.InfoContext(ctx, "simulating server processing", "serverID", cmd.ServerID)

		var err error
		outcome, err = sl.ProcessServerSimulation(ctx, txDB, cmd.ServerID)
		if err != nil {
			return fmt.Errorf("simulation failed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.InfoContext(ctx, "simulation completed",
		"serverID", cmd.ServerID,
		"wouldHaveChanged", outcome.Changed)

	if outcome.Changed {
		fmt.Printf("✓ Simulation complete: Changes would be applied for server %d\n", cmd.ServerID)
	} else {
		fmt.Printf("✓ Simulation complete: No changes needed for server %d\n", cmd.ServerID)
	}

	return nil
}

// run executes the selector logic either continuously or once
func run(ctx context.Context, configFile string, continuous bool, metricsPort int, serverID *int64) error {
	log := logger.FromContext(ctx)

	log.InfoContext(ctx, "selector starting", "version", version.Version())

	dbconn, err := ntpdb.OpenDB(ctx, configFile)
	if err != nil {
		return err
	}
	defer dbconn.Close()

	// Create and start metrics server
	metricssrv := metricsserver.New()
	version.RegisterMetric("selector", metricssrv.Registry())
	if err := metricssrv.Registry().Register(pgdb.NewPoolCollector(dbconn)); err != nil {
		return fmt.Errorf("register database pool metrics: %w", err)
	}
	go func() {
		if err := metricssrv.ListenAndServe(ctx, metricsPort); err != nil {
			log.Error("metrics server error", "err", err)
		}
	}()

	// Create metrics with the shared registry
	metrics := NewMetrics(metricssrv.Registry())

	sl, err := NewSelector(ctx, dbconn, log, metrics)
	if err != nil {
		return err
	}

	if continuous {
		go sl.trackMonitorServerCounts(ctx, time.Minute)
	}

	expback := backoff.NewExponentialBackOff()
	expback.InitialInterval = time.Second * 3
	expback.MaxInterval = time.Second * 60

	// Handle single server processing
	if serverID != nil {
		log.InfoContext(ctx, "processing single server", "serverID", *serverID)

		db := ntpdb.New(sl.dbconn)
		var outcome reviewOutcome
		err := database.WithTransaction(ctx, db, func(ctx context.Context, db ntpdb.QuerierTx) error {
			var err error
			outcome, err = sl.processServer(ctx, db, *serverID)
			if err != nil {
				return fmt.Errorf("failed to process server %d: %w", *serverID, err)
			}

			// Update review timestamp like the main Run() method does
			return sl.scheduleNextReview(ctx, db, *serverID, outcome)
		})
		if err != nil {
			return err
		}

		if outcome.Changed {
			log.InfoContext(ctx, "server processing completed with changes", "serverID", *serverID)
		} else {
			log.InfoContext(ctx, "server processing completed without changes", "serverID", *serverID)
		}
		return nil
	}

	for {

		count, err := sl.Run()
		if err != nil {
			return err
		}
		if count > 0 || !continuous {
			log.InfoContext(ctx, "processed servers", "count", count)
		}
		if !continuous {
			break
		}

		if count == 0 {
			sl := expback.NextBackOff()
			time.Sleep(sl)
		} else {
			expback.Reset()
		}

	}

	return nil
}

// Selector manages the monitor selection process
type Selector struct {
	ctx      context.Context
	dbconn   *pgxpool.Pool
	log      *slog.Logger
	metrics  *Metrics
	settings settingsProvider
}

// settingsProvider is the minimal surface Selector needs from the
// monitor-settings cache. Tests can pass nil to fall back to the historical
// constants.
type settingsProvider interface {
	Current() monitorsettings.MonitorSettings
}

// NewSelector creates a new selector instance. The monitor-settings watcher
// enables dynamic sample-count thresholds based on the current
// interval_testing.
func NewSelector(ctx context.Context, dbconn *pgxpool.Pool, log *slog.Logger, metrics *Metrics) (*Selector, error) {
	settings, err := monitorsettings.NewWatcher(ctx, ntpdb.New(dbconn), log)
	if err != nil {
		return nil, fmt.Errorf("monitor settings watcher: %w", err)
	}
	return &Selector{ctx: ctx, dbconn: dbconn, log: log, metrics: metrics, settings: settings}, nil
}

// currentSettings returns the live settings, or the defaults when no
// settings watcher is configured (unit tests).
func (sl *Selector) currentSettings() monitorsettings.MonitorSettings {
	if sl.settings == nil {
		return monitorsettings.MonitorSettings{}.WithDefaults()
	}
	return sl.settings.Current()
}

// sampleWindow returns the trailing window over which log_scores are
// counted.
func (sl *Selector) sampleWindow() time.Duration {
	return sl.currentSettings().SampleWindow.Duration
}

// minCountForActive returns the per-Run sample-count threshold for the
// testing -> active promotion, derived from the current interval_testing
// and sample_window.
func (sl *Selector) minCountForActive() int64 {
	s := sl.currentSettings()
	return monitorsettings.RequiredCountForActive(s.IntervalTesting.Duration, s.SampleWindow.Duration)
}

// minCountForTesting returns the sample-count threshold for the
// candidate -> testing promotion, derived from the current
// interval_candidate and sample_window.
func (sl *Selector) minCountForTesting() int64 {
	s := sl.currentSettings()
	return monitorsettings.RequiredCountForTesting(s.IntervalCandidate.Duration, s.SampleWindow.Duration)
}

// trackMonitorServerCounts refreshes the selector_monitor_servers metric
// every interval until ctx is done.
func (sl *Selector) trackMonitorServerCounts(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := sl.updateMonitorServerCounts(ctx); err != nil {
			sl.log.ErrorContext(ctx, "could not update monitor server counts", "err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// updateMonitorServerCounts loads the per-monitor server counts from
// server_scores into the metrics snapshot.
func (sl *Selector) updateMonitorServerCounts(ctx context.Context) error {
	rows, err := ntpdb.New(sl.dbconn).GetMonitorServerStatusCounts(ctx)
	if err != nil {
		return fmt.Errorf("get monitor server status counts: %w", err)
	}
	sl.metrics.MonitorServers.Update(rows)
	return nil
}

// Run processes all servers that need monitor review
func (sl *Selector) Run() (int, error) {
	ctx, cancel := context.WithCancel(sl.ctx)
	defer cancel()

	var count int
	db := ntpdb.New(sl.dbconn)
	err := database.WithTransaction(ctx, db, func(ctx context.Context, db ntpdb.QuerierTx) error {
		ids, err := db.GetServersMonitorReview(ctx)
		if err != nil {
			return err
		}

		count = 0
		for _, serverID := range ids {
			outcome, err := sl.processServer(ctx, db, serverID)
			if err != nil {
				// todo: rollback transaction here? Save that we did a review anyway?
				sl.log.Warn("could not process selection of monitors", "serverID", serverID, "err", err)
			}
			count++

			// A failed review is scheduled like one without changes. That
			// holds for errors that leave the transaction usable. A SQL
			// error inside processServer aborts the pgx transaction, so the
			// schedule call fails too and Run returns the error with the
			// whole batch rolled back.
			if err := sl.scheduleNextReview(ctx, db, serverID, outcome); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return count, nil
}

// ProcessServerSimulation runs the selection algorithm for a single server in simulation mode
func (sl *Selector) ProcessServerSimulation(ctx context.Context, db ntpdb.QuerierTx, serverID int64) (reviewOutcome, error) {
	return sl.processServer(ctx, db, serverID)
}

// skipUnknownStatuses drops rows whose server_scores status isn't known to
// this version, logging each one.
func (sl *Selector) skipUnknownStatuses(rows []ntpdb.GetMonitorPriorityRow, serverID int64) []ntpdb.GetMonitorPriorityRow {
	known := make([]ntpdb.GetMonitorPriorityRow, 0, len(rows))
	for _, row := range rows {
		if !knownServerStatus(row.Status) {
			sl.log.Warn("skipping monitor with unknown server status",
				"serverID", serverID,
				"monitorID", row.ID,
				"status", row.Status)
			continue
		}
		known = append(known, row)
	}
	return known
}

func (sl *Selector) processServer(ctx context.Context, db ntpdb.QuerierTx, serverID int64) (reviewOutcome, error) {
	start := time.Now()
	sl.log.Debug("processing server", "serverID", serverID)

	// Step 1: Load server information
	server, err := sl.loadServerInfo(ctx, db, serverID)
	if err != nil {
		return reviewOutcome{}, fmt.Errorf("failed to load server info: %w", err)
	}

	// Step 2: Get all assigned monitors
	assignedMonitors, err := db.GetMonitorPriority(ctx, ntpdb.GetMonitorPriorityParams{
		ServerID:            serverID,
		SampleWindowSeconds: sl.sampleWindow().Seconds(),
	})
	if err != nil {
		return reviewOutcome{}, fmt.Errorf("failed to get monitor priority: %w", err)
	}

	// Rows with a status this version doesn't know (written by a newer
	// version) are left alone, before any constraint is checked
	assignedMonitors = sl.skipUnknownStatuses(assignedMonitors, serverID)

	// No longer loading available monitors - only work with assigned monitors

	// Step 3: Build account limits from assigned monitors (still needed for promotion logic)
	accountLimits := sl.buildAccountLimitsFromMonitors(assignedMonitors)

	// Step 4: Use iterative constraint checking for account limits
	// This identifies which specific monitors exceed the per-category limits
	accountLimitViolations := sl.checkAccountConstraintsIterative(assignedMonitors, server)

	// Step 5: Evaluate all monitors against constraints
	evaluatedMonitors := make([]evaluatedMonitor, 0, len(assignedMonitors))

	// Process assigned monitors
	for _, row := range assignedMonitors {
		monitor := convertMonitorPriorityToCandidate(row)

		// Check non-account constraints for ALL monitors on EVERY run
		// This allows us to detect when constraint rules change
		var currentViolation *constraintViolation

		// First check if this monitor has an account limit violation from iterative checking
		if violation, hasAccountViolation := accountLimitViolations[monitor.ID]; hasAccountViolation {
			currentViolation = violation
		} else {
			// Check other constraints (network, same account) but skip account limits
			// since those are handled iteratively
			currentViolation = sl.checkNonAccountConstraints(&monitor, server, assignedMonitors, monitor.ServerStatus)
		}

		if currentViolation.Type != violationNone && sl.metrics != nil {
			sl.metrics.TrackConstraintViolation(&monitor, currentViolation.Type)
		}

		// Compute legacy recommendedState for backward compatibility
		state := sl.determineState(&monitor, currentViolation)

		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor:          monitor,
			currentViolation: currentViolation,
			recommendedState: state,
		})
	}

	// Step 6: Apply selection rules, limited by the promotions of the last
	// promotion_window
	promotionCounts, err := db.GetServerPromotionCounts(ctx, ntpdb.GetServerPromotionCountsParams{
		ServerID:      serverID,
		WindowSeconds: sl.currentSettings().PromotionWindow.Duration.Seconds(),
	})
	if err != nil {
		return reviewOutcome{}, fmt.Errorf("failed to get server promotion counts: %w", err)
	}
	budget := sl.promotionBudgetFor(promotionCounts, evaluatedMonitors)
	changes := sl.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, assignedMonitors, budget)
	if sl.metrics != nil {
		for _, r := range budget.refused() {
			sl.metrics.TrackPromotionLimited(r.to, r.kind)
		}
	}
	// The mode is of the server as it was found; it also sets the time of the
	// next review
	mode := budget.mode

	// Step 7: Execute changes
	// Create a map from monitor ID to monitor candidate for metrics tracking
	monitorMap := make(map[int64]*monitorCandidate)
	for _, em := range evaluatedMonitors {
		monitorMap[em.monitor.ID] = &em.monitor
	}

	changeCount := 0
	failedChanges := 0
	for _, change := range changes {
		monitor := monitorMap[change.monitorID]
		if err := sl.applyStatusChange(ctx, db, serverID, change, monitor); err != nil {
			failedChanges++
			sl.log.Error("failed to apply status change",
				"serverID", serverID,
				"monitorID", change.monitorID,
				"from", change.fromStatus,
				"to", change.toStatus,
				"error", err)
			// Continue with other changes
		} else {
			changeCount++
			sl.log.Info("applied status change",
				"serverID", serverID,
				"monitorID", change.monitorID,
				"from", change.fromStatus,
				"to", change.toStatus,
				"reason", change.reason)
		}
	}

	// Track constraint violations
	if err := sl.trackConstraintViolations(db, serverID, evaluatedMonitors); err != nil {
		sl.log.Error("failed to track constraint violations", "error", err)
		// Don't fail the whole operation for tracking errors
	}

	// Update constraint check timestamps for evaluated paused monitors
	pausedMonitors := make([]evaluatedMonitor, 0)
	for _, em := range evaluatedMonitors {
		if em.monitor.ServerStatus == ntpdb.ServerScoresStatusPaused {
			pausedMonitors = append(pausedMonitors, em)
		}
	}
	if len(pausedMonitors) > 0 {
		if err := sl.updateConstraintCheckTimestamps(db, serverID, pausedMonitors); err != nil {
			sl.log.Error("failed to update constraint check timestamps", "error", err)
			// Don't fail the whole operation for tracking errors
		}
	}

	// Track performance metrics
	if sl.metrics != nil {
		duration := time.Since(start).Seconds()

		// Track monitor pool sizes
		activeCount := 0
		testingCount := 0
		candidateCount := 0
		pausedCount := 0
		for _, em := range evaluatedMonitors {
			switch em.monitor.ServerStatus {
			case ntpdb.ServerScoresStatusActive:
				activeCount++
			case ntpdb.ServerScoresStatusTesting:
				testingCount++
			case ntpdb.ServerScoresStatusCandidate:
				candidateCount++
			case ntpdb.ServerScoresStatusPaused:
				pausedCount++
			}
		}

		sl.metrics.RecordProcessingMetrics(
			duration,
			len(evaluatedMonitors),
			changeCount,
			failedChanges,
		)

		sl.metrics.TrackServerMonitorCounts(
			activeCount,
			testingCount,
			candidateCount,
			pausedCount,
		)
	}

	// Log summary
	sl.log.Info("server processing complete",
		"serverID", serverID,
		"assignedMonitors", len(assignedMonitors),
		"evaluatedMonitors", len(evaluatedMonitors),
		"pausedMonitors", len(pausedMonitors),
		"plannedChanges", len(changes),
		"appliedChanges", changeCount,
		"failedChanges", failedChanges)

	return reviewOutcome{Changed: changeCount > 0, Mode: mode}, nil
}
