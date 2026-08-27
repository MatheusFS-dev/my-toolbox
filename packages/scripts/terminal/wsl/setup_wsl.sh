#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULES_DIR="$SCRIPT_DIR/modules"

FEATURES=(
    zsh rust zellij starship zellij_copy path word_navigation zi eza
    fzf_tab extract sudo_shortcut git_wrapper update_alias venv default_shell
)

declare -gA FEATURE_LABELS=(
    [zsh]='Zsh, Oh My Zsh, and syntax highlighting'
    [rust]='Rust and Cargo'
    [zellij]='Zellij'
    [starship]='Starship prompt'
    [zellij_copy]='Zellij OSC52 clipboard behavior'
    [path]='user-local PATH entries'
    [word_navigation]='word navigation bindings'
    [zi]='Zi plugin manager'
    [eza]='eza and its Zi integration'
    [fzf_tab]='fzf-tab completion'
    [extract]='archive extraction helper'
    [sudo_shortcut]='double-Escape sudo shortcut'
    [git_wrapper]='git clone auto-cd wrapper'
    [update_alias]='system update alias'
    [venv]='Python virtual-environment helper'
    [default_shell]='Zsh as the login shell'
)

declare -gA SKIP=()
declare -gA SELECTED=()
ASSUME_YES=false
SHOW_HELP=false
EZA_VIEW=list
EZA_VIEW_EXPLICIT=false
TARGET_USER=''
TARGET_HOME=''

usage() {
    cat <<'EOF'
Usage: sudo bash setup_wsl.sh [OPTIONS]

Options:
  -y, --yes                 Enable every compatible feature
  --eza-view=list|grid      Set eza display style (default: list)
  --skip-zsh                Skip Zsh, Zi, eza integration, fzf-tab,
                            sudo shortcut, and default-shell configuration
  --skip-rust               Skip Rust, Zellij, and Zellij clipboard setup
  --skip-zellij             Skip Zellij and Zellij clipboard setup
  --skip-starship           Skip Starship
  --skip-zellij-copy        Skip Zellij clipboard setup
  --skip-path               Skip PATH configuration
  --skip-word-navigation    Skip word navigation bindings
  --skip-zi                 Skip Zi and eza integration
  --skip-eza                Skip eza
  --skip-fzf-tab            Skip fzf-tab
  --skip-extract            Skip the extract helper
  --skip-sudo-shortcut      Skip the double-Escape sudo shortcut
  --skip-git-wrapper        Skip the git clone auto-cd wrapper
  --skip-update-alias       Skip the update alias
  --skip-venv               Skip the venv helper
  --skip-default-shell      Do not change the login shell
  -h, --help                Show this help and exit
EOF
}

reset_options() {
    ASSUME_YES=false
    SHOW_HELP=false
    EZA_VIEW=list
    EZA_VIEW_EXPLICIT=false
    SKIP=()
    SELECTED=()

    local feature
    for feature in "${FEATURES[@]}"; do
        SKIP[$feature]=false
        SELECTED[$feature]=false
    done
}

parse_args() {
    while (( $# > 0 )); do
        case "$1" in
            -y|--yes)
                ASSUME_YES=true
                ;;
            -h|--help)
                SHOW_HELP=true
                ;;
            --eza-view=list|--eza-view=grid)
                EZA_VIEW="${1#*=}"
                EZA_VIEW_EXPLICIT=true
                ;;
            --eza-view=*)
                printf 'Error: --eza-view must be list or grid.\n' >&2
                return 2
                ;;
            --skip-zsh) SKIP[zsh]=true ;;
            --skip-rust) SKIP[rust]=true ;;
            --skip-zellij) SKIP[zellij]=true ;;
            --skip-starship) SKIP[starship]=true ;;
            --skip-zellij-copy) SKIP[zellij_copy]=true ;;
            --skip-path) SKIP[path]=true ;;
            --skip-word-navigation) SKIP[word_navigation]=true ;;
            --skip-zi) SKIP[zi]=true ;;
            --skip-eza) SKIP[eza]=true ;;
            --skip-fzf-tab) SKIP[fzf_tab]=true ;;
            --skip-extract) SKIP[extract]=true ;;
            --skip-sudo-shortcut) SKIP[sudo_shortcut]=true ;;
            --skip-git-wrapper) SKIP[git_wrapper]=true ;;
            --skip-update-alias) SKIP[update_alias]=true ;;
            --skip-venv) SKIP[venv]=true ;;
            --skip-default-shell) SKIP[default_shell]=true ;;
            *)
                printf 'Error: unknown argument: %s\n' "$1" >&2
                usage >&2
                return 2
                ;;
        esac
        shift
    done
}

