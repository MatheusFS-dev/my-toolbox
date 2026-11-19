#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/packages/scripts/utils" "$test_root/packages/scripts/lib" \
    "$test_root/sudoers.d" "$test_root/bin"
cp "$repository_root/assets/payload/packages/scripts/lib/toolbox_output.sh" "$test_root/packages/scripts/lib/"
sed "s#/etc/sudoers#$test_root/sudoers#g" \
    "$repository_root/assets/payload/packages/scripts/utils/toggle_nopasswd_sudo.sh" \
    > "$test_root/packages/scripts/utils/toggle_nopasswd_sudo.sh"
printf 'root ALL=(ALL:ALL) ALL\n@includedir %s/sudoers.d\n%%sudo ALL=(ALL:ALL) ALL\n' \
    "$test_root" > "$test_root/sudoers"
chmod 440 "$test_root/sudoers"

cat > "$test_root/bin/id" <<'SH'
#!/bin/sh
if [ "$1" = -u ] && [ "$#" -eq 1 ]; then printf '0\n'; exit 0; fi
if [ "$1" = -u ]; then printf '1000\n'; exit 0; fi
exit 0
SH
cat > "$test_root/bin/sudo" <<'SH'
#!/bin/sh
grep -Fqx "@include $TEST_ROOT/sudoers.d/zz-tb-uid-1000-nopasswd" \
    "$TEST_ROOT/sudoers" && \
    test -f "$TEST_ROOT/sudoers.d/zz-tb-uid-1000-nopasswd"
SH
# The real visudo cannot validate a sudoers tree that is not owned by root, so
# this stub only enforces the sudo-rs rule that an unquoted @include path must
# be the last thing on its line.
cat > "$test_root/bin/visudo" <<'SH'
#!/bin/sh
if grep -Eq '^@include(dir)?[[:space:]]+[^"[:space:]]+[[:space:]]' "$2"; then
    printf 'visudo stub: text after an unquoted include path\n' >&2
    exit 1
fi
exit 0
SH
cat > "$test_root/bin/install" <<'SH'
#!/bin/sh
source_file=
for destination do
    if [ "$destination" = "$TEST_ROOT/sudoers.d/zz-tb-uid-1000-nopasswd" ]; then
        cp "$source_file" "$destination"
        exit $?
    fi
    source_file=$destination
done
exit 1
SH
chmod +x "$test_root/bin/"*

TEST_ROOT="$test_root" PATH="$test_root/bin:$PATH" \
    bash "$test_root/packages/scripts/utils/toggle_nopasswd_sudo.sh" testuser -y
grep -Fqx "@include $test_root/sudoers.d/zz-tb-uid-1000-nopasswd" \
    "$test_root/sudoers"
TEST_ROOT="$test_root" PATH="$test_root/bin:$PATH" \
    bash "$test_root/packages/scripts/utils/toggle_nopasswd_sudo.sh" testuser -y
if grep -Fq 'zz-tb-uid-1000-nopasswd' "$test_root/sudoers"; then
    printf 'Toolbox include remained after disabling passwordless sudo.\n' >&2
    exit 1
fi

# Include lines written by older versions carry a trailing comment and must
# still be removed.
chmod 640 "$test_root/sudoers"
printf '@include %s/sudoers.d/zz-tb-uid-1000-nopasswd # tb-nopasswd-uid-1000\n' \
    "$test_root" >> "$test_root/sudoers"
TEST_ROOT="$test_root" PATH="$test_root/bin:$PATH" \
    bash "$test_root/packages/scripts/utils/toggle_nopasswd_sudo.sh" testuser -y
if grep -Fq 'tb-nopasswd-uid-1000' "$test_root/sudoers"; then
    printf 'Legacy toolbox include remained after disabling passwordless sudo.\n' >&2
    exit 1
fi
