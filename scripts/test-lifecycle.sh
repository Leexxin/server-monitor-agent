#!/bin/sh
# Portable regression tests; all filesystem changes stay in a temporary directory.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d /tmp/sma-lifecycle-test.XXXXXX)
trap 'rm -rf -- "$test_dir"' EXIT
SMA_CTL_SOURCE_ONLY=1
export SMA_CTL_SOURCE_ONLY
. "$project_dir/scripts/sma-ctl.sh"
SMA_ROOT="$test_dir/agent"
mkdir -p "$SMA_ROOT/config"
# No privileged operations on the test host; real file permissions still apply.
chown() { :; }
assert() { "$@" || { printf 'FAIL: %s\n' "$*" >&2; exit 1; }; }
printf '# keep comment\nSMA_OPTS="--listen-address=127.0.0.1:9108 --discovery.enabled=true"\nSMA_CACHE_TTL=1s\n' > "$SMA_ROOT/config/sma.env"
ensure_token > "$test_dir/output"
first=$(cat "$SMA_ROOT/config/token")
assert test "${#first}" -eq 64
assert grep -Eq '^[0-9a-f]{64}$' "$SMA_ROOT/config/token"
assert grep -q 'SMA_CACHE_TTL=1s' "$SMA_ROOT/config/sma.env"
assert grep -q -- '--discovery.enabled=true' "$SMA_ROOT/config/sma.env"
if grep -Fq "$first" "$test_dir/output"; then fail 'Token leaked into installer output'; exit 1; fi
permissions=$(ls -l "$SMA_ROOT/config/token" | awk '{print $1}')
assert test "$permissions" = '-rw-------'
ensure_token > "$test_dir/output"
assert test "$(cat "$SMA_ROOT/config/token")" = "$first"
assert test "$(grep -c '^SMA_OPTS=' "$SMA_ROOT/config/sma.env")" -eq 1

# Configuration is data: command substitutions must never execute.
cp "$SMA_ROOT/config/sma.env" "$test_dir/good.env"
printf 'SMA_OPTS="$(touch %s/should-not-exist)"\n' "$test_dir" > "$SMA_ROOT/config/sma.env"
if load_config 2>/dev/null; then fail 'unsafe config accepted'; exit 1; fi
assert test ! -e "$test_dir/should-not-exist"
cp "$test_dir/good.env" "$SMA_ROOT/config/sma.env"

# Existing explicit token path is reused, not silently replaced by the default.
printf 'existing-token-value\n' > "$SMA_ROOT/config/custom-token"
read_options
set_option --web.auth-token-file "$SMA_ROOT/config/custom-token"
ensure_token > "$test_dir/output"
assert test "$token_file" = "$SMA_ROOT/config/custom-token"
assert test "$(cat "$token_file")" = existing-token-value
yes=0
if rotate_token < /dev/null > /dev/null 2>&1; then fail 'rotation ran without confirmation'; exit 1; fi
assert test "$(cat "$token_file")" = existing-token-value

# Failed service start restores previous token and config.
restart_check() { return 1; }
yes=1
if rotate_token > /dev/null 2>&1; then fail 'rotation ignored failed restart'; exit 1; fi
assert test "$(cat "$token_file")" = existing-token-value
if set_listen 0.0.0.0:19108 > /dev/null 2>&1; then fail 'listen ignored failed restart'; exit 1; fi
load_config
assert test "$listen" = 127.0.0.1:9108
restart_check() { return 0; }
set_listen '[::]:19108' > /dev/null
load_config
assert test "$listen" = '[::]:19108'
yes=1
rotate_token > /dev/null
assert test "$(cat "$token_file")" != existing-token-value
assert test "$(cat "$SMA_ROOT/config/token")" = "$first"

# Removal must not begin on a noninteractive invocation without --yes.
service_action() { touch "$test_dir/service-touched"; }
if uninstall < /dev/null > /dev/null 2>&1; then fail 'uninstall ran without confirmation'; exit 1; fi
assert test ! -e "$test_dir/service-touched"
assert test -f "$token_file"
id() { printf '1000\n'; }
if main token show > /dev/null 2>&1; then fail 'non-root token disclosure'; exit 1; fi
printf 'PASS: token creation/reuse, secret logging, config parsing, rollback, confirmation, permissions and root checks\n'
