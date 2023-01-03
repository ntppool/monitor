package mqserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"go.ntppool.org/monitor/mqttcm"
)

type server struct {
	clients map[string]client
	cmux    sync.RWMutex
	// todo: channel for messages?
}

type client struct {
	Online   bool
	LastSeen time.Time
}

func Setup() (*server, error) {
	clients := map[string]client{}
	return &server{clients: clients}, nil
}

func (srv *server) Run(ctx context.Context) error {
	time.Sleep(10 * time.Second)
	return fmt.Errorf("mqserver.Run not implemented")
}
func (mqs *server) MQTTRouter(ctx context.Context, statusTopic string) paho.Router {
	router := paho.NewStandardRouter()
	router.SetDebugLogger(log.Default())
	router.RegisterHandler(statusTopic, mqs.MQTTHandler)

	return router
}

func (mqs *server) MQTTHandler(p *paho.Publish) {
	log.Printf("server got message %d on %q: %s", p.PacketID, p.Topic, p.Payload)

	status := mqttcm.StatusMessage{}
	err := json.Unmarshal(p.Payload, &status)
	if err != nil {
		log.Printf("could not unmarshal status message: %s", err)
		return
	}

	path := strings.Split(p.Topic, "/")
	// log.Printf("l: %d, p: %+v", len(path), path)

	if t := path[len(path)-1]; t != "online" {
		log.Printf("skipping %q message from %q (%s)", t, p.Topic, p.Payload)
		return
	}

	name := path[len(path)-2]

	mqs.cmux.Lock()
	defer mqs.cmux.Unlock()

	cs, ok := mqs.clients[name]
	if !ok {
		cs = client{}
		mqs.clients[name] = cs
	}

	if status.Online {
		cs.Online = true
		cs.LastSeen = time.Now()
	} else {
		cs.Online = false
	}

}

/*

- http api (internal listener)
  - /mq/clients -- list client status
  - /mq/ntp -- run an immediate ntp check
    - list active clients
	  - just ok mode:
  	    - send ntp request to each, one second apart
          - stop on the first positive response
	  - all mode
	    - send check to all clients
		  - wait up to 2 seconds for results
*/
