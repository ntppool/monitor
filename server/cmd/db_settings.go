package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"

	shared "go.ntppool.org/common/monitorsettings"

	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/selector"
)

// trialLoadWindow is the window over which trial starts are counted.
const trialLoadWindow = 24 * time.Hour

type dbSettingsCmd struct {
	JSON bool `name:"json" help:"print the report as JSON"`
}

// settingsReport is the output of `db settings`.
type settingsReport struct {
	Settings []settingsField           `json:"settings"`
	Derived  monitorsettings.Derived   `json:"derived"`
	Findings []monitorsettings.Finding `json:"findings"`
	Load     []ipLoad                  `json:"load"`
	used     monitorsettings.MonitorSettings
}

type settingsField struct {
	shared.Field
	// API is "ok" or "refused" for the keys the api requires, empty for
	// the others.
	API string `json:"api,omitempty"`
}

// ipLoad is the load on the selector and the monitors for one IP version.
type ipLoad struct {
	IPVersion        string           `json:"ip_version"`
	Servers          int64            `json:"servers"`
	BootstrapServers int64            `json:"bootstrap_servers"`
	ActiveMonitors   int64            `json:"active_monitors"`
	TestingMonitors  int64            `json:"testing_monitors"`
	Rows             map[string]int64 `json:"rows"`

	CandidatesPerServer struct {
		Avg float64 `json:"avg"`
		P50 int64   `json:"p50"`
		P90 int64   `json:"p90"`
		Max int64   `json:"max"`
	} `json:"candidates_per_server"`

	Trials struct {
		Window   string  `json:"window"`
		Started  int64   `json:"started"`
		Expected float64 `json:"expected"`
	} `json:"trials"`

	Promotions struct {
		Window              string `json:"window"`
		Budget              int    `json:"budget"`
		IntoTesting         int64  `json:"into_testing"`
		IntoTestingMax      int64  `json:"into_testing_max"`
		IntoTestingAtBudget int64  `json:"into_testing_at_budget"`
		IntoActive          int64  `json:"into_active"`
		IntoActiveMax       int64  `json:"into_active_max"`
		IntoActiveAtBudget  int64  `json:"into_active_at_budget"`
	} `json:"promotions"`

	// CandidateProbesPerHour is the candidate probes per active or
	// testing monitor and hour: its candidate rows / interval_candidate.
	CandidateProbesPerHour struct {
		Avg float64 `json:"avg"`
		Max float64 `json:"max"`
	} `json:"candidate_probes_per_hour"`
}

