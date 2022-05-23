#!/bin/sh

# this is created by systemd in the systemd unit file now
# but keeping it here for now since the user is still
# setup this way.
NTPMONDIR=/var/run/ntpmon

mkdir -p /etc/ntpmon

getent group ntpmon >/dev/null || groupadd -r ntpmon
getent passwd ntpmon >/dev/null || \
    useradd -r -g ntpmon -d ${NTPMONDIR} -s /sbin/nologin \
      -c "NTP Pool Monitoring system" ntpmon

mkdir -p ${NTPMONDIR}
chmod 700 ${NTPMONDIR}
chown -R ntpmon:ntpmon ${NTPMONDIR}

exit 0
