package server

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/statusscore"
)

type CounterOpt struct {
	Name    string
	Counter int
}

type SubmitCounters struct {
	Ok         *CounterOpt
	Offset     *CounterOpt
	Timeout    *CounterOpt
	Sig        *CounterOpt
	BatchOrder *CounterOpt
}

func (srv *Server) SubmitResults(ctx context.Context, in *pb.ServerStatusList) (*pb.ServerStatusResult, error) {
	span := otrace.SpanFromContext(ctx)
	now := time.Now()

	log := logger.FromContext(ctx)

	rv := &pb.ServerStatusResult{
		Ok: false,
	}

	monitor, ctx, err := srv.getMonitor(ctx)
	if err != nil {
		log.Error("get monitor error", "err", err)
		return rv, err
	}

	log = log.With("mon_id", monitor.ID)

	if !monitor.IsLive() {
		return rv, twirp.PermissionDenied.Error("monitor not active")
	}

	if in.Version < 2 || in.Version > 3 {
		return rv, twirp.InvalidArgumentError("Version", "Unsupported data version")
	}

	counters := &SubmitCounters{
		Ok:         &CounterOpt{"ok", 0},
		Offset:     &CounterOpt{"offset", 0},
		Timeout:    &CounterOpt{"timeout", 0},
		Sig:        &CounterOpt{"signature_validation", 0},
		BatchOrder: &CounterOpt{"batch_out_of_order", 0},
	}

	defer func() {
		for _, c := range []*CounterOpt{
			counters.Ok, counters.Offset,
			counters.Timeout, counters.Sig,
			counters.BatchOrder,
		} {
			srv.m.TestsCompleted.WithLabelValues(monitor.TlsName.String, monitor.IpVersion.MonitorsIpVersion.String(), c.Name).Add(float64(c.Counter))
		}
	}()

	batchID := ulid.ULID{}
	batchID.UnmarshalText(in.BatchID)

	span.SetAttributes(attribute.String("batchID", batchID.String()))
	log = log.With("batchID", batchID.String())

	batchTime := ulid.Time(batchID.Time())

	lastSubmit := monitor.LastSubmit
	if lastSubmit.Valid {
		log.Debug("previous batch timestamp", "last_submit", lastSubmit.Time.String())
	} else {
		log.Info("monitor had no last seen!")
	}

	if batchTime.Before(lastSubmit.Time) {
		log.Warn("new batch is older than previous batch",
			"last_submit", lastSubmit.Time.String(),
			"new_submit", batchTime.String(),
		)
		// todo: add safety check of setting the monitor status to 'testing' ?

		counters.BatchOrder.Counter += len(in.List)

		span.AddEvent("Out of order batch", otrace.WithAttributes(attribute.String("previous", lastSubmit.Time.String())))

		return rv, fmt.Errorf("invalid batch submission")
	}

	srv.db.UpdateMonitorSubmit(ctx, ntpdb.UpdateMonitorSubmitParams{
		ID:         monitor.ID,
		LastSubmit: sql.NullTime{Time: batchTime, Valid: true},
		LastSeen:   sql.NullTime{Time: now, Valid: true},
	})

	clientVersion := monitor.ClientVersion
	if idx := strings.Index(clientVersion, "/"); idx >= 0 {
		clientVersion = clientVersion[0:idx]
	}

	safeZeroOffset := version.CheckVersion(clientVersion, "v3.8.5")
	// log.InfoContext(ctx, "safeZeroOffset", "version", monitor.ClientVersion, "isSafe", safeZeroOffset)

	bidb, _ := batchID.MarshalText()

	rv, err = func() (*pb.ServerStatusResult, error) {
		ctx, span := tracing.Start(ctx, "processStatus")
		defer span.End()

		for i, status := range in.List {

			if in.Version > 2 {
				ticketOk, err := srv.ValidateIPs(status.Ticket, monitor.ID, bidb, status.GetIP())
				if err != nil || !ticketOk {
					span.AddEvent("signature validation failed")
					log.Error("signature validation failed", "test_ip", status.GetIP().String(), "err", err)
					counters.Sig.Counter += len(in.List) - i
					return nil, twirp.NewError(twirp.InvalidArgument, "signature validation failed")
				}
			}

			if !safeZeroOffset {
				// client might have broken error handling for some
				// network errors, so don't trust zero offset.
				if status.Stratum == 0 && status.Offset.AsDuration() == 0 {
					if status.Error == "" {
						status.Offset = nil
						status.Error = "untrusted zero offset"
					}
				}
			}

			err = srv.processStatus(ctx, monitor, status, counters)
			if err != nil {
				span.AddEvent("error processing status", otrace.WithAttributes(attribute.String("error", err.Error())))
				log.Error("error processing status", "status", status, "err", err)
				return rv, twirp.InternalErrorWith(err)
			}
		}
		return rv, nil
	}()
	if err != nil {
		return rv, err
	}

	rv.Ok = true
	return rv, nil
}

