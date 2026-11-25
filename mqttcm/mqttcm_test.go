package mqttcm

import (
	"encoding/json"
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

func TestStatusMessageJSONUpdatedMQ(t *testing.T) {
	tests := []struct {
		online      bool
		wantUpdated bool
	}{
		{online: true, wantUpdated: true},
		{online: false, wantUpdated: false},
	}
	for _, tt := range tests {
		js, err := StatusMessageJSON(tt.online)
		if err != nil {
			t.Fatalf("StatusMessageJSON(%v): %v", tt.online, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(js, &fields); err != nil {
			t.Fatalf("unmarshal %s: %v", js, err)
		}
		if _, ok := fields["UpdatedMQ"]; ok != tt.wantUpdated {
			t.Errorf("StatusMessageJSON(%v) = %s, UpdatedMQ present %v, want %v", tt.online, js, ok, tt.wantUpdated)
		}
		if fields["Online"] != tt.online {
			t.Errorf("StatusMessageJSON(%v) = %s, wrong Online", tt.online, js)
		}
	}
}
