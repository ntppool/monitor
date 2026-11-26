// http request to mqtt requests api
package mqserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/labstack/echo/v5"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/ekko"
	"go.ntppool.org/common/ulid"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/mqttcm"
	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"
)

// staleAfter is how long a client stays online without a new "online"
// status message. Agents republish their status every 5 minutes, so this
// is three missed republishes. It covers agents whose "offline" message
// never reaches us; the broker doesn't tell subscribers when a retained
// status message expires.
const staleAfter = 15 * time.Minute

type server struct {
	cm     *autopaho.ConnectionManager
	db     *ntpdb.Queries
	dbconn *pgxpool.Pool
	log    *slog.Logger

	// clients and depEnv are protected by cmux
	clients map[string]*client
	cmux    sync.RWMutex

	promRegistry prometheus.Registerer

	rr *mqttResponseRouter

	depEnv depenv.DeploymentEnvironment
}

type client struct {
	Name      string
	Online    bool
	Version   version.Info
	UpdatedMQ time.Time
	LastSeen  time.Time
	Data      []ntpdb.Monitor
}

// isOnline reports whether the client's last status was "online" and
// LastSeen is within staleAfter of now.
func (c *client) isOnline(now time.Time) bool {
	return c.Online && now.Sub(c.LastSeen) <= staleAfter
}

func Setup(log *slog.Logger, dbconn *pgxpool.Pool, promRegistry prometheus.Registerer) (*server, error) {
	clients := map[string]*client{}
	mqs := &server{clients: clients, dbconn: dbconn, log: log.WithGroup("mqtt")}
	if dbconn != nil {
		// tests run without the db
		mqs.db = ntpdb.New(dbconn)
	}

	err := promRegistry.Register(&connectedCollector{mqs: mqs})
	if err != nil {
		return nil, err
	}
	mqs.promRegistry = promRegistry

	return mqs, nil
}

func (mqs *server) SetConnectionManager(cm *autopaho.ConnectionManager) {
	mqs.cm = cm
}

