package selector

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

// maxReserveMovesPerReview limits how many candidates the trial rule moves
// to reserve in one review of a server (trial rules 1-3). A server that
// has many candidates past their trial is brought down to the pool over a
// few reviews; it is a change-limit group of its own.
const maxReserveMovesPerReview = 20

// Reasons for the status changes of the trial rule.
const (
	reasonReserveBlocked = "reserve: blocked"
	reasonTrialEnd       = "trial_end"
	reasonTrialStart     = "trial_start"
)

// trialInput is what the trial rule needs to know about one server.
type trialInput struct {
	// candidates and reserves are the rows of the server in those statuses
	// that no earlier rule of this review changed, candidates ranked best
	// first.
	candidates, reserves []evaluatedMonitor
	// trialStartedRecently is set when a trial started within trial_every.
	trialStartedRecently bool
	now                  time.Time
	// pool is the number of candidates a server keeps once trials end.
	pool int
	// window is the length of a trial.
	window time.Duration
	// minCount is the number of samples a candidate needs over the window to
	// stay.
	minCount int64
	// pick returns a random number in [0, n). Nil means rand.IntN.
	pick func(n int) int
}

// trialInputFor builds the trial rule's input from the rows of a review,
// leaving out the rows changed by the selection rules: a candidate changed
// there is neither moved to reserve nor counted in the pool.
func (sl *Selector) trialInputFor(evaluatedMonitors []evaluatedMonitor, changes []statusChange, trialStartedRecently bool) trialInput {
	var candidates, reserves []evaluatedMonitor
	for _, em := range withoutChanged(evaluatedMonitors, changes) {
		switch em.monitor.ServerStatus {
		case ntpdb.ServerScoresStatusCandidate:
			candidates = append(candidates, em)
		case ntpdb.ServerScoresStatusReserve:
			reserves = append(reserves, em)
		}
	}
	settings := sl.currentSettings()
	return trialInput{
		candidates:           candidates,
		reserves:             reserves,
		trialStartedRecently: trialStartedRecently,
		now:                  time.Now(),
		pick:                 sl.pickReserve,
		pool:                 settings.CandidatePool,
		window:               settings.CandidateWindow.Duration,
		minCount:             sl.minCountForTesting(),
	}
}

// pastTrial reports whether a candidate has been a candidate for a whole
// trial. A row without a status_since is treated as still on trial, so the
// selector never ends a trial it can't date.
func (in trialInput) pastTrial(em evaluatedMonitor) bool {
	since := em.monitor.StatusSince
	return !since.IsZero() && in.now.Sub(since) >= in.window
}

// hasViolation reports whether a promotion of the row to testing would run
// into a constraint violation.
func hasViolation(em evaluatedMonitor) bool {
	return em.testingViolation != nil && em.testingViolation.Type != violationNone
}

// globallyLive reports whether the monitor is active or testing globally.
func globallyLive(em evaluatedMonitor) bool {
	g := em.monitor.GlobalStatus
	return g == ntpdb.MonitorsStatusActive || g == ntpdb.MonitorsStatusTesting
}

// leftToPause reports whether the row has a violation that can't change
// (same account, same subnet as the server); the pause logic pauses such a
// candidate in this review.
func leftToPause(em evaluatedMonitor) bool {
	return hasViolation(em) && isUnchangeableConstraint(em.testingViolation.Type)
}

// trialBlocked reports whether a candidate is blocked by its global status
// or a violation the selector can resolve by moving it. A row left to the
// pause logic isn't.
func trialBlocked(em evaluatedMonitor) bool {
	if hasViolation(em) {
		return !leftToPause(em)
	}
	return !globallyLive(em)
}

// eligibleForTrial reports whether a reserve row can start a trial.
func eligibleForTrial(em evaluatedMonitor) bool {
	return !hasViolation(em) && globallyLive(em)
}

// applyTrialRule moves candidates to reserve and starts trials. Only
// candidates and reserve rows are looked at, and only from the status log
// and the sample counts, so a server with every candidate unhealthy keeps
// its pool.
//
//  1. a candidate that is blocked (global status, changeable violation)
//  2. a candidate past its trial with fewer than minCount samples
//  3. candidates past their trial beyond the pool, last ranked first
//  4. when no trial started within trial_every, one random reserve row
//     becomes a candidate
//
// Rules 1-3 share maxReserveMovesPerReview.
func (sl *Selector) applyTrialRule(ctx context.Context, in trialInput) []statusChange {
	var changes []statusChange
	moved := make(map[int64]bool)

	move := func(em evaluatedMonitor, reason string) {
		moved[em.monitor.ID] = true
		changes = append(changes, statusChange{
			monitorID:  em.monitor.ID,
			fromStatus: ntpdb.ServerScoresStatusCandidate,
			toStatus:   ntpdb.ServerScoresStatusReserve,
			reason:     reason,
		})
	}
	room := func() bool { return len(changes) < maxReserveMovesPerReview }

	// Rule 1
	for _, em := range in.candidates {
		if !room() {
			break
		}
		if trialBlocked(em) {
			move(em, reasonReserveBlocked)
		}
	}

	// Rule 2, and the candidates past their trial that stay for rule 3. A
	// row left to the pause logic is neither moved nor counted in the pool.
	var kept []evaluatedMonitor
	for _, em := range in.candidates {
		if moved[em.monitor.ID] || !in.pastTrial(em) || leftToPause(em) {
			continue
		}
		if em.monitor.Count < in.minCount && room() {
			move(em, reasonTrialEnd)
			continue
		}
		kept = append(kept, em)
	}

	// Rule 3
	for i := len(kept) - 1; i >= in.pool && room(); i-- {
		move(kept[i], reasonTrialEnd)
	}

	// Rule 4
	if !in.trialStartedRecently {
		var eligible []evaluatedMonitor
		for _, em := range in.reserves {
			if eligibleForTrial(em) {
				eligible = append(eligible, em)
			}
		}
		if len(eligible) > 0 {
			pick := in.pick
			if pick == nil {
				pick = rand.IntN
			}
			em := eligible[pick(len(eligible))]
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusReserve,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     reasonTrialStart,
			})
		}
	}

	if len(changes) > 0 {
		sl.log.DebugContext(ctx, "trial rule",
			slog.Int("candidates", len(in.candidates)),
			slog.Int("reserves", len(in.reserves)),
			slog.Int("changes", len(changes)))
	}
	return changes
}