func (srv *Server) processStatus(ctx context.Context, monitor *ntpdb.Monitor, status *pb.ServerStatus, counters *SubmitCounters) error {
	tx, err := srv.dbconn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	db := srv.db.WithTx(tx)

	server, err := db.GetServerIP(ctx, status.GetIP().String())
	if err != nil {
		return err
	}
	serverScore, err := db.GetServerScore(ctx, ntpdb.GetServerScoreParams{
		MonitorID: monitor.ID,
		ServerID:  server.ID,
	})
	if err != nil {
		return err
	}

	scorer := statusscore.NewScorer()

	score, err := scorer.Score(ctx, &server, status)
	if err != nil {
		return err
	}

	serverScore.ScoreRaw = (serverScore.ScoreRaw * 0.95) + score.Step
	if score.HasMaxScore {
		serverScore.ScoreRaw = math.Min(serverScore.ScoreRaw, score.MaxScore)
	}

	if status.Stratum > 0 {
		nullStratum := sql.NullInt32{Int32: status.Stratum, Valid: true}
		db.UpdateServerScoreStratum(ctx, ntpdb.UpdateServerScoreStratumParams{
			ID:      serverScore.ID,
			Stratum: nullStratum,
		})
		db.UpdateServerStratum(ctx, ntpdb.UpdateServerStratumParams{
			ID:      server.ID,
			Stratum: nullStratum,
		})
	}

	db.UpdateServerScore(ctx, ntpdb.UpdateServerScoreParams{
		ID:       serverScore.ID,
		ScoreTs:  sql.NullTime{Time: score.Ts, Valid: true},
		ScoreRaw: serverScore.ScoreRaw,
	})

	ls := ntpdb.InsertLogScoreParams{
		ServerID:   server.ID,
		MonitorID:  sql.NullInt32{Int32: int32(monitor.ID), Valid: true}, // todo: sqlc type
		Ts:         score.Ts,
		Step:       score.Step,
		Offset:     score.Offset,
		Rtt:        score.Rtt,
		Score:      serverScore.ScoreRaw,
		Attributes: score.Attributes,
	}

	_, err = db.InsertLogScore(ctx, ls)
	if err != nil {
		return err
	}

	// todo: have score give a category
	switch {
	case ls.Step == -5:
		counters.Timeout.Counter += 1
	case ls.Step < 1:
		counters.Offset.Counter += 1
	default:
		counters.Ok.Counter += 1
	}

	// todo:
	//   if NoResponse == true OR score is low and step == 1:
	//      mark for traceroute if it's not been done recently
	//      maybe track why we traceroute'd last?
	//      schedule new monitors?
	//   if step < 0 and retesting isn't recent, mark server_scores for retesting?

	// new schemas:
	//    traceroute_queue
	//       server_id, monitor_id, last_traceroute
	//

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil

}
