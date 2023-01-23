package mqserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/monitor/api"
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

		ip, err := netip.ParseAddr(c.Param("ip"))
		if err != nil {
			return c.JSON(400, err)
		}

		ctx, cancel := context.WithTimeout(c.Request().Context(), time.Second*10)
		defer cancel()

		// spanContext := otrace.SpanContextFromContext(ctx)

		span := otrace.SpanFromContext(ctx)

		var r []*api.NTPResponse

		wg, ctx := errgroup.WithContext(ctx)

		rc := make(chan *paho.Publish)

		id, err := ulid.MakeULID(time.Now())
		if err != nil {
			return err
		}

		log.Printf("request ID %s", id)

		span.SetAttributes(attribute.String("Request ID", id.String()))
		mqs.rr.AddResponseID(id.String(), rc)
		defer mqs.rr.CloseResponseID(id.String())

		wg.Go(func() error {

			for _, clientName := range mqs.onlineClients() {

				log.Printf("sending request for %s", clientName)
				span.AddEvent(fmt.Sprintf("sending request for %s", clientName))

				topic := topics.Request(clientName, "ntp")
				responseTopic := topics.DataResponse(clientName, id.String())

				log.Printf("topics: %s => %s", topic, responseTopic)

				data := struct {
					IP string
				}{
					IP: ip.String(),
				}

				js, err := json.Marshal(&data)
				if err != nil {
					return c.JSON(500, err)
				}

				publishPacket := &paho.Publish{
					Topic: topic,
					Properties: &paho.PublishProperties{
						ResponseTopic: responseTopic,
						User:          paho.UserProperties{},
					},
					QoS:     0,
					Retain:  false,
					Payload: js,
				}

				publishPacket.Properties.User.Add("ID", id.String())
				publishPacket.Properties.User.Add("TraceID", span.SpanContext().TraceID().String())
				publishPacket.Properties.User.Add("SpanID", span.SpanContext().SpanID().String())

				log.Printf("cm: %+v", mqs.cm)

				mqs.cm.Publish(ctx, publishPacket)

				select {

				case p := <-rc:

					_, host, err := topics.ParseRequestTopic(p.Topic)
					if err != nil {
						log.Printf("could not parse servername from %s", p.Topic)
						continue
					}

					host = host[:strings.Index(host, ".")]

					log.Printf("got response publish message from %s : %+v", host, p)
					span.AddEvent("got response from " + p.Topic)

					resp := api.NTPResponse{}

					err = json.Unmarshal(p.Payload, &resp)
					if err != nil {
						// todo: track the error but just log it if
						// another response works?
						continue
					}
					resp.Server = host
					r = append(r, &resp)
					continue

				case <-time.After(time.Second * 10):
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

	r.POST("/check/ntp/:ip", mqs.CheckNTP())

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
