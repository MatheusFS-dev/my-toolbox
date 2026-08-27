#!/usr/bin/env bash
set -e

# Parse command line arguments
ASSUME_YES=false
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -y|--yes)
            ASSUME_YES=true
            shift
            ;;
        *)
            echo "Unknown argument: $1" >&2
            echo "Usage: $0 [-y|--yes]" >&2
            exit 1
            ;;
    esac
done

# Must run with sudo/root
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run with sudo." >&2
    echo "Please run: sudo $0" >&2
    exit 1
fi

# Resolve the real invoking user's home directory if run via sudo
if [[ -n "$SUDO_USER" ]]; then
    export USER="$SUDO_USER"
    export HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
fi

echo "=================================================="
echo "Kitty + Zellij Setup Interactive Prompt"
echo "=================================================="
echo "Answer Yes (y/Y) or No (n/N) for each option below."
echo "Press ENTER to accept the defaults."
echo ""

prompt_yes_no() {
    local prompt_msg="$1"
    local default_val="$2"
    if [[ "$ASSUME_YES" == "true" ]]; then
        echo "Using default: $default_val" >&2
        echo "$default_val"
        return
    fi
    local input_val
    read -r -p "$prompt_msg [Default: $default_val]: " input_val
    if [[ -z "$input_val" ]]; then
        input_val="$default_val"
    fi
    if [[ "$input_val" =~ ^[Yy]$ ]]; then
        echo "y"
    else
        echo "n"
    fi
}

# 1. Zsh + Oh My Zsh
echo "Zsh & Oh My Zsh:"
echo "  Installs Zsh shell, Oh My Zsh frame, and zsh-syntax-highlighting."
RUN_ZSH=$(prompt_yes_no "Install Zsh and Oh My Zsh?" "y")
echo ""

# 2. Rust
echo "Rust & Cargo:"
echo "  Installs rustup toolchain manager and cargo build tool."
RUN_RUST=$(prompt_yes_no "Install Rust/Cargo?" "y")
echo ""

# 3. Kitty
echo "Kitty Terminal:"
echo "  Installs the fast, GPU-accelerated terminal emulator."
RUN_KITTY=$(prompt_yes_no "Install Kitty?" "y")
echo ""

# 4. Zellij
echo "Zellij Multiplexer:"
echo "  Installs Zellij terminal workspace/multiplexer manager."
RUN_ZELLIJ=$(prompt_yes_no "Install Zellij?" "y")
echo ""

# 5. Starship
echo "Starship Prompt:"
echo "  Installs the customizable, modern shell prompt starship."
RUN_STARSHIP=$(prompt_yes_no "Install Starship?" "y")
echo ""

# 6. Zellij Copy Behavior
echo "Zellij Copy/Clipboard Behavior:"
echo "  Enables system clipboard sync for mouse selections in Zellij."
RUN_ZELLIJ_COPY=$(prompt_yes_no "Configure Zellij Copy behavior?" "y")
echo ""

# 7. Kitty Configuration
echo "Kitty configuration:"
echo "  Writes kitty.conf to ~/.config/kitty/ and sets the default shell."
RUN_KITTY_CONF=$(prompt_yes_no "Configure Kitty?" "y")
AUTO_START_ZELLIJ="n"
if [[ "$RUN_KITTY_CONF" == "y" ]]; then
    AUTO_START_ZELLIJ=$(prompt_yes_no "Should Kitty auto start Zellij?" "n")
fi
echo ""

# 8. PATH
echo "PATH configuration:"
echo "  Appends ~/.cargo/bin to your permanent PATH environment variable."
RUN_PATH=$(prompt_yes_no "Configure PATH?" "y")
echo ""

# 9. Word Navigation
echo "Word Navigation Bindings:"
echo "  Enables word-by-word cursor movement via Ctrl + Arrow keys."
RUN_WORD_NAV=$(prompt_yes_no "Configure Word navigation?" "y")
echo ""

# 10. Zi
echo "Zi Plugin Manager:"
echo "  Installs Zi, a lightweight plugin manager for Zsh."
RUN_ZI=$(prompt_yes_no "Install Zi Zsh plugin manager?" "y")
echo ""

# 11. zsh-eza
echo "zsh-eza (ls replacement):"
echo "  Installs eza utility and loads the zsh-eza plugin via Zi."
RUN_EZA=$(prompt_yes_no "Install and configure zsh-eza?" "y")
EZA_LIST_VIEW="1"
if [[ "$RUN_EZA" == "y" ]]; then
    if [[ "$ASSUME_YES" == "true" ]]; then
        echo "Using default: 1" >&2
    else
        echo "Choose ls/eza display mode:"
        echo "  1) List (one per line) [Default]"
        echo "  2) Side-by-side"
        read -r -p "Selection [1/2] [Default: 1]: " input_val
        if [[ "$input_val" == "2" ]]; then
            EZA_LIST_VIEW="2"
        fi
    fi
fi
echo ""

# 12. fzf-tab
echo "fzf-tab Completions:"
echo "  Enables rich interactive tab-completion previews via fzf."
RUN_FZF_TAB=$(prompt_yes_no "Install fzf-tab plugin?" "y")
echo ""

# 13. FiraCode Nerd Font
echo "FiraCode Nerd Font:"
echo "  Downloads FiraCode Nerd Font and configures Kitty to use it."
RUN_FONT=$(prompt_yes_no "Install FiraCode Nerd Font?" "y")
echo ""

# 14. Nautilus Integration
echo "Nautilus File Manager Integration:"
echo "  Adds 'Open in Kitty' option to the files app right-click menu."
RUN_NAUTILUS=$(prompt_yes_no "Install Nautilus integration?" "y")
echo ""

