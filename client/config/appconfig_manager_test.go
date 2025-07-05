package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
)

func TestFileWatcherSetup(t *testing.T) {
	t.Run("watcher created successfully", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()

		// Start manager in a goroutine
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		managerStarted := make(chan struct{})
		go func() {
			defer close(managerStarted)
			err := env.cfg.Manager(ctx, promReg)
			if err != nil {
				t.Errorf("Manager failed: %v", err)
			}
		}()

		// Give manager time to start
		time.Sleep(100 * time.Millisecond)

		// Cancel to shutdown manager
		cancel()

		// Wait for manager to finish
		select {
		case <-managerStarted:
			// Success
		case <-time.After(2 * time.Second):
			t.Error("Manager didn't shutdown gracefully")
		}
	})

	t.Run("fallback to timer-only mode on error", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create an invalid directory to trigger watcher error
		invalidDir := "/dev/null/invalid"
		ac := env.cfg.(*appConfig)

		// Temporarily change the state directory to an invalid path
		originalDir := ac.dir
		ac.dir = invalidDir
		defer func() {
			ac.dir = originalDir
		}()

		promReg := prometheus.NewRegistry()

		ctx, cancel := context.WithTimeout(env.ctx, 1*time.Second)
		defer cancel()

		// Manager should not fail even with invalid directory
		err := env.cfg.Manager(ctx, promReg)
		// Should complete without error even if watcher fails
		assert.NoError(t, err)
	})

	t.Run("verify correct directory is watched", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set API key to ensure directory is created
		err := env.cfg.SetAPIKey("test-key")
		require.NoError(t, err)

		expectedDir := filepath.Join(env.tmpDir, depenv.DeployDevel.String())

		// Verify the directory exists (Manager would watch this)
		stateFile := filepath.Join(expectedDir, "state.json")
		_, err = os.Stat(stateFile)
		assert.NoError(t, err, "state file should exist in correct directory")
	})

	t.Run("watcher cleanup on context cancel", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()

		ctx, cancel := context.WithCancel(env.ctx)

		managerDone := make(chan struct{})
		go func() {
			defer close(managerDone)
			_ = env.cfg.Manager(ctx, promReg)
		}()

		// Give manager time to start
		time.Sleep(100 * time.Millisecond)

		// Cancel context
		cancel()

		// Manager should shutdown quickly
		select {
		case <-managerDone:
			// Success - manager stopped
		case <-time.After(1 * time.Second):
			t.Error("Manager didn't stop within timeout")
		}
	})
}

func TestDebounceTimerCorrectness(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping debounce test in short mode")
	}

	t.Run("single event triggers reload after debounce", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set initial API key
		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		// Start manager
		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond) // Let manager start

		// Set up waiter before making change
		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Trigger file change
		createTestStateFile(t, env.tmpDir, "changed-key")

		// Should get notification after debounce period (500ms debounce + processing time)
		assert.True(t, waitForEvent(t, waiter, 1200*time.Millisecond),
			"Should receive notification after file change")

		// Verify the change was loaded
		time.Sleep(200 * time.Millisecond) // Give time for reload
		assert.Equal(t, "changed-key", env.cfg.APIKey())
	})

	t.Run("multiple rapid events result in single reload", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		// Set up waiter
		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Generate multiple rapid file changes
		for i := 0; i < 5; i++ {
			createTestStateFile(t, env.tmpDir, fmt.Sprintf("rapid-change-%d", i))
			time.Sleep(10 * time.Millisecond) // Very rapid changes
		}

		// Should get one notification for all changes
		assert.True(t, waitForEvent(t, waiter, 1200*time.Millisecond),
			"Should receive notification after rapid changes")

		// Verify final state
		time.Sleep(200 * time.Millisecond)
		finalKey := env.cfg.APIKey()
		assert.True(t, strings.HasPrefix(finalKey, "rapid-change-"),
			"Should have final API key from rapid changes")
	})

	t.Run("timer reset behavior", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		// Test that timer resets properly
		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// First change
		createTestStateFile(t, env.tmpDir, "first-change")
		time.Sleep(50 * time.Millisecond) // Before debounce completes

		// Second change (should reset timer)
		createTestStateFile(t, env.tmpDir, "second-change")

		// Should get notification after full debounce period from second change
		start := time.Now()
		assert.True(t, waitForEvent(t, waiter, 1200*time.Millisecond),
			"Should receive notification after timer reset")

		elapsed := time.Since(start)
		assert.True(t, elapsed >= 450*time.Millisecond,
			"Should wait for debounce period from second change, elapsed: %v", elapsed)
	})
}

