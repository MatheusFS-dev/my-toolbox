#!/bin/sh
set -eu

repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM
mkdir -p "$test_root/bin" "$test_root/payload" "$test_root/home" "$test_root/downloads" "$test_root/tmp"

cp "$repository_root/commands.json" "$test_root/payload/commands.json"
printf '%s\n' '0.1.5' > "$test_root/payload/version.txt"
printf '%s\n' '#!/bin/sh' 'exit 0' > "$test_root/payload/tb"
chmod 755 "$test_root/payload/tb"
python3 - "$repository_root/commands.json" "$test_root/payload" <<'PY'
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
root = pathlib.Path(sys.argv[2])
for command in catalog["commands"]:
    if command["protocol"] == "builtin":
        continue
    for relative_path in command["entrypoints"].get("linux-amd64", [])[1:]:
        path = root / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("fixture\n", encoding="utf-8")
requirements = root / "packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt"
requirements.parent.mkdir(parents=True, exist_ok=True)
requirements.write_text("toml==0.10.2\n", encoding="utf-8")
PY
tar -C "$test_root/payload" -czf "$test_root/downloads/toolbox-linux-amd64.tar.gz" .
(
    cd "$test_root/downloads"
    sha256sum toolbox-linux-amd64.tar.gz > toolbox-linux-amd64.tar.gz.sha256
)

cat > "$test_root/bin/uname" <<'SH'
#!/bin/sh
printf '%s\n' x86_64
SH
cat > "$test_root/bin/curl" <<'SH'
#!/bin/sh
set -eu
output=
url=
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) output=$2; shift 2 ;;
        -*) shift ;;
        *) url=$1; shift ;;
    esac
done
case "$url" in
    */releases/latest) printf '%s\n' '{"tag_name":"v0.1.5"}' ;;
    */toolbox-linux-amd64.tar.gz.sha256) cp "$FIXTURE_DOWNLOADS/toolbox-linux-amd64.tar.gz.sha256" "$output" ;;
    */toolbox-linux-amd64.tar.gz) cp "$FIXTURE_DOWNLOADS/toolbox-linux-amd64.tar.gz" "$output" ;;
    *) printf 'Unexpected fixture URL: %s\n' "$url" >&2; exit 1 ;;
esac
SH
cat > "$test_root/bin/mv" <<'SH'
#!/bin/sh
if [ "${FAIL_CURRENT_MOVE:-0}" -eq 1 ] && [ "$#" -eq 2 ]; then
    case "$2" in
        */current.txt) exit 1 ;;
    esac
fi
if [ "${FAIL_VERSION_MOVE:-0}" -eq 1 ] && [ "$#" -eq 2 ]; then
    case "$2" in
        */versions/0.1.5)
            mkdir -p "$2"
            printf 'partial\n' > "$2/partial"
            exit 1
            ;;
    esac
fi
exec /bin/mv "$@"
SH
chmod 755 "$test_root/bin/uname" "$test_root/bin/curl" "$test_root/bin/mv"

output=$(HOME="$test_root/home" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh")
for text in \
    ' __  __ __   __  _____ ___   ___  _     ____   _____  __' \
    '[INFO] Stage 1/7: prerequisites' \
    '[INFO] Stage 2/7: release lookup' \
    '[INFO] Stage 3/7: download' \
    '[INFO] Stage 4/7: checksum' \
    '[INFO] Stage 5/7: extraction/validation' \
    '[INFO] Stage 6/7: installation' \
    '[INFO] Stage 7/7: activation' \
    '[OK] Stage 7/7: activation' \
    'Add '; do
    if ! printf '%s\n' "$output" | grep -F "$text" >/dev/null; then
        printf 'Installer output is missing %s.\n%s\n' "$text" "$output" >&2
        exit 1
    fi
done
if printf '%s' "$output" | LC_ALL=C grep "$(printf '\033')" >/dev/null; then
    printf 'Redirected installer output contains ANSI escapes.\n' >&2
    exit 1
fi
if [ ! -f "$test_root/home/.local/share/my-toolbox/versions/0.1.5/packages/others/create_project_template.py" ]; then
    printf 'Installer did not install the fixture payload.\n' >&2
    exit 1
fi
if find "$test_root/tmp" -mindepth 1 -print | grep . >/dev/null; then
    printf 'Installer left a temporary directory behind.\n' >&2
    exit 1
fi

mkdir -p "$test_root/failure-home"
printf '%064d  toolbox-linux-amd64.tar.gz\n' 0 > "$test_root/downloads/toolbox-linux-amd64.tar.gz.sha256"
if HOME="$test_root/failure-home" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >"$test_root/failure.out" 2>&1; then
    printf 'Installer accepted an invalid checksum.\n' >&2
    exit 1
fi
if ! grep -F '[FAIL] Stage 4/7: checksum' "$test_root/failure.out" >/dev/null; then
    printf 'Checksum failure did not identify its active stage.\n' >&2
    exit 1
fi
if [ -e "$test_root/failure-home/.local/share/my-toolbox/versions/0.1.5" ]; then
    printf 'Checksum failure created a version directory.\n' >&2
    exit 1
fi
if find "$test_root/tmp" -mindepth 1 -print | grep . >/dev/null; then
    printf 'Failed installer left a temporary directory behind.\n' >&2
    exit 1
fi

(
    cd "$test_root/downloads"
    sha256sum toolbox-linux-amd64.tar.gz > toolbox-linux-amd64.tar.gz.sha256
)
mkdir -p "$test_root/publication-failure-home"
if HOME="$test_root/publication-failure-home" TMPDIR="$test_root/tmp" FAIL_VERSION_MOVE=1 FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >"$test_root/publication-failure.out" 2>&1; then
    printf 'Installer accepted a partial version publication failure.\n' >&2
    exit 1
fi
if ! grep -F '[FAIL] Stage 6/7: installation' "$test_root/publication-failure.out" >/dev/null; then
    printf 'Publication failure did not identify its active stage.\n' >&2
    exit 1
fi
if find "$test_root/publication-failure-home/.local/share/my-toolbox/versions" -mindepth 1 -print | grep . >/dev/null; then
    printf 'Publication failure left a version or staging directory behind.\n' >&2
    exit 1
fi

mkdir -p "$test_root/activation-failure-home"
if HOME="$test_root/activation-failure-home" TMPDIR="$test_root/tmp" FAIL_CURRENT_MOVE=1 FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >"$test_root/activation-failure.out" 2>&1; then
    printf 'Installer accepted an activation publication failure.\n' >&2
    exit 1
fi
if ! grep -F '[FAIL] Stage 7/7: activation' "$test_root/activation-failure.out" >/dev/null; then
    printf 'Activation failure did not identify its active stage.\n' >&2
    exit 1
fi
if [ -e "$test_root/activation-failure-home/.local/share/my-toolbox/versions/0.1.5" ] ||
    [ -e "$test_root/activation-failure-home/.local/bin/tb" ] ||
    [ -e "$test_root/activation-failure-home/.local/share/my-toolbox/current.txt" ]; then
    printf 'Activation failure left a partial installation behind.\n' >&2
    exit 1
fi
if find "$test_root/tmp" -mindepth 1 -print | grep . >/dev/null; then
    printf 'Activation failure left a temporary directory behind.\n' >&2
    exit 1
fi
