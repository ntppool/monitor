package mqserver

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/ulid"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/prometheus/client_golang/prometheus"
)

func TestResponseHandler(t *testing.T) {

	ctx := context.Background()

	mqs, err := Setup(logger.Setup(), nil, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	mqs.MQTTStatusHandler(&paho.Publish{
		Topic:   "/devel/monitors/status/uspao-abc/status",
		Payload: []byte("{\"Online\":true}"),
	})

	id, err := ulid.MakeULID(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	responseTopicParm := fmt.Sprintf("%s/data/#", "/devel/monitors")
	rr := mqs.setupResponseRouter(ctx, responseTopicParm)

	topic := fmt.Sprintf("%s/data/%s/%s", "/devel/monitors", "uspao-abc", id.String())

	rc := make(chan *paho.Publish)

	wg := sync.WaitGroup{}

	rr.AddResponseID(id.String(), rc)

	wg.Add(1)
	go func() {
		defer wg.Done()

		select {
		case p := <-rc:
			t.Logf("got publish message: %+v", p)
		case <-time.After(2 * time.Second):
			t.Log("didn't get a message")
			t.Fail()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		msg := &paho.Publish{
			Topic:   topic,
			Payload: []byte("{\"Test\":true}"),
		}
		msg.InitProperties(&packets.Properties{})

		t.Logf("sending message: %+v", msg)

		h := rr.Handler()
		h(msg)

	}()

	wg.Wait()

	rr.CloseResponseID(id.String())

}
