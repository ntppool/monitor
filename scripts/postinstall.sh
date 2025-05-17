#!/bin/sh
systemctl daemon-reload
systemctl restart 'ntppool-agent@*'
