package config

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/ntpdb"
)

func TestAPIKeyManagement(t *testing.T) {
	t.Run("SetAPIKey triggers notifications", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set up waiter before making change
		waiter := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter.Cancel()

		// Change API key
		err := env.cfg.SetAPIKey("test-api-key")
		require.NoError(t, err)

		// Should get notification
		assert.True(t, waitForEvent(t, waiter, 1*time.Second),
			"Should receive notification after API key change")

		// Verify API key was set
		assert.Equal(t, "test-api-key", env.cfg.APIKey())
	})

	t.Run("empty API key handling", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set empty API key
		err := env.cfg.SetAPIKey("")
		require.NoError(t, err)

		assert.Equal(t, "", env.cfg.APIKey())

		// Set non-empty API key
		err = env.cfg.SetAPIKey("non-empty-key")
		require.NoError(t, err)

		assert.Equal(t, "non-empty-key", env.cfg.APIKey())
	})

	t.Run("persistence across reloads", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set API key
		err := env.cfg.SetAPIKey("persistent-key")
		require.NoError(t, err)

		// Create new config instance
		cfg2, err := NewAppConfig(env.ctx, depenv.DeployDevel, env.tmpDir, false)
		require.NoError(t, err)

		// Load from disk only
		ac2 := cfg2.(*appConfig)
		err = ac2.loadFromDisk(env.ctx)
		require.NoError(t, err)

		// Verify persistence
		assert.Equal(t, "persistent-key", cfg2.APIKey())
	})

	t.Run("API key change notifications to multiple waiters", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create multiple waiters
		waiter1 := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter1.Cancel()
		waiter2 := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter2.Cancel()
		waiter3 := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter3.Cancel()

		// Change API key
		err := env.cfg.SetAPIKey("multi-waiter-key")
		require.NoError(t, err)

		// All waiters should be notified
		assert.True(t, waitForEvent(t, waiter1, 500*time.Millisecond),
			"Waiter 1 should be notified")
		assert.True(t, waitForEvent(t, waiter2, 100*time.Millisecond),
			"Waiter 2 should be notified")
		assert.True(t, waitForEvent(t, waiter3, 100*time.Millisecond),
			"Waiter 3 should be notified")
	})

	t.Run("same API key doesn't trigger notification", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set initial API key
		err := env.cfg.SetAPIKey("same-key")
		require.NoError(t, err)

		// Set up waiter after initial set
		waiter := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter.Cancel()

		// Set same API key again
		err = env.cfg.SetAPIKey("same-key")
		require.NoError(t, err)

		// Should NOT get notification for same value
		assert.False(t, waitForEvent(t, waiter, 200*time.Millisecond),
			"Should not notify when API key doesn't change")
	})
}

