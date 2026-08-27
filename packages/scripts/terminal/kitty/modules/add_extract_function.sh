#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

add_extract_function() {
    local zshrc="$HOME/.zshrc"
    local bashrc="$HOME/.bashrc"

    local extract_code
    read -r -d '' extract_code << 'EOF' || true

# Extract function (extract plugin)
extract() {
    if [ -f $1 ] ; then
        case $1 in
            *.tar.bz2)   tar xjf $1     ;;
            *.tar.gz)    tar xzf $1     ;;
            *.bz2)       bunzip2 $1     ;;
            *.rar)       unrar x $1     ;;
            *.gz)        gunzip $1      ;;
            *.tar)       tar xf $1      ;;
            *.tbz2)      tar xjf $1     ;;
            *.tgz)       tar xzf $1     ;;
            *.zip)       unzip $1       ;;
            *.Z)         uncompress $1  ;;
            *.7z)        7z x $1        ;;
            *)           echo "'$1' cannot be extracted via extract()" ;;
        esac
    else
        echo "'$1' is not a valid file"
    fi
}
EOF

    if [[ -f "$zshrc" ]]; then
        if ! grep -qF "extract() {" "$zshrc"; then
            echo "Adding extract function to $zshrc..."
            echo "$extract_code" >> "$zshrc"
        else
            echo "extract function already present in $zshrc, skipping."
        fi
    fi

    if [[ -f "$bashrc" ]]; then
        if ! grep -qF "extract() {" "$bashrc"; then
            echo "Adding extract function to $bashrc..."
            echo "$extract_code" >> "$bashrc"
        else
            echo "extract function already present in $bashrc, skipping."
        fi
    fi
}

run_step "Extract function installation" "add_extract_function"
