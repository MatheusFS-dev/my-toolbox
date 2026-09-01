#!/bin/sh
set -eu

repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM
mkdir -p "$test_root/bin" "$test_root/payload" "$test_root/home" "$test_root/downloads" "$test_root/tmp"

cp "$repository_root/commands.json" "$test_root/payload/commands.json"
cp -R "$repository_root/completions" "$test_root/payload/completions"
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

make_prerequisite_bin() {
    destination=$1
    shift
    mkdir -p "$destination"
    for prerequisite in bash zsh curl tar sha256sum cp mv rm mkdir rmdir dirname mktemp head chmod uname id sed grep cmp sudo python3; do
        case " $* " in
            *" $prerequisite "*) continue ;;
        esac
        prerequisite_path=$(command -v "$prerequisite")
        ln -s "$prerequisite_path" "$destination/$prerequisite"
    done
    # GNU tar invokes gzip through PATH for .tar.gz archives. Package managers
    # provide that transitive archive helper alongside the tested tar package.
    ln -s "$(command -v gzip)" "$destination/gzip"
}

assert_no_toolbox_files() {
    target_home=$1
    if [ -e "$target_home/.local/share/my-toolbox" ] || [ -e "$target_home/.local/bin/tb" ] ||
        [ -e "$target_home/.bashrc" ] || [ -e "$target_home/.zshrc" ]; then
        printf 'Stage 1 failure changed the target home %s.\n' "$target_home" >&2
        exit 1
    fi
}

for package_manager in apt-get dnf yum pacman zypper apk; do
    prerequisite_home="$test_root/prerequisite-$package_manager-home"
    prerequisite_bin="$test_root/prerequisite-$package_manager-bin"
    mkdir -p "$prerequisite_home"
    make_prerequisite_bin "$prerequisite_bin" curl cp mv cmp
    printf '%s\n' '#!/bin/sh' 'exit 0' > "$prerequisite_bin/$package_manager"
    chmod 755 "$prerequisite_bin/$package_manager"
    if HOME="$prerequisite_home" PATH="$prerequisite_bin" /bin/sh "$repository_root/install.sh" >"$test_root/prerequisite-$package_manager.out" 2>&1; then
        printf 'Installer accepted missing prerequisites with %s.\n' "$package_manager" >&2
        exit 1
    fi
    prerequisite_output=$(cat "$test_root/prerequisite-$package_manager.out")
    for expected in \
        'Missing required commands: curl cp mv cmp' \
        'Missing Linux packages: curl coreutils diffutils'; do
        if ! printf '%s\n' "$prerequisite_output" | grep -F "$expected" >/dev/null; then
            printf '%s prerequisite report is missing %s.\n%s\n' "$package_manager" "$expected" "$prerequisite_output" >&2
            exit 1
        fi
    done
    case "$package_manager" in
        apt-get) manual_command='apt-get update && apt-get install -y curl coreutils diffutils' ;;
        dnf) manual_command='dnf install -y curl coreutils diffutils' ;;
        yum) manual_command='yum install -y curl coreutils diffutils' ;;
        pacman) manual_command='pacman -Sy --needed --noconfirm curl coreutils diffutils' ;;
        zypper) manual_command='zypper --non-interactive install curl coreutils diffutils' ;;
        apk) manual_command='apk add curl coreutils diffutils' ;;
    esac
    if ! printf '%s\n' "$prerequisite_output" | grep -F "Install manually as root: $manual_command" >/dev/null; then
        printf '%s prerequisite report has the wrong manual command.\n%s\n' "$package_manager" "$prerequisite_output" >&2
        exit 1
    fi
    if printf '%s\n' "$prerequisite_output" | grep -F 'Install missing packages?' >/dev/null; then
        printf 'Non-interactive %s prerequisite failure prompted for input.\n' "$package_manager" >&2
        exit 1
    fi
    assert_no_toolbox_files "$prerequisite_home"
done

