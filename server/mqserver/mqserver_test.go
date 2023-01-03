package mqserver

import (
	"testing"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

func TestClientState(t *testing.T) {

	mqs, err := Setup()
	if err != nil {
		t.Fatal(err)
	}

	mqs.MQTTHandler(&paho.Publish{
		Topic:   "/devel/monitors/status/uspao-abc/online",
		Payload: []byte("{\"Online\":true}"),
	})

	var lastSeen time.Time

	if c, ok := mqs.clients["uspao-abc"]; !ok {
		t.Log("client didn't get registered")
		t.Fail()

		lastSeen = c.LastSeen

		if !c.Online {
			t.Log("client didn't get set online")
			t.Fail()
		}

		if !c.LastSeen.After(time.Now().Add(-2 * time.Second)) {
			t.Log("client didn't set LastSeen")
			t.Fail()
		}
	}

	mqs.MQTTHandler(&paho.Publish{
		Topic:   "/devel/monitors/status/uspao-abc/online",
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
