#!/bin/sh
systemctl daemon-reload
systemctl restart 'ntpmon@*'
