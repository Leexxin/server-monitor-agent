#!/bin/sh
# Destructive integration test for disposable CI Linux VMs only.
set -eu
[ "${SMA_RUN_INSTALL_TESTS:-}" = 1 ] && [ "$(id -u)" -eq 0 ] && [ "$(uname -s)" = Linux ] \
    || { echo 'Run as root with SMA_RUN_INSTALL_TESTS=1 on a disposable Linux VM.' >&2; exit 2; }
[ "$#" -eq 2 ] && [ -f "$1" ] || exit 2
[ ! -e /opt/sma ] && [ ! -e /usr/local/bin/sma-ctl ] \
    || { echo 'Refusing to test over an existing SMA installation.' >&2; exit 2; }
package=$1
version=$2
test_dir=$(mktemp -d /tmp/sma-universal-test.XXXXXX)
cleanup() {
    if [ -x /opt/sma/bin/sma-ctl ]; then /opt/sma/bin/sma-ctl uninstall --purge --yes >/dev/null 2>&1 || true; fi
    rm -rf -- "$test_dir"
}
trap cleanup EXIT
tar -xzf "$package" -C "$test_dir"
sh "$test_dir/install.sh"
[ "$(cat /opt/sma/installed-version)" = "$version" ]
[ -r "/opt/sma/docs/releases/v$version.md" ]
[ -r /opt/sma/docs/zabbix/template_sma_http_7.0.json ]
/opt/sma/bin/sma-ctl doctor
attempt=0
until curl -fsS http://127.0.0.1:9108/readyz >/dev/null; do
    attempt=$((attempt+1)); [ "$attempt" -lt 20 ] || exit 1
    sleep 1
done
token=$(/opt/sma/bin/sma-ctl token show)
sh "$test_dir/install.sh"
[ "$(/opt/sma/bin/sma-ctl token show)" = "$token" ]
/opt/sma/bin/sma-ctl uninstall --purge --yes
[ ! -e /opt/sma ]
printf 'PASS: universal install, docs, service, token preservation and uninstall\n'
