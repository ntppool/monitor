package selector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/prometheus/client_golang/prometheus"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/ntpdb"
)

// Cmd provides the command structure for CLI integration
type Cmd struct {
	Server ServerCmd `cmd:"server" help:"run continously"`
	Run    OnceCmd   `cmd:"once" help:"run once"`
}

type (
	ServerCmd struct{}
	OnceCmd   struct{}
)

func (cmd ServerCmd) Run(ctx context.Context) error {
	return Run(ctx, true)
}

func (cmd OnceCmd) Run(ctx context.Context) error {
	return Run(ctx, false)
}

// Run executes the selector logic either continuously or once
func Run(ctx context.Context, continuous bool) error {
	log := logger.FromContext(ctx)

	log.InfoContext(ctx, "selector starting", "version", version.Version())

	dbconn, err := ntpdb.OpenDB()
	if err != nil {
		return err
	}

	// Create metrics - for now we'll create a no-op metrics instance
	// This will be properly wired when integrated with the scorer command
	metrics := NewMetrics(prometheus.NewRegistry())

	sl, err := NewSelector(ctx, dbconn, log, metrics)
	if err != nil {
		return err
	}

	expback := backoff.NewExponentialBackOff()
	expback.InitialInterval = time.Second * 3
	expback.MaxInterval = time.Second * 60

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
	tx, err := sl.dbconn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	db := ntpdb.New(sl.dbconn).WithTx(tx)

	ids, err := db.GetServersMonitorReview(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
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
				return count, err
			}
		} else {
			err := db.UpdateServersMonitorReview(ctx, ntpdb.UpdateServersMonitorReviewParams{
				ServerID:   serverID,
				NextReview: sql.NullTime{Time: time.Now().Add(20 * time.Minute), Valid: true},
			})
			if err != nil {
				return count, err
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (sl *Selector) processServer(ctx context.Context, db *ntpdb.Queries, serverID uint32) (bool, error) {
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
	availableMonitors := []monitorCandidate{}

	// Step 3: Build account limits from assigned monitors
	accountLimits := sl.buildAccountLimitsFromMonitors(assignedMonitors)

	// Step 4: Evaluate all monitors against constraints
	evaluatedMonitors := make([]evaluatedMonitor, 0, len(assignedMonitors)+len(availableMonitors))

	// Process assigned monitors
	for _, row := range assignedMonitors {
		monitor := convertMonitorPriorityToCandidate(row)

		// Check constraints for ALL monitors on EVERY run
		// This allows us to detect when constraint rules change
		var currentViolation *constraintViolation
		currentViolation = sl.checkConstraints(&monitor, server, accountLimits, monitor.ServerStatus, assignedMonitors)

		if currentViolation.Type != violationNone {
			currentViolation.IsGrandfathered = sl.isGrandfathered(&monitor, server, currentViolation)

			// Track grandfathered violations in metrics
			if currentViolation.IsGrandfathered && sl.metrics != nil {
				sl.metrics.TrackConstraintViolation(&monitor, currentViolation.Type, serverID, true)
			}
		}

		// Compute legacy recommendedState for backward compatibility
		state := sl.determineState(&monitor, currentViolation)

		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor:          monitor,
			currentViolation: currentViolation,
			recommendedState: state,
		})
	}

	// Step 5: Apply selection rules
	changes := sl.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, assignedMonitors)

	// Step 6: Execute changes
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
			len(availableMonitors),
		)
	}

	// Log summary
	sl.log.Info("server processing complete",
		"serverID", serverID,
		"assignedMonitors", len(assignedMonitors),
		"availableMonitors", len(availableMonitors),
		"evaluatedMonitors", len(evaluatedMonitors),
		"plannedChanges", len(changes),
		"appliedChanges", changeCount,
		"failedChanges", failedChanges)

	return changeCount > 0, nil
}
