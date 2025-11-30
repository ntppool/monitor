package selector

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go.ntppool.org/monitor/ntpdb"
)

// trackConstraintViolations updates the database with current constraint violations
func (sl *Selector) trackConstraintViolations(
	db ntpdb.QuerierTx,
	serverID int64,
	evaluatedMonitors []evaluatedMonitor,
) error {
	for _, eval := range evaluatedMonitors {
		monitor := eval.monitor
		violation := eval.currentViolation

		// Check if we need to update the database
		shouldUpdate := false
		var newViolationType pgtype.Text
		var newViolationSince pgtype.Timestamptz
		var shouldPause bool

		if violation.Type != violationNone {
			// Defensive programming: candidates should not have limit violations,
			// but can have account/network violations that prevent promotion
			if monitor.ServerStatus == ntpdb.ServerScoresStatusCandidate && violation.Type == violationLimit {
				// Candidate monitors should not have limit violations
				sl.log.Warn("attempted to record limit violation for candidate monitor",
					"monitorID", monitor.ID,
					"serverStatus", monitor.ServerStatus,
					"violationType", violation.Type)
				// Clear any existing violation instead
				if monitor.ConstraintViolationType != nil {
					shouldUpdate = true
					// newViolationType and newViolationSince remain null for clearing
				}
				continue
			}

			// We have a current violation
			violationType := string(violation.Type)
			newViolationType = pgtype.Text{String: violationType, Valid: true}

			// Check if this is an unchangeable constraint that should result in paused status
			if isUnchangeableConstraint(violation.Type) && monitor.ServerStatus != ntpdb.ServerScoresStatusPaused {
				shouldPause = true
				shouldUpdate = true
			}

			if monitor.ConstraintViolationType == nil || *monitor.ConstraintViolationType != violationType {
				// New violation or type changed
				shouldUpdate = true
				newViolationSince = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			} else if monitor.ConstraintViolationSince != nil {
				// Same violation type, keep existing timestamp
				newViolationSince = pgtype.Timestamptz{Time: *monitor.ConstraintViolationSince, Valid: true}
			} else {
				// Missing timestamp for existing violation (shouldn't happen)
				shouldUpdate = true
				newViolationSince = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			}
		} else {
			// No current violation
			if monitor.ConstraintViolationType != nil {
				// Had a violation before, need to clear it
				shouldUpdate = true
				// newViolationType and newViolationSince remain null
			}
		}

		// Update database if needed
		if shouldUpdate {
			if violation.Type == violationNone {
				// Clear violation
				err := db.ClearServerScoreConstraintViolation(sl.ctx, ntpdb.ClearServerScoreConstraintViolationParams{
					ServerID:  serverID,
					MonitorID: monitor.ID,
				})
				if err != nil {
					sl.log.Error("failed to clear constraint violation",
						"serverID", serverID,
						"monitorID", monitor.ID,
						"error", err)
					continue
				}
				sl.log.Debug("cleared constraint violation",
					"serverID", serverID,
					"monitorID", monitor.ID)
			} else {
				// Update violation
				err := db.UpdateServerScoreConstraintViolation(sl.ctx, ntpdb.UpdateServerScoreConstraintViolationParams{
					ConstraintViolationType:  newViolationType,
					ConstraintViolationSince: newViolationSince,
					ServerID:                 serverID,
					MonitorID:                monitor.ID,
				})
				if err != nil {
					sl.log.Error("failed to update constraint violation",
						"serverID", serverID,
						"monitorID", monitor.ID,
						"violationType", violation.Type,
						"error", err)
					continue
				}
				sl.log.Debug("updated constraint violation",
					"serverID", serverID,
					"monitorID", monitor.ID,
					"violationType", violation.Type)
			}

			// If this is an unchangeable constraint, pause the monitor
			if shouldPause {
				err := db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
					Status:    ntpdb.ServerScoresStatusPaused,
					MonitorID: monitor.ID,
					ServerID:  serverID,
				})
				if err != nil {
					sl.log.Error("failed to pause monitor for unchangeable constraint",
						"serverID", serverID,
						"monitorID", monitor.ID,
						"violationType", violation.Type,
						"error", err)
					continue
				}

				// Update pause reason and last constraint check
				err = db.UpdateServerScorePauseReason(sl.ctx, ntpdb.UpdateServerScorePauseReasonParams{
					PauseReason: pgtype.Text{String: string(pauseConstraintViolation), Valid: true},
					ServerID:    serverID,
					MonitorID:   monitor.ID,
				})
				if err != nil {
					sl.log.Error("failed to update pause reason",
						"serverID", serverID,
						"monitorID", monitor.ID,
						"error", err)
					continue
				}

				sl.log.Info("paused monitor due to unchangeable constraint",
					"serverID", serverID,
					"monitorID", monitor.ID,
					"violationType", violation.Type,
					"previousStatus", monitor.ServerStatus)
			}
		}
	}

	return nil
}