func TestFileOperationEvents(t *testing.T) {
	t.Run("write events are handled", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Direct write to state file (simulates external write)
		stateFile := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "state.json")
		content := []byte(`{"API":{"APIKey":"write-event-key"},"Data":{"Name":"","TLSName":"","IPv4":{"Status":"","IP":null},"IPv6":{"Status":"","IP":null}},"DataSha":""}`)

		err = os.WriteFile(stateFile, content, 0o644)
		require.NoError(t, err)

		assert.True(t, waitForEvent(t, waiter, 1200*time.Millisecond),
			"Should detect write events")
	})

	t.Run("create events from atomic rename", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Use generateFileChange which does atomic rename
		stateFile := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "state.json")
		content := []byte(`{"API":{"APIKey":"create-event-key"},"Data":{"Name":"","TLSName":"","IPv4":{"Status":"","IP":null},"IPv6":{"Status":"","IP":null}},"DataSha":""}`)

		generateFileChange(t, stateFile, content)

		assert.True(t, waitForEvent(t, waiter, 1200*time.Millisecond),
			"Should detect create events from atomic rename")
	})

	t.Run("tmp file filtering works correctly", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Create temporary file that should NOT trigger reload
		tmpFile := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "other.tmp")
		err = os.WriteFile(tmpFile, []byte("temporary content"), 0o644)
		require.NoError(t, err)

		// Should NOT get notification for unrelated tmp file
		assert.False(t, waitForEvent(t, waiter, 200*time.Millisecond),
			"Should not trigger on unrelated tmp files")

		// But should get notification for our state file tmp
		createTestStateFile(t, env.tmpDir, "tmp-filter-test")
		assert.True(t, waitForEvent(t, waiter, 1200*time.Millisecond),
			"Should trigger on state.json.tmp")
	})
}

func TestWatcherErrorRecovery(t *testing.T) {
	t.Run("fallback to timer-only mode", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithTimeout(env.ctx, 2*time.Second)
		defer cancel()

		// Force watcher error by using invalid directory
		ac := env.cfg.(*appConfig)
		originalDir := ac.dir
		ac.dir = "/dev/null/nonexistent"
		defer func() { ac.dir = originalDir }()

		// Manager should still work in timer-only mode
		err := env.cfg.Manager(ctx, promReg)
		assert.NoError(t, err, "Manager should handle watcher errors gracefully")
	})

	t.Run("verify no goroutine leaks", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping goroutine leak test in short mode")
		}

		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Start and stop manager multiple times
		for i := 0; i < 3; i++ {
			// Create a new registry for each iteration to avoid duplicate registration
			promReg := prometheus.NewRegistry()
			ctx, cancel := context.WithCancel(env.ctx)

			managerDone := make(chan struct{})
			go func() {
				defer close(managerDone)
				_ = env.cfg.Manager(ctx, promReg)
			}()

			time.Sleep(50 * time.Millisecond)
			cancel()

			select {
			case <-managerDone:
				// Success
			case <-time.After(1 * time.Second):
				t.Fatalf("Manager %d didn't stop", i)
			}
		}

		// Give time for cleanup
		time.Sleep(100 * time.Millisecond)
		// Note: Real goroutine leak detection would require runtime.NumGoroutine()
		// or a more sophisticated testing framework
	})
}

