package statusscore

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go.ntppool.org/common/logger"

	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
)

type StatusScorer struct{}

func NewScorer() *StatusScorer {
	return &StatusScorer{}
}

func (s *StatusScorer) Score(ctx context.Context, server *ntpdb.Server, status *apiv2.ServerStatus) (*score.Score, error) {
	score, err := s.calc(ctx, server, status)
	return score, err
}

// rttColumn converts the submitted RTT to microseconds for log_scores.rtt.
//
// Monitor clients aren't trusted, and a real NTP round trip is never zero or
// negative, nor does it need more than an int32 of microseconds. The NTP
// library clamps a negative RTT to zero, so zero on a response is that same
// bad measurement: the server claims to have spent longer processing the
// request than the whole round trip took. Those store NULL and are logged,
// and are reported as implausible so the caller can treat the sample as a
// bad host. A timeout has no RTT either, and stores NULL quietly.
func rttColumn(ctx context.Context, log *slog.Logger, server *ntpdb.Server, status *apiv2.ServerStatus) (col pgtype.Int4, implausible bool) {
	if status.NoResponse {
		return pgtype.Int4{}, false
	}

	if status.Rtt == nil {
		log.WarnContext(ctx, "response status has no rtt", "server_id", server.ID)
		return pgtype.Int4{}, false
	}

	rtt := status.Rtt.AsDuration()
	us := rtt.Microseconds()
	if us <= 0 || us > math.MaxInt32 {
		log.WarnContext(ctx, "discarding implausible rtt", "server_id", server.ID, "rtt", rtt)
		return pgtype.Int4{}, true
	}

	return pgtype.Int4{Int32: int32(us), Valid: true}, false
}

func (s *StatusScorer) calc(ctx context.Context, server *ntpdb.Server, status *apiv2.ServerStatus) (*score.Score, error) {
	log := logger.FromContext(ctx)

	sc := score.Score{}

	sc.ServerID = server.ID
	sc.Ts = pgtype.Timestamptz{Time: status.Ts.AsTime(), Valid: true}

	var implausibleRtt bool
	sc.Rtt, implausibleRtt = rttColumn(ctx, log, server, status)
	if implausibleRtt && status.Stratum != 0 && status.Error == "" {
		// The timestamps are inconsistent with the round trip, so the offset
		// computed from them can't be trusted either. Same as a client
		// reporting the error itself. Stratum 0 keeps its kiss code handling.
		status.Error = "implausible rtt"
		status.Offset = nil
	}

	if status.Offset != nil {
		sc.Offset = pgtype.Float8{Float64: status.Offset.AsDuration().Seconds(), Valid: true}
	} else {
		sc.Offset = pgtype.Float8{Valid: false}
	}

	sc.HasMaxScore = false

	step := 0.0

	if status.NoResponse {
		step = -5
	} else if status.Stratum == 0 && status.Error == "RATE" {
		step = -3.5
	} else if status.Stratum == 0 && (status.Error == "RSTR" || status.Error == "DENY") {
		step = -10
		sc.HasMaxScore = true
		sc.MaxScore = -50
	} else if status.Stratum == 0 && (status.Error == "" || status.Error == "untrusted zero offset") {
		step = -2
		sc.MaxScore = 5
		sc.HasMaxScore = true
		if status.Error == "" {
			status.Error = "unexpected stratum 0"
		}
	} else if len(status.Error) > 0 || status.Offset == nil {
		step = -4 // what errors would this be that have a response but aren't RATE?
	} else {
		offsetAbs := status.AbsoluteOffset()
		if *offsetAbs > 3*time.Second || status.Stratum >= 8 {
			step = -4
			if *offsetAbs > 3*time.Second {
				sc.HasMaxScore = true
				sc.MaxScore = -20
			}
		} else if *offsetAbs > 750*time.Millisecond {
			step = -2
		} else if *offsetAbs > 25*time.Millisecond {
			offsetSecs := offsetAbs.Seconds()
			if offsetSecs <= 0.100 { // 25ms - 100ms range
				step = -6.667*offsetSecs + 1.167
			} else { // 100ms - 750ms range
				step = -2.308*offsetSecs + 0.731
			}
			// Sanity check: never exceed +1
			if step > 1 {
				step = 1
			}
		} else {
			step = 1
		}
	}

	sc.Step = step

	if status.Leap > 0 || len(status.Error) > 0 {
		log.Debug("Got attributes", "status", status)
		attributes := ntpdb.LogScoreAttributes{
			Leap: int8(status.Leap),
			// jsonb rejects \u0000; the client sends a zero
			// reference ID as NUL bytes in the error.
			Error: strings.ReplaceAll(status.Error, "\x00", ""),
		}
		b, err := json.Marshal(attributes)
		if err != nil {
			log.Warn("could not marshal attributes", "attributes", attributes, "err", err)
		}
		raw := json.RawMessage(b)
		sc.Attributes = &raw
	}

	return &sc, nil
}
