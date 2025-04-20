package server

import (
	"context"
	"fmt"
	"net/netip"

	"connectrpc.com/connect"

	"go.ntppool.org/common/logger"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

type conServer struct {
	srv *Server
}

func NewConnectServer(srv *Server) *conServer {
	return &conServer{srv: srv}
}

func (cs *conServer) GetConfig(ctx context.Context, req *connect.Request[apiv2.GetConfigRequest]) (*connect.Response[apiv2.GetConfigResponse], error) {
	cfg, err := cs.srv.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	msg, err := cfg.APIv2()
	if err != nil {
		return nil, err
	}

	cresp := connect.NewResponse(msg)

	return cresp, nil
}

func (cs *conServer) GetServers(ctx context.Context, req *connect.Request[apiv2.GetServersRequest]) (*connect.Response[apiv2.GetServersResponse], error) {
	log := logger.FromContext(ctx)
	serverList, err := cs.srv.GetServers(ctx)
	if err != nil {
		return nil, err
	}

	log.InfoContext(ctx, "got servers in apiv2", "count", len(serverList.Servers), "batchID", serverList.BatchID)

	// if len(serverList.Servers) == 0 {
	// 	log.Warn("no servers returned")
	// }

	cfg, err := serverList.Config.APIv2()
	if err != nil {
		return nil, err
	}

	resp := connect.NewResponse(&apiv2.GetServersResponse{
		BatchId: []byte(serverList.BatchID),
		Config:  cfg,
	})

	pServers := []*apiv2.Server{}
	for _, server := range serverList.Servers {
		pServer := &apiv2.Server{}

		ip, err := netip.ParseAddr(server.Ip)
		if err != nil {
			return nil, err
		}
		pServer.IpBytes, _ = ip.MarshalBinary()

		if serverList.monitor == nil {
			log.ErrorContext(ctx, "serverList.monitor is nil")
		}

		if cs.srv == nil {
			log.ErrorContext(ctx, "cs.srv is nil")
		}

		pServer.Ticket, err = cs.srv.SignIPs(serverList.monitor.ID, serverList.BatchID, &ip)
		if err != nil {
			return nil, err
		}

		pServers = append(pServers, pServer)
	}

	resp.Msg.Servers = pServers

	return resp, nil
}

func (cs *conServer) SubmitResults(ctx context.Context, req *connect.Request[apiv2.SubmitResultsRequest]) (*connect.Response[apiv2.SubmitResultsResponse], error) {
	msg := req.Msg
	if msg == nil {
		return nil, fmt.Errorf("missing message")
	}

	p := SubmitResultsParam{
		Version: msg.Version,
		List:    msg.List,
		BatchId: msg.BatchId,
	}

	ok, err := cs.srv.SubmitResults(ctx, p)

	return connect.NewResponse(&apiv2.SubmitResultsResponse{Ok: ok}), err
}
