package server

import (
	"context"
	"net/netip"

	"github.com/twitchtv/twirp"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/api/pb"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

type TwServer struct {
	srv *Server
	// ctx context.Context
	// cfg         *Config
	// tokens      *vtm.TokenManager
	// m           *metrics.Metrics
	// db          *ntpdb.Queries
	// dbconn      *sql.DB
	// log         *slog.Logger
	// shutdownFns []func(ctx context.Context) error
}

func NewTwServer(srv *Server) *TwServer {
	return &TwServer{srv: srv}
}

func (s *TwServer) checkTwirpVersionBlocking(ctx context.Context) error {
	monitor, _, _, err := s.srv.getMonitor(ctx, "")
	if err != nil {
		return err
	}

	if version.CheckVersion(monitor.ClientVersion, "v4.0.0") {
		return twirp.PermissionDenied.Error("This monitor has used ntppool-agent >= 4.0.0. Please disable ntppool-monitor and use ntppool-agent")
	}

	return nil
}

func (s *TwServer) GetConfig(ctx context.Context, in *pb.GetConfigParams) (*pb.Config, error) {
	if err := s.checkTwirpVersionBlocking(ctx); err != nil {
		return nil, err
	}

	cfg, err := s.srv.GetConfig(ctx, "")
	if err != nil {
		return nil, err
	}
	return cfg.PbConfig()
}

func (s *TwServer) GetServers(ctx context.Context, in *pb.GetServersParams) (*pb.ServerList, error) {
	if err := s.checkTwirpVersionBlocking(ctx); err != nil {
		return nil, err
	}

	serverList, err := s.srv.GetServers(ctx, "")
	if err != nil {
		return nil, err
	}

	pServers := []*pb.Server{}
	for _, server := range serverList.Servers {
		pServer := &pb.Server{}

		ip, err := netip.ParseAddr(server.Ip)
		if err != nil {
			return nil, err
		}
		pServer.IpBytes, _ = ip.MarshalBinary()

		pServer.Ticket, err = s.srv.SignIPs(serverList.monitor.ID, serverList.BatchID, &ip)
		if err != nil {
			return nil, err
		}

		pServers = append(pServers, pServer)
	}

	pcfg, err := serverList.Config.PbConfig()
	if err != nil {
		return nil, err
	}

	resp := &pb.ServerList{
		Servers: pServers,
		Config:  pcfg,
		BatchId: serverList.BatchID,
	}

	return resp, err
}

func (s *TwServer) SubmitResults(ctx context.Context, in *pb.ServerStatusList) (*pb.ServerStatusResult, error) {
	p := SubmitResultsParam{
		Version: in.Version,
		BatchId: in.BatchId,
	}
	for _, e := range in.List {
		p.List = append(p.List, &apiv2.ServerStatus{
			Ticket:     e.Ticket,
			IpBytes:    e.IpBytes,
			Ts:         e.Ts,
			Offset:     e.Offset,
			Rtt:        e.Rtt,
			Stratum:    e.Stratum,
			Leap:       e.Leap,
			Error:      e.Error,
			NoResponse: e.NoResponse,
		})
	}
	ok, err := s.srv.SubmitResults(ctx, p, "")
	resp := pb.ServerStatusResult{Ok: ok}
	return &resp, err
}
