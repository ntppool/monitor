package config

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNoExternalNetwork is a tripwire: it installs a RoundTripper on the
// package-global apiHTTPClient that refuses any request to a host that is
// not local, then drives the code through its main network paths
// (Manager, LoadAPIAppConfig, OIDC token fetch). If a future change adds
// an HTTP call that bypasses API_HOST redirection, this test will fail
// instead of silently hitting a live deployment.
func TestNoExternalNetwork(t *testing.T) {
	original := apiHTTPClient.Transport
	tripwire := &localOnlyRoundTripper{next: original}
	apiHTTPClient.Transport = tripwire
	t.Cleanup(func() { apiHTTPClient.Transport = original })

	env, cleanup := setupTestConfig(t)
	defer cleanup()

	require.NoError(t, env.cfg.SetAPIKey("test-key"))

	ctx, cancel := context.WithCancel(env.ctx)
	managerDone := make(chan struct{})
	go func() {
		defer close(managerDone)
		_ = env.cfg.Manager(ctx)
	}()

	// Let Manager start, then touch state.json to drive a reload through
	// the full load() path so LoadAPIAppConfig runs under the tripwire.
	time.Sleep(100 * time.Millisecond)
	createTestStateFile(t, env.tmpDir, "test-key-updated")
	time.Sleep(1 * time.Second) // 500ms debounce + processing

	cancel()
	select {
	case <-managerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Manager did not shut down in time")
	}

	for _, v := range tripwire.Violations() {
		t.Errorf("non-local HTTP request: %s", v)
	}
}

type localOnlyRoundTripper struct {
	next http.RoundTripper

	mu         sync.Mutex
	violations []string
}

func (r *localOnlyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if host == "127.0.0.1" || host == "localhost" || host == "::1" {
		return r.next.RoundTrip(req)
	}
	r.mu.Lock()
	r.violations = append(r.violations, fmt.Sprintf("%s %s", req.Method, req.URL))
	r.mu.Unlock()
	return nil, fmt.Errorf("blocked non-local request to %s", host)
}

func (r *localOnlyRoundTripper) Violations() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.violations))
	copy(out, r.violations)
	return out
}
