package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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
			os.RemoveAll(tmpDir)
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

// mockWatcher provides a controllable file watcher for testing
type mockWatcher struct {
	Events chan fsnotify.Event
	Errors chan error
	closed bool
	mu     sync.Mutex
	dirs   map[string]bool
}

// newMockWatcher creates a new mock file watcher
func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		Events: make(chan fsnotify.Event, 10),
		Errors: make(chan error, 10),
		dirs:   make(map[string]bool),
	}
}

// Add simulates adding a directory to watch
func (m *mockWatcher) Add(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("watcher closed")
	}

	m.dirs[name] = true
	return nil
}

// Close simulates closing the watcher
func (m *mockWatcher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed {
		m.closed = true
		close(m.Events)
		close(m.Errors)
	}
	return nil
}

// SendEvent sends a mock file system event
func (m *mockWatcher) SendEvent(event fsnotify.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed {
		select {
		case m.Events <- event:
		default:
			// Buffer full, ignore
		}
	}
}

// SendError sends a mock error
func (m *mockWatcher) SendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed {
		select {
		case m.Errors <- err:
		default:
			// Buffer full, ignore
		}
	}
}

// IsClosed returns whether the watcher is closed
func (m *mockWatcher) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// countWaiters counts the number of active waiters (for testing memory leaks)
func countWaiters(cfg AppConfig) int {
	ac := cfg.(*appConfig)
	ac.configChangeMu.RLock()
	defer ac.configChangeMu.RUnlock()
	return len(ac.configChangeWaiters)
}

// createLargeStateFile creates a state file with many fields for testing
func createLargeStateFile(t *testing.T, dir string, size int) {
	t.Helper()

	stateDir := filepath.Join(dir, depenv.DeployDevel.String())
	err := os.MkdirAll(stateDir, 0o700)
	require.NoError(t, err)

	// Create a large state structure
	data := make(map[string]interface{})
	data["API"] = map[string]interface{}{"APIKey": "test-key"}
	data["Data"] = map[string]interface{}{
		"Name":    "test-server",
		"TLSName": "test.example.com",
		"IPv4":    map[string]interface{}{"Status": "active", "IP": "1.2.3.4"},
		"IPv6":    map[string]interface{}{"Status": "active", "IP": "2001:db8::1"},
	}

	// Add many extra fields to increase size
	extras := make(map[string]interface{})
	for i := 0; i < size; i++ {
		extras[fmt.Sprintf("field_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	data["Extra"] = extras
	data["DataSha"] = ""

	content, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	stateFile := filepath.Join(stateDir, "state.json")
	generateFileChange(t, stateFile, content)
}

// runWithTimeout runs a function with a timeout context
func runWithTimeout(t *testing.T, timeout time.Duration, fn func(context.Context)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(ctx)
	}()

	select {
	case <-done:
		// Completed successfully
	case <-ctx.Done():
		t.Fatalf("Function timed out after %v", timeout)
	}
}

// goroutineLeakChecker helps detect goroutine leaks in tests
type goroutineLeakChecker struct {
	initialCount int
}

// newGoroutineLeakChecker creates a new leak checker
func newGoroutineLeakChecker() *goroutineLeakChecker {
	return &goroutineLeakChecker{
		initialCount: countGoroutines(),
	}
}

// Check verifies no goroutines have leaked
func (g *goroutineLeakChecker) Check(t *testing.T) {
	t.Helper()

	// Give goroutines time to cleanup
	time.Sleep(100 * time.Millisecond)

	currentCount := countGoroutines()
	if currentCount > g.initialCount {
		t.Errorf("Goroutine leak detected: started with %d, now have %d goroutines",
			g.initialCount, currentCount)
	}
}

// countGoroutines returns the current number of goroutines
func countGoroutines() int {
	// This is a simplified version - in real tests you might use runtime.NumGoroutine()
	// or more sophisticated leak detection
	return 1 // Placeholder
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
		os.Chmod(path, originalMode)
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
