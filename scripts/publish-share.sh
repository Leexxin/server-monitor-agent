#!/bin/sh
# Publish an already staged release to an SFTPGo public share. latest.txt goes last.
set -eu

[ "$#" -eq 1 ] || { echo 'Usage: sh scripts/publish-share.sh VERSION' >&2; exit 2; }
version=${1#v}
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$' \
    || { echo "Invalid version: $version" >&2; exit 2; }

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=${SMA_SHARE_STAGE:-$project_dir/dist/share-v$version}
share_url=${SMA_SHARE_URL:-http://192.168.33.123:8080/web/client/pubshares/MFAWcGkHLBH2ZLquo63Mcm}
[ -d "$stage/v$version" ] && [ -f "$stage/latest.txt" ] \
    || { echo "Missing staged release: $stage (run prepare-share.sh first)." >&2; exit 1; }
[ "$(tr -d '\r\n' < "$stage/latest.txt")" = "$version" ] \
    || { echo 'Staged latest.txt does not match the release version.' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo 'curl is required.' >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo 'sha256sum is required.' >&2; exit 1; }
(
    cd "$stage/v$version"
    [ "$(tr -d '\r\n' < latest.txt)" = "$version" ]
    sha256sum -c checksums.txt
    sha256sum -c install-sma.sh.sha256
    sha256sum -c sma-ctl.sh.sha256
) >/dev/null || { echo 'Versioned release files failed checksum validation.' >&2; exit 1; }
(
    cd "$stage"
    sha256sum -c install-sma.sh.sha256
    cmp -s install-sma.sh "v$version/install-sma.sh"
) >/dev/null || { echo 'Fixed installer differs from the versioned release.' >&2; exit 1; }

tmp=$(mktemp -d /tmp/sma-share-publish.XXXXXX)
trap 'rm -rf -- "$tmp"' EXIT
curl -fLsS --connect-timeout 10 --max-time 30 -c "$tmp/cookies" -b "$tmp/cookies" \
    -o "$tmp/upload.html" "${share_url%/}/upload"
csrf=$(sed -n "s/.*'X-CSRF-TOKEN': '\([^']*\)'.*/\1/p" "$tmp/upload.html" | head -n 1)
[ -n "$csrf" ] || { echo 'Could not read the SFTPGo upload CSRF token.' >&2; exit 1; }

encoded_path() {
    printf '%s' "$1" | sed 's/+/%2B/g; s/\//%2F/g'
}
fetch_remote() {
    path=$1
    encoded=$(encoded_path "$path")
    curl -fLsS --connect-timeout 10 --max-time 300 -b "$tmp/cookies" \
        -o "$tmp/remote" "${share_url%/}/browse?path=%2F$encoded" || return 1
    # A missing file can be represented by a successful HTML directory page.
    if head -c 256 "$tmp/remote" | grep -Eiq '<!doctype html|<html'; then return 1; fi
}
publish_file() {
    file=$1
    path=$2
    immutable=$3
    expected=$(sha256sum "$file" | awk '{print $1}')
    if fetch_remote "$path"; then
        current=$(sha256sum "$tmp/remote" | awk '{print $1}')
        if [ "$current" = "$expected" ]; then
            printf 'Already published: %s\n' "$path"
            return 0
        fi
        if [ "$immutable" = yes ]; then
            echo "Refusing to overwrite a different versioned artifact: $path" >&2
            return 1
        fi
    fi
    encoded=$(encoded_path "$path")
    code=$(curl -sS --connect-timeout 10 --max-time 300 -b "$tmp/cookies" -c "$tmp/cookies" \
        -H "X-CSRF-TOKEN: $csrf" -H 'Content-Type: application/octet-stream' \
        --data-binary "@$file" -o "$tmp/upload-response" -w '%{http_code}' \
        "${share_url%/}/%2F%2F$encoded?mkdir_parents=true")
    [ "$code" = 201 ] || { echo "Upload failed for $path (HTTP $code)." >&2; return 1; }
    fetch_remote "$path" || { echo "Read-back failed for $path." >&2; return 1; }
    current=$(sha256sum "$tmp/remote" | awk '{print $1}')
    [ "$current" = "$expected" ] || { echo "Read-back checksum mismatch: $path" >&2; return 1; }
    printf 'Published and verified: %s\n' "$path"
}

for file in "$stage/v$version"/*; do
    publish_file "$file" "v$version/$(basename "$file")" yes
done
publish_file "$stage/install-sma.sh" install-sma.sh no
publish_file "$stage/install-sma.sh.sha256" install-sma.sh.sha256 no
publish_file "$stage/latest.txt" latest.txt no
printf 'GOFTP latest is now v%s.\n' "$version"
