package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"

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
		{"not found", connect.NewError(connect.CodeNotFound, "no such monitor"), connect.CodeNotFound, "no such monitor"},
		{"permission denied", connect.NewError(connect.CodePermissionDenied, "monitor not active"), connect.CodePermissionDenied, "monitor not active"},
		{"wrapped invalid argument", fmt.Errorf("submit: %w", connect.NewError(connect.CodeInvalidArgument, "invalid batch ID").WithCause(errors.New("bad ulid"))), connect.CodeInvalidArgument, "invalid batch ID"},
		{"internal", connect.NewError(connect.CodeInternal, "could not get config").WithCause(errors.New("db password=secret")), connect.CodeInternal, "internal error"},
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

// Calls from agents older than minAgentVersion are refused; others reach
// the handler (Unimplemented here).
func TestAgentVersionInterceptor(t *testing.T) {
	tests := []struct {
		userAgent string
		want      connect.Code
	}{
		// User-Agent formats from version.Version()
		{"ntppool-agent/v4.0.5/0dd4039 (2026-11-20T10:00:00Z, go1.25.4)", connect.CodeUnimplemented},
		{"ntppool-agent/v4.3.0 (go1.25.4)", connect.CodeUnimplemented},
		{"ntppool-agent/dev-snapshot/0dd4039-M (go1.25.4)", connect.CodeUnimplemented},
		{"ntppool-agent/dev-snapshot (go1.25.4)", connect.CodeUnimplemented},
		{"ntppool-agent/v4.0.4/0dd4039 (2026-01-10T10:00:00Z, go1.24.2)", connect.CodeFailedPrecondition},
		{"ntppool-monitor/v3.8.6/0dd4039 (go1.22.1)", connect.CodeFailedPrecondition},
		{"", connect.CodeFailedPrecondition},
	}

	srv := connect.NewServer(agentVersionInterceptor)
	apiv2connect.RegisterMonitorServiceHandler(srv, apiv2connect.UnimplementedMonitorServiceHandler{})
	client := apiv2connect.NewMonitorServiceClient(connect.NewClient(connectinprocess.New(srv)))

	for _, tt := range tests {
		t.Run(tt.userAgent, func(t *testing.T) {
			ctx, info := connect.NewClientContext(t.Context())
			info.RequestHeader().Set("User-Agent", tt.userAgent)

			_, err := client.GetConfig(ctx, &apiv2.GetConfigRequest{MonId: "192.0.2.1"})
			if code := connect.CodeOf(err); code != tt.want {
				t.Errorf("code = %v (%v), want %v", code, err, tt.want)
			}
		})
	}
}