func (cmd *dbSettingsCmd) Run(ctx context.Context, parent *dbCmd) error {
	dbconn, err := ntpdb.OpenDB(ctx, parent.ConfigFile)
	if err != nil {
		return err
	}
	defer dbconn.Close()

	report, err := buildSettingsReport(ctx, ntpdb.New(dbconn))
	if err != nil {
		return err
	}

	if cmd.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else if err := printSettingsReport(os.Stdout, report); err != nil {
		return err
	}

	var failed int
	for _, f := range report.Findings {
		if f.Severity == monitorsettings.SeverityError {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d settings checks failed", failed)
	}
	return nil
}

func buildSettingsReport(ctx context.Context, q ntpdb.Querier) (settingsReport, error) {
	raw, err := q.GetSystemSetting(ctx, shared.SettingsKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return settingsReport{}, fmt.Errorf("reading the %q system setting: %w", shared.SettingsKey, err)
	}
	stored, fields, err := shared.Inspect(raw)
	if err != nil {
		return settingsReport{}, err
	}
	s := stored.WithDefaults()

	load, err := collectLoad(ctx, q, s)
	if err != nil {
		return settingsReport{}, err
	}
	servers := make(map[string]int64, len(load))
	for _, l := range load {
		servers[l.IPVersion] = l.Servers
	}

	r := settingsReport{
		Load:     load,
		Derived:  monitorsettings.Derive(s),
		Findings: monitorsettings.Check(fields, s, servers),
		used:     s,
	}
	for _, f := range fields {
		sf := settingsField{Field: f}
		switch {
		case f.Refused():
			sf.API = "refused"
		case f.Required:
			sf.API = "ok"
		}
		r.Settings = append(r.Settings, sf)
	}
	return r, nil
}

// collectLoad reads the load per IP version, with the parameters of s (the
// settings the selector uses).
func collectLoad(ctx context.Context, q ntpdb.Querier, s monitorsettings.MonitorSettings) ([]ipLoad, error) {
	byVersion := map[string]*ipLoad{}
	get := func(v string) *ipLoad {
		l, ok := byVersion[v]
		if !ok {
			l = &ipLoad{IPVersion: v, Rows: map[string]int64{}}
			l.Trials.Window = shared.FormatDuration(trialLoadWindow)
			l.Promotions.Window = shared.FormatDuration(s.PromotionWindow.Duration)
			l.Promotions.Budget = s.HealthPromotions
			byVersion[v] = l
		}
		return l
	}

	serverRows, err := q.GetSettingsServerLoad(ctx, selector.BootstrapModeLimit)
	if err != nil {
		return nil, fmt.Errorf("counting servers: %w", err)
	}
	for _, row := range serverRows {
		l := get(row.IpVersion)
		l.Servers = row.Servers
		l.BootstrapServers = row.BootstrapServers
		l.CandidatesPerServer.Avg = row.CandidatesAvg
		l.CandidatesPerServer.P50 = row.CandidatesP50
		l.CandidatesPerServer.P90 = row.CandidatesP90
		l.CandidatesPerServer.Max = row.CandidatesMax
		l.Trials.Expected = float64(row.Servers) * trialLoadWindow.Hours() / s.TrialEvery.Hours()
	}

	statusRows, err := q.GetSettingsStatusCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting server_scores rows: %w", err)
	}
	for _, row := range statusRows {
		get(row.IpVersion).Rows[string(row.Status)] = row.Rows
	}

	monitorRows, err := q.GetSettingsMonitorLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting monitors: %w", err)
	}
	for _, row := range monitorRows {
		l := get(row.IpVersion)
		l.ActiveMonitors = row.ActiveMonitors
		l.TestingMonitors = row.TestingMonitors
		l.CandidateProbesPerHour.Avg = row.CandidatesAvg / s.IntervalCandidate.Hours()
		l.CandidateProbesPerHour.Max = float64(row.CandidatesMax) / s.IntervalCandidate.Hours()
	}

	trialRows, err := q.GetSettingsTrialStarts(ctx, trialLoadWindow.Seconds())
	if err != nil {
		return nil, fmt.Errorf("counting trial starts: %w", err)
	}
	for _, row := range trialRows {
		get(row.IpVersion).Trials.Started = row.Trials
	}

	promotionRows, err := q.GetSettingsPromotions(ctx, ntpdb.GetSettingsPromotionsParams{
		WindowSeconds: s.PromotionWindow.Seconds(),
		Budget:        int64(s.HealthPromotions),
	})
	if err != nil {
		return nil, fmt.Errorf("counting promotions: %w", err)
	}
	for _, row := range promotionRows {
		p := &get(row.IpVersion).Promotions
		p.IntoTesting = row.IntoTesting
		p.IntoTestingMax = row.IntoTestingMax
		p.IntoTestingAtBudget = row.IntoTestingAtBudget
		p.IntoActive = row.IntoActive
		p.IntoActiveMax = row.IntoActiveMax
		p.IntoActiveAtBudget = row.IntoActiveAtBudget
	}

	load := make([]ipLoad, 0, len(byVersion))
	for _, v := range slices.Sorted(maps.Keys(byVersion)) {
		load = append(load, *byVersion[v])
	}
	return load, nil
}