validate_preflight() {
    local effective_uid="$1"
    local sudo_user="$2"
    local os_release_file="$3"
    local proc_version_file="$4"

    if [[ "$effective_uid" -ne 0 ]]; then
        printf 'Error: run this installer through sudo.\n' >&2
        return 1
    fi
    if [[ -z "$sudo_user" || "$sudo_user" == root ]]; then
        printf 'Error: SUDO_USER must identify the non-root invoking user.\n' >&2
        return 1
    fi
    if [[ ! -r "$proc_version_file" ]] || ! grep -qi microsoft "$proc_version_file"; then
        printf 'Error: this installer only supports WSL.\n' >&2
        return 1
    fi
    if [[ ! -r "$os_release_file" ]]; then
        printf 'Error: cannot read %s.\n' "$os_release_file" >&2
        return 1
    fi

    local ID='' VERSION_ID=''
    # shellcheck disable=SC1090
    source "$os_release_file"
    if [[ "$ID" != ubuntu || ( "$VERSION_ID" != 22.04 && "$VERSION_ID" != 24.04 ) ]]; then
        printf 'Error: only Ubuntu 22.04 and 24.04 are supported.\n' >&2
        return 1
    fi
}

prompt_yes_no() {
    local prompt="$1"
    local answer=''
    read -r -p "$prompt [Y/n]: " answer || true
    [[ -z "$answer" || "$answer" =~ ^[Yy]([Ee][Ss])?$ ]]
}

select_features() {
    local feature
    for feature in "${FEATURES[@]}"; do
        if [[ "${SKIP[$feature]}" == true ]]; then
            SELECTED[$feature]=false
        elif [[ "$ASSUME_YES" == true ]]; then
            SELECTED[$feature]=true
        elif prompt_yes_no "Enable ${FEATURE_LABELS[$feature]}?"; then
            SELECTED[$feature]=true
        else
            SELECTED[$feature]=false
        fi
    done

    if [[ "${SELECTED[eza]}" == true && "$ASSUME_YES" == false && "$EZA_VIEW_EXPLICIT" == false ]]; then
        local view=''
        read -r -p 'eza view, list or grid [list]: ' view || true
        if [[ "$view" == grid ]]; then
            EZA_VIEW=grid
        fi
    fi
}

disable_feature() {
    SELECTED[$1]=false
}

apply_dependency_cascades() {
    if [[ "${SELECTED[rust]}" != true ]]; then
        disable_feature zellij
        disable_feature zellij_copy
    fi
    if [[ "${SELECTED[zellij]}" != true ]]; then
        disable_feature zellij_copy
    fi
    if [[ "${SELECTED[zsh]}" != true ]]; then
        disable_feature zi
        disable_feature eza
        disable_feature fzf_tab
        disable_feature sudo_shortcut
        disable_feature default_shell
    fi
    if [[ "${SELECTED[zi]}" != true ]]; then
        disable_feature eza
    fi
}

collect_apt_packages() {
    local -A packages=()
    local package

    if [[ "${SELECTED[zsh]}" == true ]]; then
        for package in zsh git curl ca-certificates; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[rust]}" == true ]]; then
        for package in curl ca-certificates; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[zellij]}" == true ]]; then
        for package in build-essential pkg-config libssl-dev; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[starship]}" == true ]]; then
        for package in curl ca-certificates; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[zi]}" == true ]]; then
        for package in zsh git curl ca-certificates; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[eza]}" == true ]]; then
        for package in curl ca-certificates tar gzip; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[fzf_tab]}" == true ]]; then
        for package in fzf git; do packages[$package]=1; done
    fi
    if [[ "${SELECTED[extract]}" == true ]]; then
        for package in tar bzip2 gzip unzip p7zip-full unrar-free; do packages[$package]=1; done
    fi

    printf '%s\n' "${!packages[@]}" | sort
}

