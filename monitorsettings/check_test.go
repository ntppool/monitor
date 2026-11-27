package monitorsettings

import (
	"encoding/json"
	"strings"
	"testing"

	shared "go.ntppool.org/common/monitorsettings"
)

// The production setting after migration 057, with every key set.
const validSettings = `{
	"interval_active": "9m", "interval_testing": "25m", "interval_all": "60s",
	"batch_size": 60, "interval_candidate": "4h", "sample_window": "24h",
	"promotion_window": "24h", "health_promotions": 3, "review_every": "2h",
	"review_after_change": "4h", "review_bootstrap": "1h",
	"candidate_pool": 30, "trial_every": "12h", "candidate_window": "72h",
	"new_monitor_servers": 100, "allow_leap_announcements": false
}`

// withKey returns validSettings with key set to the JSON value v, or
// removed when v is empty. An empty key returns validSettings unchanged.
func withKey(t *testing.T, key, v string) []byte {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(validSettings), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	switch {
	case key == "":
	case v == "":
		delete(m, key)
	default:
		m[key] = json.RawMessage(v)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return raw
}

func check(t *testing.T, raw []byte, servers map[string]int64) []Finding {
	t.Helper()
	s, fields, err := shared.Inspect(raw)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	return Check(fields, s.WithDefaults(), servers)
}

func TestCheckValid(t *testing.T) {
	findings := check(t, []byte(validSettings), map[string]int64{"v4": 3000, "v6": 1500})
	if len(findings) > 0 {
		t.Errorf("want no findings, got %+v", findings)
	}
	// `db settings --json` prints an empty list, not null
	if findings == nil {
		t.Errorf("Check returned nil, want an empty slice")
	}
	s, err := shared.Parse([]byte(validSettings))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	d := Derive(s.WithDefaults())
	want := Derived{
		CandidateMaxSamples: 18, // 72h / 4h
		CandidateRequired:   9,  // min(ceil(0.8 * 18), 9)
		TestingMaxSamples:   57, // 24h / 25m
		TestingRequired:     32, // min(ceil(0.8 * 57), 32)
		TrialsAtOnce:        6,  // 72h / 12h
		NewServerCandidates: 36, // 30 + 6
	}
	if d != want {
		t.Errorf("derived = %+v, want %+v", d, want)
	}
}

func TestCheckFindings(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		servers map[string]int64
		want    Severity
		words   []string
	}{
		{
			// 4h over 24h: 6 samples at most, 8 needed
			name:  "candidate promotion unreachable",
			key:   "candidate_window",
			value: `"24h"`,
			want:  SeverityError,
			words: []string{"at most 6", "needs 8", "no candidate"},
		},
		{
			// 4h over 24h at interval_testing: 6 samples at most, 8 needed
			name:  "testing promotion unreachable",
			key:   "interval_testing",
			value: `"4h"`,
			want:  SeverityError,
			words: []string{"at most 6", "needs 8", "no testing monitor"},
		},
		{
			name:  "api key missing",
			key:   "new_monitor_servers",
			value: "",
			want:  SeverityError,
			words: []string{"new_monitor_servers", "missing", "adding servers"},
		},
		{
			name:  "api key under the selector minimum",
			key:   "trial_every",
			value: `"30m"`,
			want:  SeverityError,
			words: []string{"trial_every", "below minimum 1h", "default 12h"},
		},
		{
			name:  "selector key under the minimum",
			key:   "review_every",
			value: `"10s"`,
			want:  SeverityWarning,
			words: []string{"review_every", "below minimum 1m", "default 2h"},
		},
		{
			name:  "selector key missing",
			key:   "interval_all",
			value: "",
			want:  SeverityWarning,
			words: []string{"interval_all", "missing", "default 1m"},
		},
		{
			name:  "unknown key",
			key:   "trial_evry",
			value: `"6h"`,
			want:  SeverityWarning,
			words: []string{"trial_evry", "ignored"},
		},
		{
			name:    "more new_monitor_servers than servers",
			servers: map[string]int64{"v4": 3000, "v6": 99},
			want:    SeverityWarning,
			words:   []string{"new_monitor_servers 100", "99 v6 servers"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := check(t, withKey(t, tt.key, tt.value), tt.servers)
			if len(findings) != 1 {
				t.Fatalf("want one finding, got %+v", findings)
			}
			f := findings[0]
			if f.Severity != tt.want {
				t.Errorf("severity %q, want %q (%s)", f.Severity, tt.want, f.Message)
			}
			for _, w := range tt.words {
				if !strings.Contains(f.Message, w) {
					t.Errorf("message %q does not contain %q", f.Message, w)
				}
			}
		})
	}
}
