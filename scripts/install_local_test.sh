#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM
home_root=$test_root/home
data_root=$test_root/data/my-toolbox
mkdir -p "$home_root" "$data_root/versions/1.9.4" "$test_root/bin"
cat > "$test_root/fake-tb" <<'SH'
#!/bin/sh
case "$1" in
    help) exit 0 ;;
    version) cat "$(dirname "$0")/version.txt" ;;
    *) exit 1 ;;
esac
SH
cat > "$test_root/bin/go" <<'SH'
#!/bin/sh
while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then cp "$TB_TEST_BINARY" "$2"; exit 0; fi
    shift
done
exit 1
SH
chmod 755 "$test_root/fake-tb" "$test_root/bin/go"
printf '%s\n' '1.9.4' > "$data_root/current.txt"
# The fixture must evaluate its own first argument.
# shellcheck disable=SC2016
printf '#!/bin/sh\n[ "$1" = --tb-self-check ]\n' > "$test_root/reader"
chmod 755 "$test_root/reader"
printf '#!/bin/sh\nexit 0\n' > "$data_root/versions/1.9.4/tb"
chmod 755 "$data_root/versions/1.9.4/tb"

HOME="$home_root" XDG_DATA_HOME="$test_root/data" PATH="$test_root/bin:$PATH" TB_TEST_BINARY="$test_root/fake-tb" TOOLBOX_READER="$test_root/reader" "$repo_root/install-local.sh" > "$test_root/output"
version=$(cat "$data_root/current.txt")
case "$version" in 1.9.4-local.*) ;; *) printf 'Unexpected local version: %s\n' "$version" >&2; exit 1 ;; esac
test -x "$data_root/versions/$version/tb"
test -f "$data_root/versions/$version/commands.json"
test -f "$data_root/versions/$version/packages/toolbox_output.py"
test -x "$home_root/.local/bin/tb"
test "$(HOME="$home_root" XDG_DATA_HOME="$test_root/data" "$home_root/.local/bin/tb" version)" = "$version"
test "$(grep -c ' SUCCESS]' "$test_root/output")" -eq 1
# The guide must print literal shell variables for the user to copy.
# shellcheck disable=SC2016
for guidance in 'If tb command is not recognized, do:' 'export PATH="$HOME/.local/bin:$PATH"' '>> "$HOME/.bashrc"' '>> "$HOME/.zshrc"' '>> "$HOME/.profile"'; do
    grep -F "$guidance" "$test_root/output" >/dev/null
done

fresh_home=$test_root/fresh-home
mkdir -p "$fresh_home"
HOME="$fresh_home" XDG_DATA_HOME="$test_root/fresh-data" PATH="$test_root/bin:$PATH" TB_TEST_BINARY="$test_root/fake-tb" TOOLBOX_READER="$test_root/reader" "$repo_root/install-local.sh" > "$test_root/fresh-output"
fresh_version=$(cat "$test_root/fresh-data/my-toolbox/current.txt")
case "$fresh_version" in 0.0.0-local.*) ;; *) printf 'Unexpected fresh version: %s\n' "$fresh_version" >&2; exit 1 ;; esac
test "$(HOME="$fresh_home" XDG_DATA_HOME="$test_root/fresh-data" "$fresh_home/.local/bin/tb" version)" = "$fresh_version"
test "$(grep -c ' SUCCESS]' "$test_root/fresh-output")" -eq 1
grep -F 'If tb command is not recognized, do:' "$test_root/fresh-output" >/dev/null
printf 'local installer Linux test passed\n'