func TestIPConfiguration(t *testing.T) {
	t.Run("IPv4 and IPv6 configuration loading", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		ac := env.cfg.(*appConfig)

		// Test IPv4 configuration
		ipv4Addr, err := netip.ParseAddr("192.0.2.1")
		require.NoError(t, err)

		ac.lock.Lock()
		ac.Data.IPv4 = IPConfig{
			Status: ntpdb.MonitorsStatusActive,
			IP:     &ipv4Addr,
		}
		ac.lock.Unlock()

		ipv4Config := env.cfg.IPv4()
		assert.Equal(t, ntpdb.MonitorsStatusActive, ipv4Config.Status)
		assert.Equal(t, "192.0.2.1", ipv4Config.IP.String())

		// Test IPv6 configuration
		ipv6Addr, err := netip.ParseAddr("2001:db8::1")
		require.NoError(t, err)

		ac.lock.Lock()
		ac.Data.IPv6 = IPConfig{
			Status: ntpdb.MonitorsStatusTesting,
			IP:     &ipv6Addr,
		}
		ac.lock.Unlock()

		ipv6Config := env.cfg.IPv6()
		assert.Equal(t, ntpdb.MonitorsStatusTesting, ipv6Config.Status)
		assert.Equal(t, "2001:db8::1", ipv6Config.IP.String())
	})

	t.Run("IP address validation", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		ac := env.cfg.(*appConfig)

		// Test invalid IP handling
		ac.lock.Lock()
		ac.Data.IPv4 = IPConfig{
			Status: ntpdb.MonitorsStatusActive,
			IP:     nil, // No IP
		}
		ac.lock.Unlock()

		ipv4Config := env.cfg.IPv4()
		assert.Equal(t, ntpdb.MonitorsStatusActive, ipv4Config.Status)
		assert.Nil(t, ipv4Config.IP)
	})

	t.Run("status transitions", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		ac := env.cfg.(*appConfig)
		ipAddr, err := netip.ParseAddr("203.0.113.1")
		require.NoError(t, err)

		// Test different status transitions
		statuses := []ntpdb.MonitorsStatus{
			ntpdb.MonitorsStatusPaused,
			ntpdb.MonitorsStatusTesting,
			ntpdb.MonitorsStatusActive,
		}

		for _, status := range statuses {
			ac.lock.Lock()
			ac.Data.IPv4 = IPConfig{
				Status: status,
				IP:     &ipAddr,
			}
			ac.lock.Unlock()

			config := env.cfg.IPv4()
			assert.Equal(t, status, config.Status)
			assert.Equal(t, "203.0.113.1", config.IP.String())
		}
	})

	t.Run("IsLive logic for each status", func(t *testing.T) {
		ipAddr, err := netip.ParseAddr("198.51.100.1")
		require.NoError(t, err)

		tests := []struct {
			status   ntpdb.MonitorsStatus
			ip       *netip.Addr
			expected bool
		}{
			{ntpdb.MonitorsStatusActive, &ipAddr, true},
			{ntpdb.MonitorsStatusTesting, &ipAddr, true},
			{ntpdb.MonitorsStatusPaused, &ipAddr, false},
			{ntpdb.MonitorsStatusActive, nil, false}, // No IP
		}

		for _, tt := range tests {
			t.Run(fmt.Sprintf("status_%s_ip_%v", tt.status, tt.ip != nil), func(t *testing.T) {
				config := IPConfig{
					Status: tt.status,
					IP:     tt.ip,
				}

				assert.Equal(t, tt.expected, config.IsLive(),
					"IsLive() should return %v for status %s with IP %v",
					tt.expected, tt.status, tt.ip)
			})
		}
	})

	t.Run("empty IP configuration", func(t *testing.T) {
		emptyConfig := IPConfig{}
		assert.False(t, emptyConfig.IsLive(), "Empty config should not be live")
		assert.Nil(t, emptyConfig.IP)
		assert.Equal(t, ntpdb.MonitorsStatus(""), emptyConfig.Status)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("multiple readers during write", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		var wg sync.WaitGroup
		readErrors := make(chan error, 10)

		// Start multiple readers
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					// Read operations should never fail or panic
					_ = env.cfg.APIKey()
					_ = env.cfg.TLSName()
					_ = env.cfg.ServerName()
					_ = env.cfg.IPv4()
					_ = env.cfg.IPv6()
					time.Sleep(time.Microsecond) // Small delay
				}
			}(i)
		}

		// Start a writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				err := env.cfg.SetAPIKey(fmt.Sprintf("concurrent-key-%d", j))
				if err != nil {
					readErrors <- err
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()

		wg.Wait()
		close(readErrors)

		// Check for errors
		for err := range readErrors {
			t.Errorf("Concurrent access error: %v", err)
		}
	})

	t.Run("notification delivery during updates", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		var wg sync.WaitGroup
		notifications := make(chan string, 20)

		// Create multiple waiters that collect notifications
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					waiter := env.cfg.WaitForConfigChange(env.ctx)
					select {
					case <-waiter.Done():
						notifications <- fmt.Sprintf("waiter-%d-notification-%d", id, j)
					case <-time.After(2 * time.Second):
						// Timeout
					}
					waiter.Cancel()
				}
			}(i)
		}

		// Start updater
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				err := env.cfg.SetAPIKey(fmt.Sprintf("notification-key-%d", j))
				if err != nil {
					t.Errorf("SetAPIKey error: %v", err)
					return
				}
				time.Sleep(100 * time.Millisecond) // Give time for notifications
			}
		}()

		wg.Wait()
		close(notifications)

		// Count notifications
		notificationCount := 0
		for range notifications {
			notificationCount++
		}

		// Should have received multiple notifications
		assert.Greater(t, notificationCount, 0, "Should receive notifications during concurrent updates")
	})

	t.Run("verify no deadlocks with lock ordering", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping deadlock test in short mode")
		}

		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// This test ensures proper lock ordering prevents deadlocks
		done := make(chan struct{})
		go func() {
			defer close(done)

			var wg sync.WaitGroup

			// Mixed operations that acquire different locks
			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					switch id % 4 {
					case 0:
						env.cfg.SetAPIKey(fmt.Sprintf("deadlock-test-%d", id))
					case 1:
						_ = env.cfg.APIKey()
					case 2:
						waiter := env.cfg.WaitForConfigChange(env.ctx)
						waiter.Cancel()
					case 3:
						_ = env.cfg.IPv4()
					}
				}(i)
			}

			wg.Wait()
		}()

		// Test should complete without deadlock
		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(10 * time.Second):
			t.Fatal("Test appears to be deadlocked")
		}
	})
}

