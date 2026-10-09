#!/usr/bin/env bash

set -Eeuo pipefail

readonly DEFAULT_OS_RELEASE=/etc/os-release
readonly DEFAULT_XORG_CONFIG=/etc/X11/xorg.conf.d/90-tb-isolate-gpu.conf
readonly DEFAULT_STATE=/var/lib/my-toolbox/isolate-gpu.sha256
readonly EXIT_USAGE=64
declare -a GPU_ROWS=()
declare -A GPU_BY_INDEX=()
declare -a TEMPORARY_FILES=()

cleanup_temporaries() {
    local path
    for path in "${TEMPORARY_FILES[@]}"; do
        [[ -n "$path" ]] && rm -f -- "$path"
    done
}
trap cleanup_temporaries EXIT

usage() {
    cat <<'EOF'
Usage: tb isolate-gpu [status|apply|undo] [--dry-run]

Commands:
  status  Show which NVIDIA GPU is used by the active Xorg desktop.
  apply   Anchor Xorg to the desktop GPU, leaving other GPUs available.
  undo    Remove an intact configuration previously created by this tool.

Options:
  --dry-run  Show the intended change without writing or removing files.
EOF
}

os_release_path() {
    printf '%s\n' "${GPU_ISOLATION_OS_RELEASE:-$DEFAULT_OS_RELEASE}"
}

xorg_config_path() {
    printf '%s\n' "${GPU_ISOLATION_XORG_CONFIG:-$DEFAULT_XORG_CONFIG}"
}

state_path() {
    printf '%s\n' "${GPU_ISOLATION_STATE:-$DEFAULT_STATE}"
}

run_privileged() {
    if [[ -n "${GPU_ISOLATION_OS_RELEASE:-}${GPU_ISOLATION_XORG_CONFIG:-}${GPU_ISOLATION_STATE:-}" ]]; then
        "$@"
    elif [[ $EUID -eq 0 ]]; then
        "$@"
    else
        sudo -- "$@"
    fi
}

validate_platform() {
    local release_file id='' version=''
    release_file=$(os_release_path)
    if [[ ! -r "$release_file" ]]; then
        printf 'Error: cannot read operating-system metadata: %s\n' "$release_file" >&2
        return 1
    fi
    while IFS='=' read -r key value; do
        value=${value%\"}
        value=${value#\"}
        case "$key" in
            ID) id=$value ;;
            VERSION_ID) version=$value ;;
        esac
    done < "$release_file"
    if [[ "$id" != ubuntu || ( "$version" != 24.04 && "$version" != 26.04 ) ]]; then
        printf 'Error: tb isolate-gpu requires native Ubuntu 24.04 or 26.04.\n' >&2
        return 1
    fi
}

validate_desktop() {
    if ! systemctl is-active gdm3 >/dev/null 2>&1 && ! systemctl is-active gdm >/dev/null 2>&1; then
        printf 'Error: GDM is not active.\n' >&2
        return 1
    fi
    if ! pgrep -x Xorg >/dev/null 2>&1; then
        printf 'Error: an active Xorg process is required; Wayland sessions are not supported.\n' >&2
        return 1
    fi
}