run_as_target() {
    (
        umask 022
        sudo -u "$TARGET_USER" env \
            HOME="$TARGET_HOME" USER="$TARGET_USER" LOGNAME="$TARGET_USER" \
            PATH="$TARGET_HOME/.local/bin:$TARGET_HOME/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
            "$@"
    )
}

install_apt_dependencies() {
    local -a packages=()
    mapfile -t packages < <(collect_apt_packages)
    (( ${#packages[@]} == 0 )) && return 0
    apt-get update && apt-get install -y "${packages[@]}"
}

run_feature() {
    local feature="$1"
    case "$feature" in
        zsh) run_as_target bash "$MODULES_DIR/install_zsh.sh" ;;
        rust) run_as_target bash "$MODULES_DIR/install_rust.sh" ;;
        zellij) run_as_target bash "$MODULES_DIR/install_zellij.sh" ;;
        starship) run_as_target bash "$MODULES_DIR/install_starship.sh" ;;
        zellij_copy) run_as_target bash "$MODULES_DIR/configure_zellij_copy.sh" ;;
        path) run_as_target bash "$MODULES_DIR/configure_path.sh" ;;
        word_navigation) run_as_target bash "$MODULES_DIR/configure_word_navigation.sh" ;;
        zi) run_as_target bash "$MODULES_DIR/install_zi.sh" ;;
        eza) run_as_target bash "$MODULES_DIR/install_eza.sh" "$EZA_VIEW" ;;
        fzf_tab) run_as_target bash "$MODULES_DIR/install_fzf_tab.sh" ;;
        extract) run_as_target bash "$MODULES_DIR/configure_extract.sh" ;;
        sudo_shortcut) run_as_target bash "$MODULES_DIR/configure_sudo_shortcut.sh" ;;
        git_wrapper) run_as_target bash "$MODULES_DIR/configure_git_wrapper.sh" ;;
        update_alias) run_as_target bash "$MODULES_DIR/configure_update_alias.sh" ;;
        venv) run_as_target bash "$MODULES_DIR/configure_venv.sh" ;;
        default_shell) bash "$MODULES_DIR/configure_default_shell.sh" "$TARGET_USER" ;;
        *) printf 'Unknown feature: %s\n' "$feature" >&2; return 1 ;;
    esac
}

execute_selected_features() {
    local feature status
    for feature in "${FEATURES[@]}"; do
        [[ "${SELECTED[$feature]}" == true ]] || continue
        printf '\n==> %s\n' "${FEATURE_LABELS[$feature]}"
        status=0
        run_feature "$feature" || status=$?
        if (( status != 0 )); then
            printf 'WARNING: %s failed with status %d; continuing.\n' "$feature" "$status" >&2
        fi
    done
    return 0
}

prepare_target() {
    local passwd_entry
    passwd_entry="$(getent passwd "$SUDO_USER")" || {
        printf 'Error: SUDO_USER %s does not exist.\n' "$SUDO_USER" >&2
        return 1
    }
    TARGET_USER="$SUDO_USER"
    TARGET_HOME="$(cut -d: -f6 <<< "$passwd_entry")"
    if [[ -z "$TARGET_HOME" || ! -d "$TARGET_HOME" ]]; then
        printf 'Error: cannot resolve a home directory for %s.\n' "$TARGET_USER" >&2
        return 1
    fi
    export TARGET_USER TARGET_HOME
}

main() {
    reset_options
    parse_args "$@" || return $?
    if [[ "$SHOW_HELP" == true ]]; then
        usage
        return 0
    fi

    validate_preflight "$EUID" "${SUDO_USER:-}" /etc/os-release /proc/version || return 1
    prepare_target || return 1
    select_features
    apply_dependency_cascades

    printf 'Installing selected apt dependencies in one pass...\n'
    if ! install_apt_dependencies; then
        printf 'WARNING: apt dependency installation failed; continuing.\n' >&2
    fi

    if ! run_as_target bash "$MODULES_DIR/backup_configs.sh"; then
        printf 'WARNING: configuration backup failed; continuing.\n' >&2
    fi

    execute_selected_features
    printf '\nWSL terminal setup finished. Restart the terminal to load shell changes.\n'
    return 0
}

reset_options
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
