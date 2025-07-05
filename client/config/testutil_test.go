package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
)

// testEnv provides a test environment with temporary directory and config
type testEnv struct {
	ctx    context.Context
	cfg    AppConfig
	tmpDir string
}

// setupTestConfig creates a test configuration in a temporary directory
func setupTestConfig(t *testing.T) (*testEnv, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "config-test-*")
	require.NoError(t, err)

	ctx := context.Background()
	log := logger.Setup()
	ctx = logger.NewContext(ctx, log)

	cfg, err := NewAppConfig(ctx, depenv.DeployDevel, tmpDir, false)
	require.NoError(t, err)

	return &testEnv{
			ctx:    ctx,
			cfg:    cfg,
			tmpDir: tmpDir,
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
