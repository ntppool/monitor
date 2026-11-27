package selector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
)

// reviewOutcome is what one review of a server found, used to pick the time
// of the next review.
type reviewOutcome struct {
	// Changed is set when the review changed a status.
	Changed bool
	// Mode is the mode of the server when the review started.
	Mode serverMode
}

// nextReviewIn returns the time until the next review of a server. A server
// in bootstrap mode needs monitors soon and is reviewed often, unless it
// looks bad: more reviews then change nothing and only add work.
func nextReviewIn(o reviewOutcome, s monitorsettings.MonitorSettings) time.Duration {
	switch {
	case o.Mode.Bootstrap && !o.Mode.LooksBad:
		return s.ReviewBootstrap.Duration
	case o.Changed:
		return s.ReviewAfterChange.Duration
	default:
		return s.ReviewEvery.Duration
	}
}

// scheduleNextReview records the review and sets the time of the next one.
func (sl *Selector) scheduleNextReview(ctx context.Context, db ntpdb.QuerierTx, serverID int64, o reviewOutcome) error {
	next := pgtype.Timestamptz{Time: time.Now().Add(nextReviewIn(o, sl.currentSettings())), Valid: true}

	if o.Changed {
		err := db.UpdateServersMonitorReviewChanged(ctx, ntpdb.UpdateServersMonitorReviewChangedParams{
			ServerID:   serverID,
			NextReview: next,
		})
		if err != nil {
			return fmt.Errorf("schedule review of server %d: %w", serverID, err)
		}
		return nil
	}

	err := db.UpdateServersMonitorReview(ctx, ntpdb.UpdateServersMonitorReviewParams{
		ServerID:   serverID,
		NextReview: next,
	})
	if err != nil {
		return fmt.Errorf("schedule review of server %d: %w", serverID, err)
	}
	return nil
}
