#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: SMA must be removed as root." >&2
    echo "Run: sudo ./uninstall.sh" >&2
    exit 1
fi

purge=false
if [ "${1:-}" = "--purge" ]; then
    purge=true
elif [ "$#" -gt 0 ]; then
    echo "Usage: $0 [--purge]" >&2
    exit 2
fi

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    systemctl disable --now sma.service >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/sma.service
    systemctl daemon-reload
elif command -v rc-service >/dev/null 2>&1; then
    rc-service sma stop >/dev/null 2>&1 || true
    rc-update del sma default >/dev/null 2>&1 || true
    rm -f /etc/init.d/sma
fi

rm -f /opt/sma/bin/sma
rm -rf /opt/sma/docs
if [ "$purge" = true ]; then
    rm -rf /opt/sma
    echo "SMA removed, including configuration. The sma system user was retained."
else
    echo "SMA removed. Configuration was preserved in /opt/sma/config."
fi
