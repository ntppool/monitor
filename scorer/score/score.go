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

// if status.Stratum > 0 {
// 	nullStratum := sql.NullInt32{Int32: status.Stratum, Valid: true}
// 	db.UpdateServerStratum(ctx, ntpdb.UpdateServerStratumParams{
// 		ID:      server.ID,
// 		Stratum: nullStratum,
// 	})
// }
