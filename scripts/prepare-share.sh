#!/bin/sh
# Build a versioned SFTPGo upload tree from verified release packages.
set -eu

[ "$#" -eq 1 ] || { echo 'Usage: sh scripts/prepare-share.sh VERSION' >&2; exit 2; }
version=${1#v}
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$' \
    || { echo "Invalid version: $version" >&2; exit 2; }

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
packages="$project_dir/dist/packages"
stage="$project_dir/dist/share-v$version"
[ -d "$packages" ] || { echo 'Build packages first.' >&2; exit 1; }
[ ! -e "$stage" ] || { echo "Stage already exists: $stage" >&2; exit 1; }
[ "$(tr -d '\r\n' < "$packages/latest.txt")" = "$version" ] \
    || { echo 'Package latest.txt does not match the requested version.' >&2; exit 1; }

(
    cd "$packages"
    sha256sum -c checksums.txt
    sha256sum -c install-sma.sh.sha256
    sha256sum -c sma-ctl.sh.sha256
) >/dev/null

mkdir -p "$stage/v$version"
cp -p "$packages"/* "$stage/v$version/"
cp -p "$packages/install-sma.sh" "$packages/install-sma.sh.sha256" "$packages/latest.txt" "$stage/"

tar -C "$stage" -czf "$project_dir/dist/sma_${version}_gofpt_share.tar.gz" .
(
    cd "$project_dir/dist"
    sha256sum "sma_${version}_gofpt_share.tar.gz" > "sma_${version}_gofpt_share.tar.gz.sha256"
)
printf 'Prepared GOFTP layout: %s\n' "$stage"