func TestConfigChangeNotifications(t *testing.T) {
	t.Run("waiter registration", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		initialCount := countWaiters(env.cfg)

		waiter := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter.Cancel()

		assert.Equal(t, initialCount+1, countWaiters(env.cfg),
			"Should have one additional waiter")
	})

	t.Run("notification delivery to multiple waiters", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create multiple waiters
		waiters := make([]*ConfigChangeWaiter, 5)
		for i := range waiters {
			waiters[i] = env.cfg.WaitForConfigChange(env.ctx)
			defer waiters[i].Cancel()
		}

		// Trigger notification
		err := env.cfg.SetAPIKey("multi-notification-test")
		require.NoError(t, err)

		// All waiters should be notified
		for i, waiter := range waiters {
			assert.True(t, waitForEvent(t, waiter, 500*time.Millisecond),
				"Waiter %d should be notified", i)
		}
	})

	t.Run("waiter cleanup on cancel", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		initialCount := countWaiters(env.cfg)

		// Create multiple waiters
		waiters := make([]*ConfigChangeWaiter, 3)
		for i := range waiters {
			waiters[i] = env.cfg.WaitForConfigChange(env.ctx)
		}

		assert.Equal(t, initialCount+3, countWaiters(env.cfg),
			"Should have 3 additional waiters")

		// Cancel waiters one by one
		for i, waiter := range waiters {
			waiter.Cancel()
			expectedCount := initialCount + (3 - i - 1)
			assert.Equal(t, expectedCount, countWaiters(env.cfg),
				"Should have %d waiters after canceling waiter %d", expectedCount, i)
		}
	})

	t.Run("notification during manager shutdown", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		promReg := prometheus.NewRegistry()
		ctx, cancel := context.WithCancel(env.ctx)

		// Start manager
		go env.cfg.Manager(ctx, promReg)
		time.Sleep(100 * time.Millisecond)

		// Create waiter
		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Shutdown manager
		cancel()

		// Waiter context should be cancelled
		select {
		case <-waiter.Done():
			// Expected - waiter cancelled when context is done
		case <-time.After(1 * time.Second):
			t.Error("Waiter should be cancelled when manager context is cancelled")
		}
	})

	t.Run("waiter context inheritance", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create context that will be cancelled
		ctx, cancel := context.WithCancel(env.ctx)

		waiter := env.cfg.WaitForConfigChange(ctx)
		defer waiter.Cancel()

		// Cancel parent context
		cancel()

		// Waiter should be cancelled
		select {
		case <-waiter.Done():
			// Expected
		case <-time.After(500 * time.Millisecond):
			t.Error("Waiter should be cancelled when parent context is cancelled")
		}
	})
}

