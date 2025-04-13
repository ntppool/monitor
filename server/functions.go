package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/common/timeutil"
	"go.ntppool.org/common/ulid"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/jwt"
)

type MonitorSettings struct {
	IntervalActive  timeutil.Duration `json:"interval_active"`
	IntervalTesting timeutil.Duration `json:"interval_testing"`
	IntervalAll     timeutil.Duration `json:"interval_all"`
	BatchSize       int32             `json:"batch_size"`
}

func (srv *Server) getMonitor(ctx context.Context) (*ntpdb.Monitor, context.Context, error) {
	log := srv.log

	if mon, ok := ctx.Value(sctx.MonitorKey).(*ntpdb.Monitor); ok {
		if mon == nil {
			log.Error("got cached nil mon")
		}
		return mon, ctx, nil
	}

	cn := getCertificateName(ctx)

	monitor, err := srv.db.GetMonitorTLSName(ctx, sql.NullString{String: cn, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			err = twirp.NotFoundError("no such monitor")
		}
		ctx = context.WithValue(ctx, sctx.MonitorKey, nil)
		return nil, ctx, err
	}

	// log.Printf("cn: %+v, got monitor %s (%T), storing in context", cn, monitor.TlsName.String, monitor)

	ctx = context.WithValue(ctx, sctx.MonitorKey, &monitor)

	return &monitor, ctx, nil
}

func (srv *Server) getMonitorConfig(ctx context.Context, monitor *ntpdb.Monitor) (*ntpdb.MonitorConfig, error) {
	span := otrace.SpanFromContext(ctx)

	// log := srv.log

	var cfg *ntpdb.MonitorConfig

	smon, err := ntpdb.GetSystemMonitor(ctx, srv.db, "settings", monitor.IpVersion)
	if err == nil {
		cfg, err = monitor.GetConfigWithDefaults([]byte(smon.Config))
		if err != nil {
			return nil, err
		}
		span.AddEvent("Merged Configs")
	} else {
		cfg, err = monitor.GetConfig()
		if err != nil {
			return nil, err
		}
		span.AddEvent("Single Config")
	}

	return cfg, nil
}

func (srv *Server) GetConfig(ctx context.Context) (*ntpdb.MonitorConfig, error) {
	span := otrace.SpanFromContext(ctx)

	log := srv.log

	ua := ctx.Value(sctx.ClientVersionKey).(string)
	log.DebugContext(ctx, "GetConfig", "user-agent", ua)

	monitor, ctx, err := srv.getMonitor(ctx)
	if err != nil {
		return nil, err
	}
	srv.db.UpdateMonitorSeen(ctx, ntpdb.UpdateMonitorSeenParams{
		ID:       monitor.ID,
		LastSeen: sql.NullTime{Time: time.Now(), Valid: true},
	})
	span.AddEvent("UpdateMonitorSeen")

	if !monitor.IsLive() {
		return nil, twirp.PermissionDenied.Error("monitor not active")
	}

	// the client always starts by getting a config, so we track the user-agent here
	if err = srv.updateUserAgent(ctx, monitor); err != nil {
		log.Error("error updating user-agent", "err", err)
	}

	cfg, err := srv.getMonitorConfig(ctx, monitor)
	if err != nil {
		log.Error("getMonitorConfig", "err", err)
		return nil, twirp.InternalError("could not get config")
	}

	if key := srv.cfg.JWTKey; len(key) > 0 {
		jwtToken, err := jwt.GetToken(ctx, key, monitor.TlsName.String, jwt.KeyTypeStandard)
		if err != nil {
			log.Error("error generating jwtToken", "err", err)
		}

		mqttPrefix := fmt.Sprintf("/%s/monitors", srv.cfg.DeploymentEnv)

		if len(jwtToken) > 0 {
			cfg.MQTT = &checkconfig.MQTTConfig{
				Host:   "mqtt.ntppool.net",
				Port:   1883,
				JWT:    []byte(jwtToken),
				Prefix: mqttPrefix,
			}
		}
	} else {
		log.Error("JWTKey not configured")
	}

	return cfg, nil
}

func (srv *Server) signatureIPData(monitorID uint32, batchID []byte, ip *netip.Addr) ([][]byte, error) {
	monIDb := strconv.AppendInt([]byte{}, int64(monitorID), 10)

	ipb, err := ip.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := [][]byte{monIDb, batchID, ipb}

	return data, nil
}

func (srv *Server) SignIPs(monitorID uint32, batchID []byte, ip *netip.Addr) ([]byte, error) {
	data, err := srv.signatureIPData(monitorID, batchID, ip)
	if err != nil {
		return nil, err
	}
	return srv.tokens.SignBytes(data...)
}

