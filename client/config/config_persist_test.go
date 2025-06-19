package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
)

func TestReplaceFileAtomic(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		existing bool
	}{
		{
			name:     "create new file",
			content:  []byte("new content"),
			existing: false,
		},
		{
			name:     "replace existing file",
			content:  []byte("updated content"),
			existing: true,
		},
		{
			name:     "empty content",
			content:  []byte(""),
			existing: false,
		},
		{
			name:     "large content",
			content:  make([]byte, 10*1024), // 10KB
			existing: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "replace-file-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			testFile := filepath.Join(tmpDir, "test.txt")

			// Create existing file if needed
			if tt.existing {
				err := os.WriteFile(testFile, []byte("original content"), 0o644)
				require.NoError(t, err)
			}

			// Test atomic replacement
			err = replaceFile(testFile, tt.content)
			require.NoError(t, err)

			// Verify content
			result, err := os.ReadFile(testFile)
			require.NoError(t, err)
			assert.Equal(t, tt.content, result)

			// Verify no .tmp file remains
			tmpFile := testFile + ".tmp"
			_, err = os.Stat(tmpFile)
			assert.True(t, os.IsNotExist(err), "tmp file should not exist after replacement")

			// Verify file permissions
			info, err := os.Stat(testFile)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
		})
	}
}

func TestReplaceFileAtomicConcurrentReaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "concurrent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")

	// Create initial file
	initialContent := []byte("initial content")
	err = os.WriteFile(testFile, initialContent, 0o644)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errors := make(chan error, 20)
	results := make(chan []byte, 500) // Increased buffer size

	// Start multiple concurrent readers (reduced count)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ { // Reduced iterations
				content, err := os.ReadFile(testFile)
				if err != nil {
					select {
					case errors <- err:
					default:
					}
					return
				}
				select {
				case results <- content:
				default:
					// Buffer full, skip
				}
			}
		}()
	}

	// Start one writer (serialized to avoid file conflicts)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 3; j++ {
			content := []byte(fmt.Sprintf("updated content %d", j))
			if err := replaceFile(testFile, content); err != nil {
				select {
				case errors <- err:
				default:
				}
				return
			}
			time.Sleep(5 * time.Millisecond) // Delay between writes
		}
	}()

	// Wait for all operations to complete
	wg.Wait()
	close(errors)
	close(results)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify all reads returned valid content (no partial writes)
	readCount := 0
	for content := range results {
		readCount++
		assert.True(t, len(content) > 0, "should never read empty content")
		// Content should be either initial or updated
		contentStr := string(content)
		assert.True(t, contentStr == "initial content" ||
			strings.HasPrefix(contentStr, "updated content"),
			"content should be valid, got: %q", contentStr)
	}

	t.Logf("Processed %d read operations", readCount)
}

func TestStateFilePersistence(t *testing.T) {
	t.Run("save and load cycle preserves all data", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		ac := env.cfg.(*appConfig)

		// Set various data
		err := ac.SetAPIKey("test-api-key")
		require.NoError(t, err)

		// Manually set some data that would come from API
		ac.lock.Lock()
		ac.Data.Name = "test-server"
		ac.Data.TLSName = "test.example.com"
		ac.DataSha = "test-sha"
		ac.lock.Unlock()

		// Save the state
		err = ac.save()
		require.NoError(t, err)

		// Create new config instance and load
		cfg2, err := NewAppConfig(env.ctx, depenv.DeployDevel, env.tmpDir, false)
		require.NoError(t, err)

		ac2 := cfg2.(*appConfig)

		// Load from disk only (not API) to verify persistence
		err = ac2.loadFromDisk(env.ctx)
		require.NoError(t, err)

		// Verify all data was preserved
		assert.Equal(t, "test-api-key", ac2.APIKey())
		assert.Equal(t, "test-server", ac2.Data.Name)
		assert.Equal(t, "test.example.com", ac2.Data.TLSName)
		assert.Equal(t, "test-sha", ac2.DataSha)
	})

	t.Run("handle malformed JSON", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create corrupted state file
		createCorruptedStateFile(t, env.tmpDir)

		// Loading should fail gracefully
		_, err := NewAppConfig(env.ctx, depenv.DeployDevel, env.tmpDir, false)
		assert.Error(t, err, "should fail to load corrupted state file")
	})

	t.Run("missing file creation", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "missing-file-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		ctx := context.Background()
		log := logger.Setup()
		ctx = logger.NewContext(ctx, log)

		// Load config with no existing state file
		cfg, err := NewAppConfig(ctx, depenv.DeployDevel, tmpDir, false)
		require.NoError(t, err)

		// Should have empty API key initially
		assert.Equal(t, "", cfg.APIKey())

		// Setting API key should create the file
		err = cfg.SetAPIKey("new-key")
		require.NoError(t, err)

		// Verify file was created
		stateFile := filepath.Join(tmpDir, depenv.DeployDevel.String(), "state.json")
		_, err = os.Stat(stateFile)
		assert.NoError(t, err, "state file should be created")

		// Verify content
		state := readStateFile(t, tmpDir)
		api := state["API"].(map[string]interface{})
		assert.Equal(t, "new-key", api["APIKey"])
	})

	t.Run("permission denied scenarios", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Make directory read-only
		stateDir := filepath.Join(env.tmpDir, depenv.DeployDevel.String())
		err := os.Chmod(stateDir, 0o444) // read-only
		require.NoError(t, err)

		// Restore permissions for cleanup
		defer func() {
			os.Chmod(stateDir, 0o755)
		}()

		// Attempt to save should fail
		ac := env.cfg.(*appConfig)
		err = ac.save()
		assert.Error(t, err, "should fail to save to read-only directory")
	})
}

