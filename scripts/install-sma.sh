#!/bin/sh
# SMA one-command installer for Linux. Keep this file POSIX-sh compatible.
set -eu

SHARE_URL=${SMA_SHARE_URL:-http://192.168.33.123:8080/web/client/pubshares/MFAWcGkHLBH2ZLquo63Mcm}
LOG_FILE=${SMA_INSTALL_LOG:-/var/log/sma-install.log}
REQUESTED_VERSION=
FORCE=0
SHOW_TOKEN=0

usage() {
    cat <<'EOF'
Usage: sudo sh install-sma.sh [--version VERSION] [--force] [--show-token]

Options:
  --version VERSION  Install this version instead of latest.txt (its files must
                     still be present in checksums.txt).
  --force            Reinstall even when the installed version matches.
  --show-token       Print the Bearer Token after successful installation.
  -h, --help         Show this help.

Environment:
  SMA_SHARE_URL      SFTPGo public-share URL, without /browse (default: SMA LAN share)
  SMA_INSTALL_LOG    Log file (default: /var/log/sma-install.log)
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version)
            [ "$#" -ge 2 ] || { usage >&2; exit 2; }
            REQUESTED_VERSION=$2
            shift 2
            ;;
        --force) FORCE=1; shift ;;
        --show-token) SHOW_TOKEN=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) usage >&2; exit 2 ;;
    esac
done

if [ "$(id -u)" -ne 0 ]; then
    echo 'ERROR: SMA installation requires root. Run: sudo sh install-sma.sh' >&2
    exit 1
fi
[ "$(uname -s)" = Linux ] || { echo 'ERROR: Linux is required.' >&2; exit 1; }

