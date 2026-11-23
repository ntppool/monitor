package mqserver

import (
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/prometheus/client_golang/prometheus"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/monitor/ntpdb"
)

const testStatusTopic = "/devel/monitors/status/uspao-abc/status"

func publishStatus(mqs *server, online bool) {
	payload := []byte(`{"Online":false}`)
	if online {
		payload = []byte(`{"Online":true}`)
	}
	mqs.MQTTStatusHandler(&paho.Publish{Topic: testStatusTopic, Payload: payload})
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

	publishStatus(mqs, true)

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

	publishStatus(mqs, false)

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

		publishStatus(mqs, true)

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
		publishStatus(mqs, true)
		time.Sleep(staleAfter)
		check("staleAfter after the republish", true)

		time.Sleep(time.Second)
		check("past staleAfter", false)

		// a new status brings it back without a restart
		publishStatus(mqs, true)
		check("after a new online status", true)
	})
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