func (mqs *server) Run(ctx context.Context) error {
	ek, err := ekko.New("mqserver",
		ekko.WithPort(8095),
		ekko.WithPrometheus(mqs.promRegistry),
		ekko.WithEchoSetup(func(e *echo.Echo) error {
			mqs.setupRoutes(ctx, e)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("mqserver: setting up http server: %w", err)
	}

	return ek.Start(ctx)
}

func (mqs *server) MQTTRouter(ctx context.Context, topicPrefix string) paho.Router {
	router := paho.NewStandardRouter()
	// router.SetDebugLogger(log.Default())

	depEnv := sctx.GetDeploymentEnvironment(ctx)
	mqs.cmux.Lock()
	mqs.depEnv = depEnv
	mqs.cmux.Unlock()

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

// legacyStatusExpiry is the message expiry, in seconds, that agents v3.7.0
// to v4.2.x set on their status without sending it in the status message.
const legacyStatusExpiry = 86400

// statusAge returns how long the broker held a status message, by the
// broker's clock: the expiry the agent set minus what the broker has left.
// A retained status the broker replays when we subscribe, after a restart
// or reconnect, can be up to a day old; a live one has an age of 0. A
// message without an expiry (agents before v3.7.0) has an age of 0.
func statusAge(p *paho.Publish, status mqttcm.StatusMessage) (time.Duration, error) {
	if p.Properties == nil || p.Properties.MessageExpiry == nil {
		return 0, nil
	}
	set := status.MessageExpiry
	if set == 0 {
		set = legacyStatusExpiry
	}
	left := *p.Properties.MessageExpiry
	if left > set {
		return 0, fmt.Errorf("message expiry left %ds is more than the %ds the agent set", left, set)
	}
	return time.Duration(set-left) * time.Second, nil
}

func (mqs *server) MQTTStatusHandler(p *paho.Publish) {
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
	// mqs.log.Info("status message unmarshal", "msg", p.Payload, "decoded", status, "updated_time", status.Updated)

	name := path[len(path)-2]

	var expiryLeft any
	if p.Properties != nil && p.Properties.MessageExpiry != nil {
		expiryLeft = *p.Properties.MessageExpiry
	}
	mqs.log.Debug("status message", "name", name, "online", status.Online,
		"retain", p.Retain, "updated_mq", status.UpdatedMQ,
		"message_expiry", status.MessageExpiry, "expiry_left", expiryLeft)

	mqs.cmux.Lock()
	defer mqs.cmux.Unlock()

	cs, ok := mqs.clients[name]
	if !ok {
		cs = &client{}
		mqs.clients[name] = cs
	}

	if status.Online {
		cs.Online = true

		// A status of unknown age doesn't refresh LastSeen. Never move
		// LastSeen back, for example when a reconnect replays a status
		// older than one we already got.
		if age, err := statusAge(p, status); err != nil {
			mqs.log.Warn("status message age", "name", name, "err", err)
		} else if seen := time.Now().Add(-age); seen.After(cs.LastSeen) {
			cs.LastSeen = seen
		}
		cs.Version = status.Version
		cs.UpdatedMQ = status.UpdatedMQ

		ctx := context.Background()

		if mqs.db != nil { // for running tests without the DB
			mons, err := mqs.db.GetMonitorsTLSName(ctx, pgtype.Text{String: name, Valid: true})
			if err != nil {
				mqs.log.Error("fetching monitor details", "err", err, "tls_name", name)
			} else {
				mqs.log.Debug("fetched monitor data", "tls_name", name, "monitor_count", len(mons))
				for i, m := range mons {
					ipVer := "invalid"
					if m.IpVersion.Valid {
						ipVer = m.IpVersion.MonitorsIpVersion.String()
					}
					mqs.log.Debug("monitor entry",
						"index", i,
						"id", m.ID,
						"hostname", m.Hostname,
						"ip_version", ipVer,
						"status", string(m.Status))
				}
			}
			if len(mons) > 0 {
				cs.Data = mons
				cs.Name = mons[0].TlsName.String
			} else {
				mqs.log.Debug("no monitors found for TLS name", "tls_name", name)
			}
		}

	} else {
		cs.Online = false
	}

	// mqs.log.Debug("new status", "cs", cs, "status", status)
	// mqs.log.Info("new map", "clients", mqs.clients)
}

// connectedCollector exports monitors_connected from the client list when
// it's scraped, so clients that go stale drop out without a new status
// message.
type connectedCollector struct {
	mqs *server
}

var monitorsConnectedDesc = prometheus.NewDesc(
	"monitors_connected",
	"monitors connected via mqtt",
	[]string{"client", "version", "ip_version", "account", "account_id", "depenv"},
	nil,
)

func (cc *connectedCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- monitorsConnectedDesc
}

func (cc *connectedCollector) Collect(ch chan<- prometheus.Metric) {
	mqs := cc.mqs

	// the same label set can come from more than one client
	type labelSet [5]string
	counts := map[labelSet]float64{}

	for _, c := range mqs.seenClients() {
		if !c.Online {
			continue
		}
		for _, m := range c.Data {
			if !m.IpVersion.Valid {
				continue
			}

			accountID := "0"
			if m.AccountID.Valid {
				accountID = strconv.Itoa(int(m.AccountID.Int64))
			}
			counts[labelSet{
				m.Hostname,
				c.Version.Version,
				m.IpVersion.MonitorsIpVersion.String(),
				"", // account id_token not available in MQTT context
				accountID,
			}]++
		}
	}

	if len(counts) == 0 {
		return
	}

	// Clients only come from status messages, which arrive after
	// MQTTRouter has set depEnv. Its zero value panics in String(), and a
	// scrape can come before MQTTRouter runs.
	mqs.cmux.RLock()
	depEnv := mqs.depEnv.String()
	mqs.cmux.RUnlock()

	for l, n := range counts {
		ch <- prometheus.MustNewConstMetric(
			monitorsConnectedDesc, prometheus.GaugeValue, n,
			l[0], l[1], l[2], l[3], l[4], depEnv,
		)
	}
}

// seenClients returns a copy of every known client. Online is false for
// clients whose last "online" status is older than staleAfter.
func (mqs *server) seenClients() []client {
	mqs.cmux.RLock()
	defer mqs.cmux.RUnlock()

	now := time.Now()
	online := []client{}

	for _, c := range mqs.clients {
		cl := *c
		cl.Online = c.isOnline(now)
		online = append(online, cl)
	}

	// todo: shuffle list

	return online
}

// supportsAdHocNTPCheck returns true if the client version supports ad hoc NTP checks.
// This requires version 4.0.4 or newer, OR exactly version 3.8.6.
func supportsAdHocNTPCheck(clientVersion string) bool {
	// Check for version 4.0.4 or newer
	if version.CheckVersion(clientVersion, "v4.0.4") {
		return true
	}
	// Check for exactly version 3.8.6
	if version.CheckVersion(clientVersion, "v3.8.6") && !version.CheckVersion(clientVersion, "v3.8.7") {
		return true
	}
	return false
}

func (mqs *server) CheckNTP(ctx context.Context) func(*echo.Context) error {
	depEnv := sctx.GetDeploymentEnvironment(ctx)
	topics := mqttcm.NewTopics(depEnv)

	return func(c *echo.Context) error {
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

		span := otrace.SpanFromContext(ctx)

		wg, ctx := errgroup.WithContext(ctx)

		rc := make(chan *paho.Publish)

		span.SetAttributes(attribute.String("Request-ID", id.String()))
		mqs.rr.AddResponseID(ctx, id.String(), rc)
		defer mqs.rr.CloseResponseID(id.String())

		counter := sync.WaitGroup{}

		counter.Add(1) // count one for the goroutine running

		wg.Go(func() error {
			i := 0

			clients := mqs.seenClients()
			log.Info("starting ad hoc NTP check", "target_ip", ip.String(), "total_clients", len(clients))

			for _, cl := range clients {

				log := log.With("name", cl.Name)

				log.Debug("considering mqtt client",
					"client", cl.Name,
					"online", cl.Online,
					"data_count", len(cl.Data),
					"version", cl.Version.Version)

				if !cl.Online {
					log.Debug("client offline, skipping")
					continue
				}

				if cl.Data == nil {
					log.Debug("client has no monitor data, skipping")
					continue
				}

				ipVersion := "v4"
				if ip.Is6() {
					ipVersion = "v6"
				}

				// Log available IP versions for this client
				availableVersions := []string{}
				for _, m := range cl.Data {
					if m.IpVersion.Valid {
						availableVersions = append(availableVersions, m.IpVersion.MonitorsIpVersion.String())
					}
				}
				log.Debug("client IP version check",
					"needed", ipVersion,
					"available", availableVersions)

				if !slices.ContainsFunc(cl.Data, func(m ntpdb.Monitor) bool {
					return m.IpVersion.MonitorsIpVersion.String() == ipVersion
				}) {
					log.Debug("client does not support required IP version, skipping", "needed", ipVersion)
					continue
				}

				if !supportsAdHocNTPCheck(cl.Version.Version) {
					log.Debug("client version does not support ad hoc checks, skipping", "version", cl.Version.Version)
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

				log.Info("sending ad hoc NTP request",
					"client", cl.Name,
					"target_ip", ip.String(),
					"request_index", i)
				span.AddEvent(fmt.Sprintf("sending request for %s", cl.Name))

				topic := topics.Request(cl.Name, "ntp")
				responseTopic := topics.DataResponse(cl.Name, id.String())

				log.Info("mqtt topics", "request_topic", topic, "response_topic", responseTopic)

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
					log.Error("failed to send MQTT request", "err", err, "topic", topic)
					counter.Done()
				} else {
					log.Info("MQTT request sent successfully", "topic", topic)
				}
				if pubResp != nil && pubResp.ReasonCode != 0 {
					log.Warn("unexpected reasoncode in mqtt response", "code", pubResp.ReasonCode, "topic", topic)
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

func (mqs *server) setupRoutes(ctx context.Context, r *echo.Echo) {
	r.GET("/monitors/online", func(c *echo.Context) error {
		type onlineJSON struct {
			Name       string
			IpVersions []string     `json:"IpVersions"`
			IpVersion  string       `json:"IpVersion,omitempty"` // Deprecated, for compatibility
			Version    version.Info `json:",omitempty"`
			UpdatedMQ  time.Time    `json:",omitempty"`
			LastSeen   time.Time
			Online     bool
		}
		r := []onlineJSON{}

		for _, o := range mqs.seenClients() {
			if o.Data != nil {
				// Collect unique IP versions for this monitor
				ipVersions := []string{}
				ipVersionSet := make(map[string]bool)

				for _, m := range o.Data {
					if m.IpVersion.Valid {
						ipVer := m.IpVersion.MonitorsIpVersion.String()
						if !ipVersionSet[ipVer] {
							ipVersionSet[ipVer] = true
							ipVersions = append(ipVersions, ipVer)
						}
					}
				}

				// For backward compatibility, set IpVersion to first version if only one
				legacyIpVersion := ""
				if len(ipVersions) == 1 {
					legacyIpVersion = ipVersions[0]
				}

				r = append(r, onlineJSON{
					Name:       o.Name,
					IpVersions: ipVersions,
					IpVersion:  legacyIpVersion,
					Version:    o.Version,
					Online:     o.Online,
					LastSeen:   o.LastSeen,
					UpdatedMQ:  o.UpdatedMQ,
				})
			}
		}

		return c.JSON(200, r)
	})

	r.POST("/check/ntp/:ip", mqs.CheckNTP(ctx))
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
