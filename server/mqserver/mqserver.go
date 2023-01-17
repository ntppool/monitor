package mqserver

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/beevik/ntp"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/monitor/mqttcm"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/ulid"
)

type server struct {
	cm  *autopaho.ConnectionManager
	ctx context.Context

	clients map[string]*client
	cmux    sync.RWMutex

	rr *mqttResponseRouter
}

type client struct {
	Online   bool
	LastSeen time.Time
}

func Setup() (*server, error) {
	clients := map[string]*client{}
	mqs := &server{clients: clients}
	return mqs, nil
}

func (srv *server) SetConnectionManager(cm *autopaho.ConnectionManager) {
	srv.cm = cm
}

func (srv *server) Run(ctx context.Context) error {
	// todo: should this be set in Setup() instead so it doesn't need to be passed here
	// and to MQTTRouter()?
	srv.ctx = ctx
	return srv.setupEcho()
}

func (mqs *server) MQTTRouter(ctx context.Context, topicPrefix string) paho.Router {
	router := paho.NewStandardRouter()
	router.SetDebugLogger(log.Default())

	depEnv := sctx.GetDeploymentEnvironment(ctx)

	topics := mqttcm.NewTopics(depEnv)

	statusTopic := topics.StatusSubscription()
	responseTopic := topics.DataResponseSubscription()

	rr := mqs.setupResponseRouter(ctx, responseTopic)
	mqs.rr = rr

	log.Printf("statusTopic: %q", statusTopic)

	router.RegisterHandler(statusTopic, mqs.MQTTStatusHandler)
	router.RegisterHandler(responseTopic, rr.Handler())

	return router
}

func (mqs *server) MQTTStatusHandler(p *paho.Publish) {
	log.Printf("server got status message %d on %q: %s", p.PacketID, p.Topic, p.Payload)

	path := strings.Split(p.Topic, "/")
	log.Printf("l: %d, p: %+v", len(path), path)

	if t := path[len(path)-1]; t != "status" {
		log.Printf("skipping %q message from %q (%s)", t, p.Topic, p.Payload)
		return
	}

	status := mqttcm.StatusMessage{}
	err := json.Unmarshal(p.Payload, &status)
	if err != nil {
		log.Printf("could not unmarshal status message: %s", err)
		return
	}

	name := path[len(path)-2]

	mqs.cmux.Lock()
	defer mqs.cmux.Unlock()

	cs, ok := mqs.clients[name]
	if !ok {
		cs = &client{}
		mqs.clients[name] = cs
	}

	log.Printf("parsed status: %+v", status)

	if status.Online {
		cs.Online = true
		cs.LastSeen = time.Now()
	} else {
		cs.Online = false
	}

	log.Printf("new status: %+v", cs)
	log.Printf("new map: %+v", mqs.clients)

}

func (mqs *server) onlineClients() []string {
	mqs.cmux.RLock()
	defer mqs.cmux.RUnlock()

	online := []string{}

	log.Printf("onlineClients: %+v", mqs.clients)

	for n, c := range mqs.clients {
		if !c.Online {
			continue
		}
		online = append(online, n)
	}

	// todo: shuffle list

	return online
}

func (mqs *server) CheckNTP() func(echo.Context) error {

	depEnv := sctx.GetDeploymentEnvironment(mqs.ctx)
	topics := mqttcm.NewTopics(depEnv)

	return func(c echo.Context) error {

		ctx, cancel := context.WithTimeout(mqs.ctx, time.Second*10)
		defer cancel()

		var r ntp.Response

		wg, ctx := errgroup.WithContext(ctx)

		rc := make(chan *paho.Publish)

		id, err := ulid.MakeULID(time.Now())
		if err != nil {
			return err
		}

		log.Printf("request ID %s", id)

		mqs.rr.AddResponseID(id.String(), rc)
		defer mqs.rr.CloseResponseID(id.String())

		wg.Go(func() error {

			for _, clientName := range mqs.onlineClients() {

				log.Printf("sending request for %s", clientName)

				topic := topics.Request(clientName, "ntp")
				responseTopic := topics.DataResponse(clientName, id.String())

				log.Printf("topics: %s => %s", topic, responseTopic)

				publishPacket := &paho.Publish{
					Topic: topic,
					Properties: &paho.PublishProperties{
						ResponseTopic: responseTopic,
						User:          paho.UserProperties{},
					},
					QoS:    0,
					Retain: false,
				}

				publishPacket.Properties.User.Add("name", "example")

				log.Printf("cm: %+v", mqs.cm)

				mqs.cm.Publish(ctx, publishPacket)

				select {

				case p := <-rc:
					log.Printf("got response publish message: %+v", p)

					return nil
				case <-time.After(time.Second * 2):
					log.Printf("per client timeout")
					continue

				case <-ctx.Done():
					log.Printf("overall timeout")
					return ctx.Err()
				}

			}
			return nil
		})

		err = wg.Wait()

		if err != nil {
			return c.JSON(500, err)
		}

		return c.JSON(200, r)
	}
}

func (mqs *server) setupEcho() error {

	r := echo.New()
	r.Use(otelecho.Middleware("mqserver"))

	r.GET("/monitors/online", func(c echo.Context) error {
		mqs.cmux.RLock()
		defer mqs.cmux.RUnlock()

		online := map[string]*client{}

		for n, c := range mqs.clients {
			if !c.Online {
				continue
			}
			online[n] = c
		}

		return c.JSON(200, online)
	})

	r.POST("/check/ntp", mqs.CheckNTP())

	err := r.Start(":8095")
	return err
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
