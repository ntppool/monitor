package selector

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

const (
	testPool     = 30
	testWindow   = 72 * time.Hour
	testMinCount = int64(9)
)

var trialNow = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

type trialOpt func(*evaluatedMonitor)

func onTrial() trialOpt {
	return func(em *evaluatedMonitor) { em.monitor.StatusSince = trialNow.Add(-testWindow + time.Hour) }
}

func withCount(n int64) trialOpt {
	return func(em *evaluatedMonitor) {
		em.monitor.Count = n
		em.monitor.HasMetrics = n > 0
	}
}

func unhealthy() trialOpt {
	return func(em *evaluatedMonitor) { em.monitor.IsHealthy = false }
}

func withViolation(t constraintViolationType) trialOpt {
	return func(em *evaluatedMonitor) { em.testingViolation = &constraintViolation{Type: t} }
}

func globally(s ntpdb.MonitorsStatus) trialOpt {
	return func(em *evaluatedMonitor) { em.monitor.GlobalStatus = s }
}

// trialRow builds a row in status; unless an option says otherwise, it has
// been in the status for longer than the trial, has 18 samples and is
// healthy.
func trialRow(id int64, status ntpdb.ServerScoresStatus, opts ...trialOpt) evaluatedMonitor {
	em := evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           id,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			ServerStatus: status,
			StatusSince:  trialNow.Add(-testWindow - time.Hour),
			HasMetrics:   true,
			IsHealthy:    true,
			Count:        18,
		},
		currentViolation: &constraintViolation{Type: violationNone},
		testingViolation: &constraintViolation{Type: violationNone},
	}
	for _, o := range opts {
		o(&em)
	}
	return em
}

func trialRows(first, n int64, status ntpdb.ServerScoresStatus, opts ...trialOpt) []evaluatedMonitor {
	rows := make([]evaluatedMonitor, 0, n)
	for i := range n {
		rows = append(rows, trialRow(first+i, status, opts...))
	}
	return rows
}

func baseTrialInput(candidates, reserves []evaluatedMonitor) trialInput {
	return trialInput{
		candidates: candidates,
		reserves:   reserves,
		now:        trialNow,
		pool:       testPool,
		window:     testWindow,
		minCount:   testMinCount,
		pick:       func(int) int { return 0 },
	}
}

