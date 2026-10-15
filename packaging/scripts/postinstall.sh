#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "sma installation must be run as root" >&2
    exit 1
fi

if ! grep -q '^sma:' /etc/group 2>/dev/null; then
    if command -v groupadd >/dev/null 2>&1; then
        groupadd --system sma
    elif command -v addgroup >/dev/null 2>&1; then
        addgroup -S sma
    else
        echo "unable to create sma system group: groupadd/addgroup not found" >&2
        exit 1
    fi
fi

if ! id sma >/dev/null 2>&1; then
    if command -v useradd >/dev/null 2>&1; then
        nologin_shell=/usr/sbin/nologin
        [ -x "$nologin_shell" ] || nologin_shell=/sbin/nologin
        useradd --system --gid sma --home-dir /nonexistent --shell "$nologin_shell" sma
    elif command -v adduser >/dev/null 2>&1; then
        adduser -S -D -H -s /sbin/nologin -G sma sma
    else
        echo "unable to create sma system user: useradd/adduser not found" >&2
        exit 1
    fi
fi

# Do not recursively change ownership: existing 0600 tokens must remain readable
# by the sma service account across upgrades.
chown root:root /opt/sma /opt/sma/bin /opt/sma/bin/sma
chmod 0755 /opt/sma /opt/sma/bin /opt/sma/bin/sma
chown root:sma /opt/sma/config /opt/sma/config/sma.env
chmod 0750 /opt/sma/config
chmod 0640 /opt/sma/config/sma.env

/opt/sma/bin/sma-ctl prepare-auth
mkdir -p /usr/local/bin
if [ ! -e /usr/local/bin/sma-ctl ] && [ ! -L /usr/local/bin/sma-ctl ]; then
    ln -s /opt/sma/bin/sma-ctl /usr/local/bin/sma-ctl
fi
echo 'SMA 管理：sudo /opt/sma/bin/sma-ctl help'
echo '查看 Bearer Token：sudo /opt/sma/bin/sma-ctl token show'

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    # sma 0.1.0-1 incorrectly shipped an OpenRC script in DEB packages. Remove
    # that obsolete package-owned path during Debian/Ubuntu upgrades.
    if [ -n "${DPKG_MAINTSCRIPT_PACKAGE:-}" ]; then
        rm -f /etc/init.d/sma
    fi
    systemctl daemon-reload
    systemctl enable sma.service >/dev/null 2>&1 || true
    systemctl restart sma.service >/dev/null 2>&1 || echo "sma installed but could not be started; inspect: systemctl status sma" >&2
elif command -v rc-update >/dev/null 2>&1; then
    rc-update add sma default >/dev/null 2>&1 || true
    rc-service sma restart >/dev/null 2>&1 || echo "sma installed but could not be started; inspect: rc-service sma status" >&2
else
    echo "sma installed in /opt/sma; no supported service manager detected" >&2
fi

exit 0
