package cmd

//go:generate stringer -type=candidateStatus
//go:generate stringer -type=newStatusOption

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"go.ntppool.org/monitor/ntpdb"
)

func (cli *CLI) selectorCmd() *cobra.Command {

	var selectorCmd = &cobra.Command{
		Use:   "selector",
		Short: "monitor selection",
	}

	selectorCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	selectorCmd.AddCommand(
		&cobra.Command{
			Use:   "run",
			Short: "run once",
			RunE:  cli.Run(cli.selector),
		})

	selectorCmd.AddCommand(
		&cobra.Command{
			Use:   "server",
			Short: "run continously",
			RunE:  cli.Run(cli.selectorServer),
		})

	return selectorCmd
}

func (cli *CLI) selectorServer(cmd *cobra.Command, args []string) error {
	return cli.selectorRun(cmd, args, true)
}

func (cli *CLI) selector(cmd *cobra.Command, args []string) error {
	return cli.selectorRun(cmd, args, false)
}

func (cli *CLI) selectorRun(cmd *cobra.Command, args []string, continuous bool) error {

	ctx := context.Background()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		return err
	}

	sl, err := newSelector(ctx, dbconn)
	if err != nil {
		return nil
	}

	expback := backoff.NewExponentialBackOff()
	expback.InitialInterval = time.Second * 3
	expback.MaxInterval = time.Second * 60
	expback.MaxElapsedTime = 0

	for {

		count, err := sl.Run()
		if err != nil {
			return err
		}
		log.Printf("Processed %d servers", count)

		if !continuous {
			break
		}

		if count == 0 {
			sl := expback.NextBackOff()
			// log.Printf("going to sleep %s", sl)
			time.Sleep(sl)
		} else {
			expback.Reset()
		}

	}

	return nil
}

type selector struct {
	ctx    context.Context
	dbconn *sql.DB
}

func newSelector(ctx context.Context, dbconn *sql.DB) (*selector, error) {
	return &selector{ctx: ctx, dbconn: dbconn}, nil
}

type candidateState uint8

const (
	candidateUnknown candidateState = iota
	candidateIn
	candidateOut
	candidateBlock
)

type newStatus struct {
	MonitorID     int32
	MonitorStatus ntpdb.MonitorsStatus
	CurrentStatus ntpdb.ServerScoresStatus
	NewState      candidateState
	RTT           float64
}

type newStatusList []newStatus

