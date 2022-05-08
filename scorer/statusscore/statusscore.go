package statusscore

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

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
		log.Printf("Got attributes! %+v", status)
		attributes := ntpdb.LogScoreAttributes{
			Leap:  int8(status.Leap),
			Error: status.Error,
		}
		b, err := json.Marshal(attributes)
		if err != nil {
			log.Printf("could not marshal attributes %+v: %s", attributes, err)
		}
		// log.Printf("attribute JSON for %d %s", server.ID, b)
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

	if status.Stratum == 0 || status.NoResponse {
		step = -5
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