func (srv *Server) ValidateIPs(signature []byte, monitorID uint32, batchID []byte, ip *netip.Addr) (bool, error) {
	data, err := srv.signatureIPData(monitorID, batchID, ip)
	if err != nil {
		return false, err
	}
	return srv.tokens.ValidateBytes(signature, data...)
}

type ServerListResponse struct {
	BatchID []byte
	Config  *ntpdb.MonitorConfig
	Servers []ntpdb.Server
	monitor *ntpdb.Monitor
}

func (srv *Server) GetServers(ctx context.Context) (*ServerListResponse, error) {
	span := otrace.SpanFromContext(ctx)

	log := srv.log

	monitor, ctx, err := srv.getMonitor(ctx)
	if err != nil {
		return nil, err
	}

	srv.db.UpdateMonitorSeen(ctx, ntpdb.UpdateMonitorSeenParams{
		ID:       monitor.ID,
		LastSeen: sql.NullTime{Time: time.Now(), Valid: true},
	})

	if !monitor.IsLive() {
		return nil, twirp.PermissionDenied.Error("monitor not active")
	}

	monitorSettingsStr, err := srv.db.GetSystemSetting(ctx, "monitors")
	if err != nil {
		log.Warn("could not fetch monitor settings", "err", err)
	}
	var settings MonitorSettings
	if len(monitorSettingsStr) > 0 {
		err := json.Unmarshal([]byte(monitorSettingsStr), &settings)
		if err != nil {
			log.Warn("could not unmarshal monitor settings", "err", err)
		}
	}

	if settings.IntervalActive.Seconds() < 20 {
		settings.IntervalActive = timeutil.Duration{Duration: 9 * time.Minute}
	}
	if settings.IntervalTesting.Seconds() < 60 {
		settings.IntervalTesting = timeutil.Duration{Duration: 45 * time.Minute}
	}
	if settings.IntervalAll.Seconds() < 10 {
		settings.IntervalAll = timeutil.Duration{Duration: 60 * time.Second}
	}
	if settings.BatchSize <= 0 {
		settings.BatchSize = 10
	}

	log.Debug("interval settings", "intervals", settings)

	interval := settings.IntervalActive
	if monitor.Status != ntpdb.MonitorsStatusActive {
		interval = settings.IntervalTesting
	}

	p := ntpdb.GetServersParams{
		MonitorID:              monitor.ID,
		IpVersion:              ntpdb.ServersIpVersion(monitor.IpVersion.MonitorsIpVersion.String()),
		IntervalSeconds:        interval.Seconds(),
		IntervalSecondsTesting: settings.IntervalTesting.Seconds(),
		IntervalSecondsAll:     settings.IntervalAll.Seconds(),
		Limit:                  (settings.BatchSize),
		Offset:                 0,
	}

	now := time.Now()
	batchID, err := ulid.MakeULID(now)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.String("batchID", batchID.String()))

	log = log.With("traceID", span.SpanContext().TraceID(),
		"batchID", batchID.String(),
		"cn", monitor.TlsName.String,
	)
	log.With() // to avoid the not used warning above

	mcfg, err := srv.getMonitorConfig(ctx, monitor)
	if err != nil {
		return nil, err
	}

	tx, err := srv.dbconn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	db := ntpdb.New(srv.dbconn).WithTx(tx)

	servers, err := db.GetServers(ctx, p)
	if err != nil {
		return nil, err
	}

	span.AddEvent("GetServers DB select", otrace.WithAttributes(attribute.Int("serverCount", len(servers))))

	bidb, err := batchID.MarshalText()
	if err != nil {
		return nil, err
	}

	list := &ServerListResponse{
		BatchID: bidb,
		Config:  mcfg,
		Servers: servers,
		monitor: monitor,
	}

	if count := len(servers); count > 0 {
		srv.m.TestsRequested.WithLabelValues(monitor.TlsName.String, monitor.IpVersion.MonitorsIpVersion.String()).Add(float64(count))

		now := sql.NullTime{Time: time.Now(), Valid: true}

		ids := make([]uint32, len(servers))
		for i, s := range servers {
			ids[i] = s.ID
		}

		err = db.UpdateServerScoreQueue(ctx,
			ntpdb.UpdateServerScoreQueueParams{
				MonitorID: monitor.ID,
				QueueTs:   now,
				ServerIds: ids,
			},
		)
		if err != nil {
			return nil, err
		}

	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return list, nil
}

func (srv *Server) updateUserAgent(ctx context.Context, mon *ntpdb.Monitor) error {
	ua := ctx.Value(sctx.ClientVersionKey).(string)

	ua = strings.TrimPrefix(ua, "ntppool-monitor/")

	if ua != mon.ClientVersion {
		srv.db.UpdateMonitorVersion(ctx, ntpdb.UpdateMonitorVersionParams{
			ClientVersion: ua,
			ID:            mon.ID,
		})
	}

	return nil
}