func printSettingsReport(out io.Writer, r settingsReport) error {
	statusOrder := []ntpdb.ServerScoresStatus{
		ntpdb.ServerScoresStatusActive,
		ntpdb.ServerScoresStatusTesting,
		ntpdb.ServerScoresStatusCandidate,
		ntpdb.ServerScoresStatusReserve,
		ntpdb.ServerScoresStatusPaused,
	}
	var b strings.Builder

	fmt.Fprintf(&b, "Settings (system_settings %q)\n", shared.SettingsKey)
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  key\tstored\tselector uses\tapi\tnote")
	for _, f := range r.Settings {
		stored := f.Stored
		if stored == "" {
			stored = "-"
		}
		note := f.Problem
		if f.Defaulted {
			note += ", default used"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", f.Key, stored, f.Value, f.API, note)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	s, d := r.used, r.Derived
	fmt.Fprintln(&b, "\nDerived")
	fmt.Fprintf(&b, "  candidate to testing: at most %d samples in candidate_window %s at interval_candidate %s, %d needed\n",
		d.CandidateMaxSamples, shared.FormatDuration(s.CandidateWindow.Duration), shared.FormatDuration(s.IntervalCandidate.Duration), d.CandidateRequired)
	fmt.Fprintf(&b, "  testing to active: at most %d samples in sample_window %s at interval_testing %s, %d needed\n",
		d.TestingMaxSamples, shared.FormatDuration(s.SampleWindow.Duration), shared.FormatDuration(s.IntervalTesting.Duration), d.TestingRequired)
	fmt.Fprintf(&b, "  trials at once per server: %d (candidate_window / trial_every)\n", d.TrialsAtOnce)
	fmt.Fprintf(&b, "  new-server candidates: %d (candidate_pool %d + %d)\n", d.NewServerCandidates, s.CandidatePool, d.TrialsAtOnce)

	fmt.Fprintln(&b, "\nChecks")
	if len(r.Findings) == 0 {
		fmt.Fprintln(&b, "  ok")
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "  %s: %s\n", f.Severity, f.Message)
	}

	for _, l := range r.Load {
		fmt.Fprintf(&b, "\nLoad %s\n", l.IPVersion)
		fmt.Fprintf(&b, "  servers: %d, %d in bootstrap mode (at most %d active monitors)\n",
			l.Servers, l.BootstrapServers, selector.BootstrapModeLimit)
		fmt.Fprintf(&b, "  monitors: %d active, %d testing\n", l.ActiveMonitors, l.TestingMonitors)

		var rows []string
		for _, st := range statusOrder {
			rows = append(rows, fmt.Sprintf("%s %d", st, l.Rows[string(st)]))
		}
		fmt.Fprintf(&b, "  rows: %s\n", strings.Join(rows, ", "))

		c := l.CandidatesPerServer
		fmt.Fprintf(&b, "  candidates per server: avg %.1f, p50 %d, p90 %d, max %d\n", c.Avg, c.P50, c.P90, c.Max)
		fmt.Fprintf(&b, "  trials started in %s: %d, at most %.0f expected (servers x %s / trial_every %s); a new server's first candidates count too\n",
			l.Trials.Window, l.Trials.Started, l.Trials.Expected, l.Trials.Window, shared.FormatDuration(s.TrialEvery.Duration))

		p := l.Promotions
		fmt.Fprintf(&b, "  promotions in %s (health_promotions %d per server):\n", p.Window, p.Budget)
		fmt.Fprintf(&b, "    into testing: %d, at most %d on one server, %d servers at the budget\n",
			p.IntoTesting, p.IntoTestingMax, p.IntoTestingAtBudget)
		fmt.Fprintf(&b, "    into active: %d, at most %d on one server, %d servers at the budget\n",
			p.IntoActive, p.IntoActiveMax, p.IntoActiveAtBudget)

		fmt.Fprintf(&b, "  candidate probes per monitor and hour: avg %.1f, max %.1f\n",
			l.CandidateProbesPerHour.Avg, l.CandidateProbesPerHour.Max)
	}

	_, err := io.WriteString(out, b.String())
	return err
}
