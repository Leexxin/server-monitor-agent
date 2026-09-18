#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "sma removal must be run as root" >&2
    exit 1
fi

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    systemctl daemon-reload
fi

# Package managers preserve /opt/sma/config/sma.env as a configuration file.
# The dedicated sma system account is intentionally retained for safe upgrades.
exit 0
