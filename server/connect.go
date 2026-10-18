package server

import (
	"context"
	"errors"
	"net/netip"

	"connectrpc.com/connect/v2"
	"github.com/twitchtv/twirp"

	"go.ntppool.org/common/logger"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

type conServer struct {
	srv *Server
}

func NewConnectServer(srv *Server) *conServer {
	return &conServer{srv: srv}
}

// connectError converts an error from the shared Server methods to a
// connect error. connect v2 doesn't send the text of plain errors, so
// Twirp errors keep their code and message. Internal errors and anything
// else get a fixed message, and the error is logged here instead.
func connectError(ctx context.Context, monID string, err error) error {
	if _, ok := errors.AsType[*connect.Error](err); ok {
		return err
	}

	if twerr, ok := errors.AsType[twirp.Error](err); ok && twerr.Code() != twirp.Internal {
		var code connect.Code
		if uerr := code.UnmarshalText([]byte(twerr.Code())); uerr == nil {
			return connect.NewError(code, twerr.Msg()).WithCause(err)
		}
	}

	log := logger.FromContext(ctx)
	log.ErrorContext(ctx, "connect api error", "mon_id", monID, "err", err)

	return connect.NewError(connect.CodeInternal, "internal error").WithCause(err)
}

func (cs *conServer) GetConfig(ctx context.Context, req *apiv2.GetConfigRequest) (*apiv2.GetConfigResponse, error) {
	cfg, err := cs.srv.GetConfig(ctx, req.MonId)
	if err != nil {
		return nil, connectError(ctx, req.MonId, err)
	}

	msg, err := cfg.APIv2()
	if err != nil {
		return nil, connectError(ctx, req.MonId, err)
	}

	return msg, nil
}

func (cs *conServer) GetServers(ctx context.Context, req *apiv2.GetServersRequest) (*apiv2.GetServersResponse, error) {
	log := logger.FromContext(ctx)

	serverList, err := cs.srv.GetServers(ctx, req.MonId)
	if err != nil {
		return nil, connectError(ctx, req.MonId, err)
	}

	log.DebugContext(ctx, "got servers in apiv2", "count", len(serverList.Servers), "batchID", serverList.BatchID)

	// if len(serverList.Servers) == 0 {
	// 	log.Warn("no servers returned")
	// }

	cfg, err := serverList.Config.APIv2()
	if err != nil {
		return nil, connectError(ctx, req.MonId, err)
	}

	resp := &apiv2.GetServersResponse{
		BatchId: []byte(serverList.BatchID),
		Config:  cfg,
	}

	pServers := []*apiv2.Server{}
	for _, server := range serverList.Servers {
		pServer := &apiv2.Server{}

		ip, err := netip.ParseAddr(server.Ip)
		if err != nil {
			return nil, connectError(ctx, req.MonId, err)
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
			return nil, connectError(ctx, req.MonId, err)
		}

		pServers = append(pServers, pServer)
	}

	resp.Servers = pServers

	return resp, nil
}

func (cs *conServer) SubmitResults(ctx context.Context, req *apiv2.SubmitResultsRequest) (*apiv2.SubmitResultsResponse, error) {
	if req == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "missing message")
	}

	p := SubmitResultsParam{
		Version: req.Version,
		List:    req.List,
		BatchId: req.BatchId,
	}

	ok, err := cs.srv.SubmitResults(ctx, p, req.MonId)
	if err != nil {
		return nil, connectError(ctx, req.MonId, err)
	}

	return &apiv2.SubmitResultsResponse{Ok: ok}, nil
}
