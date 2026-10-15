#!/bin/sh
# Destructive lifecycle smoke test, intended ONLY for a disposable CI Linux VM.
set -eu
[ "${SMA_RUN_INSTALL_TESTS:-}" = 1 ] || { echo 'Set SMA_RUN_INSTALL_TESTS=1 on a disposable VM.' >&2; exit 2; }
[ "$(id -u)" -eq 0 ] && [ "$(uname -s)" = Linux ] || exit 2
[ "$#" -eq 1 ] && [ -f "$1" ] || exit 2
[ ! -e /opt/sma ] && [ ! -e /usr/local/bin/sma-ctl ] || { echo 'Refusing to test on an existing installation.' >&2; exit 2; }
package=$1
ctl=/opt/sma/bin/sma-ctl
ready() {
    attempt=0
    until curl -fsS http://127.0.0.1:9108/readyz >/dev/null; do
        attempt=$((attempt+1)); [ "$attempt" -lt 20 ] || return 1
        sleep 1
    done
}
authorized() {
    curl -fsS -H "Authorization: Bearer $token" http://127.0.0.1:9108/v1/snapshot >/dev/null
}
dpkg -i "$package"
for doc in README.md maintenance.md troubleshooting.md releases/v0.2.3.md; do
    [ -r "/opt/sma/docs/$doc" ] || { echo "Missing installed document: $doc" >&2; exit 1; }
done
ready
"$ctl" doctor
"$ctl" status
token=$("$ctl" token show)
[ "${#token}" -eq 64 ]
[ "$(stat -c '%a:%U' /opt/sma/config/token)" = 600:sma ]
[ "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:9108/v1/snapshot)" = 401 ]
authorized

# Native reinstall must preserve credentials and their service-readable owner.
dpkg -i "$package"
ready
[ "$("$ctl" token show)" = "$token" ]
authorized
"$ctl" disable
if systemctl is-enabled --quiet sma.service; then exit 1; fi
"$ctl" enable
systemctl is-enabled --quiet sma.service
"$ctl" stop
if systemctl is-active --quiet sma.service; then exit 1; fi
"$ctl" start
ready

# Unbindable address must roll back without losing the working endpoint.
if "$ctl" config listen 192.0.2.1:9108; then echo 'Expected bind failure' >&2; exit 1; fi
ready
authorized
"$ctl" token reset --yes
ready
new_value=$("$ctl" token show)
[ "$new_value" != "$token" ]
[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" http://127.0.0.1:9108/v1/snapshot)" = 401 ]
token=$new_value
authorized
"$ctl" uninstall --yes
[ -f /opt/sma/config/token ] && [ ! -e /opt/sma/bin/sma ]
dpkg -i "$package"
ready
[ "$("$ctl" token show)" = "$token" ]
authorized
"$ctl" uninstall --purge --yes
[ ! -e /opt/sma ] && [ ! -L /usr/local/bin/sma-ctl ]
printf 'PASS: native DEB install, authentication, reinstall, services, rollback, token rotation, uninstall and purge\n'
