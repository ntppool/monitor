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

// server.ScoreRaw = (server.ScoreRaw * 0.95) + score.Step
// if score.HasMaxScore {
// 	server.ScoreRaw = math.Min(server.ScoreRaw, score.MaxScore)
// }
// db.UpdateServer(ctx, ntpdb.UpdateServerParams{
// 	ID:       server.ID,
// 	ScoreTs:  sql.NullTime{Time: score.Ts, Valid: true},
// 	ScoreRaw: server.ScoreRaw,
// })

// if status.Stratum > 0 {
// 	nullStratum := sql.NullInt32{Int32: status.Stratum, Valid: true}
// 	db.UpdateServerStratum(ctx, ntpdb.UpdateServerStratumParams{
// 		ID:      server.ID,
// 		Stratum: nullStratum,
// 	})
// }
