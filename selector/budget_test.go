package selector

import (
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

// unlimitedBudget is for tests that exercise other rules: bootstrap mode
// does not limit promotions.
func unlimitedBudget() *promotionBudget {
	return newPromotionBudget(ntpdb.GetServerPromotionCountsRow{}, serverMode{Bootstrap: true}, 3)
}

func TestPromotionBudgetAllow(t *testing.T) {
	const (
		testing_ = ntpdb.ServerScoresStatusTesting
		active   = ntpdb.ServerScoresStatusActive
	)

	tests := []struct {
		name   string
		mode   serverMode
		counts ntpdb.GetServerPromotionCountsRow
		to     ntpdb.ServerScoresStatus
		kind   promotionKind
		want   bool
	}{
		{"normal, count 0, performance", serverMode{}, ntpdb.GetServerPromotionCountsRow{}, testing_, promotionPerformance, true},
		{"normal, count 1, performance", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 1}, testing_, promotionPerformance, false},
		{"normal, count 2, health", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 2}, testing_, promotionHealth, true},
		{"normal, count 3, fill", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 3}, testing_, promotionFill, false},
		{"normal, count 3, health", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 3}, testing_, promotionHealth, false},
		{"bootstrap, count 9, performance", serverMode{Bootstrap: true}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 9}, testing_, promotionPerformance, true},
		{"looks bad, count 0, health", serverMode{LooksBad: true}, ntpdb.GetServerPromotionCountsRow{}, testing_, promotionHealth, true},
		{"looks bad, count 1, fill", serverMode{LooksBad: true}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 1}, testing_, promotionFill, false},
		{"looks bad and bootstrap, count 1, fill", serverMode{LooksBad: true, Bootstrap: true}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 1}, testing_, promotionFill, false},
		{"into testing 3 does not limit active", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoTesting: 3}, active, promotionFill, true},
		{"into active 3 does not limit testing", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoActive: 3}, testing_, promotionFill, true},
		{"into active 1, performance", serverMode{}, ntpdb.GetServerPromotionCountsRow{IntoActive: 1}, active, promotionPerformance, false},
		{"candidate is not a promotion target", serverMode{Bootstrap: true}, ntpdb.GetServerPromotionCountsRow{}, ntpdb.ServerScoresStatusCandidate, promotionFill, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPromotionBudget(tt.counts, tt.mode, 3)
			if got := b.allow(tt.to, tt.kind); got != tt.want {
				t.Errorf("allow(%s, %s) = %v, want %v", tt.to, tt.kind, got, tt.want)
			}
		})
	}
}

// A promotion planned earlier in the review counts for the next one.
func TestPromotionBudgetRecord(t *testing.T) {
	const (
		testing_ = ntpdb.ServerScoresStatusTesting
		active   = ntpdb.ServerScoresStatusActive
	)

	b := newPromotionBudget(ntpdb.GetServerPromotionCountsRow{}, serverMode{}, 3)
	if !b.allow(testing_, promotionPerformance) {
		t.Fatal("first performance promotion into testing refused")
	}
	b.record(testing_)
	if b.allow(testing_, promotionPerformance) {
		t.Error("second performance promotion into testing allowed in the same review")
	}
	if !b.allow(active, promotionPerformance) {
		t.Error("a promotion into testing limited a promotion into active")
	}

	// fills continue up to the limit
	b = newPromotionBudget(ntpdb.GetServerPromotionCountsRow{IntoTesting: 1}, serverMode{}, 3)
	for i := 0; i < 2; i++ {
		if !b.allow(testing_, promotionFill) {
			t.Fatalf("fill %d refused", i)
		}
		b.record(testing_)
	}
	if b.allow(testing_, promotionFill) {
		t.Error("third fill allowed with 1 in the log and 2 planned")
	}
}

func TestPromotionBudgetRefused(t *testing.T) {
	b := newPromotionBudget(ntpdb.GetServerPromotionCountsRow{IntoTesting: 5, IntoActive: 5}, serverMode{}, 3)

	for i := 0; i < 3; i++ {
		b.allow(ntpdb.ServerScoresStatusTesting, promotionFill)
	}
	b.allow(ntpdb.ServerScoresStatusTesting, promotionPerformance)
	b.allow(ntpdb.ServerScoresStatusActive, promotionFill)

	want := []refusal{
		{ntpdb.ServerScoresStatusTesting, promotionFill},
		{ntpdb.ServerScoresStatusTesting, promotionPerformance},
		{ntpdb.ServerScoresStatusActive, promotionFill},
	}
	got := b.refused()
	if len(got) != len(want) {
		t.Fatalf("refused() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("refused()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestClassifyServer(t *testing.T) {
	// n active rows, the first `unhealthy` of the ones with samples unhealthy
	rows := func(active, withSamples, unhealthy int) []evaluatedMonitor {
		var out []evaluatedMonitor
		for i := 0; i < active; i++ {
			m := monitorCandidate{ID: int64(i + 1), ServerStatus: ntpdb.ServerScoresStatusActive}
			if i < withSamples {
				m.HasMetrics = true
				m.IsHealthy = i >= unhealthy
			}
			out = append(out, evaluatedMonitor{monitor: m})
		}
		return out
	}

	tests := []struct {
		name string
		rows []evaluatedMonitor
		want serverMode
	}{
		{"no rows", nil, serverMode{Bootstrap: true}},
		{"4 active", rows(4, 4, 0), serverMode{Bootstrap: true}},
		{"5 active", rows(5, 5, 0), serverMode{}},
		{"7 active, 4 of 7 unhealthy", rows(7, 7, 4), serverMode{LooksBad: true}},
		{"7 active, 3 of 7 unhealthy", rows(7, 7, 3), serverMode{}},
		// 2 unhealthy of the 5 with samples; the 2 without samples count for neither
		{"7 active, 2 without samples", rows(7, 5, 2), serverMode{}},
		{"7 active, none with samples", rows(7, 0, 0), serverMode{}},
		{"4 active, all unhealthy", rows(4, 4, 4), serverMode{Bootstrap: true, LooksBad: true}},
		{
			"testing and candidate rows are not counted",
			append(rows(7, 7, 0), evaluatedMonitor{monitor: monitorCandidate{
				ID: 100, ServerStatus: ntpdb.ServerScoresStatusTesting, HasMetrics: true,
			}}),
			serverMode{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyServer(tt.rows); got != tt.want {
				t.Errorf("classifyServer() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
