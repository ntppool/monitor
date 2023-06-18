package mqserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slog"
	"golang.org/x/mod/semver"
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
	"go.ntppool.org/monitor/version"
)

type server struct {
	cm     *autopaho.ConnectionManager
	db     *ntpdb.Queries
	dbconn *sql.DB
	ctx    context.Context
	log    *slog.Logger

	clients map[string]*client
	cmux    sync.RWMutex

	rr *mqttResponseRouter
}

type client struct {
	Name     string
	Online   bool
	Version  version.Info
	LastSeen time.Time
	Data     *ntpdb.Monitor
}

func Setup(log *slog.Logger, dbconn *sql.DB) (*server, error) {
	clients := map[string]*client{}
	mqs := &server{clients: clients, dbconn: dbconn, log: log.WithGroup("mqtt")}
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

	mqs.log.Debug("statusTopic", "name", statusTopic)

	router.RegisterHandler(statusTopic, mqs.MQTTStatusHandler)
	router.RegisterHandler(responseTopic, rr.Handler())

	return router
}

func (mqs *server) MQTTStatusHandler(p *paho.Publish) {
	mqs.log.Debug("status message", "packetID", p.PacketID, "topic", p.Topic, "payload", p.Payload)

	path := strings.Split(p.Topic, "/")

	if t := path[len(path)-1]; t != "status" {
		mqs.log.Info("skipping mqtt message", "parsed-topic", t, "topic", p.Topic, "payload", p.Payload)
		return
	}

	status := mqttcm.StatusMessage{}
	err := json.Unmarshal(p.Payload, &status)
	if err != nil {
		mqs.log.Error("could not unmarshal status message", "err", err)
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
		cs.Version = status.Version

		ctx := context.Background()

		if mqs.db != nil {
			mon, err := mqs.db.GetMonitorTLSName(ctx, sql.NullString{String: name, Valid: true})
			if err != nil {
				mqs.log.Error("fetching monitor details", "err", err)
			}
			if mon.ID != 0 {
				cs.Data = &mon
				cs.Name = mon.TlsName.String
			}
		}

	} else {
		cs.Online = false
	}

	mqs.log.Debug("new status", "status", cs)
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

		log := mqs.log.With("requestID", id)

		ip, err := netip.ParseAddr(c.Param("ip"))
		if err != nil {
			return c.JSON(400, err)
		}

		getAll := false
		if t, err := strconv.ParseBool(c.FormValue("all")); t || err != nil {
			if err != nil {
				log.Debug("parseBool error", "err", err, "t", t)
			}
			if t {
				getAll = t
			}
		}

		log = log.With("check-ip", ip)

		ctx, cancel := context.WithTimeout(c.Request().Context(), time.Second*8)
		defer cancel()

		// spanContext := otrace.SpanContextFromContext(ctx)

		span := otrace.SpanFromContext(ctx)

		wg, ctx := errgroup.WithContext(ctx)

		rc := make(chan *paho.Publish)

		span.SetAttributes(attribute.String("Request ID", id.String()))
		mqs.rr.AddResponseID(id.String(), rc)
		defer mqs.rr.CloseResponseID(id.String())

		counter := sync.WaitGroup{}

		counter.Add(1) // count one for the goroutine running

		wg.Go(func() error {

			i := 0

			for _, cl := range mqs.seenClients() {

				log := log.With("name", cl.Name)

				log.Debug("considering mqtt client", "client", cl.Name)

				if !cl.Online || cl.Data == nil {
					continue
				}

				if (cl.Data.IpVersion.MonitorsIpVersion.String() == "v6" && ip.Is4()) ||
					(cl.Data.IpVersion.MonitorsIpVersion.String() == "v4" && ip.Is6()) {
					continue
				}

				// minimum version
				minimumVersion := "v3.5.0-rc0"
				if semver.Compare(cl.Version.Version, minimumVersion) < 0 {
					// log.Debug("version too old", "v", cl.Version.Version)
					continue
				}

				if !getAll && i > 0 {

					log.Debug("taking a pause")

					select {
					case <-time.After(400 * time.Millisecond):
					case <-ctx.Done():
						log.Debug("context done, don't send more requests")
						return nil
					}

				}

				i++

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

				counter.Add(1)
				pubResp, err := mqs.cm.Publish(ctx, publishPacket)
				if err != nil {
					log.Warn("error sending request", "err", err)
					counter.Done()
				}
				if pubResp != nil && pubResp.ReasonCode != 0 {
					log.Warn("unexpected reasoncode in mqtt response", "code", pubResp.ReasonCode)
				}
			}
			counter.Done()
			return nil
		})

		var r []*api.NTPResponse
		healthyResponses := 0

		waitCh := make(chan struct{})

		go func() {
			counter.Wait()
			close(waitCh)
		}()

		wg.Go(func() error {
			log.Debug("waiting for mqtt responses")

			for {
				select {

				case <-waitCh:
					log.Debug("got all responses")
					return nil

				case p, ok := <-rc:
					if !ok {
						return nil
					}

					counter.Done()

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
						log.Warn("could not unmarshal payload", "err", err)
						continue
					}
					resp.Server = host

					log.Debug("queuing response", "resp", resp)

					if resp.NTP != nil && resp.Error == "" {
						if resp.NTP.Leap < 4 && resp.NTP.Stratum > 0 && resp.NTP.Stratum < 8 {
							healthyResponses++
						}
					}
					r = append(r, &resp)

					if !getAll && healthyResponses >= 3 {
						cancel()
					}

					continue

				case <-ctx.Done():
					log.Debug("request cancelled")
					return ctx.Err()
				}
			}
		})

		err = wg.Wait()

		log.Debug("done waiting, sending response", "len", len(r), "r", r)

		if err != nil &&
			!errors.Is(err, context.DeadlineExceeded) &&
			!errors.Is(err, context.Canceled) {
			log.Warn("check/ntp error", "err", err)
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
			Version   version.Info `json:",omitempty"`
			LastSeen  time.Time
			Online    bool
		}
		r := []onlineJSON{}

		for _, o := range mqs.seenClients() {
			if o.Data != nil {
				r = append(r, onlineJSON{
					Name:      o.Data.TlsName.String,
					IpVersion: o.Data.IpVersion.MonitorsIpVersion.String(),
					Version:   o.Version,
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
