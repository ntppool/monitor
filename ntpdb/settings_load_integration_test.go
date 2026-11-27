//go:build integration
// +build integration

package ntpdb_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// settingsLoad is every result of the `db settings` load queries.
type settingsLoad struct {
	servers    map[string]ntpdb.GetSettingsServerLoadRow
	statuses   map[string]int64 // "ip_version/status"
	monitors   map[string]ntpdb.GetSettingsMonitorLoadRow
	trials     map[string]int64
	promotions map[string]ntpdb.GetSettingsPromotionsRow
}

const (
	loadBootstrapLimit = 4
	loadWindow         = 24 * time.Hour
	loadBudget         = 2
)

func readSettingsLoad(t *testing.T, tdb *testutil.TestDB, q *ntpdb.Queries) settingsLoad {
	t.Helper()
	ctx := tdb.Context()
	l := settingsLoad{
		servers:    map[string]ntpdb.GetSettingsServerLoadRow{},
		statuses:   map[string]int64{},
		monitors:   map[string]ntpdb.GetSettingsMonitorLoadRow{},
		trials:     map[string]int64{},
		promotions: map[string]ntpdb.GetSettingsPromotionsRow{},
	}

	servers, err := q.GetSettingsServerLoad(ctx, loadBootstrapLimit)
	testutil.AssertNoError(t, err, "GetSettingsServerLoad")
	for _, r := range servers {
		l.servers[r.IpVersion] = r
	}
	statuses, err := q.GetSettingsStatusCounts(ctx)
	testutil.AssertNoError(t, err, "GetSettingsStatusCounts")
	for _, r := range statuses {
		l.statuses[r.IpVersion+"/"+string(r.Status)] = r.Rows
	}
	monitors, err := q.GetSettingsMonitorLoad(ctx)
	testutil.AssertNoError(t, err, "GetSettingsMonitorLoad")
	for _, r := range monitors {
		l.monitors[r.IpVersion] = r
	}
	trials, err := q.GetSettingsTrialStarts(ctx, loadWindow.Seconds())
	testutil.AssertNoError(t, err, "GetSettingsTrialStarts")
	for _, r := range trials {
		l.trials[r.IpVersion] = r.Trials
	}
	promotions, err := q.GetSettingsPromotions(ctx, ntpdb.GetSettingsPromotionsParams{
		WindowSeconds: loadWindow.Seconds(),
		Budget:        loadBudget,
	})
	testutil.AssertNoError(t, err, "GetSettingsPromotions")
	for _, r := range promotions {
		l.promotions[r.IpVersion] = r
	}
	return l
}

