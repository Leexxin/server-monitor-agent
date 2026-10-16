#!/bin/sh
# One command for maintainers: fetch a GitHub Release share bundle and publish it.
set -eu

[ "$#" -eq 1 ] || { echo 'Usage: sh scripts/deploy-share.sh VERSION' >&2; exit 2; }
version=${1#v}
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$' \
    || { echo "Invalid version: $version" >&2; exit 2; }

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
command -v curl >/dev/null 2>&1 || { echo 'curl is required.' >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo 'sha256sum is required.' >&2; exit 1; }
tmp=$(mktemp -d /tmp/sma-share-release.XXXXXX)
trap 'rm -rf -- "$tmp"' EXIT
name="sma_${version}_gofpt_share.tar.gz"
release_url="https://github.com/Leexxin/server-monitor-agent/releases/download/v$version"
curl -fLsS --retry 2 --connect-timeout 10 --max-time 300 -o "$tmp/$name" "$release_url/$name"
curl -fLsS --retry 2 --connect-timeout 10 --max-time 30 -o "$tmp/$name.sha256" "$release_url/$name.sha256"
(cd "$tmp" && sha256sum -c "$name.sha256")
mkdir "$tmp/unpack"
tar -xzf "$tmp/$name" -C "$tmp/unpack"
[ "$(tr -d '\r\n' < "$tmp/unpack/latest.txt")" = "$version" ] \
    || { echo 'Release bundle version mismatch.' >&2; exit 1; }
SMA_SHARE_STAGE="$tmp/unpack" sh "$project_dir/scripts/publish-share.sh" "$version"
