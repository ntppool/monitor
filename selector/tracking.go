package selector

import (
	"database/sql"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

// trackConstraintViolations updates the database with current constraint violations
func (sl *Selector) trackConstraintViolations(
	db *ntpdb.Queries,
	serverID uint32,
	evaluatedMonitors []evaluatedMonitor,
) error {
	for _, eval := range evaluatedMonitors {
		monitor := eval.monitor
		violation := eval.violation

		// Check if we need to update the database
		shouldUpdate := false
		var newViolationType sql.NullString
		var newViolationSince sql.NullTime

		if violation.Type != violationNone {
			// We have a current violation
			violationType := string(violation.Type)
			newViolationType = sql.NullString{String: violationType, Valid: true}

			if monitor.ConstraintViolationType == nil || *monitor.ConstraintViolationType != violationType {
				// New violation or type changed
				shouldUpdate = true
				newViolationSince = sql.NullTime{Time: time.Now(), Valid: true}
			} else if monitor.ConstraintViolationSince != nil {
				// Same violation type, keep existing timestamp
				newViolationSince = sql.NullTime{Time: *monitor.ConstraintViolationSince, Valid: true}
			} else {
				// Missing timestamp for existing violation (shouldn't happen)
				shouldUpdate = true
				newViolationSince = sql.NullTime{Time: time.Now(), Valid: true}
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
					"violationType", violation.Type,
					"isGrandfathered", violation.IsGrandfathered)
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
		accountID := uint32(row.AccountID.Int32)
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
		candidate.ServerStatus = ntpdb.ServerScoresStatusNew
	}

	// Health status
	if healthy, ok := row.Healthy.(int64); ok {
		candidate.IsHealthy = healthy > 0
	}

	// RTT
	if avgRtt, ok := row.AvgRtt.([]uint8); ok {
		x := sql.NullFloat64{}
		if err := x.Scan(avgRtt); err == nil {
			candidate.RTT = x.Float64
		}
	}

	// Constraint violation
	if row.ConstraintViolationType.Valid {
		candidate.ConstraintViolationType = &row.ConstraintViolationType.String
	}
	if row.ConstraintViolationSince.Valid {
		candidate.ConstraintViolationSince = &row.ConstraintViolationSince.Time
	}

	return candidate
}
