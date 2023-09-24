package statusscore

import (
	"database/sql"
	"encoding/json"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
)

type StatusScorer struct{}

func NewScorer() *StatusScorer {
	return &StatusScorer{}
}

func (s *StatusScorer) Score(server *ntpdb.Server, status *pb.ServerStatus) (*score.Score, error) {
	score, err := s.calc(server, status)
	return score, err
}

func (s *StatusScorer) calc(server *ntpdb.Server, status *pb.ServerStatus) (*score.Score, error) {

	attributeStr := sql.NullString{}

	if status.Leap > 0 || len(status.Error) > 0 {
		log := logger.Setup()
		log.Debug("Got attributes", "status", status)
		attributes := ntpdb.LogScoreAttributes{
			Leap:  int8(status.Leap),
			Error: status.Error,
		}
		b, err := json.Marshal(attributes)
		if err != nil {
			log.Warn("could not marshal attributes", "attributes", attributes, "err", err)
		}
		attributeStr.String = string(b)
		attributeStr.Valid = true
	}

	sc := score.Score{}

	sc.ServerID = server.ID
	sc.Ts = status.TS.AsTime()
	sc.Offset = sql.NullFloat64{Float64: status.Offset.AsDuration().Seconds(), Valid: true}
	sc.Rtt = sql.NullInt32{Int32: int32(status.RTT.AsDuration().Microseconds()), Valid: true}
	sc.Attributes = attributeStr

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
	} else if len(status.Error) > 0 {
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
		} else if *offsetAbs > 75*time.Millisecond {
			step = -4*offsetAbs.Seconds() + 1
		} else {
			step = 1
		}
	}

	sc.Step = step

	return &sc, nil
}
