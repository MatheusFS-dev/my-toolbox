#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    printf 'Usage: scripts/build-release.sh VERSION OUTPUT_DIRECTORY\n' >&2
    exit 1
fi

version=$1
output_directory=$2
major=${version%%.*}
remainder=${version#*.}
minor=${remainder%%.*}
patch=${remainder#*.}
case "$version" in *.*.*) ;; *) printf 'Version must be a canonical three-part version.\n' >&2; exit 1 ;; esac
for component in "$major" "$minor" "$patch"; do
    case "$component" in ''|*[!0-9]*|0[0-9]*) printf 'Version must be a canonical three-part version.\n' >&2; exit 1 ;; esac
done

# The empty assignment prevents inherited CDPATH output from contaminating pwd.
# shellcheck disable=SC1007
repository_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
mkdir -p "$output_directory"
# shellcheck disable=SC1007
output_directory=$(CDPATH= cd -- "$output_directory" && pwd)
temporary_root=$(mktemp -d)
cleanup() {
    rm -rf "$temporary_root"
}
trap cleanup EXIT HUP INT TERM

build_payload() {
    platform=$1
    goos=$2
    goarch=$3
    binary_name=$4
    payload="$temporary_root/$platform"
    mkdir -p "$payload"
    (
        cd "$repository_root"
        GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build \
            -trimpath -ldflags "-s -w -X main.version=$version" \
            -o "$payload/$binary_name" ./src
    )
    cp "$repository_root/commands.json" "$payload/commands.json"
    cp -R "$repository_root/packages" "$payload/packages"
    find "$payload/packages" -name .git -exec rm -rf {} +
    find "$payload/packages" -type d -name __pycache__ -prune -exec rm -rf {} +
    find "$payload/packages" -type f \( -name '*.pyc' -o -name '*.pyo' \) -delete
    printf '%s\n' "$version" > "$payload/version.txt"
}

build_payload linux-amd64 linux amd64 tb
build_payload linux-arm64 linux arm64 tb
build_payload windows-amd64 windows amd64 tb.exe

for platform in linux-amd64 linux-arm64; do
    archive="toolbox-$platform.tar.gz"
    tar -C "$temporary_root/$platform" -czf "$output_directory/$archive" \
        tb commands.json packages version.txt
    (
        cd "$output_directory"
        sha256sum "$archive" > "$archive.sha256"
    )
done

archive=toolbox-windows-amd64.zip
(
    cd "$temporary_root/windows-amd64"
    zip -qry "$temporary_root/$archive" .
)
mv "$temporary_root/$archive" "$output_directory/$archive"
(
    cd "$output_directory"
    sha256sum "$archive" > "$archive.sha256"
)
printf '%s\n' "$version" > "$output_directory/version.txt"