approval_home="$test_root/approval-home"
approval_bin="$test_root/approval-bin"
approval_log="$test_root/approval.log"
mkdir -p "$approval_home"
make_prerequisite_bin "$approval_bin" cp mv
ln -s "$test_root/bin/curl" "$approval_bin/curl.fixture"
rm -f "$approval_bin/curl" "$approval_bin/uname" "$approval_bin/sudo"
ln -s "$test_root/bin/curl" "$approval_bin/curl"
ln -s "$test_root/bin/uname" "$approval_bin/uname"
cat > "$approval_bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
cat > "$approval_bin/apt-get" <<'SH'
#!/bin/sh
printf 'apt-get %s\n' "$*" >> "$PACKAGE_LOG"
if [ "$1" = install ]; then
    /bin/ln -s /bin/cp "$PREREQUISITE_BIN/cp"
    /bin/ln -s /bin/mv "$PREREQUISITE_BIN/mv"
fi
SH
chmod 755 "$approval_bin/sudo" "$approval_bin/apt-get"
if ! printf 'y\n' | script -qefc "env HOME='$approval_home' ZDOTDIR='$approval_home' TMPDIR='$test_root/tmp' FIXTURE_DOWNLOADS='$test_root/downloads' PACKAGE_LOG='$approval_log' PREREQUISITE_BIN='$approval_bin' PATH='$approval_bin' /bin/sh '$repository_root/install.sh'" /dev/null >"$test_root/approval.out" 2>&1; then
    printf 'Approved prerequisite installation did not continue successfully.\n' >&2
    cat "$test_root/approval.out" >&2
    exit 1