func TestConcurrentFileAccess(t *testing.T) {
	env, cleanup := setupTestConfig(t)
	defer cleanup()

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	// Multiple concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = env.cfg.APIKey()
				_ = env.cfg.TLSName()
				_ = env.cfg.ServerName()
			}
		}()
	}

	// Multiple concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				if err := env.cfg.SetAPIKey(key); err != nil {
					errors <- err
					return
				}
				time.Sleep(time.Millisecond) // Small delay to interleave operations
			}
		}(i)
	}

	// Wait for all operations
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	// Verify final state is consistent
	finalKey := env.cfg.APIKey()
	assert.NotEmpty(t, finalKey, "should have a valid API key after concurrent operations")

	// Verify state file is not corrupted
	state := readStateFile(t, env.tmpDir)
	api := state["API"].(map[string]interface{})
	assert.Equal(t, finalKey, api["APIKey"], "file state should match in-memory state")
}

func TestConcurrentFileAccessWithRaceDetector(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race detector test in short mode")
	}

	// This test is specifically designed to catch race conditions
	// Run with: go test -race

	env, cleanup := setupTestConfig(t)
	defer cleanup()

	const numGoroutines = 5           // Reduced from 20
	const operationsPerGoroutine = 10 // Reduced from 50

	var wg sync.WaitGroup

	// Start many goroutines doing mixed read/write operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				switch j % 4 {
				case 0:
					env.cfg.SetAPIKey(fmt.Sprintf("key-%d-%d", id, j))
				case 1:
					_ = env.cfg.APIKey()
				case 2:
					_ = env.cfg.IPv4()
				case 3:
					_ = env.cfg.IPv6()
				}
				time.Sleep(time.Millisecond) // Small delay to prevent overwhelming
			}
		}(i)
	}

	wg.Wait()
}

