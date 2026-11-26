package mqserver

import (
	"encoding/json"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/prometheus/client_golang/prometheus"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/monitor/mqttcm"
	"go.ntppool.org/monitor/ntpdb"
)

const testStatusTopic = "/devel/monitors/status/uspao-abc/status"

// publishStatus passes a status message to MQTTStatusHandler. expiryLeft,
// if not nil, is the message expiry the broker has left when it delivers
// the message.
func publishStatus(t *testing.T, mqs *server, status mqttcm.StatusMessage, expiryLeft *uint32) {
	t.Helper()
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	mqs.MQTTStatusHandler(&paho.Publish{
		Topic:      testStatusTopic,
		Payload:    payload,
		Properties: &paho.PublishProperties{MessageExpiry: expiryLeft},
	})
}

func countSeries(t *testing.T, reg *prometheus.Registry, name string) int {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			return len(mf.GetMetric())
		}
	}
	return 0
}

func TestClientState(t *testing.T) {
	mqs, err := Setup(slog.Default(), nil, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	publishStatus(t, mqs, mqttcm.StatusMessage{Online: true}, nil)

	c, ok := mqs.clients["uspao-abc"]
	if !ok {
		t.Fatal("client didn't get registered")
	}

	lastSeen := c.LastSeen

	if !c.Online {
		t.Error("client didn't get set online")
	}

	if !c.LastSeen.After(time.Now().Add(-2 * time.Second)) {
		t.Error("client didn't set LastSeen")
	}

	publishStatus(t, mqs, mqttcm.StatusMessage{Online: false}, nil)

	c, ok = mqs.clients["uspao-abc"]
	if !ok {
		t.Fatal("client got removed on offline message")
	}

	if c.Online {
		t.Error("client didn't get set offline")
	}

	if lastSeen != c.LastSeen {
		t.Error("LastSeen got updated on offline message")
	}
}

func TestStaleClientGoesOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := prometheus.NewRegistry()
		mqs, err := Setup(slog.Default(), nil, reg)
		if err != nil {
			t.Fatal(err)
		}

		publishStatus(t, mqs, mqttcm.StatusMessage{Online: true}, nil)

		// MQTTRouter sets depEnv, and MQTTStatusHandler loads Data from the
		// database; the test runs without either.
		mqs.cmux.Lock()
		mqs.depEnv = depenv.DeployDevel
		mqs.clients["uspao-abc"].Data = []ntpdb.Monitor{{
			Hostname: "uspao-abc",
			IpVersion: ntpdb.NullMonitorsIpVersion{
				MonitorsIpVersion: ntpdb.MonitorsIpVersionV4,
				Valid:             true,
			},
		}}
		mqs.cmux.Unlock()

		check := func(when string, wantOnline bool) {
			t.Helper()

			clients := mqs.seenClients()
			if len(clients) != 1 {
				t.Fatalf("%s: got %d clients, want 1", when, len(clients))
			}
			if clients[0].Online != wantOnline {
				t.Errorf("%s: Online = %t, want %t", when, clients[0].Online, wantOnline)
			}

			wantSeries := 0
			if wantOnline {
				wantSeries = 1
			}
			if n := countSeries(t, reg, "monitors_connected"); n != wantSeries {
				t.Errorf("%s: monitors_connected has %d series, want %d", when, n, wantSeries)
			}
		}

		check("after online status", true)

		time.Sleep(10 * time.Minute)
		check("10 minutes later", true)

		// a republished status keeps the client online
		publishStatus(t, mqs, mqttcm.StatusMessage{Online: true}, nil)
		time.Sleep(staleAfter)
		check("staleAfter after the republish", true)

		time.Sleep(time.Second)
		check("past staleAfter", false)

		// a new status brings it back without a restart
		publishStatus(t, mqs, mqttcm.StatusMessage{Online: true}, nil)
		check("after a new online status", true)
	})
}

