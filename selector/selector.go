package selector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cenkalti/backoff/v5"

	"go.ntppool.org/common/database"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/ntpdb"
)

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
		ServerID    *uint32 `arg:"" optional:"" help:"Server ID to process (if not specified, processes all servers)"`
		MetricsPort int     `default:"9000" help:"Metrics server port" flag:"metrics-port"`
	}
	SimulateCmd struct {
		ServerID uint32 `arg:"" help:"Server ID to simulate selection for"`
		Verbose  bool   `flag:"verbose" short:"v" help:"Enable verbose debug logging"`
	}
)

func (cmd ServerCmd) Run(ctx context.Context) error {
	return Run(ctx, true, cmd.MetricsPort, nil)
}

func (cmd OnceCmd) Run(ctx context.Context) error {
	return Run(ctx, false, cmd.MetricsPort, cmd.ServerID)
}

func (cmd SimulateCmd) Run(ctx context.Context) error {
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
	dbconn, err := ntpdb.OpenDB()
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

	// Create a transaction and explicitly roll it back for simulation
	tx, err := dbconn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Always rollback for simulation

	txDB := db.WithTx(tx)

	log.InfoContext(ctx, "simulating server processing", "serverID", cmd.ServerID)

	// Call the existing processServer method
	changed, err := sl.ProcessServerSimulation(ctx, txDB, cmd.ServerID)
	if err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	log.InfoContext(ctx, "simulation completed",
		"serverID", cmd.ServerID,
		"wouldHaveChanged", changed)

	if changed {
		fmt.Printf("✓ Simulation complete: Changes would be applied for server %d\n", cmd.ServerID)
	} else {
		fmt.Printf("✓ Simulation complete: No changes needed for server %d\n", cmd.ServerID)
	}

	return nil
}

// Run executes the selector logic either continuously or once
func Run(ctx context.Context, continuous bool, metricsPort int, serverID *uint32) error {
	log := logger.FromContext(ctx)

	log.InfoContext(ctx, "selector starting", "version", version.Version())

	dbconn, err := ntpdb.OpenDB()
	if err != nil {
		return err
	}

	// Create and start metrics server
	metricssrv := metricsserver.New()
	version.RegisterMetric("selector", metricssrv.Registry())
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

	expback := backoff.NewExponentialBackOff()
	expback.InitialInterval = time.Second * 3
	expback.MaxInterval = time.Second * 60

	// Handle single server processing
	if serverID != nil {
		log.InfoContext(ctx, "processing single server", "serverID", *serverID)

		db := ntpdb.New(sl.dbconn)
		var changed bool
		err := database.WithTransaction(ctx, db, func(ctx context.Context, db ntpdb.QuerierTx) error {
			var err error
			changed, err = sl.processServer(ctx, db, *serverID)
			if err != nil {
				return fmt.Errorf("failed to process server %d: %w", *serverID, err)
			}

			// Update review timestamp like the main Run() method does
			if changed {
				err := db.UpdateServersMonitorReviewChanged(ctx, ntpdb.UpdateServersMonitorReviewChangedParams{
					ServerID:   *serverID,
					NextReview: sql.NullTime{Time: time.Now().Add(60 * time.Minute), Valid: true},
				})
				if err != nil {
					return err
				}
			} else {
				err := db.UpdateServersMonitorReview(ctx, ntpdb.UpdateServersMonitorReviewParams{
					ServerID:   *serverID,
					NextReview: sql.NullTime{Time: time.Now().Add(20 * time.Minute), Valid: true},
				})
				if err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return err
		}

		if changed {
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
	ctx     context.Context
	dbconn  *sql.DB
	log     *slog.Logger
	metrics *Metrics
}

// NewSelector creates a new selector instance
func NewSelector(ctx context.Context, dbconn *sql.DB, log *slog.Logger, metrics *Metrics) (*Selector, error) {
	return &Selector{ctx: ctx, dbconn: dbconn, log: log, metrics: metrics}, nil
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
			changed, err := sl.processServer(ctx, db, serverID)
			if err != nil {
				// todo: rollback transaction here? Save that we did a review anyway?
				sl.log.Warn("could not process selection of monitors", "serverID", serverID, "err", err)
			}
			count++

			if changed {
				err := db.UpdateServersMonitorReviewChanged(ctx, ntpdb.UpdateServersMonitorReviewChangedParams{
					ServerID:   serverID,
					NextReview: sql.NullTime{Time: time.Now().Add(60 * time.Minute), Valid: true},
				})
				if err != nil {
					return err
				}
			} else {
				err := db.UpdateServersMonitorReview(ctx, ntpdb.UpdateServersMonitorReviewParams{
					ServerID:   serverID,
					NextReview: sql.NullTime{Time: time.Now().Add(20 * time.Minute), Valid: true},
				})
				if err != nil {
					return err
				}
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
func (sl *Selector) ProcessServerSimulation(ctx context.Context, db ntpdb.QuerierTx, serverID uint32) (bool, error) {
	return sl.processServer(ctx, db, serverID)
}

func (sl *Selector) processServer(ctx context.Context, db ntpdb.QuerierTx, serverID uint32) (bool, error) {
	start := time.Now()
	sl.log.Debug("processing server", "serverID", serverID)

	// Step 1: Load server information
	server, err := sl.loadServerInfo(ctx, db, serverID)
	if err != nil {
		return false, fmt.Errorf("failed to load server info: %w", err)
	}

	// Step 2: Get all assigned monitors
	assignedMonitors, err := db.GetMonitorPriority(ctx, serverID)
	if err != nil {
		return false, fmt.Errorf("failed to get monitor priority: %w", err)
	}

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
			sl.metrics.TrackConstraintViolation(&monitor, currentViolation.Type, serverID, false)
		}

		// Compute legacy recommendedState for backward compatibility
		state := sl.determineState(&monitor, currentViolation)

		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor:          monitor,
			currentViolation: currentViolation,
			recommendedState: state,
		})
	}

	// Step 6: Apply selection rules
	changes := sl.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, assignedMonitors)

	// Step 7: Execute changes
	// Create a map from monitor ID to monitor candidate for metrics tracking
	monitorMap := make(map[uint32]*monitorCandidate)
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

	// Track performance metrics
	if sl.metrics != nil {
		duration := time.Since(start).Seconds()

		// Count globally active monitors
		globallyActiveCount := 0
		for _, em := range evaluatedMonitors {
			if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
				globallyActiveCount++
			}
		}

		// Track monitor pool sizes
		activeCount := 0
		testingCount := 0
		candidateCount := 0
		for _, em := range evaluatedMonitors {
			switch em.monitor.ServerStatus {
			case ntpdb.ServerScoresStatusActive:
				activeCount++
			case ntpdb.ServerScoresStatusTesting:
				testingCount++
			case ntpdb.ServerScoresStatusCandidate:
				candidateCount++
			}
		}

		sl.metrics.RecordProcessingMetrics(
			serverID,
			duration,
			len(evaluatedMonitors),
			changeCount,
			failedChanges,
			globallyActiveCount,
		)

		sl.metrics.TrackMonitorPoolSizes(
			serverID,
			activeCount,
			testingCount,
			candidateCount,
		)
	}

	// Log summary
	sl.log.Info("server processing complete",
		"serverID", serverID,
		"assignedMonitors", len(assignedMonitors),
		"evaluatedMonitors", len(evaluatedMonitors),
		"plannedChanges", len(changes),
		"appliedChanges", changeCount,
		"failedChanges", failedChanges)

	return changeCount > 0, nil
}
