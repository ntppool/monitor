package selector

import (
	"go.ntppool.org/monitor/ntpdb"
)

// promotionKind says why a monitor is promoted. The daily promotion budget
// treats the kinds differently.
type promotionKind uint8

const (
	// promotionFill promotes into a free slot.
	promotionFill promotionKind = iota
	// promotionHealth replaces a monitor that is not healthy.
	promotionHealth
	// promotionPerformance replaces a healthy monitor with a better one.
	promotionPerformance
)

func (k promotionKind) String() string {
	switch k {
	case promotionFill:
		return "fill"
	case promotionHealth:
		return "health"
	case promotionPerformance:
		return "performance"
	default:
		return "unknown"
	}
}

// serverMode is how the selector sees a server when it limits promotions.
type serverMode struct {
	// Bootstrap is set when the server has few active monitors; promotions
	// are not limited then.
	Bootstrap bool
	// LooksBad is set when most of the active monitors that report
	// samples are unhealthy. Something is wrong with the server, not the
	// monitors, so replacing monitors is pointless.
	LooksBad bool
}

// classifyServer decides the mode from all rows of the server. Bootstrap
// counts every active row; LooksBad only looks at the active rows with
// samples, rows without samples don't count either way.
func classifyServer(evaluated []evaluatedMonitor) serverMode {
	var active, sampled, unhealthy int
	for _, em := range evaluated {
		if em.monitor.ServerStatus != ntpdb.ServerScoresStatusActive {
			continue
		}
		active++
		if !em.monitor.HasMetrics {
			continue
		}
		sampled++
		if !em.monitor.IsHealthy {
			unhealthy++
		}
	}
	return serverMode{
		Bootstrap: active <= BootstrapModeLimit,
		LooksBad:  unhealthy*2 > sampled,
	}
}

// refusal is a promotion the budget refused. There is one per pair in a
// review, however often it was refused.
type refusal struct {
	to   ntpdb.ServerScoresStatus
	kind promotionKind
}

// promotionBudget limits the promotions across each status boundary on one
// server. It starts from the promotions in the status log and counts the
// ones planned in this review, so a second promotion in the same review
// sees the first.
//
// Per boundary, the first row that matches decides:
//
//	server looks bad                       allowed when the count is 0
//	bootstrap mode                         not limited
//	performance replacement                allowed when the count is 0
//	health replacement or fill             allowed below healthLimit
//
// Demotions are never limited, and don't count.
type promotionBudget struct {
	mode        serverMode
	healthLimit int

	intoTesting int64
	intoActive  int64

	refusals []refusal
}

func newPromotionBudget(counts ntpdb.GetServerPromotionCountsRow, mode serverMode, healthLimit int) *promotionBudget {
	return &promotionBudget{
		mode:        mode,
		healthLimit: healthLimit,
		intoTesting: counts.IntoTesting,
		intoActive:  counts.IntoActive,
	}
}

// promotionBudgetFor returns the budget for one review of a server: counts
// are the promotions in the status log, evaluated are all rows of the server.
func (sl *Selector) promotionBudgetFor(counts ntpdb.GetServerPromotionCountsRow, evaluated []evaluatedMonitor) *promotionBudget {
	return newPromotionBudget(counts, classifyServer(evaluated), sl.currentSettings().HealthPromotions)
}

// counter returns the count of promotions into to, or nil for statuses that
// aren't a promotion target.
func (b *promotionBudget) counter(to ntpdb.ServerScoresStatus) *int64 {
	switch to {
	case ntpdb.ServerScoresStatusTesting:
		return &b.intoTesting
	case ntpdb.ServerScoresStatusActive:
		return &b.intoActive
	}
	return nil
}

// allow reports whether a promotion into to is within the budget. A refusal
// is remembered for refused().
func (b *promotionBudget) allow(to ntpdb.ServerScoresStatus, kind promotionKind) bool {
	counter := b.counter(to)
	if counter == nil {
		return false
	}
	n := *counter

	var allowed bool
	switch {
	case b.mode.LooksBad:
		allowed = n < 1
	case b.mode.Bootstrap:
		allowed = true
	case kind == promotionPerformance:
		allowed = n < 1
	default:
		allowed = n < int64(b.healthLimit)
	}

	if !allowed {
		r := refusal{to: to, kind: kind}
		for _, seen := range b.refusals {
			if seen == r {
				return false
			}
		}
		b.refusals = append(b.refusals, r)
	}
	return allowed
}

// record counts a planned promotion into to.
func (b *promotionBudget) record(to ntpdb.ServerScoresStatus) {
	if n := b.counter(to); n != nil {
		*n++
	}
}

// refused returns the promotions the budget refused, once per pair.
func (b *promotionBudget) refused() []refusal {
	return b.refusals
}
