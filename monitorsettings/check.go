package monitorsettings

import (
	"fmt"
	"maps"
	"slices"

	shared "go.ntppool.org/common/monitorsettings"
)

// Severity says whether a Finding is an error or a warning.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Finding is one problem with the stored settings.
type Finding struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Derived are values that follow from the settings the selector uses.
type Derived struct {
	// CandidateMaxSamples is the most samples a candidate gets in
	// candidate_window at interval_candidate; CandidateRequired is the
	// count it needs to be promoted to testing.
	CandidateMaxSamples int64 `json:"candidate_max_samples"`
	CandidateRequired   int64 `json:"candidate_required"`

	// TestingMaxSamples and TestingRequired are the same for testing to
	// active, at interval_testing over sample_window.
	TestingMaxSamples int64 `json:"testing_max_samples"`
	TestingRequired   int64 `json:"testing_required"`

	// TrialsAtOnce is how many trials run at once on a server:
	// candidate_window / trial_every.
	TrialsAtOnce int `json:"trials_at_once"`

	// NewServerCandidates is how many candidates a new server starts
	// with: candidate_pool + TrialsAtOnce.
	NewServerCandidates int `json:"new_server_candidates"`
}

// Derive computes the derived values of s, the settings after
// WithDefaults.
func Derive(s MonitorSettings) Derived {
	return Derived{
		CandidateMaxSamples: MaxSamplesInWindow(s.IntervalCandidate.Duration, s.CandidateWindow.Duration),
		CandidateRequired:   RequiredCountForTesting(s.IntervalCandidate.Duration, s.CandidateWindow.Duration),
		TestingMaxSamples:   MaxSamplesInWindow(s.IntervalTesting.Duration, s.SampleWindow.Duration),
		TestingRequired:     RequiredCountForActive(s.IntervalTesting.Duration, s.SampleWindow.Duration),
		TrialsAtOnce:        shared.TrialsAtOnce(s.TrialEvery.Duration, s.CandidateWindow.Duration),
		NewServerCandidates: shared.NewServerCandidates(s.CandidatePool, s.TrialEvery.Duration, s.CandidateWindow.Duration),
	}
}

// Check reports the problems of the stored settings. fields are from
// shared.Inspect, s is the settings after WithDefaults, and servers is the
// number of servers per IP version (nil when not known).
func Check(fields []shared.Field, s MonitorSettings, servers map[string]int64) []Finding {
	// not nil, so `db settings --json` prints [] without findings
	findings := []Finding{}
	add := func(sev Severity, format string, args ...any) {
		findings = append(findings, Finding{Severity: sev, Message: fmt.Sprintf(format, args...)})
	}

	for _, f := range fields {
		switch {
		case f.Problem == "":
		case f.Problem == shared.ProblemUnknownKey:
			add(SeverityWarning, "%s is not a known key and is ignored", f.Key)
		case f.Refused():
			msg := fmt.Sprintf("%s is %s: adding servers and activating monitors fail", f.Key, f.Problem)
			if f.Defaulted {
				msg += fmt.Sprintf("; the selector uses the default %s", f.Value)
			}
			add(SeverityError, "%s", msg)
		case f.Defaulted:
			add(SeverityWarning, "%s is %s: the selector uses the default %s", f.Key, f.Problem, f.Value)
		default:
			add(SeverityWarning, "%s is %s", f.Key, f.Problem)
		}
	}

	d := Derive(s)
	if d.CandidateRequired > d.CandidateMaxSamples {
		add(SeverityError,
			"a candidate gets at most %d samples in candidate_window %s at interval_candidate %s and needs %d: no candidate is promoted to testing",
			d.CandidateMaxSamples, shared.FormatDuration(s.CandidateWindow.Duration), shared.FormatDuration(s.IntervalCandidate.Duration), d.CandidateRequired)
	}
	if d.TestingRequired > d.TestingMaxSamples {
		add(SeverityError,
			"a testing monitor gets at most %d samples in sample_window %s at interval_testing %s and needs %d: no testing monitor is promoted to active",
			d.TestingMaxSamples, shared.FormatDuration(s.SampleWindow.Duration), shared.FormatDuration(s.IntervalTesting.Duration), d.TestingRequired)
	}

	for _, v := range slices.Sorted(maps.Keys(servers)) {
		if n := servers[v]; int64(s.NewMonitorServers) > n {
			add(SeverityWarning, "new_monitor_servers %d is more than the %d %s servers", s.NewMonitorServers, n, v)
		}
	}
	return findings
}