func (sl *selector) Run() (int, error) {

	tx, err := sl.dbconn.BeginTx(sl.ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	db := ntpdb.New(sl.dbconn).WithTx(tx)

	ids, err := db.GetServersMonitorReview(sl.ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, serverID := range ids {
		changed, err := sl.processServer(db, serverID)
		if err != nil {
			// todo: rollback transaction here? Save that we did a review anyway?
			log.Printf("could not process selection of monitors for server %d: %s", serverID, err)
		}
		count++

		if changed {
			err := db.UpdateServersMonitorReviewChanged(sl.ctx, ntpdb.UpdateServersMonitorReviewChangedParams{
				ServerID:   serverID,
				NextReview: sql.NullTime{Time: time.Now().Add(60 * time.Minute), Valid: true},
			})
			if err != nil {
				return count, err
			}
		} else {
			err := db.UpdateServersMonitorReview(sl.ctx, ntpdb.UpdateServersMonitorReviewParams{
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

func (sl *selector) processServer(db *ntpdb.Queries, serverID int32) (bool, error) {

	targetNumber := 5

	log := slog.Default().With("serverID", serverID)

	log.Info("processServer")

	// the list comes sorted
	prilist, err := db.GetMonitorPriority(sl.ctx, serverID)
	if err != nil {
		return false, err
	}

	// newStatusList
	nsl := newStatusList{}

	currentActiveMonitors := 0
	currentTestingMonitors := 0

	for _, candidate := range prilist {

		var currentStatus ntpdb.ServerScoresStatus

		if candidate.Status.Valid {
			currentStatus = candidate.Status.ServerScoresStatus
		} else {
			currentStatus = ntpdb.ServerScoresStatusNew
		}

		if currentStatus == ntpdb.ServerScoresStatusNew {
			// insert if it's not there already, don't check errors
			db.InsertServerScore(sl.ctx, ntpdb.InsertServerScoreParams{
				MonitorID: candidate.ID,
				ServerID:  serverID,
				ScoreRaw:  0,
				CreatedOn: time.Now(),
			})
			err := db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
				MonitorID: candidate.ID,
				ServerID:  serverID,
				Status:    ntpdb.ServerScoresStatusTesting,
			})
			if err != nil {
				return false, err
			}
			// don't consider this monitor a candidate now as there'll be no monitoring data
			continue
		}

		healthy := candidate.Healthy.(int64)

		rtt := 0.0
		if avgUint, ok := candidate.AvgRtt.([]uint8); ok {
			x := sql.NullFloat64{}
			err := x.Scan(avgUint)
			if err != nil {
				return false, err
			}
			rtt = x.Float64
		} else {
			return false, fmt.Errorf("could not decode avg_rtt type %T", candidate.AvgRtt)
		}

		newStatus := newStatus{
			MonitorID:     candidate.ID,
			MonitorStatus: candidate.MonitorStatus,
			CurrentStatus: currentStatus,
			RTT:           rtt,
		}

		switch candidate.MonitorStatus {
		case ntpdb.MonitorsStatusActive:
			var s candidateState

			switch {

			case healthy == 0:
				s = candidateOut

			// case rtt < 0.01:
			//  rtt is low when there were no successful probes,
			//  but this should also result in healthy == 0 because
			//  the average step value is low.
			// 	s = candidateOut

			case candidate.Count < 5:
				s = candidateOut

			default: // must be healthy == 1
				s = candidateIn
			}

			newStatus.NewState = s
			nsl = append(nsl, newStatus)

			switch candidate.Status.ServerScoresStatus {
			case ntpdb.ServerScoresStatusActive:
				currentActiveMonitors++
			case ntpdb.ServerScoresStatusTesting:
				currentTestingMonitors++
			}

			continue

		case ntpdb.MonitorsStatusTesting:
			newStatus.NewState = candidateOut
			nsl = append(nsl, newStatus)
			continue

		default:
			newStatus.NewState = candidateBlock
			nsl = append(nsl, newStatus)
			continue
		}
	}

	okMonitors := 0
	blockedMonitors := 0
	for _, ns := range nsl {
		switch ns.NewState {
		// "out" counts for "is an option to keep, blocked is not
		case candidateIn, candidateOut:
			okMonitors++

		case candidateBlock:
			blockedMonitors++
		}

	}

	log.Info("monitor counts", "input", len(prilist), "new", len(nsl))
	log.Info("monitor status", "ok", okMonitors, "active", currentActiveMonitors, "blocked", blockedMonitors)

	allowedChanges := 1
	toAdd := targetNumber - currentActiveMonitors

	if blockedMonitors > 1 {
		allowedChanges = 2
	}

	if currentActiveMonitors == 0 {
		allowedChanges = (targetNumber / 2) + 1
	}

	if targetNumber > okMonitors {
		return false, fmt.Errorf("not enough healthy and active monitors for server %d", serverID)
	}

	for _, ns := range nsl {
		log.Info("nsl",
			"monitorID", ns.MonitorID,
			"monitorStatus", ns.MonitorStatus,
			"currentStatus", ns.CurrentStatus,
			"newState", ns.NewState,
			"rtt", ns.RTT,
		)
	}

	maxRemovals := allowedChanges

	log.Info("changes allowed", "toAdd", toAdd, "maxRemovals", maxRemovals)

	changed := false

	// remove candidates for removal
	for _, stateToRemove := range []candidateState{candidateBlock, candidateOut} {
		for i := len(nsl) - 1; i >= 0; i-- {
			if maxRemovals <= 0 {
				break
			}
			if nsl[i].CurrentStatus != ntpdb.ServerScoresStatusActive {
				continue
			}
			if nsl[i].NewState == stateToRemove {
				log.Info("removing", "monitorID", nsl[i].MonitorID)
				db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
					MonitorID: nsl[i].MonitorID,
					ServerID:  serverID,
					Status:    ntpdb.ServerScoresStatusTesting,
				})
				nsl[i].CurrentStatus = ntpdb.ServerScoresStatusTesting
				changed = true
				currentActiveMonitors--
				maxRemovals--
				toAdd++
			}
		}
	}

	log.Info("work after removals", "toAdd", toAdd)

	// replace removed monitors
	for _, ns := range nsl {
		log.Info("add loop", "toAdd", toAdd, "allowedChanges", allowedChanges)
		if allowedChanges <= 0 || toAdd <= 0 {
			break
		}
		if ns.NewState != candidateIn || ns.CurrentStatus == ntpdb.ServerScoresStatusActive {
			continue
		}
		log.Info("adding", "monitorID", ns.MonitorID)
		db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
			MonitorID: ns.MonitorID,
			ServerID:  serverID,
			Status:    ntpdb.ServerScoresStatusActive,
		})
		changed = true
		ns.CurrentStatus = ntpdb.ServerScoresStatusActive
		currentActiveMonitors++
		allowedChanges--
		toAdd--
	}

	for allowedChanges > 0 {
		better, replace, ok := nsl.IsOutOfOrder()
		if !ok {
			break
		}

		err = db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
			MonitorID: replace,
			ServerID:  serverID,
			Status:    ntpdb.ServerScoresStatusTesting,
		})
		if err != nil {
			return changed, err
		}
		toAdd++

		if toAdd > 0 {
			err = db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
				MonitorID: better,
				ServerID:  serverID,
				Status:    ntpdb.ServerScoresStatusActive,
			})
			if err != nil {
				return changed, err
			}
			toAdd--
			log.Info("replaced", "replacedMonitorID", replace, "monitorID", better)
		} else {
			log.Info("removed", "monitorID", replace)
		}

		changed = true
		allowedChanges--
	}

	return changed, nil

}

// IsOutOfOrder returns the "most out of order" of the currently active monitors.
// The second return parameter is the ID of the better monitor candidate,
// the first return parameter the ID to be replaced. The last parameter
// is false if no relevant replacement was found.
func (nsl newStatusList) IsOutOfOrder() (int32, int32, bool) {

	best := int32(-1)
	replace := int32(-1)

	for _, ns := range nsl {
		if ns.NewState != candidateIn {
			continue
		}
		switch ns.CurrentStatus {
		case ntpdb.ServerScoresStatusActive:
			// only replace if we found a replacement
			if best != -1 {
				replace = ns.MonitorID
			}

		case ntpdb.ServerScoresStatusTesting:
			if best == -1 {
				best = ns.MonitorID
			}

		}

	}

	if best < 0 || replace < 0 {
		return 0, 0, false
	}

	return best, replace, true
}
