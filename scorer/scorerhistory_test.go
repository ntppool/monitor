package scorer

import (
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

func TestLastScore(t *testing.T) {

	sm := &ScorerMap{
		lastScore: map[int]*lastUpdate{},
	}

	ts3 := time.Now()
	ts1 := ts3.Add(-15 * time.Minute)
	ts2 := ts1.Add(1 * time.Minute)

	ls := &ntpdb.LogScore{ServerID: 1, Score: 10}

	ls.Ts = ts1
	new := sm.IsNew(ls)
	if !new {
		t.Fatalf("first test should be 'new'")
	}

	ls.Ts = ts2
	new = sm.IsNew(ls)
	if new {
		t.Fatalf("second test should not be 'new' (too recent)")
	}

	ls.Ts = ts3
	new = sm.IsNew(ls)
	if !new {
		t.Fatalf("third test should be 'new' (15 minutes later)")
	}

}