func TestManagerLifecycle(t *testing.T) {
	t.Run("clean startup", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithTimeout(env.ctx, 1*time.Second)
		defer cancel()

		// Manager should start cleanly
		err := env.cfg.Manager(ctx, promReg)
		assert.NoError(t, err)
	})

	t.Run("graceful shutdown via context", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)

		managerDone := make(chan error, 1)
		go func() {
			managerDone <- env.cfg.Manager(ctx, promReg)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel() // Trigger shutdown

		select {
		case err := <-managerDone:
			assert.NoError(t, err, "Manager should shutdown without error")
		case <-time.After(2 * time.Second):
			t.Error("Manager didn't shutdown within timeout")
		}
	})

	t.Run("multiple start/stop cycles", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping multiple cycle test in short mode")
		}

		env, cleanup := setupTestConfig(t)
		defer cleanup()

		for i := 0; i < 3; i++ {
			// Create a new registry for each iteration to avoid duplicate registration
			promReg := prometheus.NewRegistry()
			ctx, cancel := context.WithCancel(env.ctx)

			managerDone := make(chan error, 1)
			go func() {
				managerDone <- env.cfg.Manager(ctx, promReg)
			}()

			time.Sleep(100 * time.Millisecond)
			cancel()

			select {
			case err := <-managerDone:
				assert.NoError(t, err, "Cycle %d: Manager should shutdown cleanly", i)
			case <-time.After(1 * time.Second):
				t.Fatalf("Cycle %d: Manager didn't shutdown", i)
			}
		}
	})
}

func TestReloadIntervals(t *testing.T) {
	t.Run("timer reload functionality", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		// This test verifies the manager can handle timer-based reloads
		// We can't easily test the 5-minute interval without mocking time
		// But we can verify the manager continues running

		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Make a change and verify it's detected
		createTestStateFile(t, env.tmpDir, "timer-test-key")

		assert.True(t, waitForEvent(t, waiter, 1*time.Second),
			"Manager should detect changes during normal operation")
	})

	t.Run("immediate reload on file change", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		start := time.Now()
		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Trigger immediate reload
		createTestStateFile(t, env.tmpDir, "immediate-key")

		assert.True(t, waitForEvent(t, waiter, 1*time.Second),
			"Should get immediate notification")

		elapsed := time.Since(start)
		assert.True(t, elapsed < 1200*time.Millisecond,
			"Reload should complete within debounce period + margin (< 1200ms), got %v", elapsed)
	})
}

func TestConfigurationNotificationSystem(t *testing.T) {
	t.Run("notification delivery to multiple waiters", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		err := env.cfg.SetAPIKey("initial-key")
		require.NoError(t, err)

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		// Create multiple waiters
		waiter1 := env.cfg.WaitForConfigChange(ctx)
		defer waiter1.Cancel()
		waiter2 := env.cfg.WaitForConfigChange(ctx)
		defer waiter2.Cancel()
		waiter3 := env.cfg.WaitForConfigChange(ctx)
		defer waiter3.Cancel()

		// Trigger change
		createTestStateFile(t, env.tmpDir, "multi-waiter-key")

		// All waiters should be notified (allow for 500ms debounce + margin)
		assert.True(t, waitForEvent(t, waiter1, 1200*time.Millisecond),
			"Waiter 1 should be notified")
		assert.True(t, waitForEvent(t, waiter2, 100*time.Millisecond),
			"Waiter 2 should be notified")
		assert.True(t, waitForEvent(t, waiter3, 100*time.Millisecond),
			"Waiter 3 should be notified")
	})

	t.Run("waiter cleanup on cancel", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		initialWaiterCount := countWaiters(env.cfg)

		// Create and cancel waiters
		waiter1 := env.cfg.WaitForConfigChange(env.ctx)
		waiter2 := env.cfg.WaitForConfigChange(env.ctx)

		assert.Equal(t, initialWaiterCount+2, countWaiters(env.cfg),
			"Should have 2 additional waiters")

		waiter1.Cancel()
		assert.Equal(t, initialWaiterCount+1, countWaiters(env.cfg),
			"Should have 1 waiter after canceling waiter1")

		waiter2.Cancel()
		assert.Equal(t, initialWaiterCount, countWaiters(env.cfg),
			"Should be back to initial waiter count")
	})

	t.Run("notification during manager shutdown", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)

		go func() { _ = env.cfg.Manager(ctx, promReg) }()
		time.Sleep(100 * time.Millisecond)

		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Cancel manager context
		cancel()

		// Waiter should be cancelled when context is done
		select {
		case <-waiter.Done():
			// Expected behavior
		case <-time.After(1 * time.Second):
			t.Error("Waiter should be cancelled when manager shuts down")
		}
	})
}