// The load queries count across the whole database. The test runs in a
// repeatable read transaction, so rows other packages commit meanwhile are
// invisible, and compares the counts before and after its own rows. The
// distribution (average, percentiles, max) is only compared when the
// snapshot has no v6 servers of its own.
func TestSettingsLoadQueries(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	ctx := tdb.Context()

	tx, err := tdb.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	testutil.AssertNoError(t, err, "begin")
	defer func() { _ = tx.Rollback(ctx) }()
	q := ntpdb.New(tx)

	before := readSettingsLoad(t, tdb, q)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	// ids 934xx: other packages' tests commit rows with their own ids
	// at the same time, and a shared id fails the insert here
	const accountID = 93420
	exec(`INSERT INTO accounts (id, name, created_on) VALUES ($1, 'settings-load', NOW())`, accountID)

	const (
		m1 = 93421 + iota
		m2
		m3
		m4
		m5
		mPaused
		mScorer
	)
	monitor := func(id int64, status, typ string) {
		t.Helper()
		exec(`INSERT INTO monitors (id, tls_name, account_id, ip, ip_version, status, type, config, created_on)
			VALUES ($1, $2, $3, $4, 'v6', $5, $6, '{}', NOW())`,
			id, fmt.Sprintf("load-%d.test", id), accountID, fmt.Sprintf("2001:db8::%d", id), status, typ)
	}
	monitor(m1, "active", "monitor")
	monitor(m2, "testing", "monitor")
	monitor(m3, "active", "monitor")
	monitor(m4, "active", "monitor")
	monitor(m5, "active", "monitor")
	monitor(mPaused, "paused", "monitor")
	monitor(mScorer, "active", "score")

	const (
		s1 = 93401 + iota
		s2
		s3
		s4
		sDeleted
		sV4
	)
	server := func(id int64, ipVersion, ip string, deleted bool) {
		t.Helper()
		var deletion any
		if deleted {
			deletion = time.Now().AddDate(0, 0, -1)
		}
		exec(`INSERT INTO servers (id, ip, ip_version, deletion_on, created_on) VALUES ($1, $2, $3, $4, NOW())`,
			id, ip, ipVersion, deletion)
	}
	server(s1, "v6", "2001:db8:1::1", false)
	server(s2, "v6", "2001:db8:1::2", false)
	server(s3, "v6", "2001:db8:1::3", false)
	server(s4, "v6", "2001:db8:1::4", false)
	server(sDeleted, "v6", "2001:db8:1::5", true)
	server(sV4, "v4", "192.0.2.106", false)

	// rows are inserted with a status and moved with the selector's
	// UpdateServerScoreStatus; the status log trigger records both
	row := func(serverID, monitorID int64, status string, moves ...ntpdb.ServerScoresStatus) {
		t.Helper()
		exec(`INSERT INTO server_scores (server_id, monitor_id, status, score_raw, created_on) VALUES ($1, $2, $3, 0, NOW())`,
			serverID, monitorID, status)
		for _, to := range moves {
			err := q.UpdateServerScoreStatus(ctx, ntpdb.UpdateServerScoreStatusParams{
				Status: to, MonitorID: monitorID, ServerID: serverID,
			})
			testutil.AssertNoError(t, err, "UpdateServerScoreStatus(%d, %d, %s)", serverID, monitorID, to)
		}
	}
	const (
		toCandidate = ntpdb.ServerScoresStatusCandidate
		toTesting   = ntpdb.ServerScoresStatusTesting
		toActive    = ntpdb.ServerScoresStatusActive
	)
	// s1: three candidates (three trial starts), plus the scorer's own row
	row(s1, m1, "candidate")
	row(s1, m2, "candidate")
	row(s1, m3, "candidate")
	row(s1, mScorer, "candidate")
	// s2: a reserve row made a candidate (a trial start), a candidate
	// promoted to testing (a trial start, one promotion into testing)
	row(s2, m1, "reserve", toCandidate)
	row(s2, m2, "candidate", toTesting)
	row(s2, m3, "active")
	// s3: two promotions into active
	row(s3, m1, "testing", toActive)
	row(s3, m2, "testing", toActive)
	row(s3, m3, "active")
	// s4: five active rows (not bootstrap mode), a paused monitor's
	// candidate (a trial start), and a demotion that isn't a promotion
	row(s4, m1, "active")
	row(s4, m2, "active", toTesting, toActive)
	row(s4, m3, "active")
	row(s4, m4, "active")
	row(s4, m5, "active")
	row(s4, mPaused, "candidate")
	// a deleted server counts nowhere
	row(sDeleted, m1, "candidate", toTesting, toActive)

	after := readSettingsLoad(t, tdb, q)

	delta := func(name string, want, b, a int64) {
		t.Helper()
		if got := a - b; got != want {
			t.Errorf("%s: +%d, want +%d", name, got, want)
		}
	}
	delta("v6 servers", 4, before.servers["v6"].Servers, after.servers["v6"].Servers)
	delta("v4 servers", 1, before.servers["v4"].Servers, after.servers["v4"].Servers)
	// s1 (0 active), s2 (1) and s3 (3) are in bootstrap mode, s4 (5) isn't
	delta("v6 bootstrap servers", 3, before.servers["v6"].BootstrapServers, after.servers["v6"].BootstrapServers)
	delta("v4 bootstrap servers", 1, before.servers["v4"].BootstrapServers, after.servers["v4"].BootstrapServers)

	for status, want := range map[string]int64{"candidate": 5, "testing": 1, "active": 9, "reserve": 0} {
		key := "v6/" + status
		delta(key+" rows", want, before.statuses[key], after.statuses[key])
	}

	delta("v6 active monitors", 4, before.monitors["v6"].ActiveMonitors, after.monitors["v6"].ActiveMonitors)
	delta("v6 testing monitors", 1, before.monitors["v6"].TestingMonitors, after.monitors["v6"].TestingMonitors)

	// s1 x3, s2 x2, s4 x1; the scorer's row and the deleted server don't count
	delta("v6 trial starts", 6, before.trials["v6"], after.trials["v6"])

	// s2: m2 into testing; s3: m1, m2 into active; s4: m2 back into active
	delta("v6 into testing", 1, before.promotions["v6"].IntoTesting, after.promotions["v6"].IntoTesting)
	delta("v6 into active", 3, before.promotions["v6"].IntoActive, after.promotions["v6"].IntoActive)

	if before.servers["v6"].Servers > 0 || before.monitors["v6"].ActiveMonitors+before.monitors["v6"].TestingMonitors > 0 {
		t.Logf("the snapshot has v6 servers or monitors of its own; not comparing the distributions")
		return
	}

	// candidates per server: s1 3, s2 1, s3 0, s4 1
	wantServers := ntpdb.GetSettingsServerLoadRow{
		IpVersion: "v6", Servers: 4, CandidatesAvg: 1.25,
		CandidatesP50: 1, CandidatesP90: 3, CandidatesMax: 3, BootstrapServers: 3,
	}
	testutil.AssertEqual(t, wantServers, after.servers["v6"], "v6 server load")

	// candidates per active or testing monitor: m1 2, m2 1, m3 1, m4 0,
	// m5 0; the paused monitor isn't counted
	wantMonitors := ntpdb.GetSettingsMonitorLoadRow{
		IpVersion: "v6", ActiveMonitors: 4, TestingMonitors: 1, CandidatesAvg: 0.8, CandidatesMax: 2,
	}
	testutil.AssertEqual(t, wantMonitors, after.monitors["v6"], "v6 monitor load")

	// per server: s2 1 into testing; s3 2 into active, s4 1
	wantPromotions := ntpdb.GetSettingsPromotionsRow{
		IpVersion:   "v6",
		IntoTesting: 1, IntoTestingMax: 1, IntoTestingAtBudget: 0,
		IntoActive: 3, IntoActiveMax: 2, IntoActiveAtBudget: 1,
	}
	testutil.AssertEqual(t, wantPromotions, after.promotions["v6"], "v6 promotions")
}

