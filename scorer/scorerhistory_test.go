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

	first := time.Now().Add(-20 * time.Minute)
	ts1 := first.Add(0 * time.Minute)
	ts2 := first.Add(1 * time.Minute)
	ts3 := first.Add(10 * time.Minute)
	ts4 := first.Add(11 * time.Minute)

	ls := &ntpdb.LogScore{ServerID: 1, Score: 19.8022327058068}

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

	ls.Ts = ts4
	ls.Score = 19.8022327058
	new = sm.IsNew(ls)
	if !new {
		t.Fatalf("fourth test should be 'new' (different score)")
	}

}
