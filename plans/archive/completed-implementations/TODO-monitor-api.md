# Monitor Config Editor Sub-command (Revised)

## Overview
Create a kubectl-style sub-command system for editing monitor configurations in the `monitor-api` command. The system treats "defaults" as a special monitor name, with IP version as an optional parameter.

## Implementation Plan

### 1. Command Structure
Add a new `config` sub-command to the existing `ApiCmd` with this structure:
```
monitor-api config get <monitor-name> [v4|v6]     # Show current config
monitor-api config edit <monitor-name> [v4|v6]    # Edit config in $EDITOR
monitor-api config set <monitor-name> [v4|v6] <key> <value>  # Set specific field

# Special case for system defaults:
monitor-api config get defaults [v4|v6]          # Show system defaults
monitor-api config edit defaults [v4|v6]         # Edit system defaults
monitor-api config set defaults [v4|v6] <key> <value>  # Set default field
```

### 2. Behavior Logic
- **Monitor names**: Regular monitors can have v4, v6, or both configurations
- **"defaults" monitor**: Special case that maps to `settings-v4.system` and `settings-v6.system`
- **IP version parameter**:
  - If specified (v4/v6): Edit only that IP version
  - If omitted: Edit both v4 and v6 (where applicable)
  - For regular monitors: Look up existing IP versions in database

### 3. Key Features
- **Interactive editing**: Use `$EDITOR` environment variable (fallback to `vi`) for editing JSON config
- **Multi-version editing**: When no IP version specified, show both v4/v6 configs in single editor session
- **JSON validation**: Validate JSON syntax before saving
- **Config merging**: For regular monitors, show merged config (defaults + specific) in `get` operations
- **Backup/rollback**: Create backup before editing with ability to rollback
- **Diff display**: Show what changed after editing

### 4. File Structure
Create new files:
- `server/cmd/config.go` - Main config command implementation
- `server/cmd/config_edit.go` - Editor functionality and JSON handling
- `server/cmd/config_monitor.go` - Monitor-specific config operations

### 5. Implementation Details
- Extend existing `ApiCmd` struct with new `Config configCmd` field
- Map "defaults" to system monitor names (`settings-v4.system`, `settings-v6.system`)
- For regular monitors, query database to find existing IP versions
- When editing both versions, present as structured JSON with `v4` and `v6` keys
- Reuse existing database connection patterns from `db.go`
- Use existing `Monitor.GetConfigWithDefaults()` for merged views

### 6. Example Usage
```bash
# Edit defaults for both IP versions
monitor-api config edit defaults

# Edit defaults for v4 only
monitor-api config edit defaults v4

# Edit specific monitor (both versions if it has both)
monitor-api config edit my-monitor

# Edit specific monitor v6 only
monitor-api config edit my-monitor v6

# View merged config for a monitor
monitor-api config get my-monitor v4
```

### 7. Error Handling
- Handle monitor not found gracefully
- Handle case where requested IP version doesn't exist for monitor
- Validate JSON before database update
- Provide clear error messages for malformed configs
- Handle editor failures (e.g., user cancels edit)

This approach provides a clean, intuitive interface where "defaults" is treated as a special monitor name, and IP versions are optional parameters that determine scope.

## Implementation Status

### Completed ✅
1. **Extended ApiCmd struct** - Added new `Config configCmd` field to `server/cmd/cmd.go`
2. **Created main config command** - Implemented `server/cmd/config.go` with complete functionality
3. **Config get functionality** - Shows merged configs for regular monitors, raw configs for defaults
4. **Config edit functionality** - Interactive editing with `$EDITOR`, supports both single and multi-version editing
5. **Config set functionality** - Dot notation support for setting specific JSON keys
6. **JSON validation** - Validates JSON before saving, with option to continue with invalid JSON
7. **Added SQL query** - Added `UpdateMonitorConfig` query to `query.sql`

### Next Steps 🔄
1. **Generate SQL code** - Run `make sqlc` to generate the new database query function
2. **Update config.go** - Replace manual SQL execution with generated `UpdateMonitorConfig` function
3. **Test implementation** - Verify all commands work correctly
4. **Format code** - Run `gofumpt -w` on modified Go files
5. **Documentation** - Update README.md with new config management commands

### Features Implemented
- ✅ kubectl-style command interface
- ✅ "defaults" as special monitor name mapping to `settings-v[46].system`
- ✅ Optional IP version parameter (edits both if omitted)
- ✅ JSON validation with confirmation prompt for invalid JSON
- ✅ Config merging for `get` operations (shows merged view for regular monitors)
- ✅ Dot notation support in `set` command
- ✅ Pretty-printed JSON output
- ✅ Temporary file handling for editor sessions
- ✅ Error handling for missing monitors/versions

### Usage Examples
```bash
# View system defaults for IPv4
monitor-api config get defaults v4

# Edit all configurations for a monitor
monitor-api config edit my-monitor

# Set a specific value
monitor-api config set defaults v4 samples 10

# Edit just IPv6 config for a monitor
monitor-api config edit my-monitor v6
```