func TestApplyTrialRule(t *testing.T) {
	const (
		cand = ntpdb.ServerScoresStatusCandidate
		res  = ntpdb.ServerScoresStatusReserve
	)

	tests := []struct {
		name                 string
		candidates, reserves []evaluatedMonitor
		// pool is testPool when 0
		pool int
		// trialDue is set when no trial started within trial_every
		trialDue bool
		// toReserve maps monitor ID to the reason it moves to reserve
		toReserve map[int64]string
		// started maps monitor ID to the reason it moves from reserve to candidate
		started map[int64]string
	}{
		{
			name:       "past trial beyond the pool, last ranked move; one trial starts",
			candidates: trialRows(1, 40, cand),
			reserves:   trialRows(500, 5, res),
			trialDue:   true,
			toReserve:  idsWithReason(31, 40, reasonTrialEnd),
			started:    map[int64]string{500: reasonTrialStart},
		},
		{
			name:       "no trial starts when one started recently",
			candidates: trialRows(1, 40, cand),
			reserves:   trialRows(500, 5, res),
			toReserve:  idsWithReason(31, 40, reasonTrialEnd),
		},
		{
			name:       "a pool at its size keeps every candidate past its trial",
			candidates: trialRows(1, 30, cand),
		},
		{
			name:       "candidates on trial are not counted in the pool",
			candidates: append(trialRows(1, 30, cand), trialRows(100, 6, cand, onTrial())...),
		},
		{
			name: "on trial with no samples stays, past trial with too few samples goes",
			candidates: []evaluatedMonitor{
				trialRow(1, cand, onTrial(), withCount(0)),
				trialRow(2, cand, withCount(0)),
				trialRow(3, cand, withCount(5)),
				trialRow(4, cand),
			},
			pool:      12,
			toReserve: map[int64]string{2: reasonTrialEnd, 3: reasonTrialEnd},
		},
		{
			name:       "every candidate unhealthy keeps the pool",
			candidates: trialRows(1, 25, cand, unhealthy()),
		},
		{
			name: "blocked by a changeable violation or the global status",
			candidates: []evaluatedMonitor{
				trialRow(1, cand, onTrial(), withViolation(violationNetworkDiversity)),
				trialRow(2, cand, onTrial(), globally(ntpdb.MonitorsStatusPaused)),
				trialRow(3, cand, onTrial(), globally(ntpdb.MonitorsStatusPending)),
				trialRow(4, cand, onTrial()),
			},
			toReserve: map[int64]string{1: reasonReserveBlocked, 2: reasonReserveBlocked, 3: reasonReserveBlocked},
		},
		{
			name: "an unchangeable violation is left to the pause logic",
			candidates: []evaluatedMonitor{
				trialRow(1, cand, onTrial(), withViolation(violationNetworkSameSubnet)),
				trialRow(2, cand, withViolation(violationAccount)),
				trialRow(3, cand, withViolation(violationAccount), globally(ntpdb.MonitorsStatusPaused)),
			},
		},
		{
			// The pause logic pauses them in this review: one without
			// samples isn't moved to reserve first, and one ranked first
			// doesn't push the last good candidate out of the pool.
			name: "a row left to the pause logic is neither moved nor counted in the pool",
			candidates: append([]evaluatedMonitor{
				trialRow(98, cand, withViolation(violationNetworkSameSubnet)),
				trialRow(99, cand, withViolation(violationAccount), withCount(0)),
			}, trialRows(1, 30, cand)...),
		},
		{
			name:       "a blocked row is not counted in the pool",
			candidates: append(trialRows(1, 30, cand), trialRow(99, cand, globally(ntpdb.MonitorsStatusPaused))),
			toReserve:  map[int64]string{99: reasonReserveBlocked},
		},
		{
			name: "reserve rows that can't start a trial",
			reserves: []evaluatedMonitor{
				trialRow(500, res, globally(ntpdb.MonitorsStatusPending)),
				trialRow(501, res, withViolation(violationNetworkDiversity)),
				trialRow(503, res),
			},
			trialDue: true,
			started:  map[int64]string{503: reasonTrialStart},
		},
		{
			name:     "no eligible reserve row",
			reserves: []evaluatedMonitor{trialRow(500, res, globally(ntpdb.MonitorsStatusPaused))},
			trialDue: true,
		},
		{
			name:       "a row without status_since is never past its trial",
			candidates: trialRows(1, 40, cand, func(em *evaluatedMonitor) { em.monitor.StatusSince = time.Time{} }),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := &Selector{log: slog.Default()}
			in := baseTrialInput(tt.candidates, tt.reserves)
			in.trialStartedRecently = !tt.trialDue
			if tt.pool != 0 {
				in.pool = tt.pool
			}
			changes := sl.applyTrialRule(context.Background(), in)

			gotReserve := map[int64]string{}
			gotStarted := map[int64]string{}
			for _, c := range changes {
				switch {
				case c.fromStatus == cand && c.toStatus == res:
					gotReserve[c.monitorID] = c.reason
				case c.fromStatus == res && c.toStatus == cand:
					gotStarted[c.monitorID] = c.reason
				default:
					t.Errorf("unexpected change %+v", c)
				}
			}
			if !maps.Equal(gotReserve, tt.toReserve) {
				t.Errorf("moves to reserve = %v, want %v", gotReserve, tt.toReserve)
			}
			if !maps.Equal(gotStarted, tt.started) {
				t.Errorf("trial starts = %v, want %v", gotStarted, tt.started)
			}
		})
	}
}

func idsWithReason(from, to int64, reason string) map[int64]string {
	m := map[int64]string{}
	for id := from; id <= to; id++ {
		m[id] = reason
	}
	return m
}

// Rows an earlier rule changed are left out of the trial rule: neither
// moved, counted in the pool, nor picked for a trial.
func TestTrialInputForLeavesChangedRows(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	rows := append(trialRows(1, 3, ntpdb.ServerScoresStatusCandidate),
		trialRows(500, 2, ntpdb.ServerScoresStatusReserve)...)
	rows = append(rows, trialRow(900, ntpdb.ServerScoresStatusActive))
	changes := []statusChange{{monitorID: 2}, {monitorID: 501}}

	in := sl.trialInputFor(rows, changes, true)

	ids := func(rows []evaluatedMonitor) []int64 {
		var out []int64
		for _, em := range rows {
			out = append(out, em.monitor.ID)
		}
		return out
	}
	if got := ids(in.candidates); !slices.Equal(got, []int64{1, 3}) {
		t.Errorf("candidates = %v, want [1 3]", got)
	}
	if got := ids(in.reserves); !slices.Equal(got, []int64{500}) {
		t.Errorf("reserves = %v, want [500]", got)
	}
	if !in.trialStartedRecently {
		t.Error("trialStartedRecently not passed on")
	}
}

