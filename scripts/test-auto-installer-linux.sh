#!/bin/sh
# Destructive integration test for disposable CI Linux VMs only.
set -eu
[ "${SMA_RUN_INSTALL_TESTS:-}" = 1 ] && [ "$(id -u)" -eq 0 ] && [ "$(uname -s)" = Linux ] \
    || { echo 'Run as root with SMA_RUN_INSTALL_TESTS=1 on a disposable Linux VM.' >&2; exit 2; }
[ "$#" -eq 2 ] && [ -d "$1" ] || exit 2
stage=$1
version=$2
[ ! -e /opt/sma ] && [ ! -e /usr/local/bin/sma-ctl ] \
    || { echo 'Refusing to test over an existing SMA installation.' >&2; exit 2; }
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d /tmp/sma-auto-test.XXXXXX)
server_pid=
cleanup() {
    if [ -x /opt/sma/bin/sma-ctl ]; then /opt/sma/bin/sma-ctl uninstall --purge --yes >/dev/null 2>&1 || true; fi
    if [ -n "$server_pid" ]; then
        kill "$server_pid" 2>/dev/null || true
        wait "$server_pid" 2>/dev/null || true
    fi
    rm -rf -- "$test_dir"
}
trap cleanup EXIT
python3 "$project_dir/scripts/test-share-server.py" "$stage" > "$test_dir/port" 2> "$test_dir/server.log" &
server_pid=$!
attempt=0
while [ ! -s "$test_dir/port" ]; do
    attempt=$((attempt+1)); [ "$attempt" -lt 30 ] || { echo 'Fixture server did not start.' >&2; exit 1; }
    sleep 1
done
port=$(cat "$test_dir/port")
SMA_SHARE_URL="http://127.0.0.1:$port" sh "$stage/install-sma.sh"
[ "$(cat /opt/sma/installed-version)" = "$version" ]
/opt/sma/bin/sma-ctl doctor
attempt=0
until curl -fsS http://127.0.0.1:9108/readyz >/dev/null; do
    attempt=$((attempt+1)); [ "$attempt" -lt 20 ] || exit 1
    sleep 1
done
[ "$(/opt/sma/bin/sma-ctl token show | tr -d '\r\n' | wc -c)" -eq 64 ]
/opt/sma/bin/sma-ctl uninstall --purge --yes
[ ! -e /opt/sma ]
printf 'PASS: fixed entry, latest index, versioned files, checksum, service and uninstall\n'