load_gpu_inventory() {
    local output row index uuid bus name
    if ! output=$(nvidia-smi --query-gpu=index,uuid,pci.bus_id,name --format=csv,noheader,nounits); then
        printf 'Error: failed to query NVIDIA GPU inventory.\n' >&2
        return 1
    fi
    GPU_ROWS=()
    GPU_BY_INDEX=()
    while IFS= read -r row; do
        IFS=',' read -r index uuid bus name <<< "$row"
        index=$(trim "${index:-}")
        uuid=$(trim "${uuid:-}")
        bus=$(trim "${bus:-}")
        name=$(trim "${name:-}")
        if [[ ! "$index" =~ ^[0-9]+$ || -z "$uuid" || -z "$name" ||
              ! "$bus" =~ ^[[:xdigit:]]{8}:[[:xdigit:]]{2}:[[:xdigit:]]{2}\.[[:xdigit:]]$ ||
              -n "${GPU_BY_INDEX[$index]:-}" ]]; then
            printf 'Error: NVIDIA returned an invalid GPU inventory row: %s\n' "$row" >&2
            return 1
        fi
        GPU_ROWS+=("$row")
        GPU_BY_INDEX[$index]=$row
    done <<< "$output"
    if (( ${#GPU_ROWS[@]} < 2 )); then
        printf 'Error: at least two NVIDIA GPUs are required.\n' >&2
        return 1
    fi
}

load_desktop_gpu() {
    local xorg_pids pmon row gpu pid type
    xorg_pids=" $(pgrep -x Xorg | tr '\n' ' ')"
    if ! pmon=$(nvidia-smi pmon -c 1 2>/dev/null); then
        printf 'Error: failed to query NVIDIA graphics processes.\n' >&2
        return 1
    fi
    DESKTOP_GPU=''
    while read -r gpu pid type _; do
        [[ "$gpu" == \#* || -z "$gpu" ]] && continue
        if [[ "$type" == G || "$type" == C+G ]] && [[ "$xorg_pids" == *" $pid "* ]]; then
            if [[ -n "$DESKTOP_GPU" && "$DESKTOP_GPU" != "$gpu" ]]; then
                printf 'Error: Xorg is active on more than one NVIDIA GPU; refusing to choose an anchor.\n' >&2
                return 1
            fi
            DESKTOP_GPU=$gpu
        fi
    done <<< "$pmon"
    if [[ -z "$DESKTOP_GPU" ]]; then
        printf 'Error: could not identify the NVIDIA GPU used by Xorg.\n' >&2
        return 1
    fi
    row=${GPU_BY_INDEX[$DESKTOP_GPU]:-}
    if [[ -z "$row" ]]; then
        printf 'Error: Xorg reported an NVIDIA GPU outside the current inventory.\n' >&2
        return 1
    fi
}

trim() {
    local value=$1
    value=${value#"${value%%[![:space:]]*}"}
    value=${value%"${value##*[![:space:]]}"}
    printf '%s' "$value"
}

gpu_field() {
    local row=$1 field=$2 first second third fourth
    IFS=',' read -r first second third fourth <<< "$row"
    case "$field" in
        index) trim "$first" ;;
        uuid) trim "$second" ;;
        bus) trim "$third" ;;
        name) trim "$fourth" ;;
    esac
}

xorg_bus_id() {
    local address=$1 domain bus slot function
    domain=${address%%:*}
    address=${address#*:}
    bus=${address%%:*}
    address=${address#*:}
    slot=${address%%.*}
    function=${address##*.}
    if (( 16#$domain == 0 )); then
        printf 'PCI:%d:%d:%d' "$((16#$bus))" "$((16#$slot))" "$((16#$function))"
    else
        printf 'PCI:%d@%d:%d:%d' "$((16#$bus))" "$((16#$domain))" "$((16#$slot))" "$((16#$function))"
    fi
}

render_config() {
    local row bus
    row=${GPU_BY_INDEX[$DESKTOP_GPU]}
    bus=$(xorg_bus_id "$(gpu_field "$row" bus)")
    cat <<EOF
# Managed by tb isolate-gpu. Do not edit this file manually.
# To remove this configuration, run: tb isolate-gpu undo
Section "ServerFlags"
    Option "AutoAddGPU" "false"
    Option "AutoBindGPU" "false"
EndSection

Section "ServerLayout"
    Identifier "tb-isolated-layout"
    Option "AllowNVIDIAGPUScreens" "false"
EndSection

Section "Device"
    Identifier "tb-desktop-gpu"
    Driver "nvidia"
    BusID "$bus"
EndSection
EOF
}

show_status() {
    local row index name label
    printf 'NVIDIA GPU isolation status:\n'
    for row in "${GPU_ROWS[@]}"; do
        index=$(gpu_field "$row" index)
        name=$(gpu_field "$row" name)
        label=available
        [[ "$index" == "$DESKTOP_GPU" ]] && label=desktop-active
        printf 'GPU %s: %s [%s]\n' "$index" "$name" "$label"
    done
    if [[ -e "$(xorg_config_path)" ]]; then
        printf 'Managed Xorg configuration: %s\n' "$(xorg_config_path)"
    else
        printf 'Managed Xorg configuration: not installed\n'
    fi
}

managed_checksum() {
    local config state expected actual
    config=$(xorg_config_path)
    state=$(state_path)
    if [[ -L "$config" || -L "$state" || ! -f "$config" || ! -f "$state" ]]; then
        return 1
    fi
    read -r expected < "$state"
    [[ "$expected" =~ ^[[:xdigit:]]{64}$ ]] || return 1
    actual=$(sha256sum "$config")
    actual=${actual%% *}
    [[ "$expected" == "$actual" ]]
}

validate_managed_paths() {
    local target parent current component
    for target in "$(xorg_config_path)" "$(state_path)"; do
        if [[ "$target" != /* ]]; then
            printf 'Error: managed paths must be absolute: %s\n' "$target" >&2
            return 1
        fi
        if [[ -L "$target" ]]; then
            printf 'Error: managed path is a symbolic link: %s\n' "$target" >&2
            return 1
        fi
        if [[ -e "$target" && ! -f "$target" ]]; then
            printf 'Error: managed path is not a regular file: %s\n' "$target" >&2
            return 1
        fi
        parent=${target%/*}
        current=/
        IFS='/' read -r -a components <<< "${parent#/}"
        for component in "${components[@]}"; do
            [[ -z "$component" ]] && continue
            current=${current%/}/$component
            if [[ -L "$current" ]]; then
                printf 'Error: managed parent is a symbolic link: %s\n' "$current" >&2
                return 1
            fi
            if [[ -e "$current" && ! -d "$current" ]]; then
                printf 'Error: managed parent is not a directory: %s\n' "$current" >&2
                return 1
            fi
        done
    done
}

validate_xorg_conflicts() {
    local root config candidate
    root=${GPU_ISOLATION_XORG_ROOT:-/etc/X11}
    config=$(xorg_config_path)
    shopt -s nullglob
    for candidate in "$root/xorg.conf" "$root/xorg.conf.d/"*.conf; do
        [[ "$candidate" == "$config" || ! -f "$candidate" || -L "$candidate" ]] && continue
        if grep -Eiq 'Auto(Add|Bind)GPU|AllowNVIDIAGPUScreens|^[[:space:]]*BusID[[:space:]]' "$candidate"; then
            printf 'Error: existing Xorg GPU configuration may conflict: %s\n' "$candidate" >&2
            shopt -u nullglob
            return 1
        fi
    done
    shopt -u nullglob
}

ensure_safe_apply_target() {
    local config state
    config=$(xorg_config_path)
    state=$(state_path)
    validate_managed_paths || return
    validate_xorg_conflicts || return
    if [[ -e "$config" || -L "$config" || -e "$state" || -L "$state" ]]; then
        if ! managed_checksum; then
            printf 'Error: existing GPU isolation files are incomplete or fail their managed checksum; refusing to overwrite them.\n' >&2
            return 1
        fi
    fi
}

apply_config() {
    local dry_run=$1 config state temporary_config temporary_state backup_config='' backup_state='' checksum answer had_managed=0
    config=$(xorg_config_path)
    state=$(state_path)
    ensure_safe_apply_target || return
    printf 'Desktop GPU: %s\n' "$(gpu_field "${GPU_BY_INDEX[$DESKTOP_GPU]}" name)"
    printf 'Xorg configuration:\n'
    render_config
    if (( dry_run )); then
        printf 'Dry run: no files were changed.\n'
        return
    fi
    read -r -p 'Apply this configuration? [y/N] ' answer
    [[ "$answer" == y || "$answer" == Y ]] || { printf 'Cancelled.\n'; return; }
    if managed_checksum; then
        had_managed=1
        backup_config=$(mktemp)
        backup_state=$(mktemp)
        TEMPORARY_FILES+=("$backup_config" "$backup_state")
        cat "$config" > "$backup_config"
        cat "$state" > "$backup_state"
    fi
    temporary_config=$(mktemp)
    temporary_state=$(mktemp)
    TEMPORARY_FILES+=("$temporary_config" "$temporary_state")
    render_config > "$temporary_config"
    checksum=$(sha256sum "$temporary_config")
    checksum=${checksum%% *}
    printf '%s\n' "$checksum" > "$temporary_state"
    if ! run_privileged mkdir -p "${config%/*}" "${state%/*}"; then
        printf 'Error: failed to create managed directories.\n' >&2
        return 1
    fi
    ensure_safe_apply_target || return
    if ! run_privileged install -m 0644 "$temporary_config" "$config"; then
        printf 'Error: failed to install Xorg configuration.\n' >&2
        return 1
    fi
    if ! run_privileged install -m 0600 "$temporary_state" "$state"; then
        printf 'Error: failed to install checksum state; rolling back.\n' >&2
        if (( had_managed )); then
            if ! run_privileged install -m 0644 "$backup_config" "$config" ||
               ! run_privileged install -m 0600 "$backup_state" "$state"; then
                printf 'Error: rollback failed; inspect %s and %s before restarting GDM.\n' "$config" "$state" >&2
                return 1
            fi
        else
            if ! run_privileged rm -f -- "$config" "$state"; then
                printf 'Error: rollback failed; inspect %s and %s before restarting GDM.\n' "$config" "$state" >&2
                return 1
            fi
        fi
        return 1
    fi
    printf 'Applied. Restart GDM or reboot before relying on GPU isolation.\n'
}

undo_config() {
    local dry_run=$1 config state answer
    config=$(xorg_config_path)
    state=$(state_path)
    validate_managed_paths || return
    if [[ ! -e "$config" && ! -L "$config" && ! -e "$state" && ! -L "$state" ]]; then
        printf 'No managed GPU isolation configuration is installed.\n'
        return
    fi
    if ! managed_checksum; then
        printf 'Error: managed configuration checksum does not match; refusing to remove files.\n' >&2
        return 1
    fi
    if (( dry_run )); then
        printf 'Would remove %s and %s.\n' "$config" "$state"
        return
    fi
    read -r -p 'Remove the managed GPU isolation configuration? [y/N] ' answer
    [[ "$answer" == y || "$answer" == Y ]] || { printf 'Cancelled.\n'; return; }
    run_privileged rm -f "$config" "$state"
    printf 'Removed. Restart GDM or reboot to restore normal GPU discovery.\n'
}

main() {
    local command=status dry_run=0 argument
    if (( $# > 0 )) && [[ "$1" != --dry-run ]]; then
        command=$1
        shift
    fi
    for argument in "$@"; do
        case "$argument" in
            --dry-run) dry_run=1 ;;
            -h|--help|help) usage; return ;;
            *) usage >&2; return "$EXIT_USAGE" ;;
        esac
    done
    case "$command" in
        -h|--help|help) usage; return ;;
        status|apply)
            validate_platform || return
            validate_desktop || return
            load_gpu_inventory || return
            load_desktop_gpu || return
            if [[ "$command" == status ]]; then show_status; else apply_config "$dry_run"; fi
            ;;
        undo)
            validate_platform || return
            undo_config "$dry_run"
            ;;
        *) usage >&2; return "$EXIT_USAGE" ;;
    esac
}

main "$@"
