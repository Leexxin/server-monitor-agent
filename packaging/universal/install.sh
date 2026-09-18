#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: SMA must be installed as root." >&2
    echo "Run: sudo ./install.sh" >&2
    exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install_root=/opt/sma

case "$(uname -s)" in
    Linux) ;;
    *) echo "ERROR: SMA supports Linux only." >&2; exit 1 ;;
esac

install -d -m 0755 "$install_root/bin" "$install_root/docs"
install -d -m 0750 "$install_root/config"
install -m 0755 "$script_dir/sma" "$install_root/bin/sma"
install -m 0644 "$script_dir/README.md" "$install_root/README.md"
install -m 0644 "$script_dir/docs/api-integration.md" "$install_root/docs/api-integration.md"
install -m 0644 "$script_dir/docs/server-monitor-agent-development.md" "$install_root/docs/server-monitor-agent-development.md"
install -m 0644 "$script_dir/docs/installation.md" "$install_root/docs/installation.md"

if [ ! -e "$install_root/config/sma.env" ]; then
    install -m 0640 "$script_dir/sma.env" "$install_root/config/sma.env"
fi

if ! grep -q '^sma:' /etc/group 2>/dev/null; then
    if command -v groupadd >/dev/null 2>&1; then
        groupadd --system sma
    elif command -v addgroup >/dev/null 2>&1; then
        addgroup -S sma
    else
        echo "ERROR: groupadd/addgroup is required." >&2
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
        echo "ERROR: useradd/adduser is required." >&2
        exit 1
    fi
fi

chown -R root:root "$install_root"
chown root:sma "$install_root/config" "$install_root/config/sma.env"

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    install -m 0644 "$script_dir/sma.service" /etc/systemd/system/sma.service
    systemctl daemon-reload
    systemctl enable sma.service >/dev/null 2>&1 || true
    systemctl restart sma.service >/dev/null 2>&1 || true
    echo "SMA installed. Check it with: systemctl status sma"
elif command -v rc-update >/dev/null 2>&1; then
    install -m 0755 "$script_dir/sma.openrc" /etc/init.d/sma
    rc-update add sma default >/dev/null 2>&1 || true
    rc-service sma restart >/dev/null 2>&1 || true
    echo "SMA installed. Check it with: rc-service sma status"
else
    echo "SMA installed in /opt/sma. Start it manually: /opt/sma/bin/sma"
fi
