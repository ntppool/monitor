package blend

import "go.ntppool.org/monitor/ntpdb"

type BlendScore struct {
}

func (s *BlendScore) Score(ls []*ntpdb.LogScore) ([]*ntpdb.LogScore, error) {
	return nil, nil
}