// GetSettingsTrialStarts and GetSettingsPromotions copy the conditions of
// the selector's GetTrialStartedSince and GetServerPromotionCounts. Each
// case puts status log rows on a server of its own; the settings queries
// have to count them exactly when the selector's queries do.
func TestSettingsQueriesMatchSelector(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	ctx := tdb.Context()

	tx, err := tdb.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	testutil.AssertNoError(t, err, "begin")
	defer func() { _ = tx.Rollback(ctx) }()
	q := ntpdb.New(tx)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	// 935xx: no other test uses these ids
	const (
		accountID = 93520
		monitorID = 93521
		scorerID  = 93522
		serverID  = 93501
	)
	exec(`INSERT INTO accounts (id, name, created_on) VALUES ($1, 'settings-match', NOW())`, accountID)
	for _, m := range []struct {
		id  int64
		typ string
	}{{monitorID, "monitor"}, {scorerID, "score"}} {
		exec(`INSERT INTO monitors (id, tls_name, account_id, ip, ip_version, status, type, config, created_on)
			VALUES ($1, $2, $3, $4, 'v6', 'active', $5, '{}', NOW())`,
			m.id, fmt.Sprintf("match-%d.test", m.id), accountID, fmt.Sprintf("2001:db8::%d", m.id), m.typ)
	}

	const (
		trialWindow     = 12 * time.Hour
		promotionWindow = 24 * time.Hour
	)
	type logRow struct {
		monitor  int64
		ago      time.Duration
		op       string
		old, new any
	}
	tests := []struct {
		name string
		rows []logRow
	}{
		{"insert candidate", []logRow{{monitorID, time.Hour, "insert", nil, "candidate"}}},
		{"insert candidate before the trial window", []logRow{{monitorID, 13 * time.Hour, "insert", nil, "candidate"}}},
		{"reserve to candidate", []logRow{{monitorID, time.Hour, "update", "reserve", "candidate"}}},
		{"testing to candidate", []logRow{{monitorID, time.Hour, "update", "testing", "candidate"}}},
		{"candidate to reserve", []logRow{{monitorID, time.Hour, "update", "candidate", "reserve"}}},
		{"paused to reserve", []logRow{{monitorID, time.Hour, "update", "paused", "reserve"}}},
		{"insert reserve", []logRow{{monitorID, time.Hour, "insert", nil, "reserve"}}},
		{"snapshot candidate", []logRow{{monitorID, time.Hour, "snapshot", nil, "candidate"}}},
		{"delete candidate", []logRow{{monitorID, time.Hour, "delete", "candidate", nil}}},
		{"scorer inserts candidate", []logRow{{scorerID, time.Hour, "insert", nil, "candidate"}}},
		{"candidate to testing", []logRow{{monitorID, time.Hour, "update", "candidate", "testing"}}},
		{"testing to active", []logRow{{monitorID, time.Hour, "update", "testing", "active"}}},
		{"active to testing", []logRow{{monitorID, time.Hour, "update", "active", "testing"}}},
		{"candidate to testing before the promotion window", []logRow{{monitorID, 25 * time.Hour, "update", "candidate", "testing"}}},
		{"scorer candidate to testing", []logRow{{scorerID, time.Hour, "update", "candidate", "testing"}}},
		{"two promotions and a trial", []logRow{
			{monitorID, time.Hour, "update", "candidate", "testing"},
			{monitorID, 2 * time.Hour, "update", "testing", "active"},
			{monitorID, 3 * time.Hour, "update", "testing", "active"},
			{monitorID, 4 * time.Hour, "update", "reserve", "candidate"},
		}},
	}

	// cases where the queries counted something, so the comparison isn't
	// only between zeros
	var trialCases, promotionCases int
	now := time.Now()
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid := int64(serverID + i)
			exec(`INSERT INTO servers (id, ip, ip_version, created_on) VALUES ($1, $2, 'v6', NOW())`,
				sid, fmt.Sprintf("2001:db8:2::%d", sid))

			trials := func() int64 {
				rows, err := q.GetSettingsTrialStarts(ctx, trialWindow.Seconds())
				testutil.AssertNoError(t, err, "GetSettingsTrialStarts")
				for _, r := range rows {
					if r.IpVersion == "v6" {
						return r.Trials
					}
				}
				return 0
			}
			promotions := func() (intoTesting, intoActive int64) {
				rows, err := q.GetSettingsPromotions(ctx, ntpdb.GetSettingsPromotionsParams{
					WindowSeconds: promotionWindow.Seconds(),
					Budget:        1,
				})
				testutil.AssertNoError(t, err, "GetSettingsPromotions")
				for _, r := range rows {
					if r.IpVersion == "v6" {
						return r.IntoTesting, r.IntoActive
					}
				}
				return 0, 0
			}

			trialsBefore := trials()
			testingBefore, activeBefore := promotions()
			for _, r := range tt.rows {
				exec(`INSERT INTO server_scores_status_log (server_id, monitor_id, ts, op, old_status, new_status)
					VALUES ($1, $2, $3, $4, $5::server_scores_status, $6::server_scores_status)`,
					sid, r.monitor, now.Add(-r.ago), r.op, r.old, r.new)
			}
			trialStarts := trials() - trialsBefore
			testingAfter, activeAfter := promotions()

			started, err := q.GetTrialStartedSince(ctx, ntpdb.GetTrialStartedSinceParams{
				ServerID:     sid,
				SinceSeconds: trialWindow.Seconds(),
			})
			testutil.AssertNoError(t, err, "GetTrialStartedSince")
			testutil.AssertEqual(t, started, trialStarts > 0, "GetTrialStartedSince vs %d trial starts counted", trialStarts)

			counts, err := q.GetServerPromotionCounts(ctx, ntpdb.GetServerPromotionCountsParams{
				ServerID:      sid,
				WindowSeconds: promotionWindow.Seconds(),
			})
			testutil.AssertNoError(t, err, "GetServerPromotionCounts")
			testutil.AssertEqual(t, counts.IntoTesting, testingAfter-testingBefore, "promotions into testing")
			testutil.AssertEqual(t, counts.IntoActive, activeAfter-activeBefore, "promotions into active")
			if started {
				trialCases++
			}
			if counts.IntoTesting+counts.IntoActive > 0 {
				promotionCases++
			}
		})
	}
	// insert candidate, reserve to candidate, the last case
	testutil.AssertEqual(t, 3, trialCases, "cases with a trial start")
	// candidate to testing, testing to active, the last case
	testutil.AssertEqual(t, 3, promotionCases, "cases with a promotion")
}