# 15. Extract Function
echo "Extract Function Utility:"
echo "  Adds 'extract' command to unzip/untar archives automatically."
RUN_EXTRACT=$(prompt_yes_no "Add extract function utility?" "y")
echo ""

# 16. Sudo Shortcut
echo "Double-ESC Sudo Command line:"
echo "  Allows prefixing the current terminal command line with 'sudo' by pressing ESC twice."
RUN_SUDO_SHORTCUT=$(prompt_yes_no "Add double-ESC sudo shortcut?" "y")
echo ""

# 17. Git clone auto-cd wrapper
echo "Git Clone Auto-CD Wrapper:"
echo "  Automatically changes directory (cd) into the cloned folder after 'git clone'."
echo "  Example: 'git clone <url>' will clone and immediately 'cd' into it."
RUN_GIT_WRAPPER=$(prompt_yes_no "Configure git clone auto-cd wrapper?" "y")
echo ""

# 18. Update alias
echo "System Update Alias:"
echo "  Adds 'update' alias that runs 'sudo apt update && sudo apt upgrade'."
echo "  Example: Simply type 'update' to upgrade your system packages."
RUN_UPDATE_ALIAS=$(prompt_yes_no "Configure 'update' alias?" "y")
echo ""

# 19. Python venv auto-activator
echo "Python venv Auto-Activator:"
echo "  Adds 'venv' function to automatically search for and activate nearby python .venv environments."
RUN_VENV=$(prompt_yes_no "Configure Python venv auto-activator?" "y")
echo ""

# 20. Default Terminal
echo "System Default Terminal:"
echo "  Sets Kitty as the default terminal (x-terminal-emulator)."
RUN_DEFAULT_TERMINAL=$(prompt_yes_no "Configure system default terminal?" "y")
echo ""


# --- Main execution ---
echo "Starting Kitty + Zellij setup..."
echo ""

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
MODULES_DIR="$SCRIPT_DIR/modules"

if [[ "$RUN_ZSH" == "y" ]]; then
    bash "$MODULES_DIR/install_zsh_and_oh_my_zsh.sh"
fi

if [[ "$RUN_RUST" == "y" ]]; then
    bash "$MODULES_DIR/install_rust.sh"
fi

if [[ "$RUN_KITTY" == "y" ]]; then
    bash "$MODULES_DIR/install_kitty.sh"
fi

if [[ "$RUN_ZELLIJ" == "y" ]]; then
    bash "$MODULES_DIR/install_zellij.sh"
fi

if [[ "$RUN_STARSHIP" == "y" ]]; then
    bash "$MODULES_DIR/install_starship.sh"
fi

if [[ "$RUN_ZELLIJ_COPY" == "y" ]]; then
    bash "$MODULES_DIR/configure_zellij_copy_behavior.sh"
fi

if [[ "$RUN_KITTY_CONF" == "y" ]]; then
    bash "$MODULES_DIR/configure_kitty.sh" "$AUTO_START_ZELLIJ"
fi

if [[ "$RUN_PATH" == "y" ]]; then
    bash "$MODULES_DIR/configure_path.sh"
fi

if [[ "$RUN_WORD_NAV" == "y" ]]; then
    bash "$MODULES_DIR/add_word_navigation.sh"
fi

if [[ "$RUN_ZI" == "y" ]]; then
    bash "$MODULES_DIR/install_zi.sh"
fi

if [[ "$RUN_EZA" == "y" ]]; then
    bash "$MODULES_DIR/install_zsh_eza.sh" "$EZA_LIST_VIEW"
fi

if [[ "$RUN_FZF_TAB" == "y" ]]; then
    bash "$MODULES_DIR/install_fzf_tab.sh"
fi

if [[ "$RUN_FONT" == "y" ]]; then
    bash "$MODULES_DIR/install_firacode_nerd_font.sh"
fi

if [[ "$RUN_NAUTILUS" == "y" ]]; then
    bash "$MODULES_DIR/nautilus_integration.sh"
fi

if [[ "$RUN_EXTRACT" == "y" ]]; then
    bash "$MODULES_DIR/add_extract_function.sh"
fi

if [[ "$RUN_SUDO_SHORTCUT" == "y" ]]; then
    bash "$MODULES_DIR/add_sudo_shortcut.sh"
fi

if [[ "$RUN_GIT_WRAPPER" == "y" ]]; then
    bash "$MODULES_DIR/install_git_clone_wrapper.sh"
fi

if [[ "$RUN_UPDATE_ALIAS" == "y" ]]; then
    bash "$MODULES_DIR/install_update_alias.sh"
fi

if [[ "$RUN_VENV" == "y" ]]; then
    bash "$MODULES_DIR/install_venv.sh"
fi

if [[ "$RUN_DEFAULT_TERMINAL" == "y" ]]; then
    bash "$MODULES_DIR/configure_default_terminal.sh"
fi

# Correct ownership of user config files
if [[ -n "$SUDO_USER" ]]; then
    chown -R "$SUDO_USER:$SUDO_USER" "$HOME/.config" "$HOME/.local" "$HOME/.oh-my-zsh" "$HOME/.cargo" "$HOME/.zshrc" "$HOME/.bashrc" 2>/dev/null || true
fi

echo ""
echo "Setup complete."
echo "You can now launch Kitty from your application menu or by running: kitty"
echo "If you use Zsh, restart your shell or run: source ~/.zshrc"
echo "Also, to fix notifications, install this extension:"
echo "sudo apt update && sudo apt install -y gnome-shell-extension-manager"
echo "extension-manager"
echo "Then, install: Junk Notification Cleaner"