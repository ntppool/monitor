package server

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"connectrpc.com/connect/v2"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

// minAgentVersion is the oldest agent version monitor-api serves. Agents
// before v4.0.5 republish their MQTT status only once an hour.
const minAgentVersion = "v4.0.5"

// agentVersionInterceptor refuses calls from agents older than
// minAgentVersion, by the version in the User-Agent header. The agent logs
// the error message, so it tells the operator to upgrade.
func agentVersionInterceptor(next connect.ServerFunc) connect.ServerFunc {
	return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		info, ok := connect.CallInfoForServerContext(ctx)
		if !ok {
			return connect.NewError(connect.CodeInternal, "no call info")
		}
		userAgent := info.RequestHeader().Get("User-Agent")
		v := parseClientVersion(userAgent)
		if !version.CheckVersion(v, minAgentVersion) {
			// otelconnect counts the refusals. An old agent retries
			// every few minutes, so log at debug level.
			logger.FromContext(ctx).DebugContext(ctx, "refusing old agent",
				"user_agent", userAgent, "procedure", spec.Procedure, "peer", info.PeerAddr)
			return connect.NewError(connect.CodeFailedPrecondition,
				fmt.Sprintf("ntppool-agent %q is too old; upgrade to %s or newer", v, minAgentVersion))
		}
		return next(ctx, spec, stream)
	}
}

type conServer struct {
	srv *Server
}

func NewConnectServer(srv *Server) *conServer {
	return &conServer{srv: srv}
}

// connectError converts an error from the shared Server methods to a
// connect error. Connect errors keep their code and message, except
// CodeInternal. Those and any other errors are logged here and the client
// gets a fixed "internal error".
func connectError(ctx context.Context, monID string, err error) error {
	if cerr, ok := errors.AsType[*connect.Error](err); ok && cerr.Code() != connect.CodeInternal {
		return cerr
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
