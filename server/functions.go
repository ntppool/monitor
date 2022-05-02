package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	sctx "go.ntppool.org/monitor/server/context"

	"github.com/twitchtv/twirp"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/ntpdb"
	"inet.af/netaddr"
)

func (srv *Server) getMonitor(ctx context.Context) (*ntpdb.Monitor, error) {

	if mon, ok := ctx.Value(sctx.MonitorKey).(*ntpdb.Monitor); ok {
		if mon == nil {
			log.Printf("got cached nil mon")
		}
		return mon, nil
	}

	cn := getCertificateName(ctx)

	log.Printf("cn: %+v, getting monitor", cn)

	monitor, err := srv.db.GetMonitorTLSName(ctx, sql.NullString{String: cn, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			err = twirp.NotFoundError("no such monitor")
		}
		ctx = context.WithValue(ctx, sctx.MonitorKey, nil)
		return nil, err
	}

	log.Printf("cn: %+v, got monitor %s, storing in context", cn, monitor.TlsName.String)

	ctx = context.WithValue(ctx, sctx.MonitorKey, monitor)

	log.Printf("cn: %+v, returning", cn)

	return &monitor, nil
}

func (srv *Server) GetConfig(ctx context.Context, in *pb.GetConfigParams) (*pb.Config, error) {

	ua := ctx.Value(sctx.ClientVersion).(string)
	log.Printf("user agent: %v", ua)

	monitor, err := srv.getMonitor(ctx)
	if err != nil {
		return nil, err
	}
	srv.db.UpdateMonitorSeen(ctx, ntpdb.UpdateMonitorSeenParams{
		ID:       monitor.ID,
		LastSeen: sql.NullTime{Time: time.Now(), Valid: true},
	})

	// the client always starts by getting a config, so we just track the user-agent here
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
	} else {
		cfg, err = monitor.GetConfig()
		if err != nil {
			return nil, err
		}
	}

	return cfg.PbConfig()
}

func (srv *Server) GetServers(ctx context.Context, in *pb.GetServersParams) (*pb.ServerList, error) {
	monitor, err := srv.getMonitor(ctx)
	if err != nil {
		return nil, err
	}

	log.Printf("GetServers(%s) starting", monitor.TlsName.String)

	srv.db.UpdateMonitorSeen(ctx, ntpdb.UpdateMonitorSeenParams{
		ID:       monitor.ID,
		LastSeen: sql.NullTime{Time: time.Now(), Valid: true},
	})

	if !monitor.IsLive() {
		return nil, fmt.Errorf("monitor not active")
	}

	intervalMinutes := 8
	intervalMinutesAll := 2

	if monitor.Status != ntpdb.MonitorsStatusActive {
		intervalMinutes = 60
	}

	p := ntpdb.GetServersParams{
		MonitorID:          monitor.ID,
		IpVersion:          ntpdb.ServersIpVersion(monitor.IpVersion),
		IntervalMinutes:    intervalMinutes,
		IntervalMinutesAll: intervalMinutesAll,
		Limit:              10,
		Offset:             0,
	}

	log.Printf("GetServers(%s) making BatchID", monitor.TlsName.String)

	now := time.Now()
	batchID, err := makeULID(now)
	if err != nil {
		return nil, err
	}

	log.Printf("GetServers(%s)/%s GetConfig", monitor.TlsName.String, batchID.String())

	mcfg, err := monitor.GetConfig()
	if err != nil {
		return nil, err
	}

	cfg, err := mcfg.PbConfig()
	if err != nil {
		return nil, err
	}

	log.Printf("GetServers(%s)/%s GetServers", monitor.TlsName.String, batchID.String())

	servers, err := srv.db.GetServers(ctx, p)
	if err != nil {
		return nil, err
	}

	pServers := []*pb.Server{}

	bidb, err := batchID.MarshalText()
	if err != nil {
		return nil, err
	}

	log.Printf("GetServers(%s)/%s serializing data", monitor.TlsName.String, batchID.String())

	for _, server := range servers {
		pServer := &pb.Server{}

		ip, err := netaddr.ParseIP(server.Ip)
		if err != nil {
			return nil, err
		}
		pServer.IPBytes, _ = ip.MarshalBinary()

		pServer.Ticket, err = srv.tokens.Sign(monitor.ID, bidb, &ip)
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
		log.Printf("GetServers() BatchID for monitor %d: %s", monitor.ID, batchID.String())
		srv.m.TestsRequested.WithLabelValues(monitor.TlsName.String, monitor.IpVersion.String()).Add(float64(count))
	}

	log.Printf("GetServers(%s)/%s done", monitor.TlsName.String, batchID.String())

	return list, nil
}

func (srv *Server) updateUserAgent(ctx context.Context, mon *ntpdb.Monitor) error {
	ua := ctx.Value(sctx.ClientVersion).(string)

	ua = strings.TrimPrefix(ua, "ntppool-monitor/")

	if ua != mon.ClientVersion {
		srv.db.UpdateMonitorVersion(ctx, ntpdb.UpdateMonitorVersionParams{
			ClientVersion: ua,
			ID:            mon.ID,
		})
	}

	return nil
}
