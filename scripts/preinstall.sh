#!/bin/sh

# Use /var/lib for persistent state storage (survives reboots)
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
