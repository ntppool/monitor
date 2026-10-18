package server

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"github.com/twitchtv/twirp"

	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"
)

type errorHandler struct {
	apiv2connect.UnimplementedMonitorServiceHandler
	err error
}

func (h *errorHandler) GetConfig(ctx context.Context, req *apiv2.GetConfigRequest) (*apiv2.GetConfigResponse, error) {
	return nil, connectError(ctx, req.MonId, h.err)
}

func TestConnectError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code connect.Code
		msg  string
	}{
		{"twirp not found", twirp.NotFoundError("no such monitor"), connect.CodeNotFound, "no such monitor"},
		{"twirp permission denied", twirp.PermissionDenied.Error("monitor not active"), connect.CodePermissionDenied, "monitor not active"},
		{"twirp invalid argument", twirp.InvalidArgumentError("Version", "Unsupported data version"), connect.CodeInvalidArgument, "Version Unsupported data version"},
		{"twirp internal", twirp.InternalErrorWith(errors.New("db password=secret")), connect.CodeInternal, "internal error"},
		{"twirp malformed", twirp.NewError(twirp.Malformed, "bad input"), connect.CodeInternal, "internal error"},
		{"plain error", errors.New("db password=secret"), connect.CodeInternal, "internal error"},
		{"connect error", connect.NewError(connect.CodeUnavailable, "try later"), connect.CodeUnavailable, "try later"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := connect.NewServer()
			apiv2connect.RegisterMonitorServiceHandler(srv, &errorHandler{err: tt.err})
			client := apiv2connect.NewMonitorServiceClient(connect.NewClient(connectinprocess.New(srv)))

			_, err := client.GetConfig(t.Context(), &apiv2.GetConfigRequest{MonId: "192.0.2.1"})
			cerr, ok := errors.AsType[*connect.Error](err)
			if !ok {
				t.Fatalf("got %v, want a connect error", err)
			}
			if cerr.Code() != tt.code {
				t.Errorf("code = %v, want %v", cerr.Code(), tt.code)
			}
			if cerr.Message() != tt.msg {
				t.Errorf("message = %q, want %q", cerr.Message(), tt.msg)
			}
		})
	}
}