func TestApplyTrialRuleMoveCap(t *testing.T) {
	sl := &Selector{log: slog.Default()}

	// 60 candidates past their trial and 5 blocked: the cap is shared by
	// rules 1-3, and rule 1 goes first.
	rows := append(trialRows(1, 60, ntpdb.ServerScoresStatusCandidate),
		trialRows(100, 5, ntpdb.ServerScoresStatusCandidate, globally(ntpdb.MonitorsStatusPaused))...)
	in := baseTrialInput(rows, trialRows(500, 3, ntpdb.ServerScoresStatusReserve))

	changes := sl.applyTrialRule(context.Background(), in)

	toReserve, started := 0, 0
	blocked := 0
	for _, c := range changes {
		if c.toStatus == ntpdb.ServerScoresStatusReserve {
			toReserve++
			if c.reason == reasonReserveBlocked {
				blocked++
			}
		} else {
			started++
		}
	}
	if toReserve != maxReserveMovesPerReview {
		t.Errorf("%d moves to reserve, want %d", toReserve, maxReserveMovesPerReview)
	}
	if blocked != 5 {
		t.Errorf("%d blocked moves, want 5 (rule 1 goes first)", blocked)
	}
	if started != 1 {
		t.Errorf("%d trial starts, want 1 (not limited by the cap)", started)
	}
}

func TestApplyTrialRuleRandomPick(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	reserves := trialRows(500, 4, ntpdb.ServerScoresStatusReserve)
	reserves[1].monitor.GlobalStatus = ntpdb.MonitorsStatusPaused // not eligible

	var asked []int
	in := baseTrialInput(nil, reserves)
	in.pick = func(n int) int {
		asked = append(asked, n)
		return n - 1
	}
	changes := sl.applyTrialRule(context.Background(), in)

	if len(asked) != 1 || asked[0] != 3 {
		t.Errorf("pick asked with %v, want one call with the 3 eligible rows", asked)
	}
	if len(changes) != 1 || changes[0].monitorID != 503 {
		t.Errorf("changes = %+v, want the last eligible row 503 to start a trial", changes)
	}
}

// The default pick must return rows of the input, never fail.
func TestApplyTrialRuleDefaultPick(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	in := baseTrialInput(nil, trialRows(500, 5, ntpdb.ServerScoresStatusReserve))
	in.pick = nil
	for range 20 {
		changes := sl.applyTrialRule(context.Background(), in)
		if len(changes) != 1 || changes[0].monitorID < 500 || changes[0].monitorID > 504 {
			t.Fatalf("changes = %+v, want one trial start of rows 500-504", changes)
		}
	}
}

func TestRule7UnpausesToReserve(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	paused := trialRow(7, ntpdb.ServerScoresStatusPaused)
	paused.monitor.LastConstraintCheck = nil

	changes := sl.applyRule7ConstraintResolution(context.Background(),
		selectionContext{server: &serverInfo{ID: 1}}, []evaluatedMonitor{paused})

	if len(changes) != 1 {
		t.Fatalf("changes = %+v, want one", changes)
	}
	c := changes[0]
	if c.monitorID != 7 || c.fromStatus != ntpdb.ServerScoresStatusPaused || c.toStatus != ntpdb.ServerScoresStatusReserve {
		t.Errorf("change = %+v, want monitor 7 paused -> reserve", c)
	}
}

// A reserve row takes part in no rule but the trial rule: it is not counted
// as a candidate, and a server with only reserve rows plans only a trial.
func TestSelectionRulesIgnoreReserve(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	server := &serverInfo{ID: 14, AccountID: ptr(int64(9999)), IP: "192.0.2.1", IPVersion: "4"}

	var rows []evaluatedMonitor
	for i := range int64(150) {
		rows = append(rows, sampledMonitor(sl, 5000+i, ntpdb.ServerScoresStatusReserve, true, 500))
	}
	changes := sl.applySelectionRules(context.Background(), rows, server, map[int64]*accountLimit{}, nil,
		sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, rows))
	if len(changes) != 0 {
		t.Errorf("reserve rows alone planned %+v, want nothing (no promotion to testing either)", changes)
	}

	changes = sl.applyTrialRule(context.Background(), sl.trialInputFor(rows, nil, false))
	if len(changes) != 1 || changes[0].reason != reasonTrialStart ||
		changes[0].fromStatus != ntpdb.ServerScoresStatusReserve || changes[0].toStatus != ntpdb.ServerScoresStatusCandidate {
		t.Errorf("changes = %+v, want one trial start", changes)
	}
}

func TestClassifyServerIgnoresReserve(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	var rows []evaluatedMonitor
	for i := range int64(3) {
		rows = append(rows, sampledMonitor(sl, 1+i, ntpdb.ServerScoresStatusActive, true, 500))
	}
	for i := range int64(150) {
		rows = append(rows, sampledMonitor(sl, 5000+i, ntpdb.ServerScoresStatusReserve, false, 500))
	}
	mode := classifyServer(rows)
	if !mode.Bootstrap || mode.LooksBad {
		t.Errorf("mode = %+v, want bootstrap and not looking bad with 3 active rows", mode)
	}
}
