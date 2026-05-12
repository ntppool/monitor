package scorer

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.ntppool.org/monitor/ntpdb"
)

func TestLastScore(t *testing.T) {
	sm := &ScorerMap{
		lastScore: map[int]*lastUpdate{},
	}

	first := time.Now().Add(-20 * time.Minute)
	ts1 := first.Add(0 * time.Minute)
	ts2 := first.Add(1 * time.Minute)
	ts3 := first.Add(15 * time.Minute)
	ts4 := first.Add(20 * time.Minute)

	ls := &ntpdb.LogScore{ServerID: 1, Score: 19.8022327058068}

	ls.Ts = pgtype.Timestamptz{Time: ts1, Valid: true}
	new := sm.IsNew(ls)
	if !new {
		t.Fatalf("first test should be 'new'")
	}

	ls.Ts = pgtype.Timestamptz{Time: ts2, Valid: true}
	new = sm.IsNew(ls)
	if new {
		t.Fatalf("second test should not be 'new' (too recent)")
	}

	ls.Ts = pgtype.Timestamptz{Time: ts3, Valid: true}
	new = sm.IsNew(ls)
	if !new {
		t.Fatalf("third test should be 'new' (15 minutes later)")
	}

	ls.Ts = pgtype.Timestamptz{Time: ts4, Valid: true}
	ls.Score = 19.8022327058
	new = sm.IsNew(ls)
	if !new {
		t.Fatalf("fourth test should be 'new' (different score)")
	}
}

func TestScorerMap_SkipIteration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		durable    map[int]*lastUpdate
		batchLocal map[int]lastUpdate
		ls         ntpdb.LogScore
		want       bool
	}{
		{
			name:    "near-realtime never skips",
			durable: map[int]*lastUpdate{1: {ts: now.Add(-3 * time.Minute), score: 20.0}},
			ls:      ntpdb.LogScore{ServerID: 1, Score: 20.0, Ts: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true}},
			want:    false,
		},
		{
			name: "no prior compute does not skip",
			ls:   ntpdb.LogScore{ServerID: 1, Score: 20.0, Ts: pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true}},
			want: false,
		},
		{
			name:    "mild lag, close score, within 5 min: skip",
			durable: map[int]*lastUpdate{1: {ts: now.Add(-32 * time.Minute), score: 20.0}},
			ls:      ntpdb.LogScore{ServerID: 1, Score: 20.1, Ts: pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true}},
			want:    true,
		},
		{
			name:    "mild lag, close score, beyond 5 min window: do not skip",
			durable: map[int]*lastUpdate{1: {ts: now.Add(-36 * time.Minute), score: 20.0}},
			ls:      ntpdb.LogScore{ServerID: 1, Score: 20.0, Ts: pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true}},
			want:    false,
		},
		{
			name:    "mild lag, score drift >5%: do not skip",
			durable: map[int]*lastUpdate{1: {ts: now.Add(-32 * time.Minute), score: 20.0}},
			ls:      ntpdb.LogScore{ServerID: 1, Score: 25.0, Ts: pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true}},
			want:    false,
		},
		{
			name:    "heavy lag, close score within 18 min window: skip",
			durable: map[int]*lastUpdate{1: {ts: now.Add(-135 * time.Minute), score: 20.0}},
			ls:      ntpdb.LogScore{ServerID: 1, Score: 20.0, Ts: pgtype.Timestamptz{Time: now.Add(-120 * time.Minute), Valid: true}},
			want:    true,
		},
		{
			name:    "heavy lag, beyond 18 min window: do not skip",
			durable: map[int]*lastUpdate{1: {ts: now.Add(-145 * time.Minute), score: 20.0}},
			ls:      ntpdb.LogScore{ServerID: 1, Score: 20.0, Ts: pgtype.Timestamptz{Time: now.Add(-120 * time.Minute), Valid: true}},
			want:    false,
		},
		{
			// Within a batch, batchLocal entries take precedence over the
			// stale durable entry so subsequent ls's for the same server
			// dedup against the most recent in-batch compute.
			name:       "batchLocal takes precedence over durable",
			durable:    map[int]*lastUpdate{1: {ts: now.Add(-2 * time.Hour), score: 20.0}},
			batchLocal: map[int]lastUpdate{1: {ts: now.Add(-31 * time.Minute), score: 20.0}},
			ls:         ntpdb.LogScore{ServerID: 1, Score: 20.1, Ts: pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true}},
			want:       true, // would be false if measured from the 2h-old durable entry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &ScorerMap{
				lastScore:    map[int]*lastUpdate{},
				lastComputed: map[int]*lastUpdate{},
			}
			for k, v := range tt.durable {
				sm.lastComputed[k] = v
			}
			batchLocal := map[int]lastUpdate{}
			for k, v := range tt.batchLocal {
				batchLocal[k] = v
			}
			got := sm.skipIteration(&tt.ls, batchLocal)
			if got != tt.want {
				t.Errorf("skipIteration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScorerMap_SkipIteration_DoesNotMutate(t *testing.T) {
	now := time.Now()
	prevTs := now.Add(-32 * time.Minute)
	sm := &ScorerMap{
		lastScore:    map[int]*lastUpdate{},
		lastComputed: map[int]*lastUpdate{1: {ts: prevTs, score: 20.0}},
	}
	batchLocal := map[int]lastUpdate{}
	ls := ntpdb.LogScore{ServerID: 1, Score: 20.1, Ts: pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true}}

	_ = sm.skipIteration(&ls, batchLocal)

	if got := sm.lastComputed[1]; !got.ts.Equal(prevTs) || got.score != 20.0 {
		t.Errorf("skipIteration mutated lastComputed; got %+v", got)
	}
	if len(batchLocal) != 0 {
		t.Errorf("skipIteration mutated batchLocal: %+v", batchLocal)
	}
}

func TestScorerMap_CommitComputed(t *testing.T) {
	preExistingTs := time.Now().Add(-3 * time.Hour)
	sm := &ScorerMap{
		lastScore: map[int]*lastUpdate{},
		lastComputed: map[int]*lastUpdate{
			7: {ts: preExistingTs, score: 99.0},
		},
	}
	batchTs := time.Now().Add(-30 * time.Minute)
	batchLocal := map[int]lastUpdate{
		42: {ts: batchTs, score: 20.5}, // new entry
		7:  {ts: batchTs, score: 50.0}, // overwrite existing
	}

	sm.commitComputed(batchLocal)

	got42, ok := sm.lastComputed[42]
	if !ok || !got42.ts.Equal(batchTs) || got42.score != 20.5 {
		t.Errorf("server 42: got %+v, want ts=%v score=20.5", got42, batchTs)
	}
	got7 := sm.lastComputed[7]
	if !got7.ts.Equal(batchTs) || got7.score != 50.0 {
		t.Errorf("server 7 not overwritten: got %+v", got7)
	}
}