fi
if [ "$(cat "$approval_log")" != "apt-get update
apt-get install -y coreutils" ]; then
    printf 'Approved prerequisite installation used the wrong package commands.\n%s\n' "$(cat "$approval_log")" >&2
    exit 1
fi
if [ ! -f "$approval_home/.local/share/my-toolbox/current.txt" ]; then
    printf 'Approved prerequisite installation did not resume the toolbox installation.\n' >&2
    exit 1
fi

decline_home="$test_root/decline-home"
decline_bin="$test_root/decline-bin"
decline_log="$test_root/decline.log"
mkdir -p "$decline_home"
make_prerequisite_bin "$decline_bin" cmp
rm -f "$decline_bin/sudo"
cat > "$decline_bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
cat > "$decline_bin/apt-get" <<'SH'
#!/bin/sh
printf 'called\n' >> "$PACKAGE_LOG"
SH
chmod 755 "$decline_bin/sudo" "$decline_bin/apt-get"
if printf '\n' | script -qefc "env HOME='$decline_home' PACKAGE_LOG='$decline_log' PATH='$decline_bin' /bin/sh '$repository_root/install.sh'" /dev/null >"$test_root/decline.out" 2>&1; then
    printf 'Installer continued after prerequisite installation was declined.\n' >&2
    exit 1
fi
if ! grep -F 'Install missing packages? [y/N]' "$test_root/decline.out" >/dev/null ||
    ! grep -F 'Install manually as root: apt-get update && apt-get install -y diffutils' "$test_root/decline.out" >/dev/null; then
    printf 'Declined prerequisite installation did not provide exact remediation.\n' >&2
    cat "$test_root/decline.out" >&2
    exit 1
fi
if [ -e "$decline_log" ]; then
    printf 'Declined prerequisite installation invoked the package manager.\n' >&2
    exit 1
fi
assert_no_toolbox_files "$decline_home"

failed_packages_home="$test_root/failed-packages-home"
failed_packages_bin="$test_root/failed-packages-bin"
failed_packages_log="$test_root/failed-packages.log"
mkdir -p "$failed_packages_home"
make_prerequisite_bin "$failed_packages_bin" cmp
rm -f "$failed_packages_bin/sudo"
cat > "$failed_packages_bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
cat > "$failed_packages_bin/apt-get" <<'SH'
#!/bin/sh
printf 'apt-get %s\n' "$*" >> "$PACKAGE_LOG"
[ "$1" = update ]
SH
chmod 755 "$failed_packages_bin/sudo" "$failed_packages_bin/apt-get"
if printf 'y\n' | script -qefc "env HOME='$failed_packages_home' PACKAGE_LOG='$failed_packages_log' PATH='$failed_packages_bin' /bin/sh '$repository_root/install.sh'" /dev/null >"$test_root/failed-packages.out" 2>&1; then
    printf 'Installer continued after package installation failed.\n' >&2
    exit 1
fi
if ! grep -F 'Package installation failed. Retry manually: sudo apt-get update && sudo apt-get install -y diffutils' "$test_root/failed-packages.out" >/dev/null; then
    printf 'Failed package installation did not provide exact remediation.\n' >&2
    cat "$test_root/failed-packages.out" >&2
    exit 1
fi
assert_no_toolbox_files "$failed_packages_home"

incomplete_packages_home="$test_root/incomplete-packages-home"
incomplete_packages_bin="$test_root/incomplete-packages-bin"
mkdir -p "$incomplete_packages_home"
make_prerequisite_bin "$incomplete_packages_bin" cmp
rm -f "$incomplete_packages_bin/sudo"
cat > "$incomplete_packages_bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
printf '%s\n' '#!/bin/sh' 'exit 0' > "$incomplete_packages_bin/apt-get"
chmod 755 "$incomplete_packages_bin/sudo" "$incomplete_packages_bin/apt-get"
if printf 'y\ny\n' | script -qefc "env HOME='$incomplete_packages_home' PATH='$incomplete_packages_bin' /bin/sh '$repository_root/install.sh'" /dev/null >"$test_root/incomplete-packages.out" 2>&1; then
    printf 'Installer continued when package installation left a command unavailable.\n' >&2
    exit 1
fi
if [ "$(grep -Foc 'Install missing packages? [y/N]' "$test_root/incomplete-packages.out")" -ne 1 ] ||
    ! grep -F 'Missing required commands after package installation: cmp' "$test_root/incomplete-packages.out" >/dev/null; then
    printf 'Incomplete package installation did not fail after one prompt and a complete recheck.\n' >&2
    cat "$test_root/incomplete-packages.out" >&2
    exit 1
fi
assert_no_toolbox_files "$incomplete_packages_home"

missing_elevation_home="$test_root/missing-elevation-home"
missing_elevation_bin="$test_root/missing-elevation-bin"
mkdir -p "$missing_elevation_home"
make_prerequisite_bin "$missing_elevation_bin" cmp sudo
printf '%s\n' '#!/bin/sh' 'exit 99' > "$missing_elevation_bin/apt-get"
chmod 755 "$missing_elevation_bin/apt-get"
if HOME="$missing_elevation_home" PATH="$missing_elevation_bin" /bin/sh "$repository_root/install.sh" >"$test_root/missing-elevation.out" 2>&1; then
    printf 'Installer continued without required elevation.\n' >&2
    exit 1
fi
if ! grep -F 'Install manually as root: apt-get update && apt-get install -y diffutils' "$test_root/missing-elevation.out" >/dev/null; then
    printf 'Missing elevation did not provide the root command.\n' >&2
    cat "$test_root/missing-elevation.out" >&2
    exit 1
fi
assert_no_toolbox_files "$missing_elevation_home"

unsupported_manager_home="$test_root/unsupported-manager-home"
unsupported_manager_bin="$test_root/unsupported-manager-bin"
mkdir -p "$unsupported_manager_home"
make_prerequisite_bin "$unsupported_manager_bin" cmp
if HOME="$unsupported_manager_home" PATH="$unsupported_manager_bin" /bin/sh "$repository_root/install.sh" >"$test_root/unsupported-manager.out" 2>&1; then
    printf 'Installer continued without a supported package manager.\n' >&2
    exit 1
fi
if ! grep -F 'No supported package manager was found. Install these packages manually: diffutils' "$test_root/unsupported-manager.out" >/dev/null; then
    printf 'Unsupported package manager failure was not actionable.\n' >&2
    cat "$test_root/unsupported-manager.out" >&2
    exit 1
fi
assert_no_toolbox_files "$unsupported_manager_home"

environment_bin="$test_root/environment-bin"
make_prerequisite_bin "$environment_bin"
rm -f "$environment_bin/uname"
cat > "$environment_bin/uname" <<'SH'
#!/bin/sh
printf '%s\n' "${TEST_ARCHITECTURE:-x86_64}"
SH
chmod 755 "$environment_bin/uname"

environment_home="$test_root/environment-home"
mkdir -p "$environment_home"
if HOME="$environment_home" XDG_DATA_HOME=relative-data ZDOTDIR=relative-zsh TEST_ARCHITECTURE=i686 PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/environment.out" 2>&1; then
    printf 'Installer accepted invalid environment paths and architecture.\n' >&2
    exit 1
fi
for expected in \
    'Unsupported Linux architecture: i686' \
    'XDG_DATA_HOME must be an absolute path: relative-data' \
    'ZDOTDIR must be an absolute path: relative-zsh'; do
    if ! grep -F "$expected" "$test_root/environment.out" >/dev/null; then
        printf 'Aggregated environment report is missing %s.\n' "$expected" >&2
        cat "$test_root/environment.out" >&2
        exit 1
    fi
done
if grep -F 'Install missing packages?' "$test_root/environment.out" >/dev/null; then
    printf 'Non-package environment failures triggered package installation.\n' >&2
    exit 1
fi
assert_no_toolbox_files "$environment_home"

combined_home="$test_root/combined-home"
combined_bin="$test_root/combined-bin"
mkdir -p "$combined_home"
make_prerequisite_bin "$combined_bin" cmp
rm -f "$combined_bin/uname" "$combined_bin/sudo"
cat > "$combined_bin/uname" <<'SH'
#!/bin/sh
printf '%s\n' i686
SH
cat > "$combined_bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
printf '%s\n' '#!/bin/sh' 'exit 99' > "$combined_bin/apt-get"
chmod 755 "$combined_bin/uname" "$combined_bin/sudo" "$combined_bin/apt-get"
if printf 'y\n' | script -qefc "env HOME='$combined_home' PATH='$combined_bin' /bin/sh '$repository_root/install.sh'" /dev/null >"$test_root/combined.out" 2>&1; then
    printf 'Installer accepted combined package and environment failures.\n' >&2
    exit 1
fi
for expected in \
    'Missing required commands: cmp' \
    'Missing Linux packages: diffutils' \
    'Unsupported Linux architecture: i686' \
    'Install manually as root: apt-get update && apt-get install -y diffutils'; do
    if ! grep -F "$expected" "$test_root/combined.out" >/dev/null; then
        printf 'Combined Stage 1 report is missing %s.\n' "$expected" >&2
        cat "$test_root/combined.out" >&2
        exit 1
    fi
done
if grep -F 'Install missing packages?' "$test_root/combined.out" >/dev/null; then
    printf 'Combined non-package failure prompted for package installation.\n' >&2
    exit 1
fi
assert_no_toolbox_files "$combined_home"

missing_home="$test_root/missing-home-target"
if HOME='' PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/missing-home.out" 2>&1; then
    printf 'Installer accepted an empty HOME.\n' >&2
    exit 1
fi
if ! grep -F 'HOME is not set to an absolute path.' "$test_root/missing-home.out" >/dev/null ||
    ! grep -F '[FAIL] Stage 1/7: prerequisites' "$test_root/missing-home.out" >/dev/null; then
    printf 'Empty HOME failure was not an actionable Stage 1 report.\n' >&2
    cat "$test_root/missing-home.out" >&2
    exit 1
fi
assert_no_toolbox_files "$missing_home"

bad_tmp_home="$test_root/bad-tmp-home"
mkdir -p "$bad_tmp_home"
if HOME="$bad_tmp_home" TMPDIR="$test_root/does-not-exist" PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/bad-tmp.out" 2>&1; then
    printf 'Installer accepted an unusable temporary directory.\n' >&2
    exit 1
fi
if ! grep -F "Cannot create a temporary directory under $test_root/does-not-exist." "$test_root/bad-tmp.out" >/dev/null; then
    printf 'Temporary-directory failure was not actionable.\n' >&2
    cat "$test_root/bad-tmp.out" >&2
    exit 1
fi
assert_no_toolbox_files "$bad_tmp_home"

readonly_home="$test_root/readonly-home"
mkdir -p "$readonly_home"
chmod 555 "$readonly_home"
if HOME="$readonly_home" ZDOTDIR="$readonly_home" PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/readonly.out" 2>&1; then
    chmod 755 "$readonly_home"
    printf 'Installer accepted unwritable installation paths.\n' >&2
    exit 1
fi
chmod 755 "$readonly_home"
for expected in \
    "Toolbox data path is not writable: $readonly_home/.local/share/my-toolbox" \
    "Toolbox wrapper path is not writable: $readonly_home/.local/bin"; do
    if ! grep -F "$expected" "$test_root/readonly.out" >/dev/null; then
        printf 'Installation-path failure is missing %s.\n' "$expected" >&2
        cat "$test_root/readonly.out" >&2
        exit 1
    fi
done
assert_no_toolbox_files "$readonly_home"

unsearchable_home="$test_root/unsearchable-home"
mkdir -p "$unsearchable_home"
chmod 222 "$unsearchable_home"
if HOME="$unsearchable_home" ZDOTDIR="$unsearchable_home" PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/unsearchable.out" 2>&1; then
    chmod 755 "$unsearchable_home"
    printf 'Installer accepted a non-searchable installation path.\n' >&2
    exit 1
fi
chmod 755 "$unsearchable_home"
if ! grep -F "Toolbox data path is not writable: $unsearchable_home/.local/share/my-toolbox" "$test_root/unsearchable.out" >/dev/null; then
    printf 'Non-searchable installation-path failure was not actionable.\n' >&2
    cat "$test_root/unsearchable.out" >&2
    exit 1
fi
assert_no_toolbox_files "$unsearchable_home"

bash_parent_home="$test_root/bash-parent-home"
bash_parent_data="$test_root/bash-parent-data"
mkdir -p "$bash_parent_home/.local/bin" "$bash_parent_data"
chmod 555 "$bash_parent_home"
if HOME="$bash_parent_home" XDG_DATA_HOME="$bash_parent_data" ZDOTDIR="$bash_parent_data" PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/bash-parent.out" 2>&1; then
    chmod 755 "$bash_parent_home"
    printf 'Installer accepted an unwritable Bash profile parent.\n' >&2
    exit 1
fi
chmod 755 "$bash_parent_home"
if ! grep -F "Bash profile path is not writable: $bash_parent_home" "$test_root/bash-parent.out" >/dev/null; then
    printf 'Bash profile parent failure was not actionable.\n' >&2
    cat "$test_root/bash-parent.out" >&2
    exit 1
fi
if [ -e "$bash_parent_data/my-toolbox" ] || [ -e "$bash_parent_home/.local/bin/tb" ]; then
    printf 'Bash profile parent failure created toolbox files.\n' >&2
    exit 1
fi

nested_path_home="$test_root/nested-path-home"
mkdir -p "$nested_path_home/.local/share/my-toolbox/versions"
chmod 555 "$nested_path_home/.local/share/my-toolbox/versions"
if HOME="$nested_path_home" ZDOTDIR="$nested_path_home" PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/nested-path.out" 2>&1; then
    chmod 755 "$nested_path_home/.local/share/my-toolbox/versions"
    printf 'Installer accepted an unwritable existing versions directory.\n' >&2
    exit 1
fi
chmod 755 "$nested_path_home/.local/share/my-toolbox/versions"
if ! grep -F "Toolbox versions path is not writable: $nested_path_home/.local/share/my-toolbox/versions" "$test_root/nested-path.out" >/dev/null; then
    printf 'Nested versions-path failure was not actionable.\n' >&2
    cat "$test_root/nested-path.out" >&2
    exit 1
fi
if [ -e "$nested_path_home/.local/share/my-toolbox/current.txt" ] || [ -e "$nested_path_home/.local/bin/tb" ]; then
    printf 'Nested versions-path failure created toolbox publication files.\n' >&2
    exit 1
fi

leaf_conflict_home="$test_root/leaf-conflict-home"
mkdir -p "$leaf_conflict_home/.local/share/my-toolbox/current.txt" "$leaf_conflict_home/.local/bin/tb" "$leaf_conflict_home/.bashrc" "$leaf_conflict_home/.zshrc"
if HOME="$leaf_conflict_home" ZDOTDIR="$leaf_conflict_home" PATH="$environment_bin" /bin/sh "$repository_root/install.sh" >"$test_root/leaf-conflict.out" 2>&1; then
    printf 'Installer accepted conflicting managed leaf targets.\n' >&2
    exit 1
fi
for expected in \
    "Toolbox current file has unsupported type: $leaf_conflict_home/.local/share/my-toolbox/current.txt" \
    "Toolbox wrapper has unsupported type: $leaf_conflict_home/.local/bin/tb" \
    "Bash profile has unsupported type: $leaf_conflict_home/.bashrc" \
    "Zsh profile has unsupported type: $leaf_conflict_home/.zshrc"; do
    if ! grep -F "$expected" "$test_root/leaf-conflict.out" >/dev/null; then
        printf 'Managed leaf report is missing %s.\n' "$expected" >&2
        cat "$test_root/leaf-conflict.out" >&2
        exit 1
    fi
done

no_python_home="$test_root/no-python-home"
no_python_bin="$test_root/no-python-bin"
mkdir -p "$no_python_home"
make_prerequisite_bin "$no_python_bin" python3
rm -f "$no_python_bin/curl" "$no_python_bin/uname" "$no_python_bin/mv"
ln -s "$test_root/bin/curl" "$no_python_bin/curl"
ln -s "$test_root/bin/uname" "$no_python_bin/uname"
ln -s "$test_root/bin/mv" "$no_python_bin/mv"
if ! HOME="$no_python_home" ZDOTDIR="$no_python_home" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$no_python_bin" /bin/sh "$repository_root/install.sh" >"$test_root/no-python.out" 2>&1; then
    printf 'Installer rejected an environment without Python.\n' >&2
    cat "$test_root/no-python.out" >&2
    exit 1
fi
if grep -Ei 'Missing.*Python|apt-get install.*python|Install missing packages.*(bash|zsh|sudo)' "$test_root/no-python.out" >/dev/null; then
    printf 'Bootstrap proposed a tool-specific or optional dependency.\n' >&2
    cat "$test_root/no-python.out" >&2
    exit 1
fi
no_python_wrapper="$no_python_home/.local/bin/tb"
if grep -F sed "$no_python_wrapper" >/dev/null ||
    ! HOME="$no_python_home" XDG_DATA_HOME='' PATH="$test_root/empty-path" "$no_python_wrapper" version >/dev/null; then
    printf 'Installed Linux wrapper retained a sed dependency or failed without PATH utilities.\n' >&2
    exit 1
fi

for shell_case in bash-only zsh-only neither; do
    shell_home="$test_root/$shell_case-home"
    shell_bin="$test_root/$shell_case-bin"
    mkdir -p "$shell_home"
    case "$shell_case" in
        bash-only) make_prerequisite_bin "$shell_bin" zsh python3 ;;
        zsh-only) make_prerequisite_bin "$shell_bin" bash python3 ;;
        neither) make_prerequisite_bin "$shell_bin" bash zsh python3 ;;
    esac
    rm -f "$shell_bin/curl" "$shell_bin/uname" "$shell_bin/mv"
    ln -s "$test_root/bin/curl" "$shell_bin/curl"
    ln -s "$test_root/bin/uname" "$shell_bin/uname"
    ln -s "$test_root/bin/mv" "$shell_bin/mv"
    if ! HOME="$shell_home" ZDOTDIR="$shell_home" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$shell_bin" /bin/sh "$repository_root/install.sh" >"$test_root/$shell_case.out" 2>&1; then
        printf 'Installer rejected %s shell availability.\n' "$shell_case" >&2
        cat "$test_root/$shell_case.out" >&2
        exit 1
    fi
    [ -f "$shell_home/.local/share/my-toolbox/current.txt" ] || { printf '%s did not install toolbox.\n' "$shell_case" >&2; exit 1; }
    [ -f "$shell_home/.local/share/my-toolbox/completions/tb.bash" ] || { printf '%s did not publish completions.\n' "$shell_case" >&2; exit 1; }
    case "$shell_case" in
        bash-only) [ -f "$shell_home/.bashrc" ] && [ ! -e "$shell_home/.zshrc" ] ;;
        zsh-only) [ ! -e "$shell_home/.bashrc" ] && [ -f "$shell_home/.zshrc" ] ;;
        neither) [ ! -e "$shell_home/.bashrc" ] && [ ! -e "$shell_home/.zshrc" ] ;;
    esac || { printf '%s modified the wrong shell profiles.\n' "$shell_case" >&2; exit 1; }
