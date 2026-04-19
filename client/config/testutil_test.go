package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
)

// testEnv provides a test environment with temporary directory and config.
// The fakeAPI serves the HTTP endpoints client/config would otherwise reach
// on the live devel deployment.
type testEnv struct {
	ctx    context.Context
	cfg    AppConfig
	tmpDir string
	api    *fakeAPI
}

// fakeAPI is an in-process httptest.Server that serves the endpoints
// client/config calls: /monitor/api/config and /api/oidc/token.
//
// setupTestConfig points API_HOST at this server (process-global env var),
// so every code path that calls depenv.DeploymentEnvironment.APIHost() —
// LoadAPIAppConfig, JWT token refresh, Manager reloads — hits this handler
// instead of the network. That means tests in this package MUST NOT call
// t.Parallel(): parallel subtests would race over API_HOST.
//
// Default handlers satisfy the common case: an authorized /monitor/api/config
// request returns an empty MonitorStatusConfig (no cert, no IPs), and the
// OIDC endpoint returns 401. Tests that need different behavior replace
// configFunc or tokenFunc before triggering the code path.
type fakeAPI struct {
	srv *httptest.Server

	mu         sync.Mutex
	configFunc http.HandlerFunc
	tokenFunc  http.HandlerFunc

	configCalls atomic.Int64
	tokenCalls  atomic.Int64
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()

	api := &fakeAPI{}
	api.configFunc = defaultConfigHandler
	api.tokenFunc = defaultTokenHandler

	mux := http.NewServeMux()
	mux.HandleFunc("/monitor/api/config", func(w http.ResponseWriter, r *http.Request) {
		api.configCalls.Add(1)
		api.mu.Lock()
		h := api.configFunc
		api.mu.Unlock()
		h(w, r)
	})
	mux.HandleFunc("/api/oidc/token", func(w http.ResponseWriter, r *http.Request) {
		api.tokenCalls.Add(1)
		api.mu.Lock()
		h := api.tokenFunc
		api.mu.Unlock()
		h(w, r)
	})

	api.srv = httptest.NewServer(mux)
	t.Cleanup(api.srv.Close)
	return api
}

// URL returns the base URL of the fake API server.
func (a *fakeAPI) URL() string {
	return a.srv.URL
}

// SetConfigHandler replaces the handler for /monitor/api/config.
func (a *fakeAPI) SetConfigHandler(h http.HandlerFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configFunc = h
}

// SetTokenHandler replaces the handler for /api/oidc/token.
func (a *fakeAPI) SetTokenHandler(h http.HandlerFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokenFunc = h
}

func defaultConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" || r.Header.Get("Authorization") == "Bearer " {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// MonitorStatusConfig with zero-value fields — SaveCertificates is a no-op
	// when TLS.Cert is empty, and LoadAPIAppConfig skips IPs that have an
	// empty IP string.
	_ = json.NewEncoder(w).Encode(MonitorStatusConfig{})
}

func defaultTokenHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// setupTestConfig creates a test configuration backed by an in-process
// fakeAPI. API_HOST is set before NewAppConfig so every HTTP call the
// package makes under test is redirected to the fake server.
func setupTestConfig(t *testing.T) (*testEnv, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "config-test-*")
	require.NoError(t, err)

	api := newFakeAPI(t)
	t.Setenv("API_HOST", api.URL())

	ctx := context.Background()
	log := logger.Setup()
	ctx = logger.NewContext(ctx, log)

	cfg, err := NewAppConfig(ctx, depenv.DeployDevel, tmpDir, false)
	require.NoError(t, err)

	return &testEnv{
			ctx:    ctx,
			cfg:    cfg,
			tmpDir: tmpDir,
			api:    api,
		}, func() {
			_ = os.RemoveAll(tmpDir)
		}
}

// waitForEvent waits for a configuration change event with timeout
func waitForEvent(t *testing.T, waiter *ConfigChangeWaiter, timeout time.Duration) bool {
	t.Helper()

	select {
	case <-waiter.Done():
		return true
	case <-time.After(timeout):
		t.Logf("Timeout waiting for config change event")
		return false
	}
}

// generateFileChange creates a controlled file change using atomic operations
func generateFileChange(t *testing.T, path string, content []byte) {
	t.Helper()

	err := os.WriteFile(path+".tmp", content, 0o600)
	require.NoError(t, err)

	err = os.Rename(path+".tmp", path)
	require.NoError(t, err)
}

// createTestStateFile creates a state.json file with the given API key
func createTestStateFile(t *testing.T, dir, apiKey string) {
	t.Helper()

	stateDir := filepath.Join(dir, depenv.DeployDevel.String())
	err := os.MkdirAll(stateDir, 0o700)
	require.NoError(t, err)

	state := map[string]interface{}{
		"API": map[string]interface{}{
			"APIKey": apiKey,
		},
		"Data": map[string]interface{}{
			"Name":    "",
			"TLSName": "",
			"IPv4":    map[string]interface{}{"Status": "", "IP": nil},
			"IPv6":    map[string]interface{}{"Status": "", "IP": nil},
		},
		"DataSha": "",
	}

	content, err := json.MarshalIndent(state, "", "  ")
	require.NoError(t, err)

	stateFile := filepath.Join(stateDir, "state.json")
	generateFileChange(t, stateFile, content)
}

// readStateFile reads and parses the state.json file
func readStateFile(t *testing.T, dir string) map[string]interface{} {
	t.Helper()

	stateFile := filepath.Join(dir, depenv.DeployDevel.String(), "state.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var state map[string]interface{}
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	return state
}

// countWaiters counts the number of active waiters (for testing memory leaks)
func countWaiters(cfg AppConfig) int {
	ac := cfg.(*appConfig)
	ac.configChangeMu.RLock()
	defer ac.configChangeMu.RUnlock()
	return len(ac.configChangeWaiters)
}

// temporarilyBreakFile makes a file temporarily unreadable for testing error conditions
func temporarilyBreakFile(t *testing.T, path string, duration time.Duration) {
	t.Helper()

	// Get original permissions
	info, err := os.Stat(path)
	require.NoError(t, err)
	originalMode := info.Mode()

	// Make file unreadable
	err = os.Chmod(path, 0o000)
	require.NoError(t, err)

	// Restore permissions after duration
	go func() {
		time.Sleep(duration)
		_ = os.Chmod(path, originalMode)
	}()
}

// createCorruptedStateFile creates a state file with invalid JSON
func createCorruptedStateFile(t *testing.T, dir string) {
	t.Helper()

	stateDir := filepath.Join(dir, depenv.DeployDevel.String())
	err := os.MkdirAll(stateDir, 0o700)
	require.NoError(t, err)

	// Create invalid JSON
	content := []byte(`{"API":{"APIKey":"test-key"` + "\x00" + `}`) // Invalid JSON with null byte

	stateFile := filepath.Join(stateDir, "state.json")
	generateFileChange(t, stateFile, content)
}

// isRunningAsRoot checks if the current process is running as root
func isRunningAsRoot() bool {
	return os.Geteuid() == 0
}