case "$LOG_FILE" in
    /*) ;;
    *) echo 'ERROR: SMA_INSTALL_LOG must be an absolute path.' >&2; exit 2 ;;
esac
umask 077
touch "$LOG_FILE"
chmod 0600 "$LOG_FILE"

log() {
    printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" | tee -a "$LOG_FILE"
}

fatal() {
    log "ERROR: $*" >&2
    exit 1
}

work_dir=$(mktemp -d /tmp/sma-install.XXXXXX) || fatal 'Cannot create temporary directory.'
cleanup() { rm -rf -- "$work_dir"; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

run_logged() {
    log "RUN: $*"
    if "$@" >"$work_dir/command.log" 2>&1; then
        cat "$work_dir/command.log" | tee -a "$LOG_FILE"
    else
        code=$?
        cat "$work_dir/command.log" | tee -a "$LOG_FILE"
        fatal "Command failed (exit $code): $*"
    fi
}

download() {
    name=$1
    output=$2
    # Names are constrained to release artifact basenames before this function.
    url="${SHARE_URL%/}/browse?path=%2F$name"
    log "Downloading $name"
    if command -v curl >/dev/null 2>&1; then
        curl --fail --location --silent --show-error --retry 3 \
            --connect-timeout 10 --max-time 300 --output "$output" "$url" \
            >>"$LOG_FILE" 2>&1 || fatal "Download failed: $name"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$output" "$url" >>"$LOG_FILE" 2>&1 \
            || fatal "Download failed: $name"
    else
        fatal 'curl or wget is required.'
    fi
    [ -s "$output" ] || fatal "Downloaded file is empty: $name"
}

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    service_manager=systemd
elif command -v rc-update >/dev/null 2>&1 && command -v rc-service >/dev/null 2>&1; then
    service_manager=openrc
else
    fatal 'No supported service manager found (systemd or OpenRC). Cannot configure autostart.'
fi

machine=$(uname -m)
case "$machine" in
    x86_64|amd64) go_arch=amd64; package_arch=x86_64 ;;
    aarch64|arm64) go_arch=arm64; package_arch=aarch64 ;;
    *) fatal "Unsupported CPU architecture: $machine" ;;
esac

os_id=
os_like=
if [ -r /etc/os-release ]; then
    # os-release is a system-owned shell-compatible assignment file.
    . /etc/os-release
    os_id=${ID:-}
    os_like=${ID_LIKE:-}
fi
case " $os_id $os_like " in
    *' alpine '*) package_kind=apk ;;
    *' debian '*|*' ubuntu '*) package_kind=deb ;;
    *' rhel '*|*' fedora '*|*' centos '*|*' rocky '*|*' almalinux '*|*' ol '*|*' suse '*|*' opensuse '*) package_kind=rpm ;;
    *) package_kind=tar ;;
esac

if [ "$package_kind" = deb ] && ! command -v dpkg >/dev/null 2>&1; then
    fatal 'Debian package manager dpkg was not found.'
fi
if [ "$package_kind" = rpm ] && ! command -v rpm >/dev/null 2>&1; then
    fatal 'RPM package manager was not found.'
fi
if [ "$package_kind" = apk ] && ! command -v apk >/dev/null 2>&1; then
    fatal 'Alpine package manager apk was not found.'
fi

download latest.txt "$work_dir/latest.txt"
if [ -n "$REQUESTED_VERSION" ]; then
    version=$REQUESTED_VERSION
else
    version=$(tr -d '\r\n' < "$work_dir/latest.txt")
fi
case "$version" in
    ''|*[!0-9A-Za-z.+-]*) fatal "Invalid version: $version" ;;
esac
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$' \
    || fatal "Invalid version: $version"

download checksums.txt "$work_dir/checksums.txt"
case "$package_kind" in
    deb) pattern="^sma_${version}_linux_${go_arch}\\.deb$" ;;
    rpm) pattern="^sma-${version}-[0-9]+\\.${package_arch}\\.rpm$" ;;
    apk) pattern="^sma_${version}_${package_arch}\\.apk$" ;;
    tar) pattern="^sma_${version}_linux_${package_arch}\\.tar\\.gz$" ;;
esac
matches=$(awk '{name=$2; sub(/^\.\//, "", name); print name}' "$work_dir/checksums.txt" | grep -E "$pattern" || true)
count=$(printf '%s\n' "$matches" | grep -c . || true)
[ "$count" -eq 1 ] || fatal "Expected exactly one $package_kind package for $machine and $version; found $count."
filename=$matches
expected=$(awk -v target="$filename" '{name=$2; sub(/^\.\//, "", name); if (name==target) print $1}' "$work_dir/checksums.txt")
printf '%s\n' "$expected" | grep -Eq '^[0-9a-fA-F]{64}$' \
    || fatal "Invalid SHA-256 for $filename"

installed=
case "$package_kind" in
    deb) installed=$(dpkg-query -W -f='${Version}' sma 2>/dev/null || true) ;;
    rpm) installed=$(rpm -q --qf '%{VERSION}' sma 2>/dev/null || true) ;;
    apk) installed=$(apk info -v sma 2>/dev/null | sed -n 's/^sma-\([0-9][^-]*\)-r[0-9]*$/\1/p' | head -n 1) ;;
    tar) [ ! -r /opt/sma/installed-version ] || installed=$(tr -d '\r\n' < /opt/sma/installed-version) ;;
esac
case "$installed" in
    "$version"|"$version"-*) same_version=1 ;;
    *) same_version=0 ;;
esac

log "Detected: OS=${os_id:-unknown} arch=$machine package=$package_kind service=$service_manager installed=${installed:-none} target=$version"
if [ "$same_version" -eq 1 ] && [ "$FORCE" -eq 0 ]; then
    log "SMA $version is already installed; checking autostart and service status."
else
    download "$filename" "$work_dir/$filename"
    command -v sha256sum >/dev/null 2>&1 || fatal 'sha256sum is required to verify the package.'
    actual=$(sha256sum "$work_dir/$filename" | awk '{print $1}')
    [ "$actual" = "$expected" ] || fatal "SHA-256 mismatch for $filename (expected $expected, got $actual)."
    log "SHA-256 verified: $filename"
    case "$package_kind" in
        deb) run_logged dpkg -i "$work_dir/$filename" ;;
        rpm) run_logged rpm -Uvh --replacepkgs "$work_dir/$filename" ;;
        apk) run_logged apk add --allow-untrusted "$work_dir/$filename" ;;
        tar)
            mkdir "$work_dir/universal"
            run_logged tar -xzf "$work_dir/$filename" -C "$work_dir/universal"
            run_logged sh "$work_dir/universal/install.sh"
            ;;
    esac
    printf '%s\n' "$version" > /opt/sma/installed-version
    chmod 0644 /opt/sma/installed-version
fi

download sma-ctl.sh "$work_dir/sma-ctl.sh"
download sma-ctl.sh.sha256 "$work_dir/sma-ctl.sh.sha256"
command -v sha256sum >/dev/null 2>&1 || fatal 'sha256sum is required.'
ctl_expected=$(awk '$2=="sma-ctl.sh" {print $1}' "$work_dir/sma-ctl.sh.sha256")
printf '%s\n' "$ctl_expected" | grep -Eq '^[0-9a-f]{64}$' || fatal 'Invalid sma-ctl checksum.'
ctl_actual=$(sha256sum "$work_dir/sma-ctl.sh" | awk '{print $1}')
[ "$ctl_actual" = "$ctl_expected" ] || fatal 'sma-ctl checksum mismatch.'
install -m 0755 "$work_dir/sma-ctl.sh" /opt/sma/bin/sma-ctl
chown root:root /opt/sma/bin/sma-ctl
mkdir -p /usr/local/bin
if [ ! -e /usr/local/bin/sma-ctl ] && [ ! -L /usr/local/bin/sma-ctl ]; then
    ln -s /opt/sma/bin/sma-ctl /usr/local/bin/sma-ctl
elif [ "$(readlink /usr/local/bin/sma-ctl 2>/dev/null || true)" != /opt/sma/bin/sma-ctl ]; then
    log 'Existing /usr/local/bin/sma-ctl was preserved; use /opt/sma/bin/sma-ctl.'
fi
run_logged /opt/sma/bin/sma-ctl prepare-auth

if [ "$service_manager" = systemd ]; then
    run_logged systemctl daemon-reload
    run_logged systemctl enable sma.service
    run_logged systemctl restart sma.service
    run_logged systemctl is-active --quiet sma.service
    run_logged systemctl is-enabled --quiet sma.service
else
    run_logged rc-update add sma default
    run_logged rc-service sma restart
    run_logged rc-service sma status
fi
log "SMA $version installed and enabled at boot. Log: $LOG_FILE"
run_logged /opt/sma/bin/sma-ctl config show
log '管理：sudo /opt/sma/bin/sma-ctl help'
log '查看 Token：sudo /opt/sma/bin/sma-ctl token show'
log '检查状态：sudo /opt/sma/bin/sma-ctl status'
log '卸载：sudo /opt/sma/bin/sma-ctl uninstall'
if [ "$SHOW_TOKEN" -eq 1 ]; then
    # Deliberately outside log()/run_logged(): never persist the secret to our log.
    printf '\nBearer Token（仅本次标准输出，请妥善保存）：\n'
    /opt/sma/bin/sma-ctl token show
fi
