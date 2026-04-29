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

	rr.AddResponseID(ctx, id.String(), rc)

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

func TestResponseHandlerReceiverGone(t *testing.T) {
	mqs, err := Setup(logger.Setup(), nil, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	rr := mqs.setupResponseRouter(context.Background(), "/devel/monitors/data/#")

	id, err := ulid.MakeULID(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	rc := make(chan *paho.Publish)
	rr.AddResponseID(reqCtx, id.String(), rc)

	msg := &paho.Publish{
		Topic:   fmt.Sprintf("/devel/monitors/data/%s/%s", "uspao-abc", id.String()),
		Payload: []byte("{}"),
	}
	msg.InitProperties(&packets.Properties{})

	handlerDone := make(chan struct{})
	go func() {
		h := rr.Handler()
		h(msg)
		close(handlerDone)
	}()

	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("handler blocked despite receiver context being done")
	}

	// AddResponseID must not block while a stuck handler is in flight.
	addDone := make(chan struct{})
	go func() {
		rr.AddResponseID(context.Background(), "next-id", make(chan *paho.Publish))
		close(addDone)
	}()
	select {
	case <-addDone:
	case <-time.After(2 * time.Second):
		t.Fatal("AddResponseID blocked after stuck handler — router lock starved")
	}

	rr.CloseResponseID(id.String())
	rr.CloseResponseID("next-id")
}
