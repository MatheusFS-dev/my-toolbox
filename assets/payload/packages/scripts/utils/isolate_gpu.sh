#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/../lib/toolbox_output.sh"
tb_output_init isolate-gpu

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
        tb_error "Cannot read operating-system metadata at $release_file; nothing changed." >&2
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
        tb_error 'Native Ubuntu 24.04 or 26.04 is required; nothing changed.' >&2
        return 1
    fi
}

validate_desktop() {
    if ! systemctl is-active gdm3 >/dev/null 2>&1 && ! systemctl is-active gdm >/dev/null 2>&1; then
        tb_error 'GDM is not active; start GDM and retry. Nothing changed.' >&2
        return 1
    fi
    if ! pgrep -x Xorg >/dev/null 2>&1; then
        tb_error 'An active Xorg process is required; Wayland sessions are not supported. Nothing changed.' >&2
        return 1
    fi
}

load_gpu_inventory() {
    local output row index uuid bus name
    if ! output=$(nvidia-smi --query-gpu=index,uuid,pci.bus_id,name --format=csv,noheader,nounits); then
        tb_error 'Failed to query NVIDIA GPU inventory; verify nvidia-smi and retry. Nothing changed.' >&2
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
            tb_error "NVIDIA returned an invalid GPU inventory row: $row. Nothing changed." >&2
            return 1
        fi
        GPU_ROWS+=("$row")
        GPU_BY_INDEX[$index]=$row
    done <<< "$output"
    if (( ${#GPU_ROWS[@]} < 2 )); then
        tb_error 'At least two NVIDIA GPUs are required; nothing changed.' >&2
        return 1
    fi
}

load_desktop_gpu() {
    local xorg_pids pmon row gpu pid type
    xorg_pids=" $(pgrep -x Xorg | tr '\n' ' ')"
    if ! pmon=$(nvidia-smi pmon -c 1 2>/dev/null); then
        tb_error 'Failed to query NVIDIA graphics processes; verify nvidia-smi permissions and retry.' >&2
        return 1
    fi
    DESKTOP_GPU=''
    while read -r gpu pid type _; do
        [[ "$gpu" == \#* || -z "$gpu" ]] && continue
        if [[ "$type" == G || "$type" == C+G ]] && [[ "$xorg_pids" == *" $pid "* ]]; then
            if [[ -n "$DESKTOP_GPU" && "$DESKTOP_GPU" != "$gpu" ]]; then
                tb_error 'Xorg is active on more than one NVIDIA GPU; no anchor was selected and nothing changed.' >&2
                return 1
            fi
            DESKTOP_GPU=$gpu
        fi
    done <<< "$pmon"
    if [[ -z "$DESKTOP_GPU" ]]; then
        tb_error 'Could not identify the NVIDIA GPU used by Xorg; nothing changed.' >&2
        return 1
    fi
    row=${GPU_BY_INDEX[$DESKTOP_GPU]:-}
    if [[ -z "$row" ]]; then
        tb_error 'Xorg reported an NVIDIA GPU outside the current inventory; refresh the driver state and retry.' >&2
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
    tb_info 'NVIDIA GPU isolation status:'
    for row in "${GPU_ROWS[@]}"; do
        index=$(gpu_field "$row" index)
        name=$(gpu_field "$row" name)
        label=available
        [[ "$index" == "$DESKTOP_GPU" ]] && label=desktop-active
        tb_info "GPU $index: $name [$label]"
    done
    if [[ -e "$(xorg_config_path)" ]]; then
        tb_info "Managed Xorg configuration: $(xorg_config_path)"
    else
        tb_info 'Managed Xorg configuration: not installed'
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
            tb_error "Managed path must be absolute: $target. Nothing changed." >&2
            return 1
        fi
        if [[ -L "$target" ]]; then
            tb_error "Managed path is a symbolic link: $target. Nothing changed." >&2
            return 1
        fi
        if [[ -e "$target" && ! -f "$target" ]]; then
            tb_error "Managed path is not a regular file: $target. Nothing changed." >&2
            return 1
        fi
        parent=${target%/*}
        current=/
        IFS='/' read -r -a components <<< "${parent#/}"
        for component in "${components[@]}"; do
            [[ -z "$component" ]] && continue
            current=${current%/}/$component
            if [[ -L "$current" ]]; then
                tb_error "Managed parent is a symbolic link: $current. Nothing changed." >&2
                return 1
            fi
            if [[ -e "$current" && ! -d "$current" ]]; then
                tb_error "Managed parent is not a directory: $current. Nothing changed." >&2
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
            tb_error "Existing Xorg GPU configuration may conflict: $candidate. Nothing changed." >&2
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
            tb_error 'Existing GPU isolation files are incomplete or fail their managed checksum; nothing was overwritten. Inspect them manually.' >&2
            return 1
        fi
    fi
}

apply_config() {
    local dry_run=$1 config state temporary_config temporary_state backup_config='' backup_state='' checksum answer had_managed=0
    config=$(xorg_config_path)
    state=$(state_path)
    ensure_safe_apply_target || return
    tb_info "Desktop GPU: $(gpu_field "${GPU_BY_INDEX[$DESKTOP_GPU]}" name)"
    tb_info 'Proposed Xorg configuration:'
    render_config
    if (( dry_run )); then
        tb_warn 'Dry run complete; no files were changed.'
        return
    fi
    tb_prompt 'Apply this configuration? [y/N] (default: n):'
    read -r answer
    [[ "$answer" == y || "$answer" == Y ]] || { tb_info 'Cancelled; no files were changed.'; return; }
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
        tb_error 'Failed to create managed directories; no configuration was installed.' >&2
        return 1
    fi
    ensure_safe_apply_target || return
    if ! run_privileged install -m 0644 "$temporary_config" "$config"; then
        tb_error 'Failed to install the Xorg configuration; the previous configuration remains.' >&2
        return 1
    fi
    if ! run_privileged install -m 0600 "$temporary_state" "$state"; then
        tb_error 'Failed to install checksum state; rolling back the Xorg configuration.' >&2
        if (( had_managed )); then
            if ! run_privileged install -m 0644 "$backup_config" "$config" ||
               ! run_privileged install -m 0600 "$backup_state" "$state"; then
                tb_error "Rollback failed; inspect $config and $state before restarting GDM." >&2
                return 1
            fi
        else
            if ! run_privileged rm -f -- "$config" "$state"; then
                tb_error "Rollback failed; inspect $config and $state before restarting GDM." >&2
                return 1
            fi
        fi
        return 1
    fi
    tb_info 'Restart GDM or reboot before relying on GPU isolation.'
    tb_success 'GPU isolation configuration applied successfully.'
}

undo_config() {
    local dry_run=$1 config state answer
    config=$(xorg_config_path)
    state=$(state_path)
    validate_managed_paths || return
    if [[ ! -e "$config" && ! -L "$config" && ! -e "$state" && ! -L "$state" ]]; then
        tb_info 'No managed GPU isolation configuration is installed; nothing changed.'
        return
    fi
    if ! managed_checksum; then
        tb_error 'Managed configuration checksum does not match; no files were removed. Inspect the managed files manually.' >&2
        return 1
    fi
    if (( dry_run )); then
        tb_warn "Dry run complete; would remove $config and $state."
        return
    fi
    tb_prompt 'Remove the managed GPU isolation configuration? [y/N] (default: n):'
    read -r answer
    [[ "$answer" == y || "$answer" == Y ]] || { tb_info 'Cancelled; no files were changed.'; return; }
    tb_info 'Removing the managed GPU isolation configuration...'
    run_privileged rm -f "$config" "$state"
    tb_info 'Restart GDM or reboot to restore normal GPU discovery.'
    tb_success 'GPU isolation configuration removed successfully.'
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
    tb_info 'Checking GPU isolation prerequisites and current state...'
    case "$command" in
        -h|--help|help) usage; return ;;
        status|apply)
            validate_platform || return
            validate_desktop || return
            load_gpu_inventory || return
            load_desktop_gpu || return
            if [[ "$command" == status ]]; then
                show_status
                tb_success 'GPU isolation status checked successfully.'
            else
                apply_config "$dry_run"
            fi
            ;;
        undo)
            validate_platform || return
            undo_config "$dry_run"
            ;;
        *) usage >&2; return "$EXIT_USAGE" ;;
    esac
}

main "$@"