done

mkdir -p "$test_root/home/zsh"
printf '%s' 'bash unrelated' > "$test_root/home/.bashrc"
printf '%s\n' 'zsh unrelated' > "$test_root/home/zsh/.zshrc"
output=$(HOME="$test_root/home" ZDOTDIR="$test_root/home/zsh" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh")
expected_banner=$(cat <<'EOF'
███╗   ███╗██╗   ██╗    ████████╗ ██████╗  ██████╗ ██╗     ██████╗  ██████╗ ██╗  ██╗
████╗ ████║╚██╗ ██╔╝    ╚══██╔══╝██╔═══██╗██╔═══██╗██║     ██╔══██╗██╔═══██╗╚██╗██╔╝
██╔████╔██║ ╚████╔╝        ██║   ██║   ██║██║   ██║██║     ██████╔╝██║   ██║ ╚███╔╝
██║╚██╔╝██║  ╚██╔╝         ██║   ██║   ██║██║   ██║██║     ██╔══██╗██║   ██║ ██╔██╗
██║ ╚═╝ ██║   ██║          ██║   ╚██████╔╝╚██████╔╝███████╗██████╔╝╚██████╔╝██╔╝ ██╗
╚═╝     ╚═╝   ╚═╝          ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝╚═════╝  ╚═════╝ ╚═╝  ╚═╝
EOF
)
actual_banner=$(printf '%s\n' "$output" | sed -n '1,6p')
if [ "$actual_banner" != "$expected_banner" ]; then
    printf 'Installer output has the wrong banner.\nExpected:\n%s\nActual:\n%s\n' "$expected_banner" "$actual_banner" >&2
    exit 1
fi
for text in \
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
for completion in _tb tb.bash tb.ps1; do
    if ! cmp -s "$repository_root/completions/$completion" "$test_root/home/.local/share/my-toolbox/completions/$completion"; then
        printf 'Installer did not publish completion asset %s.\n' "$completion" >&2
        exit 1
    fi
done
{
    printf '%s\n' 'bash unrelated'
    printf '%s\n' '# >>> my-toolbox completion >>>'
    printf ". '%s'\n" "$test_root/home/.local/share/my-toolbox/completions/tb.bash"
    printf '%s\n' '# <<< my-toolbox completion <<<'
} > "$test_root/expected-bashrc"
{
    printf '%s\n\n' 'zsh unrelated'
    printf '%s\n' '# >>> my-toolbox completion >>>'
    printf "source '%s'\n" "$test_root/home/.local/share/my-toolbox/completions/_tb"
    printf '%s\n' '# <<< my-toolbox completion <<<'
} > "$test_root/expected-zshrc"
if ! cmp -s "$test_root/expected-bashrc" "$test_root/home/.bashrc"; then
    printf 'Installer did not preserve and activate the Bash profile exactly.\n' >&2
    exit 1
fi
if ! cmp -s "$test_root/expected-zshrc" "$test_root/home/zsh/.zshrc"; then
    printf 'Installer did not preserve and activate the Zsh profile exactly.\n' >&2
    exit 1
fi
rm -rf "$test_root/home/.local/share/my-toolbox/completions"
printf '%s' 'bash unrelated' > "$test_root/home/.bashrc"
printf '%s\n' 'zsh unrelated' > "$test_root/home/zsh/.zshrc"
HOME="$test_root/home" ZDOTDIR="$test_root/home/zsh" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >/dev/null
if ! cmp -s "$test_root/expected-bashrc" "$test_root/home/.bashrc" || ! cmp -s "$test_root/expected-zshrc" "$test_root/home/zsh/.zshrc"; then
    printf 'Existing installation did not repair shell completion activation.\n' >&2
    exit 1
fi
for completion in _tb tb.bash tb.ps1; do
    if ! cmp -s "$repository_root/completions/$completion" "$test_root/home/.local/share/my-toolbox/completions/$completion"; then
        printf 'Existing installation did not repair completion asset %s.\n' "$completion" >&2
        exit 1
    fi
done
HOME="$test_root/home" ZDOTDIR="$test_root/home/zsh" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >/dev/null
for profile in "$test_root/home/.bashrc" "$test_root/home/zsh/.zshrc"; do
    if [ "$(grep -Fxc '# >>> my-toolbox completion >>>' "$profile")" -ne 1 ] || [ "$(grep -Fxc '# <<< my-toolbox completion <<<' "$profile")" -ne 1 ]; then
        printf 'Installer duplicated a managed completion block in %s.\n' "$profile" >&2
        exit 1
    fi
done
if find "$test_root/tmp" -mindepth 1 -print | grep . >/dev/null; then
    printf 'Installer left a temporary directory behind.\n' >&2
    exit 1
fi

mkdir -p "$test_root/missing-profiles-home"
HOME="$test_root/missing-profiles-home" ZDOTDIR="$test_root/missing-profiles-home/config/zsh" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >/dev/null
if [ ! -f "$test_root/missing-profiles-home/.bashrc" ] || [ ! -f "$test_root/missing-profiles-home/config/zsh/.zshrc" ]; then
    printf 'Installer did not create missing Bash and Zsh profiles.\n' >&2
    exit 1
fi
for profile in "$test_root/missing-profiles-home/.bashrc" "$test_root/missing-profiles-home/config/zsh/.zshrc"; do
    if [ "$(grep -Fxc '# >>> my-toolbox completion >>>' "$profile")" -ne 1 ] || [ "$(grep -Fxc '# <<< my-toolbox completion <<<' "$profile")" -ne 1 ]; then
        printf 'Installer did not activate completion in new profile %s.\n' "$profile" >&2
        exit 1
    fi
done

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
printf '%s' 'original bash bytes' > "$test_root/activation-failure-home/.bashrc"
if HOME="$test_root/activation-failure-home" ZDOTDIR="$test_root/activation-failure-home/config/zsh" TMPDIR="$test_root/tmp" FAIL_CURRENT_MOVE=1 FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >"$test_root/activation-failure.out" 2>&1; then
    printf 'Installer accepted an activation publication failure.\n' >&2
    exit 1
fi
if ! grep -F '[FAIL] Stage 7/7: activation' "$test_root/activation-failure.out" >/dev/null; then
    printf 'Activation failure did not identify its active stage.\n' >&2
    exit 1
fi
if [ -e "$test_root/activation-failure-home/.local/share/my-toolbox/versions/0.1.5" ] ||
    [ -e "$test_root/activation-failure-home/.local/bin/tb" ] ||
    [ -e "$test_root/activation-failure-home/.local/share/my-toolbox/current.txt" ] ||
    [ -e "$test_root/activation-failure-home/.local/share/my-toolbox/completions" ]; then
    printf 'Activation failure left a partial installation behind.\n' >&2
    exit 1
fi
if [ "$(cat "$test_root/activation-failure-home/.bashrc")" != 'original bash bytes' ] ||
    [ -e "$test_root/activation-failure-home/config" ]; then
    printf 'Activation failure did not restore the original shell profiles.\n' >&2
    exit 1
fi
if find "$test_root/tmp" -mindepth 1 -print | grep . >/dev/null; then
    printf 'Activation failure left a temporary directory behind.\n' >&2
    exit 1
fi

mkdir -p "$test_root/malformed-home/zsh"
printf '%s\n' 'unrelated' '# >>> my-toolbox completion >>>' > "$test_root/malformed-home/.bashrc"
printf '%s\n' 'zsh unrelated' > "$test_root/malformed-home/zsh/.zshrc"
if HOME="$test_root/malformed-home" ZDOTDIR="$test_root/malformed-home/zsh" TMPDIR="$test_root/tmp" FIXTURE_DOWNLOADS="$test_root/downloads" PATH="$test_root/bin:/usr/bin:/bin" sh "$repository_root/install.sh" >"$test_root/malformed.out" 2>&1; then
    printf 'Installer accepted malformed completion markers.\n' >&2
    exit 1
fi
if ! grep -F 'Malformed my-toolbox completion markers' "$test_root/malformed.out" >/dev/null; then
    printf 'Malformed completion markers did not fail explicitly.\n' >&2
    exit 1
fi
if [ "$(cat "$test_root/malformed-home/.bashrc")" != "unrelated
# >>> my-toolbox completion >>>" ]; then
    printf 'Malformed-marker failure changed the Bash profile.\n' >&2
    exit 1
fi
if [ -e "$test_root/malformed-home/.local/share/my-toolbox/versions/0.1.5" ] ||
    [ -e "$test_root/malformed-home/.local/bin/tb" ] ||
    [ -e "$test_root/malformed-home/.local/share/my-toolbox/completions" ]; then
    printf 'Malformed-marker failure left a partial installation behind.\n' >&2
    exit 1
fi
