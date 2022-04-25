package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/twitchtv/twirp"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/ntpdb"
	"inet.af/netaddr"
)

func getCertificateName(ctx context.Context) string {
	cn := ctx.Value(certificateKey)
	if name, ok := cn.(string); ok {
		return name
	}
	log.Fatalf("certificateKey didn't return a string")
	return ""
}

func (srv *Server) getMonitor(ctx context.Context) (*ntpdb.Monitor, error) {
	cn := getCertificateName(ctx)
	log.Printf("cn: %+v", cn)

	monitor, err := srv.db.GetMonitorTLSName(ctx, sql.NullString{String: cn, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, twirp.NotFoundError("no such monitor")
		}
		return nil, err
	}

	return &monitor, nil
}

func (srv *Server) GetConfig(ctx context.Context, in *pb.GetConfigParams) (*pb.Config, error) {
	monitor, err := srv.getMonitor(ctx)
	if err != nil {
		return nil, err
	}
	return monitor.GetPbConfig()
}

func (srv *Server) GetServers(ctx context.Context, in *pb.GetServersParams) (*pb.ServerList, error) {
	monitor, err := srv.getMonitor(ctx)
	if err != nil {
		return nil, err
	}

	if !monitor.IsLive() {
		return nil, fmt.Errorf("monitor not active")
	}

	intervalMinutes := 8

	if monitor.Status == ntpdb.MonitorsStatusTesting {
		intervalMinutes = 60

	}

	p := ntpdb.GetServersParams{
		MonitorID:          monitor.ID,
		IpVersion:          ntpdb.ServersIpVersion(monitor.IpVersion),
		IntervalMinutes:    intervalMinutes,
		IntervalMinutesAll: 3,
		Limit:              10,
		Offset:             0,
	}

	cfg, err := monitor.GetPbConfig()
	if err != nil {
		return nil, err
	}

	servers, err := srv.db.GetServers(ctx, p)
	if err != nil {
		return nil, err
	}

	pServers := []*pb.Server{}

	now := time.Now()
	batchID, err := makeULID(now)
	if err != nil {
		return nil, err
	}

	bidb, err := batchID.MarshalText()
	if err != nil {
		return nil, err
	}

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

	if len(pServers) > 0 {
		log.Printf("GetServers() BatchID for monitor %d: %s", monitor.ID, batchID.String())
	}

	return list, nil
}
