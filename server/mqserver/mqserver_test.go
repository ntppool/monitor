package mqserver

import (
	"testing"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"golang.org/x/exp/slog"
)

func TestClientState(t *testing.T) {

	mqs, err := Setup(slog.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}

	mqs.MQTTStatusHandler(&paho.Publish{
		Topic:   "/devel/monitors/status/uspao-abc/status",
		Payload: []byte("{\"Online\":true}"),
	})

	var lastSeen time.Time

	c, ok := mqs.clients["uspao-abc"]

	if !ok {
		t.Log("client didn't get registered")
		t.FailNow()
	}

	lastSeen = c.LastSeen

	if !c.Online {
		t.Log("client didn't get set online")
		t.Fail()
	}

	if !c.LastSeen.After(time.Now().Add(-2 * time.Second)) {
		t.Log("client didn't set LastSeen")
		t.Fail()
	}

	mqs.MQTTStatusHandler(&paho.Publish{
		Topic:   "/devel/monitors/status/uspao-abc/status",
		Payload: []byte("{\"Online\":false}"),
	})

	if c, ok := mqs.clients["uspao-abc"]; !ok {
		t.Log("client didn't get registered (offline!)")
		t.Fail()

		if lastSeen != c.LastSeen {
			t.Log("last seen got updated on offline message")
		}

		if c.Online {
			t.Log("client didn't get set offline")
			t.Fail()
		}
	}

}
