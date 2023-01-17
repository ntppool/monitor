package mqserver

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"go.ntppool.org/monitor/server/ulid"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
)

func TestResponseHandler(t *testing.T) {

	ctx := context.Background()

	mqs, err := Setup()
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
			log.Printf("got publish message: %+v", p)
		case <-time.After(2 * time.Second):
			log.Printf("didn't get a message")
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

		log.Printf("sending message: %+v", msg)

		h := rr.Handler()
		h(msg)

	}()

	wg.Wait()

	rr.CloseResponseID(id.String())

}