func TestWaiterMemoryManagement(t *testing.T) {
	t.Run("create many waiters and verify cleanup", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping memory management test in short mode")
		}

		env, cleanup := setupTestConfig(t)
		defer cleanup()

		initialCount := countWaiters(env.cfg)

		// Create many waiters
		const numWaiters = 100
		waiters := make([]*ConfigChangeWaiter, numWaiters)
		for i := 0; i < numWaiters; i++ {
			waiters[i] = env.cfg.WaitForConfigChange(env.ctx)
		}

		assert.Equal(t, initialCount+numWaiters, countWaiters(env.cfg),
			"Should have %d additional waiters", numWaiters)

		// Cancel all waiters
		for _, waiter := range waiters {
			waiter.Cancel()
		}

		assert.Equal(t, initialCount, countWaiters(env.cfg),
			"Should be back to initial waiter count")
	})

	t.Run("abandoned waiter detection", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		initialCount := countWaiters(env.cfg)

		// Create waiter without explicit cleanup (simulates abandoned waiter)
		func() {
			env.cfg.WaitForConfigChange(env.ctx)
			// Waiter goes out of scope without Cancel()
		}()

		// Waiter should still be registered (no automatic cleanup)
		assert.Equal(t, initialCount+1, countWaiters(env.cfg),
			"Abandoned waiter should still be registered")

		// Note: In real implementation, you might want garbage collection
		// or timeout-based cleanup of abandoned waiters
	})

	t.Run("waiter ID overflow handling", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		ac := env.cfg.(*appConfig)

		// Set ID to near overflow
		ac.configChangeMu.Lock()
		ac.configChangeNextID = ^uint64(0) - 2 // Close to overflow
		ac.configChangeMu.Unlock()

		// Create waiters that would cause overflow
		waiter1 := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter1.Cancel()
		waiter2 := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter2.Cancel()
		waiter3 := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter3.Cancel()

		// Should handle overflow gracefully (IDs will wrap around)
		assert.Equal(t, 3, len(ac.configChangeWaiters))
	})
}

func TestConfigurationInterfaceMethods(t *testing.T) {
	t.Run("basic getters", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		ac := env.cfg.(*appConfig)

		// Set some test values
		ac.lock.Lock()
		ac.Data.Name = "test-server"
		ac.Data.TLSName = "test.example.com"
		ac.lock.Unlock()

		assert.Equal(t, "test-server", env.cfg.ServerName())
		assert.Equal(t, "test.example.com", env.cfg.TLSName())
		assert.Equal(t, depenv.DeployDevel, env.cfg.Env())
	})

	t.Run("certificate methods exist", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// These methods should exist and not panic
		assert.False(t, env.cfg.HaveCertificate())

		_, _, _, err := env.cfg.CertificateDates()
		assert.Error(t, err) // Should error when no certificate

		valid, _, err := env.cfg.CheckCertificateValidity(env.ctx)
		assert.Error(t, err) // Should error when no certificate
		assert.False(t, valid)
	})

	t.Run("wait methods", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Set API key so WaitUntilAPIKey completes
		err := env.cfg.SetAPIKey("test-key")
		require.NoError(t, err)

		// WaitUntilAPIKey should complete quickly since we have an API key
		ctx, cancel := context.WithTimeout(env.ctx, 5*time.Second)
		defer cancel()

		err = env.cfg.WaitUntilAPIKey(ctx)
		assert.NoError(t, err)

		// WaitUntilLive will timeout since we don't have live IPs or certificates
		ctx2, cancel2 := context.WithTimeout(env.ctx, 100*time.Millisecond)
		defer cancel2()

		err = env.cfg.WaitUntilLive(ctx2)
		// Should timeout with context deadline exceeded
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestEnvironmentSpecificBehavior(t *testing.T) {
	environments := []depenv.DeploymentEnvironment{
		depenv.DeployDevel,
		depenv.DeployTest,
		depenv.DeployProd,
	}

	for _, env := range environments {
		t.Run(fmt.Sprintf("environment_%s", env.String()), func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", fmt.Sprintf("env-test-%s-*", env.String()))
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			ctx := context.Background()
			log := logger.Setup()
			ctx = logger.NewContext(ctx, log)

			cfg, err := NewAppConfig(ctx, env, tmpDir, false)
			require.NoError(t, err)

			// Each environment should work independently
			assert.Equal(t, env, cfg.Env())

			// Should be able to set API key
			err = cfg.SetAPIKey(fmt.Sprintf("key-for-%s", env.String()))
			require.NoError(t, err)

			assert.Equal(t, fmt.Sprintf("key-for-%s", env.String()), cfg.APIKey())
		})
	}
}
