#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
    echo "Usage: $0 [version]" >&2
    exit 2
fi

version=${1:-0.1.0}
case "$version" in
    v*) version=${version#v} ;;
esac

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release_dir="$project_dir/dist/packages"
stage_dir="$project_dir/dist/stage"
nfpm_bin=${NFPM_BIN:-nfpm}
go_cache=${SMA_GOCACHE:-/tmp/sma-go-cache}
revision=${SMA_REVISION:-unknown}
build_time=${SMA_BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
source_epoch=${SOURCE_DATE_EPOCH:-$(date -u +%s)}
case "$version" in
    0.1.0) default_package_release=2 ;;
    *) default_package_release=1 ;;
esac
package_release=${SMA_PACKAGE_RELEASE:-$default_package_release}

if ! command -v "$nfpm_bin" >/dev/null 2>&1 && [ ! -x "$nfpm_bin" ]; then
    echo "ERROR: nfpm was not found. Set NFPM_BIN=/path/to/nfpm." >&2
    exit 1
fi

rm -rf "$release_dir" "$stage_dir"
mkdir -p "$release_dir" "$stage_dir"

build_arch() {
    goarch=$1
    package_arch=$2
    universal_arch=$3
    binary="$stage_dir/$goarch/sma"
    universal="$stage_dir/universal-$goarch"
    mkdir -p "$(dirname "$binary")" "$universal/docs" "$universal/discovery"

    CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOCACHE="$go_cache" \
        go build -trimpath \
        -ldflags "-s -w -X sma/internal/buildinfo.Version=v$version -X sma/internal/buildinfo.Revision=$revision -X sma/internal/buildinfo.BuildTime=$build_time" \
        -o "$binary" ./cmd/sma
    mkdir -p "$stage_dir/package"
    install -m 0755 "$binary" "$stage_dir/package/sma"

    NFPM_ARCH="$package_arch" NFPM_VERSION="$version" NFPM_RELEASE="$package_release" SOURCE_DATE_EPOCH="$source_epoch" \
        "$nfpm_bin" package --config packaging/nfpm.yaml --packager deb \
        --target "$release_dir/sma_${version}_linux_${package_arch}.deb"

    if command -v ar >/dev/null 2>&1 && command -v tar >/dev/null 2>&1; then
        deb_listing=$(ar p "$release_dir/sma_${version}_linux_${package_arch}.deb" data.tar.gz | tar -tzf -)
        case "$deb_listing" in
            *etc/init.d/sma*)
                echo "ERROR: DEB package must not contain /etc/init.d/sma" >&2
                exit 1
                ;;
        esac
        case "$deb_listing" in
            *usr/lib/systemd/system/sma.service*) ;;
            *)
                echo "ERROR: DEB package is missing the systemd unit" >&2
                exit 1
                ;;
        esac
    fi

    NFPM_ARCH="$package_arch" NFPM_VERSION="$version" NFPM_RELEASE="$package_release" SOURCE_DATE_EPOCH="$source_epoch" \
        "$nfpm_bin" package --config packaging/nfpm.yaml --packager rpm \
        --target "$release_dir/sma-${version}-${package_release}.${universal_arch}.rpm"
    NFPM_ARCH="$package_arch" NFPM_VERSION="$version" NFPM_RELEASE="$package_release" SOURCE_DATE_EPOCH="$source_epoch" \
        "$nfpm_bin" package --config packaging/nfpm.yaml --packager apk \
        --target "$release_dir/sma_${version}_${universal_arch}.apk"

    install -m 0755 "$binary" "$universal/sma"
    install -m 0755 packaging/universal/install.sh "$universal/install.sh"
    install -m 0755 packaging/universal/uninstall.sh "$universal/uninstall.sh"
    install -m 0644 packaging/config/sma.env "$universal/sma.env"
    install -m 0644 packaging/systemd/sma.service "$universal/sma.service"
    install -m 0755 packaging/openrc/sma "$universal/sma.openrc"
    install -m 0644 README.md "$universal/README.md"
    install -m 0644 docs/api-integration.md "$universal/docs/api-integration.md"
    install -m 0644 docs/server-monitor-agent-development.md "$universal/docs/server-monitor-agent-development.md"
    install -m 0644 docs/installation.md "$universal/docs/installation.md"
    install -m 0644 docs/discovery-api-integration.md "$universal/docs/discovery-api-integration.md"
    install -m 0755 packaging/discovery/common-services.sh "$universal/discovery/common-services.sh"
    tar -C "$universal" -czf "$release_dir/sma_${version}_linux_${universal_arch}.tar.gz" .
}

cd "$project_dir"
build_arch amd64 amd64 x86_64
build_arch arm64 arm64 aarch64

(
    cd "$release_dir"
    sha256sum ./*.deb ./*.rpm ./*.apk ./*.tar.gz > checksums.txt
)

echo "Packages created in $release_dir"