// updateConstraintCheckTimestamps updates the last_constraint_check timestamp for evaluated paused monitors
func (sl *Selector) updateConstraintCheckTimestamps(
	db ntpdb.QuerierTx,
	serverID int64,
	evaluatedPausedMonitors []evaluatedMonitor,
) error {
	for _, eval := range evaluatedPausedMonitors {
		monitor := eval.monitor

		// Get pause reason from the monitor
		var pauseReasonValue pauseReason
		if monitor.PauseReason != nil {
			pauseReasonValue = pauseReason(*monitor.PauseReason)
		} else {
			// Default to constraint violation for backward compatibility
			pauseReasonValue = pauseConstraintViolation
		}

		// Check if we evaluated this monitor for constraint resolution
		if sl.shouldCheckConstraintResolution(monitor, pauseReasonValue) {
			err := db.UpdateServerScoreLastConstraintCheck(sl.ctx, ntpdb.UpdateServerScoreLastConstraintCheckParams{
				ServerID:  serverID,
				MonitorID: monitor.ID,
			})
			if err != nil {
				sl.log.Error("failed to update last constraint check",
					"serverID", serverID,
					"monitorID", monitor.ID,
					"error", err)
				continue
			}
		}
	}

	return nil
}

// convertMonitorPriorityToCandidate converts a GetMonitorPriorityRow to monitorCandidate
func convertMonitorPriorityToCandidate(row ntpdb.GetMonitorPriorityRow) monitorCandidate {
	candidate := monitorCandidate{
		ID:           row.ID,
		GlobalStatus: row.MonitorStatus,
		HasMetrics:   true, // If in GetMonitorPriority results, it has metrics
	}

	// ID Token for metrics
	if row.IDToken.Valid {
		candidate.IDToken = row.IDToken.String
	}

	// TLS Name for metrics
	if row.TlsName.Valid {
		candidate.TLSName = row.TlsName.String
	}

	// Account ID
	if row.AccountID.Valid {
		accountID := row.AccountID.Int64
		candidate.AccountID = &accountID
	}

	// Monitor IP
	if row.MonitorIp.Valid {
		candidate.IP = row.MonitorIp.String
	}

	// Server status
	if row.Status.Valid {
		candidate.ServerStatus = row.Status.ServerScoresStatus
	} else {
		// This should not happen - all monitors in GetMonitorPriority should have a status
		candidate.ServerStatus = ntpdb.ServerScoresStatusCandidate
	}

	// Health status
	candidate.IsHealthy = row.Healthy

	// RTT
	candidate.RTT = float64(row.AvgRtt)

	// Priority (from database calculation)
	candidate.Priority = int(row.MonitorPriority)

	// Count (number of data points)
	candidate.Count = row.Count

	// If priority is invalid (negative), mark monitor as unhealthy
	// Note: Priority 0 is valid and represents the best possible performance
	if candidate.Priority < 0 {
		candidate.IsHealthy = false
	}

	// Constraint violation
	if row.ConstraintViolationType.Valid {
		candidate.ConstraintViolationType = &row.ConstraintViolationType.String
	}
	if row.ConstraintViolationSince.Valid {
		candidate.ConstraintViolationSince = &row.ConstraintViolationSince.Time
	}

	// Last constraint check (new field)
	if row.LastConstraintCheck.Valid {
		candidate.LastConstraintCheck = &row.LastConstraintCheck.Time
	}

	// Pause reason (new field)
	if row.PauseReason.Valid {
		candidate.PauseReason = &row.PauseReason.String
	}

	return candidate
}
