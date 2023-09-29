package score

import "go.ntppool.org/monitor/ntpdb"

type ScoreAttributes struct {
	ntpdb.LogScoreAttributes
}

type Score struct {
	ntpdb.LogScore

	HasMaxScore bool
	MaxScore    float64
}

func (s *Score) AsLogScore() *ntpdb.LogScore {
	return nil
}
