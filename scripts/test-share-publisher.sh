#!/bin/sh
# Safe local publishing regression test; no production service is touched.
set -eu
[ "$#" -eq 1 ] || { echo 'Usage: sh scripts/test-share-publisher.sh VERSION' >&2; exit 2; }
version=${1#v}
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d /tmp/sma-publisher-test.XXXXXX)
server_pid=
cleanup() {
    if [ -n "$server_pid" ]; then
        kill "$server_pid" 2>/dev/null || true
        wait "$server_pid" 2>/dev/null || true
    fi
    rm -rf -- "$test_dir"
}
trap cleanup EXIT
mkdir "$test_dir/files"
python3 "$project_dir/scripts/test-share-server.py" "$test_dir/files" > "$test_dir/port" 2> "$test_dir/server.log" &
server_pid=$!
attempt=0
while [ ! -s "$test_dir/port" ]; do
    attempt=$((attempt+1)); [ "$attempt" -lt 30 ] || { echo 'Fixture server did not start.' >&2; exit 1; }
    sleep 1
done
port=$(cat "$test_dir/port")
SMA_SHARE_URL="http://127.0.0.1:$port" sh "$project_dir/scripts/publish-share.sh" "$version"
cmp "$project_dir/dist/share-v$version/latest.txt" "$test_dir/files/latest.txt"
cmp "$project_dir/dist/share-v$version/install-sma.sh" "$test_dir/files/install-sma.sh"
cmp "$project_dir/dist/share-v$version/v$version/checksums.txt" "$test_dir/files/v$version/checksums.txt"
SMA_SHARE_URL="http://127.0.0.1:$port" sh "$project_dir/scripts/publish-share.sh" "$version" >/dev/null
printf 'PASS: versioned upload, read-back verification, latest-last and idempotence\n'
