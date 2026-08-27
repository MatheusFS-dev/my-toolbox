#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_git_clone_wrapper() {
    local targets=()
    if [[ -f "$HOME/.bashrc" ]]; then
        targets+=("$HOME/.bashrc")
    fi
    if [[ -f "$HOME/.zshrc" ]]; then
        targets+=("$HOME/.zshrc")
    fi

    local start_marker="# >>> git-clone-cd-wrapper >>>"
    local end_marker="# <<< git-clone-cd-wrapper <<<"

    for file in "${targets[@]}"; do
        # Remove existing block first to be idempotent
        if [[ -f "$file" ]]; then
            local temp_file
            temp_file=$(mktemp)
            awk -v start="$start_marker" -v end="$end_marker" '
                $0 == start { skip = 1; next }
                $0 == end { skip = 0; next }
                skip != 1 { print }
            ' "$file" > "$temp_file"
            cat "$temp_file" > "$file"
            rm -f "$temp_file"
        fi

        echo "Configuring git clone auto-cd wrapper in $file..."
        cat >> "$file" << 'EOF'

# >>> git-clone-cd-wrapper >>>
_git_clone_cd_target() {
    local repo=""
    local target=""
    local positional_count=0
    local arg=""

    while [ "$#" -gt 0 ]; do
        arg="$1"

        case "$arg" in
            --)
                shift

                while [ "$#" -gt 0 ]; do
                    positional_count=$((positional_count + 1))

                    if [ "$positional_count" -eq 1 ]; then
                        repo="$1"
                    else
                        target="$1"
                    fi

                    shift
                done

                break
                ;;

            -b|-o|-u|-c|-j|--branch|--origin|--upload-pack|--template|--reference|--reference-if-able|--separate-git-dir|--jobs|--depth|--shallow-since|--shallow-exclude|--config|--filter|--server-option)
                shift

                if [ "$#" -gt 0 ]; then
                    shift
                fi
                ;;

            --*=*)
                shift
                ;;

            --*)
                shift
                ;;

            -*)
                shift
                ;;

            *)
                positional_count=$((positional_count + 1))

                if [ "$positional_count" -eq 1 ]; then
                    repo="$1"
                else
                    target="$1"
                fi

                shift
                ;;
        esac
    done

    if [ -n "$target" ]; then
        printf "%s\n" "$target"
        return 0
    fi

    if [ -n "$repo" ]; then
        repo="${repo%/}"
        repo="${repo##*/}"
        repo="${repo##*:}"
        repo="${repo%.git}"

        printf "%s\n" "$repo"
    fi
}

git() {
    if [ "$#" -gt 0 ] && [ "$1" = "clone" ]; then
        local target=""
        local exit_code=0

        target="$(_git_clone_cd_target "${@:2}")"

        command git "$@"
        exit_code=$?

        if [ "$exit_code" -eq 0 ] && [ -n "$target" ] && [ -d "$target" ]; then
            cd -- "$target" || return "$exit_code"
            echo "git-clone-cd: Automatically changed directory to $target"
        fi

        return "$exit_code"
    fi

    command git "$@"
}
# <<< git-clone-cd-wrapper <<<
EOF
    done
}

run_step "Git clone auto-cd wrapper installation" "install_git_clone_wrapper"
