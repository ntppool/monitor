package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	"go.ntppool.org/common/version"
	apitls "go.ntppool.org/monitor/api/tls"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"
)

func TestGetServerName(t *testing.T) {
	s, err := getServerName("jphnd1-21wase0.devel.mon.ntppool.dev")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	if s != "https://api.devel.mon.ntppool.dev" {
		t.Logf("got unexpected development server URL: %s", s)
		t.Fail()

	}
}

type testAuthProvider struct {
	apitls.AuthProvider
	jwt    string
	apiKey string
}

func (ap testAuthProvider) GetJWTToken(context.Context) (string, error) { return ap.jwt, nil }
func (ap testAuthProvider) GetAPIKey() string                           { return ap.apiKey }

func TestHeaderInterceptor(t *testing.T) {
	tests := []struct {
		name string
		ap   testAuthProvider
		auth string
	}{
		{"jwt", testAuthProvider{jwt: "jwt-token", apiKey: "api-key"}, "Bearer jwt-token"},
		{"api key fallback", testAuthProvider{apiKey: "api-key"}, "Bearer api-key"},
		{"no credentials", testAuthProvider{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got http.Header
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.WriteHeader(http.StatusNotFound)
			}))
			defer ts.Close()

			client := apiv2connect.NewMonitorServiceClient(connect.NewClient(
				connecthttp.NewTransport(ts.Client(), ts.URL),
				NewHeaderInterceptor(tt.ap),
			))
			_, _ = client.GetConfig(t.Context(), &apiv2.GetConfigRequest{})

			if got == nil {
				t.Fatal("request didn't reach the server")
			}
			if ua := got.Get("User-Agent"); ua != "ntppool-agent/"+version.Version() {
				t.Errorf("User-Agent = %q", ua)
			}
			if auth := got.Get("Authorization"); auth != tt.auth {
				t.Errorf("Authorization = %q, want %q", auth, tt.auth)
			}
		})
	}
}
