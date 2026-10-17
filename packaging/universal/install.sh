#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: SMA must be installed as root." >&2
    echo "Run as root (use sudo where available): sh ./install.sh" >&2
    exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install_root=/opt/sma
umask 077

copy_file() {
    copy_temp="$2.new.$$"
    cp "$1" "$copy_temp"
    chmod "$3" "$copy_temp"
    mv -f "$copy_temp" "$2"
}

case "$(uname -s)" in
    Linux) ;;
    *) echo "ERROR: SMA supports Linux only." >&2; exit 1 ;;
esac

if [ -r /etc/rc.common ] && { command -v procd >/dev/null 2>&1 || [ -x /sbin/procd ]; }; then
    # OpenWrt images commonly omit account-management utilities. Install only
    # the missing tools; never edit /etc/passwd or run the agent as root.
    need_group=0
    need_user=0
    if ! grep -q '^sma:' /etc/group 2>/dev/null && ! command -v groupadd >/dev/null 2>&1 && ! command -v addgroup >/dev/null 2>&1; then need_group=1; fi
    if ! id sma >/dev/null 2>&1 && ! command -v useradd >/dev/null 2>&1 && ! command -v adduser >/dev/null 2>&1; then need_user=1; fi
    if [ "$need_group" -eq 1 ] || [ "$need_user" -eq 1 ]; then
        if command -v opkg >/dev/null 2>&1; then
            opkg update
            if [ "$need_group" -eq 1 ]; then opkg install shadow-groupadd; fi
            if [ "$need_user" -eq 1 ]; then opkg install shadow-useradd; fi
        fi
        if [ "$need_group" -eq 1 ] && ! command -v groupadd >/dev/null 2>&1 && ! command -v addgroup >/dev/null 2>&1; then
            echo 'ERROR: Install shadow-groupadd before installing SMA.' >&2
            exit 1
        fi
        if [ "$need_user" -eq 1 ] && ! command -v useradd >/dev/null 2>&1 && ! command -v adduser >/dev/null 2>&1; then
            echo 'ERROR: Install shadow-useradd before installing SMA.' >&2
            exit 1
        fi
    fi
fi

mkdir -p "$install_root/bin" "$install_root/docs/releases" "$install_root/docs/windows" "$install_root/discovery/scripts" "$install_root/config"
chmod 0755 "$install_root" "$install_root/bin" "$install_root/docs" "$install_root/docs/releases" "$install_root/docs/windows" "$install_root/discovery" "$install_root/discovery/scripts"
chmod 0750 "$install_root/config"
copy_file "$script_dir/sma" "$install_root/bin/sma" 0755
copy_file "$script_dir/sma-ctl.sh" "$install_root/bin/sma-ctl" 0755
copy_file "$script_dir/README.md" "$install_root/README.md" 0644
copy_file "$script_dir/CHANGELOG.md" "$install_root/CHANGELOG.md" 0644
for source in "$script_dir/docs/README.md" "$script_dir/docs/maintenance.md" "$script_dir/docs/troubleshooting.md"; do
    copy_file "$source" "$install_root/docs/$(basename "$source")" 0644
done
for source in "$script_dir/docs/releases/"*.md; do
    copy_file "$source" "$install_root/docs/releases/$(basename "$source")" 0644
done
copy_file "$script_dir/docs/windows/README.md" "$install_root/docs/windows/README.md" 0644
for name in zabbix-integration api-integration sma-center-version-metrics server-monitor-agent-development installation automatic-installation lifecycle discovery-api-integration; do
    copy_file "$script_dir/docs/$name.md" "$install_root/docs/$name.md" 0644
done
copy_file "$script_dir/discovery/common-services.sh" "$install_root/discovery/scripts/common-services.sh" 0755

if [ ! -e "$install_root/config/sma.env" ]; then
    copy_file "$script_dir/sma.env" "$install_root/config/sma.env" 0640
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

chown root:root "$install_root" "$install_root/bin" "$install_root/bin/sma"
chown root:sma "$install_root/config" "$install_root/config/sma.env"

"$install_root/bin/sma-ctl" prepare-auth
mkdir -p /usr/local/bin
if [ ! -e /usr/local/bin/sma-ctl ] && [ ! -L /usr/local/bin/sma-ctl ]; then
    ln -s "$install_root/bin/sma-ctl" /usr/local/bin/sma-ctl
fi
echo 'SMA 管理（root）：/opt/sma/bin/sma-ctl help'
echo '查看 Bearer Token（root）：/opt/sma/bin/sma-ctl token show'

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    copy_file "$script_dir/sma.service" /etc/systemd/system/sma.service 0644
    systemctl daemon-reload
    systemctl enable sma.service >/dev/null 2>&1 || true
    systemctl restart sma.service >/dev/null 2>&1 || true
    echo "SMA installed. Check it with: systemctl status sma"
elif command -v rc-update >/dev/null 2>&1; then
    copy_file "$script_dir/sma.openrc" /etc/init.d/sma 0755
    rc-update add sma default >/dev/null 2>&1 || true
    rc-service sma restart >/dev/null 2>&1 || true
    echo "SMA installed. Check it with: rc-service sma status"
elif [ -r /etc/rc.common ] && { command -v procd >/dev/null 2>&1 || [ -x /sbin/procd ]; }; then
    copy_file "$script_dir/sma.procd" /etc/init.d/sma 0755
    /etc/init.d/sma enable
    /etc/init.d/sma restart
    echo "SMA installed. Check it with: /etc/init.d/sma status"
else
    echo "SMA installed in /opt/sma. Start it manually: /opt/sma/bin/sma"
fi
copy_file "$script_dir/installed-version" "$install_root/installed-version" 0644
