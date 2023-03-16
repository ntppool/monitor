package mqserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/mqttcm"
	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/ulid"
)

type server struct {
	cm     *autopaho.ConnectionManager
	db     *ntpdb.Queries
	dbconn *sql.DB
	ctx    context.Context

	clients map[string]*client
	cmux    sync.RWMutex

	rr *mqttResponseRouter
}

type client struct {
	Name     string
	Online   bool
	LastSeen time.Time
	Data     *ntpdb.Monitor
}

func Setup(dbconn *sql.DB) (*server, error) {
	clients := map[string]*client{}
	mqs := &server{clients: clients, dbconn: dbconn}
	if dbconn != nil {
		// tests run without the db
		mqs.db = ntpdb.New(dbconn)
	}

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
	// router.SetDebugLogger(log.Default())

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
	slog.Debug("status message", "packetID", p.PacketID, "topic", p.Topic, "payload", p.Payload)

	path := strings.Split(p.Topic, "/")

	if t := path[len(path)-1]; t != "status" {
		slog.Info("skipping mqtt message", "parsed-topic", t, "topic", p.Topic, "payload", p.Payload)
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

	if status.Online {
		cs.Online = true
		cs.LastSeen = time.Now()

		ctx := context.Background()

		if mqs.db != nil {
			mon, err := mqs.db.GetMonitorTLSName(ctx, sql.NullString{String: name, Valid: true})
			if err != nil {
				log.Printf("fetching monitor details: %s", err)
			}
			if mon.ID != 0 {
				cs.Data = &mon
				cs.Name = mon.TlsName.String
			}
		}

	} else {
		cs.Online = false
	}

	slog.Debug("new status", "status", cs)
	// log.Printf("new map: %+v", mqs.clients)

}

func (mqs *server) seenClients() []client {
	mqs.cmux.RLock()
	defer mqs.cmux.RUnlock()

	online := []client{}

	for _, c := range mqs.clients {
		online = append(online, *c)
	}

	// todo: shuffle list

	return online
}

func (mqs *server) CheckNTP() func(echo.Context) error {

	depEnv := sctx.GetDeploymentEnvironment(mqs.ctx)
	topics := mqttcm.NewTopics(depEnv)

	return func(c echo.Context) error {

		id, err := ulid.MakeULID(time.Now())
		if err != nil {
			return err
		}

		log := slog.With("requestID", id)

		ip, err := netip.ParseAddr(c.Param("ip"))
		if err != nil {
			return c.JSON(400, err)
		}

		log = log.With("ip", ip)

		ctx, cancel := context.WithTimeout(c.Request().Context(), time.Second*10)
		defer cancel()

		// spanContext := otrace.SpanContextFromContext(ctx)

		span := otrace.SpanFromContext(ctx)

		var r []*api.NTPResponse

		wg, ctx := errgroup.WithContext(ctx)

		rc := make(chan *paho.Publish)

		span.SetAttributes(attribute.String("Request ID", id.String()))
		mqs.rr.AddResponseID(id.String(), rc)
		defer mqs.rr.CloseResponseID(id.String())

		wg.Go(func() error {

			for _, cl := range mqs.seenClients() {

				if !cl.Online || cl.Data == nil {
					continue
				}

				if cl.Data.IpVersion.MonitorsIpVersion.String() == "v6" && ip.Is4() {
					continue
				}

				log.Info("sending request", "name", cl.Name)
				span.AddEvent(fmt.Sprintf("sending request for %s", cl.Name))

				topic := topics.Request(cl.Name, "ntp")
				responseTopic := topics.DataResponse(cl.Name, id.String())

				log.Debug("topics", "topic", topic, "responseTopic", responseTopic)

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

				// log.Printf("cm: %+v", mqs.cm)

				mqs.cm.Publish(ctx, publishPacket)

				select {

				case p := <-rc:

					_, host, err := topics.ParseRequestTopic(p.Topic)
					if err != nil {
						log.Error("could not parse servername", "topic", p.Topic)
						continue
					}

					host = host[:strings.Index(host, ".")]

					log.Info("response publish message", "from", host, "payload", p)
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
					log.Info("per client timeout")
					continue

				case <-ctx.Done():
					log.Info("overall timeout")
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
		type onlineJSON struct {
			Name      string
			IpVersion string
			LastSeen  time.Time
			Online    bool
		}
		r := []onlineJSON{}

		for _, o := range mqs.seenClients() {
			if o.Data != nil {
				r = append(r, onlineJSON{
					Name:      o.Data.TlsName.String,
					IpVersion: o.Data.IpVersion.MonitorsIpVersion.String(),
					Online:    o.Online,
					LastSeen:  o.LastSeen,
				})
			}
		}

		return c.JSON(200, r)
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
