package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
)

func TestStateDirPriority(t *testing.T) {
	// Save original environment and restore after tests
	originalMonitorStateDir := os.Getenv("MONITOR_STATE_DIR")
	originalStateDirectory := os.Getenv("STATE_DIRECTORY")
	defer func() {
		os.Setenv("MONITOR_STATE_DIR", originalMonitorStateDir)
		os.Setenv("STATE_DIRECTORY", originalStateDirectory)
	}()

	t.Run("explicit MONITOR_STATE_DIR takes priority", func(t *testing.T) {
		// Clear both environment variables first
		os.Unsetenv("MONITOR_STATE_DIR")
		os.Unsetenv("STATE_DIRECTORY")

		// Set both environment variables
		t.Setenv("MONITOR_STATE_DIR", "/custom/monitor/state")
		t.Setenv("STATE_DIRECTORY", "/systemd/state")

		cmd := &ClientCmd{}
		err := cmd.BeforeApply()
		require.NoError(t, err)

		// Should prefer MONITOR_STATE_DIR over STATE_DIRECTORY
		assert.Equal(t, "/custom/monitor/state", cmd.StateDir)
	})

	t.Run("STATE_DIRECTORY used when MONITOR_STATE_DIR not set", func(t *testing.T) {
		// Clear both environment variables first
		os.Unsetenv("MONITOR_STATE_DIR")
		os.Unsetenv("STATE_DIRECTORY")

		// Set only STATE_DIRECTORY
		t.Setenv("STATE_DIRECTORY", "/systemd/state")

		cmd := &ClientCmd{}
		err := cmd.BeforeApply()
		require.NoError(t, err)

		// Should use STATE_DIRECTORY
		assert.Equal(t, "/systemd/state", cmd.StateDir)
	})

	t.Run("fallback to user config dir when neither env var set", func(t *testing.T) {
		// Clear both environment variables
		os.Unsetenv("MONITOR_STATE_DIR")
		os.Unsetenv("STATE_DIRECTORY")

		cmd := &ClientCmd{}
		err := cmd.BeforeApply()
		require.NoError(t, err)

		// Should fall back to user config directory
		expectedPath, err := os.UserConfigDir()
		require.NoError(t, err)
		expectedPath = filepath.Join(expectedPath, "ntppool-agent")

		assert.Equal(t, expectedPath, cmd.StateDir)
	})

	t.Run("explicit StateDir overrides environment variables", func(t *testing.T) {
		// Set environment variables
		t.Setenv("MONITOR_STATE_DIR", "/custom/monitor/state")
		t.Setenv("STATE_DIRECTORY", "/systemd/state")

		// Set explicit StateDir
		cmd := &ClientCmd{
			StateDir: "/explicit/state/dir",
		}
		err := cmd.BeforeApply()
		require.NoError(t, err)

		// Should keep explicit StateDir
		assert.Equal(t, "/explicit/state/dir", cmd.StateDir)
	})

	t.Run("empty MONITOR_STATE_DIR falls back to STATE_DIRECTORY", func(t *testing.T) {
		// Set empty MONITOR_STATE_DIR and valid STATE_DIRECTORY
		t.Setenv("MONITOR_STATE_DIR", "")
		t.Setenv("STATE_DIRECTORY", "/systemd/state")

		cmd := &ClientCmd{}
		err := cmd.BeforeApply()
		require.NoError(t, err)

		// Should use STATE_DIRECTORY since MONITOR_STATE_DIR is empty
		assert.Equal(t, "/systemd/state", cmd.StateDir)
	})
}

func TestLoadFromDiskWithMigration(t *testing.T) {
	t.Run("loadFromDisk calls migration when state file missing", func(t *testing.T) {
		// This is an integration test that verifies the migration is called
		// during the normal loadFromDisk flow

		tmpDir, err := os.MkdirTemp("", "migration-integration-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		runtimeDir := filepath.Join(tmpDir, "runtime")
		stateDir := filepath.Join(tmpDir, "state")

		require.NoError(t, os.MkdirAll(runtimeDir, 0o700))
		require.NoError(t, os.MkdirAll(stateDir, 0o700))

		// Create old state file in runtime directory
		oldStateDir := filepath.Join(runtimeDir, depenv.DeployTest.String())
		require.NoError(t, os.MkdirAll(oldStateDir, 0o700))

		oldStateFile := filepath.Join(oldStateDir, "state.json")
		oldStateData := `{"API":{"APIKey":"migrated-key"},"Data":{"Name":"migrated.example.com"}}`
		require.NoError(t, os.WriteFile(oldStateFile, []byte(oldStateData), 0o600))

		// Set environment to simulate systemd runtime directory
		t.Setenv("RUNTIME_DIRECTORY", runtimeDir)

		// Create cmd with state directory configuration
		cmd := &ClientCmd{
			StateDir:  stateDir,
			DeployEnv: depenv.DeployTest,
		}

		// Setup context
		ctx := context.Background()
		log := logger.Setup()
		ctx = logger.NewContext(ctx, log)

		// Create a minimal kong context - we just need it to not be nil
		kctx := &kong.Context{}

		// Apply both before and after hooks to trigger config loading
		err = cmd.BeforeApply()
		require.NoError(t, err)

		// This should trigger migration during config loading
		err = cmd.AfterApply(kctx, ctx)
		require.NoError(t, err)

		// Verify migration happened by checking the new state file exists
		newStateFile := filepath.Join(stateDir, depenv.DeployTest.String(), "state.json")
		assert.FileExists(t, newStateFile)

		// Verify content was migrated correctly
		newData, err := os.ReadFile(newStateFile)
		require.NoError(t, err)
		assert.JSONEq(t, oldStateData, string(newData))
	})
}
