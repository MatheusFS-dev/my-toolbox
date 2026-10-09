#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
script="$root/packages/scripts/utils/isolate_gpu.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/bin" "$tmp/root/etc/X11/xorg.conf.d" "$tmp/root/var/lib/my-toolbox"
cat > "$tmp/os-release" <<'EOF'
ID=ubuntu
VERSION_ID="24.04"
EOF

cat > "$tmp/bin/nvidia-smi" <<'EOF'
#!/usr/bin/env bash
case "$*" in
    *--query-gpu=index,uuid,pci.bus_id,name*)
        printf '%s\n' \
            '0, GPU-desktop, 00000000:01:00.0, NVIDIA RTX Desktop' \
            '1, GPU-worker, 00000000:02:00.0, NVIDIA RTX Worker'
        ;;
    *pmon*)
        printf '%s\n' '# gpu pid type sm mem enc dec command' '0 4242 G - - - - Xorg'
        ;;
    *) exit 2 ;;
esac
EOF
cat > "$tmp/bin/pgrep" <<'EOF'
#!/usr/bin/env bash
[[ "$*" == *Xorg* ]] && printf '4242\n'
EOF
cat > "$tmp/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
[[ "$1" == is-active && ( "$2" == gdm3 || "$2" == gdm ) ]]
EOF
chmod +x "$tmp/bin/nvidia-smi" "$tmp/bin/pgrep" "$tmp/bin/systemctl"

export PATH="$tmp/bin:$PATH"
export GPU_ISOLATION_OS_RELEASE="$tmp/os-release"
export GPU_ISOLATION_XORG_CONFIG="$tmp/root/etc/X11/xorg.conf.d/90-tb-isolate-gpu.conf"
export GPU_ISOLATION_STATE="$tmp/root/var/lib/my-toolbox/isolate-gpu.sha256"
export GPU_ISOLATION_XORG_ROOT="$tmp/root/etc/X11"

bash "$script" status > "$tmp/status"
grep -Fq 'GPU 0: NVIDIA RTX Desktop [desktop-active]' "$tmp/status"
grep -Fq 'GPU 1: NVIDIA RTX Worker [available]' "$tmp/status"

bash "$script" apply --dry-run > "$tmp/apply"
grep -Fq 'Section "Device"' "$tmp/apply"
grep -Fq 'BusID "PCI:1:0:0"' "$tmp/apply"
grep -Fq 'Option "AutoAddGPU" "false"' "$tmp/apply"
grep -Fq 'Option "AutoBindGPU" "false"' "$tmp/apply"
grep -Fq 'Option "AllowNVIDIAGPUScreens" "false"' "$tmp/apply"
grep -Fq 'tb isolate-gpu undo' "$tmp/apply"
if grep -Fq 'Section "Screen"' "$tmp/apply" ||
   [[ $(grep -Fc 'Section "Device"' "$tmp/apply") -ne 1 ]]; then
    printf 'Dry-run configuration was not anchor-only.\n' >&2
    exit 1
fi
test ! -e "$GPU_ISOLATION_XORG_CONFIG"
test ! -e "$GPU_ISOLATION_STATE"

bash "$script" apply <<< 'y' > "$tmp/applied"
test -f "$GPU_ISOLATION_XORG_CONFIG"
test -f "$GPU_ISOLATION_STATE"
bash "$script" undo --dry-run > "$tmp/undo"
grep -Fq "Would remove $GPU_ISOLATION_XORG_CONFIG" "$tmp/undo"
test -f "$GPU_ISOLATION_XORG_CONFIG"

printf 'y\n' | bash "$script" undo > "$tmp/undo-real"
test ! -e "$GPU_ISOLATION_XORG_CONFIG"
test ! -e "$GPU_ISOLATION_STATE"

sed -i 's/00000000:01:00.0/00000001:01:00.0/' "$tmp/bin/nvidia-smi"
bash "$script" apply --dry-run > "$tmp/domain"
grep -Fq 'BusID "PCI:1@1:0:0"' "$tmp/domain"
sed -i 's/00000001:01:00.0/00000000:01:00.0/' "$tmp/bin/nvidia-smi"

ln -s "$tmp/missing" "$GPU_ISOLATION_XORG_CONFIG"
if bash "$script" apply --dry-run > "$tmp/error" 2>&1; then
    printf 'Apply accepted a dangling managed-path symlink.\n' >&2
    exit 1
fi
grep -Fq 'symbolic link' "$tmp/error"
rm "$GPU_ISOLATION_XORG_CONFIG"

original_config=$GPU_ISOLATION_XORG_CONFIG
mkdir -p "$tmp/root/linked-xorg-target"
ln -s "$tmp/root/linked-xorg-target" "$tmp/root/linked-xorg-parent"
export GPU_ISOLATION_XORG_CONFIG="$tmp/root/linked-xorg-parent/90-tb-isolate-gpu.conf"
if bash "$script" apply --dry-run > "$tmp/error" 2>&1; then
    printf 'Apply accepted a symbolic-link parent directory.\n' >&2
    exit 1
