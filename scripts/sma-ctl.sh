#!/bin/sh
# POSIX shell lifecycle command. Secret values are printed only by token show.
set -eu

SMA_ROOT=/opt/sma
SMA_SHARE_URL=${SMA_SHARE_URL:-http://192.168.33.123:8080/web/client/pubshares/MFAWcGkHLBH2ZLquo63Mcm}

fail() { printf '错误：%s\n' "$*" >&2; return 1; }
help() {
    cat <<'EOF'
SMA 管理工具（修改操作请以 root 身份执行；系统提供 sudo 时也可使用）

  status                      查看版本、运行状态、开机自启和访问地址
  doctor                      检查配置、Token 权限和服务状态
  start | stop | restart      启动、停止、重启
  enable | disable            开启、关闭开机自启（不改变当前运行状态）
  logs [行数]                 查看服务日志，默认 100 行
  config show                 查看有效监听地址和 Token 文件位置
  config listen IP:端口       修改监听地址并重启，失败时恢复配置
  token ensure                补齐鉴权配置；保留已有 Token，并重启
  token show                  显示当前 Bearer Token（请妥善保存）
  token reset [--yes]         确认后更换 Token 并重启，旧凭据立即失效
  upgrade [--version X.Y.Z]   从共享目录升级（默认最新版本）
  uninstall [--purge] [--yes] 卸载；默认备份并保留配置，--purge 清除配置
  help                        显示帮助

默认路径：/opt/sma；安装日志：/var/log/sma-install.log
Token 不写入安装日志或上传至分享目录。直接运行 sma-ctl 可查看本帮助。
EOF
}

# Read a single-line EnvironmentFile assignment without executing shell content.
read_setting() {
    awk -v key="$1" '
        $0 ~ "^[[:space:]]*" key "=" {
            value=$0; sub("^[[:space:]]*" key "=", "", value)
            sub(/\r$/, "", value)
            q=substr(value,1,1)
            if (q=="\042" || q=="\047") {
                if (substr(value,length(value),1)!=q) exit 2
                value=substr(value,2,length(value)-2)
            }
            found=value
        }
        END { print found }
    ' "$SMA_ROOT/config/sma.env"
}
read_options() {
    options=$(read_setting SMA_OPTS) || { fail '配置必须是单行 SMA_OPTS="..." 格式，请检查 sma.env。'; return 1; }
    # All supported service managers must interpret this assignment identically.
    case "$options" in
        *\"*|*\'*|*\`*|*\$*|*\\*) fail 'SMA_OPTS 包含复杂引用，请手动维护该配置；管理工具未修改它。'; return 1 ;;
    esac
}
option_value() {
    printf '%s\n' "$options" | awk -v key="$1" '{
        for(i=1;i<=NF;i++) {
            if ($i==key) {i++; v=$i}
            else if(index($i,key "=")==1) v=substr($i,length(key)+2)
        }
    } END {print v}'
}
load_config() {
    [ -r "$SMA_ROOT/config/sma.env" ] || { fail '未找到 SMA 配置，请先安装。'; return 1; }
    read_options || return 1
    listen=$(option_value --listen-address)
    [ -n "$listen" ] || listen=$(read_setting SMA_LISTEN_ADDRESS)
    listen=${listen:-127.0.0.1:9108}
    token_file=$(option_value --web.auth-token-file)
    [ -n "$token_file" ] || token_file=$(read_setting SMA_AUTH_TOKEN_FILE)
}
set_option() {
    options=$(printf '%s\n' "$options" | awk -v key="$1" -v value="$2" '{
        out=""
        for(i=1;i<=NF;i++) {
            if($i==key) {i++; continue}
            if(index($i,key "=")==1) continue
            out=out (out==""?"":" ") $i
        }
        print out (out==""?"":" ") key "=" value
    }')
    config_tmp=$(mktemp "$SMA_ROOT/config/.sma.env.XXXXXX") || return 1
    if ! awk '!/^[[:space:]]*SMA_OPTS=/' "$SMA_ROOT/config/sma.env" > "$config_tmp"; then
        rm -f "$config_tmp"; return 1
    fi
    printf 'SMA_OPTS="%s"\n' "$options" >> "$config_tmp"
    chmod 0640 "$config_tmp" || return 1
    chown root:sma "$config_tmp" || return 1
    mv "$config_tmp" "$SMA_ROOT/config/sma.env"
}
safe_token_path() {
    case "$token_file" in
        /*) ;;
        *) fail 'Token 路径必须是绝对路径。'; return 1 ;;
    esac
    case "$token_file" in
        *[!a-zA-Z0-9_./-]*) fail 'Token 路径含空格或特殊字符，请手动配置。'; return 1 ;;
    esac
    [ ! -L "$token_file" ] || { fail 'Token 文件不能是符号链接。'; return 1; }
    [ ! -e "$token_file" ] || [ -f "$token_file" ] || { fail 'Token 路径不是普通文件。'; return 1; }
}
new_token() {
    token_tmp=$(mktemp "$(dirname "$token_file")/.sma-token.XXXXXX") || return 1
    # 32 random bytes / 256 bits. sha256sum is already required by the installer;
    # od is optional and absent on some OpenWrt/BusyBox images.
    if ! dd if=/dev/urandom of="$token_tmp" bs=32 count=1 2>/dev/null; then
        rm -f "$token_tmp"; return 1
    fi
    if [ "$(wc -c < "$token_tmp" | tr -d '[:space:]')" -ne 32 ]; then
        rm -f "$token_tmp"; fail '随机数读取不完整。'; return 1
    fi
    value=$(sha256sum "$token_tmp" | awk '{print $1}') || { rm -f "$token_tmp"; return 1; }
    case "$value" in
        *[!0-9a-f]*) rm -f "$token_tmp"; fail '生成随机 Token 失败。'; return 1 ;;
    esac
    [ "${#value}" -eq 64 ] || { rm -f "$token_tmp"; fail '生成随机 Token 失败。'; return 1; }
    printf '%s\n' "$value" > "$token_tmp"
    unset value
    chmod 0600 "$token_tmp" || return 1
    chown sma:sma "$token_tmp" || return 1
    mv "$token_tmp" "$token_file"
}
ensure_token() {
    load_config || return 1
    token_file=${token_file:-$SMA_ROOT/config/token}
    safe_token_path || return 1
    if [ ! -s "$token_file" ]; then
        [ "$token_file" = "$SMA_ROOT/config/token" ] || { fail '自定义 Token 文件不存在或为空，请恢复该文件。'; return 1; }
        new_token || return 1
        printf '已生成本机独立 Bearer Token。\n'
    fi
    # The agent rejects group/other access and runs as the sma user.
    chown sma:sma "$token_file" || return 1
    chmod 0600 "$token_file" || return 1
    size=$(wc -c < "$token_file" | tr -d ' ')
    [ "$size" -le 4096 ] && [ -n "$(tr -d '[:space:]' < "$token_file")" ] \
        || { fail 'Token 文件为空或超过 4096 字节。'; return 1; }
    set_option --web.auth-token-file "$token_file" || return 1
    printf '鉴权已配置。Token 文件：%s（权限 0600，属主 sma）\n' "$token_file"
}
manager() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        service_manager=systemd
    elif command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
        service_manager=openrc
    elif [ -r /etc/rc.common ] && { command -v procd >/dev/null 2>&1 || [ -x /sbin/procd ]; }; then
        service_manager=procd
    else
        fail '未发现运行中的 systemd、OpenRC 或 procd。'; return 1
    fi
}
service_action() {
    manager || return 1
    if [ "$service_manager" = systemd ]; then
        case "$1" in
            start|restart)
                # Explicit maintenance/recovery should work even after failed
                # binds or rapid operations hit systemd's start rate limit.
                systemctl reset-failed sma.service || return 1
                systemctl "$1" sma.service
                ;;
            *) systemctl "$1" sma.service ;;
        esac
    elif [ "$service_manager" = openrc ]; then
        case "$1" in
            enable) rc-update add sma default ;;
            disable) rc-update del sma default ;;
            stop) rc-service --ifstarted sma stop ;;
            *) rc-service sma "$1" ;;
        esac
    else
        /etc/init.d/sma "$1"
    fi
}
restart_check() {
    service_action restart || return 1
    sleep 1
    if [ "$service_manager" = systemd ]; then
        systemctl is-active --quiet sma.service
    elif [ "$service_manager" = openrc ]; then
        rc-service sma status
    else
        /etc/init.d/sma status
    fi
}
confirm() {
    [ "${yes:-0}" -eq 1 ] && return 0
    [ -t 0 ] || { fail '非交互模式需显式添加 --yes。'; return 1; }
    printf '%s 输入 yes 确认：' "$1"
    read -r answer
    [ "$answer" = yes ] || { printf '已取消。\n'; return 1; }
}
show_config() {
    load_config || return 1
    printf '监听地址：%s\nToken 文件：%s\n配置文件：%s/config/sma.env\n' "$listen" "${token_file:-未启用鉴权}" "$SMA_ROOT"
}
set_listen() {
    address=$1
    case "$address" in
        *[!a-zA-Z0-9.:\[\]-]*|''|:*|*: ) fail '地址格式示例：127.0.0.1:9108、0.0.0.0:9108、[::]:9108'; return 1 ;;
    esac
    port=${address##*:}
    case "$port" in ''|*[!0-9]*) fail '监听端口必须是 1–65535。'; return 1 ;; esac
    [ "$port" -ge 1 ] && [ "$port" -le 65535 ] && [ "$address" != "$port" ] || { fail '监听端口必须是 1–65535。'; return 1; }
    ensure_token || return 1
    backup=$(mktemp "$SMA_ROOT/config/.listen-backup.XXXXXX")
    cp -p "$SMA_ROOT/config/sma.env" "$backup"
    set_option --listen-address "$address"
    if restart_check; then
        rm -f "$backup"
        printf '监听地址已更新：%s\n' "$address"
    else
        mv "$backup" "$SMA_ROOT/config/sma.env"
        restart_check || true
        fail '新地址启动失败，已恢复原配置；请运行 sma-ctl logs。'; return 1
    fi
}
rotate_token() {
    confirm '更换后所有客户端需更新 Bearer Token。' || return 1
    ensure_token || return 1
    backup=$(mktemp "$(dirname "$token_file")/.token-backup.XXXXXX")
    cp -p "$token_file" "$backup"
    new_token || { rm -f "$backup"; return 1; }
    if restart_check; then
        rm -f "$backup"
        printf 'Token 已更换。运行 sma-ctl token show 查看（需 root）。\n'
    else
        mv "$backup" "$token_file"
        restart_check || true
        fail '重启失败，已恢复旧 Token。'; return 1
    fi
}
fetch() {
    if command -v curl >/dev/null 2>&1; then
        curl -fLsS --connect-timeout 10 --max-time 300 --retry 2 \
            "${SMA_SHARE_URL%/}/browse?path=%2F$1" -o "$2"
    else
        wget -q -O "$2" "${SMA_SHARE_URL%/}/browse?path=%2F$1"
    fi
}
upgrade() {
    upgrade_dir=$(mktemp -d /tmp/sma-upgrade.XXXXXX)
    trap 'rm -rf -- "$upgrade_dir"' EXIT
    fetch install-sma.sh "$upgrade_dir/install-sma.sh"
    fetch install-sma.sh.sha256 "$upgrade_dir/install-sma.sh.sha256"
    (cd "$upgrade_dir" && sha256sum -c install-sma.sh.sha256) || { fail '安装脚本校验失败。'; return 1; }
    sh "$upgrade_dir/install-sma.sh" "$@"
}
uninstall() {
    purge=0; yes=0
    for arg in "$@"; do
        case "$arg" in --purge) purge=1 ;; --yes) yes=1 ;; *) fail "未知参数：$arg"; return 1 ;; esac
    done
    if [ "$purge" -eq 1 ]; then
        confirm '将卸载 SMA 并删除 /opt/sma 中的配置和 Token（保留日志、系统用户和已有备份）。' || return 1
    else
        confirm '将停止并卸载 SMA，备份和保留配置及 Token。' || return 1
    fi
    config_backup=
    if [ "$purge" -eq 0 ] && [ -d "$SMA_ROOT/config" ]; then
        mkdir -p /var/backups/sma
        chmod 0700 /var/backups/sma
        config_backup=$(mktemp -d /var/backups/sma/uninstall.XXXXXX)
        cp -pR "$SMA_ROOT/config" "$config_backup/config"
        printf '配置备份：%s/config\n' "$config_backup"
    fi
    service_action stop || { fail '停止失败，卸载已中止。'; return 1; }
    service_action disable || { fail '关闭开机自启失败，卸载已中止。'; return 1; }
    if command -v dpkg-query >/dev/null 2>&1 && [ "$(dpkg-query -W -f='${db:Status-Status}' server-monitor-agent 2>/dev/null || true)" = installed ]; then
        if [ "$purge" -eq 1 ]; then dpkg --purge server-monitor-agent; else dpkg --remove server-monitor-agent; fi
    elif command -v dpkg-query >/dev/null 2>&1 && [ "$(dpkg-query -W -f='${db:Status-Status}' sma 2>/dev/null || true)" = installed ] && dpkg-query -L sma 2>/dev/null | grep -Fxq /opt/sma/bin/sma; then
        if [ "$purge" -eq 1 ]; then dpkg --purge sma; else dpkg --remove sma; fi
    elif command -v rpm >/dev/null 2>&1 && rpm -q sma >/dev/null 2>&1; then
        rpm -e sma
    elif command -v apk >/dev/null 2>&1 && apk info -e sma >/dev/null 2>&1; then
        apk del sma
    else
        rm -f /etc/systemd/system/sma.service /etc/init.d/sma
    fi
    if [ "$service_manager" = systemd ]; then systemctl daemon-reload; fi
    if [ "$purge" -eq 1 ]; then
        rm -rf -- "$SMA_ROOT"
    else
        rm -rf -- "$SMA_ROOT/bin" "$SMA_ROOT/docs" "$SMA_ROOT/discovery"
        rm -f "$SMA_ROOT/README.md" "$SMA_ROOT/CHANGELOG.md" "$SMA_ROOT/installed-version"
        if [ -n "$config_backup" ]; then
            mkdir -p "$SMA_ROOT"
            cp -pR "$config_backup/config" "$SMA_ROOT/"
        fi
    fi
    if [ "$(readlink /usr/local/bin/sma-ctl 2>/dev/null || true)" = "$SMA_ROOT/bin/sma-ctl" ]; then
        rm -f /usr/local/bin/sma-ctl
    fi
    printf '卸载完成。安装日志仍保留在 /var/log/sma-install.log。\n'
}
main() {
    action=${1:-help}
    [ "$#" -eq 0 ] || shift
    case "$action" in help|-h|--help) help; return ;; esac
    [ "$(id -u)" -eq 0 ] || { fail '请以 root 身份执行 sma-ctl（系统提供 sudo 时也可使用）。'; return 1; }
    umask 077
    case "$action" in
        status)
            show_config
            if [ -r "$SMA_ROOT/installed-version" ]; then printf '版本：'; cat "$SMA_ROOT/installed-version"; fi
            manager
            if [ "$service_manager" = systemd ]; then
                systemctl is-enabled sma.service || true
                systemctl --no-pager --full status sma.service
            elif [ "$service_manager" = openrc ]; then rc-update show default; rc-service sma status
            else
                if /etc/init.d/sma enabled; then printf '开机自启：已启用\n'; else printf '开机自启：未启用\n'; fi
                /etc/init.d/sma status
            fi
            ;;
        doctor)
            show_config
            [ -x "$SMA_ROOT/bin/sma" ] || { fail 'Agent 程序缺失。'; return 1; }
            [ -n "$token_file" ] && [ -s "$token_file" ] || { fail '未配置有效 Token，请运行 sma-ctl token ensure。'; return 1; }
            # Verify as the actual service user; this also catches inaccessible parent directories.
            su -s /bin/sh sma -c 'test -r "$1"' sh "$token_file" || { fail 'sma 用户无法读取 Token。'; return 1; }
            permissions=$(ls -l "$token_file" | awk '{print $1}')
            case "$permissions" in -???------*) ;; *) fail 'Token 应设置为 0600。'; return 1 ;; esac
            service_action status
            printf '检查通过。\n'
            ;;
        start|stop|restart|enable|disable) [ "$#" -eq 0 ] || { help; return 2; }; service_action "$action" ;;
        logs)
            lines=${1:-100}
            case "$lines" in ''|*[!0-9]*) fail '日志行数必须为正整数。'; return 1 ;; esac
            manager
            if [ "$service_manager" = systemd ]; then journalctl -u sma.service -n "$lines" --no-pager;
            elif [ "$service_manager" = procd ]; then logread | tail -n "$lines";
            elif [ -f /var/log/sma.log ]; then tail -n "$lines" /var/log/sma.log;
            else fail '旧版 OpenRC 服务未配置日志文件；请检查系统日志或升级安装包。'; return 1; fi
            ;;
        config)
            case "${1:-show}" in
                show) show_config ;;
                listen) [ "$#" -eq 2 ] || { help; return 2; }; set_listen "$2" ;;
                *) help; return 2 ;;
            esac
            ;;
        token)
            case "${1:-}" in
                show) load_config; [ -n "$token_file" ] && [ -s "$token_file" ] || { fail '尚未生成 Token，请运行 sma-ctl token ensure。'; return 1; }; cat "$token_file" ;;
                ensure) ensure_token; restart_check ;;
                reset) yes=0; [ "${2:-}" != --yes ] || yes=1; rotate_token ;;
                *) help; return 2 ;;
            esac
            ;;
        # Installer calls this before its own restart/verification.
        prepare-auth) ensure_token ;;
        upgrade) upgrade "$@" ;;
        uninstall) uninstall "$@" ;;
        *) help; return 2 ;;
    esac
}

if [ "${SMA_CTL_SOURCE_ONLY:-0}" != 1 ]; then main "$@"; fi
