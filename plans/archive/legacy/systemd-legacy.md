# Systemd Legacy Support Plan

## Overview
Add support for Enterprise Linux 7 and other systems with systemd versions older than 235, which lack support for the `StateDirectory` directive. Additionally, fix the inconsistency between preinstall.sh using `/var/run` and systemd using `/var/lib`.

## Background
- Systemd 235+ supports `StateDirectory` and `StateDirectoryMode` directives
- EL7 (RHEL 7, CentOS 7) ships with systemd 219
- Current preinstall.sh incorrectly uses `/var/run/ntppool-agent` for the home directory
- The systemd unit file correctly uses `/var/lib/ntppool-agent` via StateDirectory
- Monitor state should persist across reboots, so `/var/lib` is the correct location

## Implementation Plan

### 1. Update preinstall.sh

**Changes needed:**
- Change default home directory from `/var/run/ntppool-agent` to `/var/lib/ntppool-agent`
- Add logic to migrate existing users from `/var/run/ntppool-agent` to `/var/lib/ntppool-agent`
- Ensure proper permissions and ownership

**Implementation:**
```bash
#!/bin/sh

NTPMONDIR=/var/lib/ntppool-agent
OLD_NTPMONDIR=/var/run/ntppool-agent

# Create group if it doesn't exist
getent group ntpmon >/dev/null || groupadd -r ntpmon

# Check if user exists and potentially needs migration
if getent passwd ntpmon >/dev/null; then
    # Get current home directory
    CURRENT_HOME=$(getent passwd ntpmon | cut -d: -f6)

    # If user has old home directory, update it
    if [ "$CURRENT_HOME" = "$OLD_NTPMONDIR" ]; then
        usermod -d ${NTPMONDIR} ntpmon

        # Migrate any existing state files
        if [ -d "$OLD_NTPMONDIR" ] && [ ! -d "$NTPMONDIR" ]; then
            mkdir -p ${NTPMONDIR}
            cp -a ${OLD_NTPMONDIR}/* ${NTPMONDIR}/ 2>/dev/null || true
        fi
    fi
else
    # Create new user with correct home directory
    useradd -r -g ntpmon -d ${NTPMONDIR} -s /sbin/nologin \
        -c "NTP Pool Monitoring system" ntpmon
fi

# Ensure state directory exists with correct permissions
mkdir -p ${NTPMONDIR}
chmod 700 ${NTPMONDIR}
chown -R ntpmon:ntpmon ${NTPMONDIR}

exit 0
```

### 2. Update postinstall.sh

**Changes needed:**
- Detect systemd version
- For systemd < 235, create a modified unit file
- Place the modified unit file in `/etc/systemd/system/` to override the packaged one

**Implementation:**
```bash
#!/bin/sh

# Function to get systemd version
get_systemd_version() {
    systemctl --version | head -1 | awk '{print $2}'
}

# Check if we need to create a legacy systemd unit file
SYSTEMD_VERSION=$(get_systemd_version)

if [ "$SYSTEMD_VERSION" -lt 235 ] 2>/dev/null; then
    # Create modified unit file for older systemd
    cat > /etc/systemd/system/ntppool-agent@.service << 'EOF'
[Unit]
Description=NTP Pool Monitor (%i)
After=chronyd.service
Wants=network-online.target
StartLimitInterval=300
StartLimitBurst=5

[Service]
Type=simple
User=ntpmon
WorkingDirectory=/var/lib/ntppool-agent

# Create state directory since StateDirectory is not supported
ExecStartPre=/bin/mkdir -p /var/lib/ntppool-agent
ExecStartPre=/bin/chown ntpmon:ntpmon /var/lib/ntppool-agent
ExecStartPre=/bin/chmod 700 /var/lib/ntppool-agent

EnvironmentFile=-/etc/default/ntppool-agent
ExecStart=/usr/bin/ntppool-agent --env %i monitor
Restart=always
TimeoutStartSec=10
RestartSec=120

[Install]
WantedBy=multi-user.target
EOF
fi

# Reload systemd and restart services
systemctl daemon-reload
systemctl restart 'ntppool-agent@*'
```

### 3. Application Code Considerations

**Current state directory detection (from CLAUDE.md):**
- Priority: `$MONITOR_STATE_DIR` > `$STATE_DIRECTORY` > user config directory
- The application already handles `STATE_DIRECTORY` environment variable
- Automatic migration from RuntimeDirectory to StateDirectory on startup

**No changes needed** - The application already supports the `STATE_DIRECTORY` environment variable that we're setting in the legacy unit file.

### 4. Testing Strategy

**Test on various systems:**
1. **EL7 (systemd 219)**:
   - Verify unit file is modified during postinstall
   - Verify state directory is created with correct permissions
   - Verify STATE_DIRECTORY environment variable is set
   - Test service start/stop/restart

2. **Modern systems (systemd 235+)**:
   - Verify original unit file is used
   - Verify StateDirectory creates `/var/lib/ntppool-agent`
   - Test service functionality unchanged

3. **User migration scenarios**:
   - New installation: User created with `/var/lib/ntppool-agent` home
   - Upgrade with existing user at `/var/run/ntppool-agent`: Home directory updated
   - Upgrade with existing user already at `/var/lib/ntppool-agent`: No changes

### 5. Rollback Plan

If issues arise:
1. Remove the override unit file: `rm /etc/systemd/system/ntppool-agent@.service`
2. Run `systemctl daemon-reload`
3. The original packaged unit file will be used again

### 6. Documentation Updates

Update relevant documentation to note:
- Minimum systemd version for full feature support is 235
- Legacy systems use ExecStartPre to create directories
- State is always stored in `/var/lib/ntppool-agent` regardless of systemd version

## Summary

This plan ensures:
1. Consistent use of `/var/lib/ntppool-agent` for persistent state
2. Support for older systemd versions (< 235) like those in EL7
3. Smooth migration for existing installations
4. No changes required to the application code
5. Modern systems continue to use systemd's StateDirectory feature