// LastSeen comes from how long the broker held the status, by the broker's
// clock, so a stopped agent's old retained "online" status, replayed after a
// monitor-api restart or broker reconnect, doesn't count as fresh.
func TestStatusLastSeen(t *testing.T) {
	tests := []struct {
		name string
		// liveAgo, if set, sends a live status this long before the
		// tested one.
		liveAgo time.Duration
		status  mqttcm.StatusMessage
		// expiryLeft is the message expiry the broker has left.
		expiryLeft *uint32
		// wantSeenAgo is how long before the tested status LastSeen is.
		wantSeenAgo time.Duration
		wantOnline  bool
	}{
		{
			name:        "held 20 minutes",
			status:      mqttcm.StatusMessage{Online: true, MessageExpiry: 86400},
			expiryLeft:  paho.Uint32(86400 - 20*60),
			wantSeenAgo: 20 * time.Minute,
			wantOnline:  false,
		},
		{
			name:        "held 2 minutes",
			status:      mqttcm.StatusMessage{Online: true, MessageExpiry: 86400},
			expiryLeft:  paho.Uint32(86400 - 2*60),
			wantSeenAgo: 2 * time.Minute,
			wantOnline:  true,
		},
		{
			name:        "agent sets a shorter expiry",
			status:      mqttcm.StatusMessage{Online: true, MessageExpiry: 3600},
			expiryLeft:  paho.Uint32(3600 - 2*60),
			wantSeenAgo: 2 * time.Minute,
			wantOnline:  true,
		},
		{
			name:        "no MessageExpiry in the status (agent v3.7.0 to v4.2.x)",
			status:      mqttcm.StatusMessage{Online: true},
			expiryLeft:  paho.Uint32(86400 - 20*60),
			wantSeenAgo: 20 * time.Minute,
			wantOnline:  false,
		},
		{
			name:       "live status",
			status:     mqttcm.StatusMessage{Online: true, MessageExpiry: 86400},
			expiryLeft: paho.Uint32(86400),
			wantOnline: true,
		},
		{
			name:       "no message expiry (agent older than v3.7.0)",
			status:     mqttcm.StatusMessage{Online: true},
			wantOnline: true,
		},
		{
			// The age is unknown, so LastSeen stays at the live status.
			name:        "more expiry left than the agent set",
			liveAgo:     time.Minute,
			status:      mqttcm.StatusMessage{Online: true, MessageExpiry: 3600},
			expiryLeft:  paho.Uint32(86000),
			wantSeenAgo: time.Minute,
			wantOnline:  true,
		},
		{
			name:        "reconnect replays a status older than the last live one",
			liveAgo:     time.Minute,
			status:      mqttcm.StatusMessage{Online: true, MessageExpiry: 86400},
			expiryLeft:  paho.Uint32(86400 - 10*60),
			wantSeenAgo: time.Minute,
			wantOnline:  true,
		},
		{
			name:        "reconnect replays a status sent while disconnected",
			liveAgo:     6 * time.Minute,
			status:      mqttcm.StatusMessage{Online: true, MessageExpiry: 86400},
			expiryLeft:  paho.Uint32(86400 - 60),
			wantSeenAgo: time.Minute,
			wantOnline:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mqs, err := Setup(slog.Default(), nil, prometheus.NewRegistry())
				if err != nil {
					t.Fatal(err)
				}

				if tt.liveAgo > 0 {
					publishStatus(t, mqs, mqttcm.StatusMessage{Online: true, MessageExpiry: 86400}, paho.Uint32(86400))
					time.Sleep(tt.liveAgo)
				}

				now := time.Now()
				publishStatus(t, mqs, tt.status, tt.expiryLeft)

				clients := mqs.seenClients()
				if len(clients) != 1 {
					t.Fatalf("got %d clients, want 1", len(clients))
				}
				if got, want := clients[0].LastSeen, now.Add(-tt.wantSeenAgo); !got.Equal(want) {
					t.Errorf("LastSeen is %s before the status, want %s", now.Sub(got), tt.wantSeenAgo)
				}
				if clients[0].Online != tt.wantOnline {
					t.Errorf("Online = %t, want %t", clients[0].Online, tt.wantOnline)
				}
			})
		})
	}
}

// A scrape can come before MQTTRouter has set depEnv.
func TestCollectBeforeMQTTRouter(t *testing.T) {
	reg := prometheus.NewRegistry()
	if _, err := Setup(slog.Default(), nil, reg); err != nil {
		t.Fatal(err)
	}
	if n := countSeries(t, reg, "monitors_connected"); n != 0 {
		t.Errorf("monitors_connected has %d series, want 0", n)
	}
}
