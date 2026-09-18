#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "sma removal must be run as root" >&2
    exit 1
fi

# Debian passes "upgrade" during upgrades; RPM passes 1. Do not stop the old
# service in those cases because postinstall will restart it after replacement.
case "${1:-}" in
    upgrade|1|2)
        exit 0
        ;;
esac

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    systemctl disable --now sma.service >/dev/null 2>&1 || true
elif command -v rc-service >/dev/null 2>&1; then
    rc-service sma stop >/dev/null 2>&1 || true
    rc-update del sma default >/dev/null 2>&1 || true
fi

exit 0
