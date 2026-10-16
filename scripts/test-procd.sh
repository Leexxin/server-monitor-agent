#!/bin/sh
# Safe procd service-script regression test; no system files are changed.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d /tmp/sma-procd-test.XXXXXX)
trap 'rm -rf -- "$test_dir"' EXIT
events="$test_dir/events"
procd_open_instance() { printf 'open\n' >> "$events"; }
procd_close_instance() { printf 'close\n' >> "$events"; }
procd_set_param() { printf '%s\n' "$*" >> "$events"; }
SMA_OPTS='--listen-address=127.0.0.1:9108 --web.auth-token-file=/opt/sma/config/token'
. "$project_dir/packaging/procd/sma"
[ "$USE_PROCD" = 1 ]
start_service
grep -Fxq 'command /opt/sma/bin/sma --listen-address=127.0.0.1:9108 --web.auth-token-file=/opt/sma/config/token' "$events"
grep -Fxq 'user sma' "$events"
grep -Fxq 'group sma' "$events"
grep -Fxq 'respawn 3600 5 5' "$events"
grep -Fxq 'stdout 1' "$events"
grep -Fxq 'stderr 1' "$events"
grep -Fxq 'close' "$events"
printf 'PASS: procd command, unprivileged user, respawn and log forwarding\n'
