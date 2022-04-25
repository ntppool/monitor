package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/twitchtv/twirp"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/ntpdb"
)

func (srv *Server) SubmitResults(ctx context.Context, in *pb.ServerStatusList) (*pb.ServerStatusResult, error) {
	monitor, err := srv.getMonitor(ctx)
	if err != nil {
		log.Printf("get monitor error: %s", err)
		return nil, err
	}

	if !monitor.IsLive() {
		return nil, fmt.Errorf("monitor not active")
	}

	if in.Version < 2 || in.Version > 3 {
		return nil, twirp.InvalidArgumentError("Version", "Unsupported data version")
	}

	batchID := ulid.ULID{}
	batchID.UnmarshalText(in.BatchID)

	log.Printf("SubmitServers() BatchID for monitor %d: %s", monitor.ID, batchID.String())

	bidb, _ := batchID.MarshalText()

	// todo: check that the new batchID is newer than the last 'seen' state in the monitor table

	for _, status := range in.List {

		if in.Version > 2 {
			ticketOk, err := srv.tokens.Validate(monitor.ID, bidb, status.GetIP(), status.Ticket)
			if err != nil || !ticketOk {
				log.Printf("monitor %d signature validation failed for %q %s", monitor.ID, status.GetIP().String(), err)
				return nil, fmt.Errorf("signature validation failed")
			}
		}

		err = srv.processStatus(ctx, monitor, status)
		if err != nil {
			log.Printf("error processing status %+v: %s", status, err)
			return nil, err
		}
	}

	rv := &pb.ServerStatusResult{}
	rv.Ok = true

	return rv, nil
}

func (srv *Server) processStatus(ctx context.Context, monitor *ntpdb.Monitor, status *pb.ServerStatus) error {

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

	// todo:
	//   skip if there's no server_score

	// todo: rate limit how often each monitor can upload results for a server
	//   check valid ticket
	//   check timestamp on the server score

	hasMaxScore := false
	maxScore := 0.0
	step := 0.0

	if status.Stratum == 0 || status.NoResponse {
		step = -5
	} else {
		offsetAbs := status.AbsoluteOffset()
		if *offsetAbs > 3*time.Second || status.Stratum >= 8 {
			step = -4
			if *offsetAbs > 3*time.Second {
				hasMaxScore = true
				maxScore = -20
			}
		} else if *offsetAbs > 750*time.Millisecond {
			step = -2
		} else if *offsetAbs > 75*time.Millisecond {
			step = -4*offsetAbs.Seconds() + 1
		} else {
			step = 1
		}
	}

	ts := sql.NullTime{Time: status.TS.AsTime(), Valid: true}

	server.ScoreRaw = (server.ScoreRaw * 0.95) + step
	if hasMaxScore {
		server.ScoreRaw = math.Min(server.ScoreRaw, maxScore)
	}
	db.UpdateServer(ctx, ntpdb.UpdateServerParams{
		ID:       server.ID,
		ScoreTs:  ts,
		ScoreRaw: server.ScoreRaw,
	})

	serverScore.ScoreRaw = (serverScore.ScoreRaw * 0.95) + step
	if hasMaxScore {
		serverScore.ScoreRaw = math.Min(serverScore.ScoreRaw, maxScore)
	}
	db.UpdateServerScore(ctx, ntpdb.UpdateServerScoreParams{
		ID:       serverScore.ID,
		ScoreTs:  ts,
		ScoreRaw: serverScore.ScoreRaw,
	})

	if status.Stratum > 0 {
		nullStratum := sql.NullInt32{Int32: status.Stratum, Valid: true}
		db.UpdateServerStratum(ctx, ntpdb.UpdateServerStratumParams{
			ID:      server.ID,
			Stratum: nullStratum,
		})
		db.UpdateServerScoreStratum(ctx, ntpdb.UpdateServerScoreStratumParams{
			ID:      serverScore.ID,
			Stratum: nullStratum,
		})
	}

	attributeStr := sql.NullString{}

	if status.Leap > 0 || len(status.Error) > 0 {
		log.Printf("Got attributes! %+v", status)
		attributes := ntpdb.LogScoreAttributes{
			Leap:  int8(status.Leap),
			Error: status.Error,
		}
		b, err := json.Marshal(attributes)
		if err != nil {
			return err
		}
		log.Printf("attribute JSON for %d %s", server.ID, b)
		attributeStr.String = string(b)
		attributeStr.Valid = true
	}
	// TODO:
	// for my $a (qw(leap error warning)) {
	// 	$log_score{attributes}->{$a} = $status->{$a}
	// 	  if $status->{$a};
	// }

	ls := ntpdb.InsertLogScoreParams{
		ServerID:   server.ID,
		Ts:         ts.Time,
		Step:       step,
		Offset:     sql.NullFloat64{Float64: status.Offset.AsDuration().Seconds(), Valid: true},
		Rtt:        sql.NullInt32{Int32: int32(status.RTT.AsDuration().Microseconds()), Valid: true},
		Score:      server.ScoreRaw,
		Attributes: attributeStr,
	}

	err = db.InsertLogScore(ctx, ls)
	if err != nil {
		return err
	}

	ls.Score = serverScore.ScoreRaw
	ls.MonitorID = sql.NullInt32{Int32: monitor.ID, Valid: true}
	err = db.InsertLogScore(ctx, ls)
	if err != nil {
		return err
	}

	// todo:
	//   if NoResponse == true OR score is low and step == 1:
	//      mark for traceroute if it's not been done recently
	//      maybe track why we traceroute'd last?
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