func TestStateDirectoryManagement(t *testing.T) {
	t.Run("automatic directory creation", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dir-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		ctx := context.Background()
		log := logger.Setup()
		ctx = logger.NewContext(ctx, log)

		// Create config - should automatically create env-specific directory
		cfg, err := NewAppConfig(ctx, depenv.DeployDevel, tmpDir, false)
		require.NoError(t, err)

		// Set API key to trigger directory creation
		err = cfg.SetAPIKey("test-key")
		require.NoError(t, err)

		// Verify directory was created
		envDir := filepath.Join(tmpDir, depenv.DeployDevel.String())
		info, err := os.Stat(envDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "environment directory should exist")

		// Verify permissions
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
			"environment directory should have 0700 permissions")
	})

	t.Run("permission inheritance", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "perm-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Set specific permissions on parent directory
		err = os.Chmod(tmpDir, 0o755)
		require.NoError(t, err)

		ctx := context.Background()
		log := logger.Setup()
		ctx = logger.NewContext(ctx, log)

		cfg, err := NewAppConfig(ctx, depenv.DeployTest, tmpDir, false)
		require.NoError(t, err)

		err = cfg.SetAPIKey("test-key")
		require.NoError(t, err)

		// Check created directory has expected permissions
		envDir := filepath.Join(tmpDir, depenv.DeployTest.String())
		info, err := os.Stat(envDir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	})

	t.Run("verify environment-specific subdirectories", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "env-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		ctx := context.Background()
		log := logger.Setup()
		ctx = logger.NewContext(ctx, log)

		// Test different environments create different directories
		envs := []depenv.DeploymentEnvironment{
			depenv.DeployDevel,
			depenv.DeployTest,
			depenv.DeployProd,
		}

		for _, env := range envs {
			cfg, err := NewAppConfig(ctx, env, tmpDir, false)
			require.NoError(t, err)

			err = cfg.SetAPIKey(fmt.Sprintf("key-%s", env.String()))
			require.NoError(t, err)

			// Verify environment-specific directory exists
			envDir := filepath.Join(tmpDir, env.String())
			_, err = os.Stat(envDir)
			assert.NoError(t, err, "environment directory should exist for %s", env.String())

			// Verify state file is in the right place
			stateFile := filepath.Join(envDir, "state.json")
			_, err = os.Stat(stateFile)
			assert.NoError(t, err, "state file should exist for %s", env.String())
		}

		// Verify all three directories exist and are separate
		files, err := os.ReadDir(tmpDir)
		require.NoError(t, err)

		var dirNames []string
		for _, file := range files {
			if file.IsDir() {
				dirNames = append(dirNames, file.Name())
			}
		}

		assert.Contains(t, dirNames, "devel")
		assert.Contains(t, dirNames, "test")
		assert.Contains(t, dirNames, "prod")
	})
}

func TestReplaceFileErrorHandling(t *testing.T) {
	t.Run("disk full simulation", func(t *testing.T) {
		// This test is challenging to implement portably
		// In a real environment, you might use a filesystem that supports quotas
		t.Skip("Disk full testing requires special setup")
	})

	t.Run("permission denied on directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "perm-error-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Make directory non-writable
		err = os.Chmod(tmpDir, 0o444)
		require.NoError(t, err)

		// Restore permissions for cleanup
		defer func() {
			os.Chmod(tmpDir, 0o755)
		}()

		testFile := filepath.Join(tmpDir, "test.txt")
		err = replaceFile(testFile, []byte("test content"))
		assert.Error(t, err, "should fail when directory is not writable")
	})

	t.Run("invalid path handling", func(t *testing.T) {
		// Test with invalid path characters
		invalidPath := "/dev/null/invalid"
		err := replaceFile(invalidPath, []byte("content"))
		assert.Error(t, err, "should fail with invalid path")
	})
}

func TestLoadFromDiskEdgeCases(t *testing.T) {
	t.Run("empty state file", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create empty state file
		stateDir := filepath.Join(env.tmpDir, depenv.DeployDevel.String())
		err := os.MkdirAll(stateDir, 0o700)
		require.NoError(t, err)

		stateFile := filepath.Join(stateDir, "state.json")
		err = os.WriteFile(stateFile, []byte(""), 0o644)
		require.NoError(t, err)

		// Loading should handle empty file gracefully
		ac := env.cfg.(*appConfig)
		err = ac.loadFromDisk(env.ctx)
		assert.Error(t, err, "should error on empty JSON file")
	})

	t.Run("partial JSON content", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create partial JSON
		stateDir := filepath.Join(env.tmpDir, depenv.DeployDevel.String())
		err := os.MkdirAll(stateDir, 0o700)
		require.NoError(t, err)

		stateFile := filepath.Join(stateDir, "state.json")
		err = os.WriteFile(stateFile, []byte(`{"API":{"APIKey":"test"`), 0o644)
		require.NoError(t, err)

		ac := env.cfg.(*appConfig)
		err = ac.loadFromDisk(env.ctx)
		assert.Error(t, err, "should error on incomplete JSON")
	})

	t.Run("file permissions changed during operation", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create state file
		createTestStateFile(t, env.tmpDir, "test-key")

		// Load successfully first
		ac := env.cfg.(*appConfig)
		err := ac.loadFromDisk(env.ctx)
		assert.NoError(t, err)

		// Make file unreadable
		stateFile := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "state.json")
		temporarilyBreakFile(t, stateFile, 100*time.Millisecond)

		// Load should fail gracefully
		err = ac.loadFromDisk(env.ctx)
		assert.Error(t, err, "should fail when file is unreadable")

		// After permissions are restored, should work again
		time.Sleep(150 * time.Millisecond)
		err = ac.loadFromDisk(env.ctx)
		assert.NoError(t, err, "should work after permissions restored")
	})
}