fi
grep -Fq 'managed parent is a symbolic link' "$tmp/error"
export GPU_ISOLATION_XORG_CONFIG=$original_config

cat > "$GPU_ISOLATION_XORG_ROOT/xorg.conf.d/20-existing-gpu.conf" <<'EOF'
Section "ServerFlags"
    Option "AutoAddGPU" "true"
EndSection
EOF
if bash "$script" apply --dry-run > "$tmp/error" 2>&1; then
    printf 'Apply accepted a conflicting Xorg GPU configuration.\n' >&2
    exit 1
fi
grep -Fq 'existing Xorg GPU configuration may conflict' "$tmp/error"
rm "$GPU_ISOLATION_XORG_ROOT/xorg.conf.d/20-existing-gpu.conf"

real_install=$(command -v install)
cat > "$tmp/bin/install" <<EOF
#!/usr/bin/env bash
if [[ "\${*: -1}" == "$GPU_ISOLATION_STATE" ]]; then exit 73; fi
exec "$real_install" "\$@"
EOF
chmod +x "$tmp/bin/install"
if printf 'y\n' | bash "$script" apply > "$tmp/error" 2>&1; then
    printf 'Apply accepted a failed checksum-state installation.\n' >&2
    exit 1
fi
test ! -e "$GPU_ISOLATION_XORG_CONFIG"
test ! -e "$GPU_ISOLATION_STATE"
rm "$tmp/bin/install"

bash "$script" apply <<< 'y' > "$tmp/applied"

printf '\n# changed\n' >> "$GPU_ISOLATION_XORG_CONFIG"
if bash "$script" undo --dry-run > "$tmp/error" 2>&1; then
    printf 'Undo accepted a modified managed configuration.\n' >&2
    exit 1
fi
grep -Fq 'checksum' "$tmp/error"

rm "$GPU_ISOLATION_XORG_CONFIG" "$GPU_ISOLATION_STATE"
cat > "$tmp/bin/nvidia-smi" <<'EOF'
#!/usr/bin/env bash
case "$*" in
    *--query-gpu=index,uuid,pci.bus_id,name*)
        printf '%s\n' '2, GPU-desktop, 00000000:01:00.0, NVIDIA RTX Desktop' 'driver query failed'
        exit 1
        ;;
    *pmon*) printf '%s\n' '# gpu pid type sm mem enc dec command' '2 4242 G - - - - Xorg' ;;
    *) exit 2 ;;
esac
EOF
chmod +x "$tmp/bin/nvidia-smi"
if bash "$script" status > "$tmp/error" 2>&1; then
    printf 'A failed NVIDIA inventory query was accepted.\n' >&2
    exit 1
fi
grep -Fq 'query NVIDIA GPU inventory' "$tmp/error"

cat > "$tmp/bin/nvidia-smi" <<'EOF'
#!/usr/bin/env bash
case "$*" in
    *--query-gpu=index,uuid,pci.bus_id,name*)
        printf '%s\n' \
            '2, GPU-desktop, 00000000:01:00.0, NVIDIA RTX Desktop' \
            '5, GPU-worker, 00000000:02:00.0, NVIDIA RTX Worker'
        ;;
    *pmon*) printf '%s\n' '# gpu pid type sm mem enc dec command' '2 4242 G - - - - Xorg' ;;
    *) exit 2 ;;
esac
EOF
chmod +x "$tmp/bin/nvidia-smi"
bash "$script" apply --dry-run > "$tmp/sparse"
grep -Fq 'BusID "PCI:1:0:0"' "$tmp/sparse"

sed -i 's/24\.04/25.04/' "$GPU_ISOLATION_OS_RELEASE"
if bash "$script" status > "$tmp/error" 2>&1; then
    printf 'Unsupported Ubuntu version was accepted.\n' >&2
    exit 1
fi
grep -Fq 'Ubuntu 24.04 or 26.04' "$tmp/error"

sed -i 's/25\.04/24.04/' "$GPU_ISOLATION_OS_RELEASE"
cat > "$tmp/bin/nvidia-smi" <<'EOF'
#!/usr/bin/env bash
case "$*" in
    *--query-gpu=index,uuid,pci.bus_id,name*) printf '%s\n' '0, GPU-only, 00000000:01:00.0, NVIDIA RTX Only' ;;
    *pmon*) printf '%s\n' '# gpu pid type sm mem enc dec command' '0 4242 G - - - - Xorg' ;;
    *) exit 2 ;;
esac
EOF
chmod +x "$tmp/bin/nvidia-smi"
if bash "$script" apply --dry-run > "$tmp/error" 2>&1; then
    printf 'Single-GPU configuration was accepted.\n' >&2
    exit 1
fi
grep -Fq 'at least two NVIDIA GPUs' "$tmp/error"

echo 'gpu isolation tests passed'
