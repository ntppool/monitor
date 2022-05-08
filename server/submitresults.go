package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

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

	rv := &pb.ServerStatusResult{
		Ok: false,
	}

	monitor, ctx, err := srv.getMonitor(ctx)
	if err != nil {
		log.Printf("get monitor error: %s", err)
		return rv, err
	}

	if !monitor.IsLive() {
		return rv, fmt.Errorf("monitor not active")
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

	log.Printf("method=SubmitResults cn=%s traceID=%s batchID=%s",
		monitor.TlsName.String, span.SpanContext().TraceID(), batchID.String())

	batchTime := ulid.Time(batchID.Time())

	lastSubmit := monitor.LastSubmit
	if lastSubmit.Valid {
		log.Printf("monitor %d previous batch was %s", monitor.ID, lastSubmit.Time.String())
	} else {
		log.Printf("monitor %d had no last seen!", monitor.ID)
	}

	if batchTime.Before(lastSubmit.Time) {
		log.Printf("monitor %d previous batch was %s; new batch is older %s (%s)",
			monitor.ID,
			lastSubmit.Time.String(),
			batchTime.String(),
			batchID.String(),
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

	bidb, _ := batchID.MarshalText()

	for i, status := range in.List {

		if in.Version > 2 {
			ticketOk, err := srv.tokens.Validate(monitor.ID, bidb, status.GetIP(), status.Ticket)
			if err != nil || !ticketOk {
				span.AddEvent("signature validation failed")
				log.Printf("monitor %d signature validation failed for %q %s", monitor.ID, status.GetIP().String(), err)
				counters.Sig.Counter += len(in.List) - i
				return nil, twirp.NewError(twirp.InvalidArgument, "signature validation failed")
			}
		}

		err = srv.processStatus(ctx, monitor, status, counters)
		if err != nil {
			span.AddEvent("error processing status", otrace.WithAttributes(attribute.String("error", err.Error())))
			log.Printf("error processing status %+v: %s", status, err)
			return rv, twirp.InternalErrorWith(err)
		}
	}

	rv.Ok = true
	return rv, nil
}

func (srv *Server) processStatus(ctx context.Context, monitor *ntpdb.Monitor, status *pb.ServerStatus, counters *SubmitCounters) error {
	ctx, span := srv.tracer.Start(ctx, "processStatus")
	defer span.End()

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

	score, err := scorer.Score(&server, status)
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
	}

	ls := ntpdb.InsertLogScoreParams{
		ServerID:   server.ID,
		MonitorID:  sql.NullInt32{Int32: monitor.ID, Valid: true},
		Ts:         score.Ts,
		Step:       score.Step,
		Offset:     score.Offset,
		Rtt:        score.Rtt,
		Score:      serverScore.ScoreRaw,
		Attributes: score.Attributes,
	}

	err = db.InsertLogScore(ctx, ls)
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
