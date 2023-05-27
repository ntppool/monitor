package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/jwt"
	"go.ntppool.org/monitor/server/ulid"
)

func (srv *Server) getMonitor(ctx context.Context) (*ntpdb.Monitor, context.Context, error) {

	if mon, ok := ctx.Value(sctx.MonitorKey).(*ntpdb.Monitor); ok {
		if mon == nil {
			log.Printf("got cached nil mon")
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

func (srv *Server) GetConfig(ctx context.Context, in *pb.GetConfigParams) (*pb.Config, error) {
	span := otrace.SpanFromContext(ctx)

	ua := ctx.Value(sctx.ClientVersionKey).(string)
	log.Printf("user agent: %v", ua)

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
		log.Printf("error updating user-agent: %s", err)
	}

	var cfg *ntpdb.MonitorConfig

	smon, err := srv.db.GetSystemMonitor(ctx, "settings", monitor.IpVersion)
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

	if key := srv.cfg.JWTKey; len(key) > 0 {
		jwtToken, err := jwt.GetToken(key, monitor.TlsName.String, false)
		if err != nil {
			log.Printf("error generating jwtToken: %s", err)
		}

		mqttPrefix := fmt.Sprintf("/%s/monitors", srv.cfg.DeploymentEnv)

		if len(jwtToken) > 0 {
			cfg.MQTT = &pb.MQTTConfig{
				Host:   []byte("mqtt.ntppool.net"),
				Port:   1883,
				JWT:    []byte(jwtToken),
				Prefix: []byte(mqttPrefix),
			}
		}
	} else {
		log.Printf("JWTKey not configured")
	}

	return cfg.PbConfig()
}

func (srv *Server) signatureIPData(monitorID int32, batchID []byte, ip *netip.Addr) ([][]byte, error) {

	monIDb := strconv.AppendInt([]byte{}, int64(monitorID), 10)

	ipb, err := ip.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := [][]byte{monIDb, batchID, ipb}

	return data, nil
}

func (srv *Server) SignIPs(monitorID int32, batchID []byte, ip *netip.Addr) ([]byte, error) {
	data, err := srv.signatureIPData(monitorID, batchID, ip)
	if err != nil {
		return nil, err
	}
	return srv.tokens.SignBytes(data...)
}

func (srv *Server) ValidateIPs(signature []byte, monitorID int32, batchID []byte, ip *netip.Addr) (bool, error) {
	data, err := srv.signatureIPData(monitorID, batchID, ip)
	if err != nil {
		return false, err
	}
	return srv.tokens.ValidateBytes(signature, data...)
}

func (srv *Server) GetServers(ctx context.Context, in *pb.GetServersParams) (*pb.ServerList, error) {
	span := otrace.SpanFromContext(ctx)

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

	interval := 9 * time.Minute
	intervalTesting := 45 * time.Minute
	intervalAll := 75 * time.Second

	if monitor.Status != ntpdb.MonitorsStatusActive {
		interval = intervalTesting
	}

	p := ntpdb.GetServersParams{
		MonitorID:              monitor.ID,
		IpVersion:              ntpdb.ServersIpVersion(monitor.IpVersion.MonitorsIpVersion.String()),
		IntervalSeconds:        interval.Seconds(),
		IntervalSecondsTesting: intervalTesting.Seconds(),
		IntervalSecondsAll:     intervalAll.Seconds(),
		Limit:                  10,
		Offset:                 0,
	}

	now := time.Now()
	batchID, err := ulid.MakeULID(now)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.String("batchID", batchID.String()))

	log.Printf("method=GetServers cn=%s traceID=%s batchID=%s",
		monitor.TlsName.String, span.SpanContext().TraceID(), batchID.String())

	mcfg, err := monitor.GetConfig()
	if err != nil {
		return nil, err
	}

	cfg, err := mcfg.PbConfig()
	if err != nil {
		return nil, err
	}

	servers, err := srv.db.GetServers(ctx, p)
	if err != nil {
		return nil, err
	}

	span.AddEvent("GetServers DB select", otrace.WithAttributes(attribute.Int("serverCount", len(servers))))

	pServers := []*pb.Server{}

	bidb, err := batchID.MarshalText()
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		pServer := &pb.Server{}

		ip, err := netip.ParseAddr(server.Ip)
		if err != nil {
			return nil, err
		}
		pServer.IPBytes, _ = ip.MarshalBinary()

		pServer.Ticket, err = srv.SignIPs(monitor.ID, bidb, &ip)
		if err != nil {
			return nil, err
		}

		pServers = append(pServers, pServer)
	}

	list := &pb.ServerList{
		Config:  cfg,
		Servers: pServers,
		BatchID: bidb,
	}

	if count := len(pServers); count > 0 {
		srv.m.TestsRequested.WithLabelValues(monitor.TlsName.String, monitor.IpVersion.MonitorsIpVersion.String()).Add(float64(count))
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
