#!/bin/sh

# Function to get systemd version
get_systemd_version() {
    systemctl --version | head -1 | awk '{print $2}'
}

# Check if we need to create a legacy systemd unit file
SYSTEMD_VERSION=$(get_systemd_version)

if [ "$SYSTEMD_VERSION" -lt 235 ] 2>/dev/null; then
    # Create modified unit file for older systemd (< 235)
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

# Set STATE_DIRECTORY environment variable
Environment=STATE_DIRECTORY=/var/lib/ntppool-agent

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
