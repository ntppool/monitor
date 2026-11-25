package mqttcm

import (
	"reflect"
	"testing"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
)

func TestSetOfflineWill(t *testing.T) {
	const topic = "/devel/monitors/status/x"
	payload := []byte(`{"Online":false}`)

	cfg := autopaho.ClientConfig{}
	setOfflineWill(&cfg, topic, payload)

	wantWill := paho.WillMessage{Topic: topic, Retain: true, Payload: payload}
	if cfg.WillMessage == nil || !reflect.DeepEqual(*cfg.WillMessage, wantWill) {
		t.Errorf("WillMessage = %+v, want %+v", cfg.WillMessage, wantWill)
	}
	if cfg.WillProperties == nil || cfg.WillProperties.WillDelayInterval == nil {
		t.Fatalf("WillProperties has no will delay: %+v", cfg.WillProperties)
	}

	// A clean disconnect must ask the broker to publish the will.
	if cfg.DisconnectPacketBuilder == nil {
		t.Fatal("DisconnectPacketBuilder not set")
	}
	d := cfg.DisconnectPacketBuilder()
	if d == nil || d.ReasonCode != packets.DisconnectDisconnectWithWillMessage {
		t.Errorf("disconnect packet = %+v, want reason code 0x04", d)
	}
}
