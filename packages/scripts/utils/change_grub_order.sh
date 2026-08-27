#!/usr/bin/env bash

set -euo pipefail

GRUB_DEFAULT_FILE="/etc/default/grub"
GRUB_CFG_FILE="/boot/grub/grub.cfg"
GRUB_ENV_FILE="/boot/grub/grubenv"
BACKUP_SUFFIX="$(date +%Y%m%d_%H%M%S)"

print_header() {
    # Args:
    #     None.
    #
    # Returns:
    #     None.
    cat <<'EOF'
============================================================
GRUB default boot entry changer for Ubuntu 24
============================================================

This script will:
1. Detect boot entries from /boot/grub/grub.cfg
2. Show the current configured GRUB_DEFAULT
3. Let you select a new default boot entry
4. Back up /etc/default/grub
5. Update /etc/default/grub
6. Run update-grub

EOF
}

require_root() {
    # Args:
    #     None.
    #
    # Returns:
    #     None.
    if [[ "${EUID}" -ne 0 ]]; then
        echo "Error: run this script with sudo."
        echo "Example: sudo ./change_grub_default.sh"
        exit 1
    fi
}

require_files_and_commands() {
    # Args:
    #     None.
    #
    # Returns:
    #     None.
    if [[ ! -f "${GRUB_DEFAULT_FILE}" ]]; then
        echo "Error: ${GRUB_DEFAULT_FILE} was not found."
        exit 1
    fi

    if [[ ! -f "${GRUB_CFG_FILE}" ]]; then
        echo "Error: ${GRUB_CFG_FILE} was not found."
        echo "Try running: sudo update-grub"
        exit 1
    fi

    if ! command -v python3 >/dev/null 2>&1; then
        echo "Error: python3 is required."
        exit 1
    fi

    if ! command -v update-grub >/dev/null 2>&1; then
        echo "Error: update-grub was not found."
        exit 1
    fi
}

parse_grub_entries() {
    # Args:
    #     None.
    #
    # Returns:
    #     Prints tab-separated fields:
    #     selection_number, grub_numeric_path, grub_title_path, grub_entry_id.
    python3 - "${GRUB_CFG_FILE}" <<'PY'
import re
import sys
from pathlib import Path


def brace_delta_outside_quotes(line):
    """Counts the net brace delta outside quoted strings.

    Args:
        line: One line from a GRUB configuration file.

    Returns:
        The number of opening braces minus closing braces outside quotes.
    """
    delta = 0
    quote = None
    escaped = False

    for char in line:
        if escaped:
            escaped = False
            continue

        if char == "\\":
            escaped = True
            continue

        if quote is not None:
            if char == quote:
                quote = None
            continue

        if char in ("'", '"'):
            quote = char
            continue

        if char == "#":
            break

        if char == "{":
            delta += 1
        elif char == "}":
            delta -= 1

    return delta


def extract_quoted_strings(line):
    """Extracts quoted strings from a GRUB configuration line.

    Args:
        line: One line from a GRUB configuration file.

    Returns:
        A list containing quoted string contents without quote characters.
    """
    matches = re.findall(r"""(['"])((?:\\.|(?!\1).)*?)\1""", line)
    return [value for _, value in matches]


def is_menuentry_line(line):
    """Checks whether a line starts a GRUB menuentry block.

    Args:
        line: One line from a GRUB configuration file.

    Returns:
        True if the line starts a menuentry block, otherwise False.
    """
    return re.match(r"^\s*menuentry\s+", line) is not None


def is_submenu_line(line):
    """Checks whether a line starts a GRUB submenu block.

    Args:
        line: One line from a GRUB configuration file.

    Returns:
        True if the line starts a submenu block, otherwise False.
    """
    return re.match(r"^\s*submenu\s+", line) is not None


def extract_entry_id(line):
    """Extracts a generated GRUB entry ID when available.

    Args:
        line: One menuentry line from a GRUB configuration file.

    Returns:
        The detected entry ID, or an empty string when none is found.
    """
    quoted = extract_quoted_strings(line)

    if "$menuentry_id_option" in line and len(quoted) >= 2:
        return quoted[-1]

    id_match = re.search(r"--id[=\s]+(['\"]?)([^'\"\s{]+)\1", line)
    if id_match:
        return id_match.group(2)

    return ""


def join_path(parts):
    """Joins GRUB path components using GRUB's submenu separator.

    Args:
        parts: Path components for a GRUB menu entry.

    Returns:
        A GRUB title path string.
    """
    return ">".join(str(part) for part in parts)


def parse_grub_cfg(path):
    """Parses GRUB menu entries and submenu paths.

    Args:
        path: Path to grub.cfg.

    Returns:
        A list of dictionaries describing bootable menu entries.
    """
    lines = Path(path).read_text(errors="replace").splitlines()

    contexts = [
        {
            "title_path": [],
            "index_prefix": [],
            "item_count": 0,
            "body_level": 0,
        }
    ]

    brace_level = 0
    entries = []

    for line in lines:
        if is_submenu_line(line):
            quoted = extract_quoted_strings(line)
            if quoted:
                parent = contexts[-1]
                submenu_index = parent["item_count"]
                parent["item_count"] += 1

                title = quoted[0]
                contexts.append(
                    {
                        "title_path": parent["title_path"] + [title],
                        "index_prefix": parent["index_prefix"] + [submenu_index],
                        "item_count": 0,
                        "body_level": brace_level + 1,
                    }
                )

        elif is_menuentry_line(line):
            quoted = extract_quoted_strings(line)
            if quoted:
                parent = contexts[-1]
                entry_index = parent["item_count"]
                parent["item_count"] += 1

                title = quoted[0]
                numeric_path = join_path(parent["index_prefix"] + [entry_index])
                title_path = join_path(parent["title_path"] + [title])
                entry_id = extract_entry_id(line)

                entries.append(
                    {
                        "numeric_path": numeric_path,
                        "title_path": title_path,
                        "entry_id": entry_id,
                    }
                )

        brace_level += brace_delta_outside_quotes(line)

        while len(contexts) > 1 and brace_level < contexts[-1]["body_level"]:
            contexts.pop()

    return entries


def main():
    """Prints parsed GRUB entries as tab-separated rows.

    Args:
        None.

    Returns:
        None.
    """
    grub_cfg_path = sys.argv[1]
    entries = parse_grub_cfg(grub_cfg_path)

    for selection, entry in enumerate(entries, start=1):
        print(
            f"{selection}\t"
            f"{entry['numeric_path']}\t"
            f"{entry['title_path']}\t"
            f"{entry['entry_id']}"
        )


if __name__ == "__main__":
    main()
PY
}

get_configured_grub_default() {
    # Args:
    #     None.
    #
    # Returns:
    #     Prints the configured GRUB_DEFAULT value.
    local value

    value="$(
        awk -F= '
            /^[[:space:]]*GRUB_DEFAULT[[:space:]]*=/ {
                value=$0
                sub(/^[[:space:]]*GRUB_DEFAULT[[:space:]]*=[[:space:]]*/, "", value)
                print value
                exit
            }
        ' "${GRUB_DEFAULT_FILE}" || true
    )"

    if [[ -z "${value}" ]]; then
        echo "0"
        return
    fi

    value="${value%\"}"
    value="${value#\"}"
    value="${value%\'}"
    value="${value#\'}"

    echo "${value}"
}

get_saved_grub_entry() {
    # Args:
    #     None.
    #
    # Returns:
    #     Prints the saved GRUB entry when available.
    if command -v grub-editenv >/dev/null 2>&1 && [[ -f "${GRUB_ENV_FILE}" ]]; then
        grub-editenv "${GRUB_ENV_FILE}" list 2>/dev/null \
            | awk -F= '/^saved_entry=/ {print $2; exit}'
    fi
}

print_entries() {
    # Args:
    #     None.
    #
    # Returns:
    #     None.
    local line
    local selection
    local numeric_path
    local title_path
    local entry_id

    echo
    echo "Detected GRUB boot entries:"
    echo

    printf "%-6s %-14s %s\n" "No." "GRUB path" "Entry"
    printf "%-6s %-14s %s\n" "----" "---------" "-----"

    for line in "${GRUB_ENTRIES[@]}"; do
        IFS=$'\t' read -r selection numeric_path title_path entry_id <<< "${line}"
        printf "%-6s %-14s %s\n" "${selection}" "${numeric_path}" "${title_path}"
    done

    echo
}

resolve_default() {
    # Args:
    #     $1: GRUB_DEFAULT value.
    #
    # Returns:
    #     Prints a human-readable resolution of the default entry.
    local default_value="$1"
    local effective_value="${default_value}"
    local saved_entry
    local line
    local selection
    local numeric_path
    local title_path
    local entry_id

    if [[ "${default_value}" == "saved" ]]; then
        saved_entry="$(get_saved_grub_entry || true)"

        if [[ -z "${saved_entry}" ]]; then
            echo "GRUB_DEFAULT=saved, but no saved_entry was found in ${GRUB_ENV_FILE}."
            return
        fi

        effective_value="${saved_entry}"
        echo "GRUB_DEFAULT=saved"
        echo "Saved entry: ${saved_entry}"
    fi

    for line in "${GRUB_ENTRIES[@]}"; do
        IFS=$'\t' read -r selection numeric_path title_path entry_id <<< "${line}"

        if [[ "${effective_value}" == "${numeric_path}" ]]; then
            echo "Resolved default: [${selection}] ${title_path}"
            return
        fi

        if [[ "${effective_value}" == "${title_path}" ]]; then
            echo "Resolved default: [${selection}] ${title_path}"
            return
        fi

        if [[ -n "${entry_id}" && "${effective_value}" == "${entry_id}" ]]; then
            echo "Resolved default: [${selection}] ${title_path}"
            return
        fi
    done

    echo "Could not resolve current default from parsed entries."
    echo "Configured/effective value: ${effective_value}"
}

prompt_yes_no() {
    # Args:
    #     $1: Prompt text.
    #     $2: Default answer, y or n.
    #
    # Returns:
    #     Success for yes, failure for no.
    local prompt="$1"
    local default_answer="$2"
    local answer

    while true; do
        if [[ "${default_answer}" == "y" ]]; then
            read -r -p "${prompt} [Y/n]: " answer
            answer="${answer:-y}"
        else
            read -r -p "${prompt} [y/N]: " answer
            answer="${answer:-n}"
        fi

        case "${answer}" in
            y|Y|yes|YES)
                return 0
                ;;
            n|N|no|NO)
                return 1
                ;;
            *)
                echo "Please answer y or n."
                ;;
        esac
    done
}

choose_entry() {
    # Args:
    #     None.
    #
    # Returns:
    #     Prints the selected GRUB title path.
    local max_choice="${#GRUB_ENTRIES[@]}"
    local choice
    local line
    local selection
    local numeric_path
    local title_path
    local entry_id

    while true; do
        read -r -p "Select the new default entry number [1-${max_choice}]: " choice

        if ! [[ "${choice}" =~ ^[0-9]+$ ]]; then
            echo "Invalid input. Use a number."
            continue
        fi

        if (( choice < 1 || choice > max_choice )); then
            echo "Invalid choice. Use a number between 1 and ${max_choice}."
            continue
        fi

        for line in "${GRUB_ENTRIES[@]}"; do
            IFS=$'\t' read -r selection numeric_path title_path entry_id <<< "${line}"

            if [[ "${selection}" == "${choice}" ]]; then
                echo "${title_path}"
                return
            fi
        done
    done
}

backup_grub_default_file() {
    # Args:
    #     None.
    #
    # Returns:
    #     Prints the backup path.
    local backup_path="${GRUB_DEFAULT_FILE}.backup_${BACKUP_SUFFIX}"

    cp -a "${GRUB_DEFAULT_FILE}" "${backup_path}"
    echo "${backup_path}"
}

update_grub_default_value() {
    # Args:
    #     $1: New GRUB_DEFAULT value.
    #
    # Returns:
    #     None.
    local new_default="$1"

    python3 - "${GRUB_DEFAULT_FILE}" "${new_default}" <<'PY'
import sys
from pathlib import Path


def quote_grub_value(value):
    """Quotes a value for /etc/default/grub.

    Args:
        value: Raw GRUB_DEFAULT value.

    Returns:
        A safely double-quoted shell assignment value.
    """
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def update_grub_default(path, new_value):
    """Updates or appends GRUB_DEFAULT in /etc/default/grub.

    Args:
        path: Path to /etc/default/grub.
        new_value: New default boot entry value.

    Returns:
        None.
    """
    grub_file = Path(path)
    lines = grub_file.read_text(errors="replace").splitlines()
    output = []
    replaced = False

    for line in lines:
        stripped = line.lstrip()

        if not stripped.startswith("#") and stripped.startswith("GRUB_DEFAULT="):
            output.append(f"GRUB_DEFAULT={quote_grub_value(new_value)}")
            replaced = True
        else:
            output.append(line)

    if not replaced:
        output.append(f"GRUB_DEFAULT={quote_grub_value(new_value)}")

    grub_file.write_text("\n".join(output) + "\n")


def main():
    """Runs the GRUB_DEFAULT update.

    Args:
        None.

    Returns:
        None.
    """
    update_grub_default(sys.argv[1], sys.argv[2])


if __name__ == "__main__":
    main()
PY
}

disable_save_default_if_requested() {
    # Args:
    #     None.
    #
    # Returns:
    #     None.
    local current_save_default

    current_save_default="$(
        awk -F= '
            /^[[:space:]]*GRUB_SAVEDEFAULT[[:space:]]*=/ {
                value=$0
                sub(/^[[:space:]]*GRUB_SAVEDEFAULT[[:space:]]*=[[:space:]]*/, "", value)
                gsub(/["'\'']/, "", value)
                print value
                exit
            }
        ' "${GRUB_DEFAULT_FILE}" || true
    )"

    if [[ "${current_save_default}" != "true" ]]; then
        return
    fi

    echo
    echo "Warning: GRUB_SAVEDEFAULT=true is active."
    echo "That can make GRUB remember the last manually selected boot entry."

    if prompt_yes_no "Disable GRUB_SAVEDEFAULT so the selected default stays fixed?" "y"; then
        python3 - "${GRUB_DEFAULT_FILE}" <<'PY'
import sys
from pathlib import Path


def disable_save_default(path):
    """Disables GRUB_SAVEDEFAULT in /etc/default/grub.

    Args:
        path: Path to /etc/default/grub.

    Returns:
        None.
    """
    grub_file = Path(path)
    lines = grub_file.read_text(errors="replace").splitlines()
    output = []
    replaced = False

    for line in lines:
        stripped = line.lstrip()

        if not stripped.startswith("#") and stripped.startswith("GRUB_SAVEDEFAULT="):
            output.append("GRUB_SAVEDEFAULT=false")
            replaced = True
        else:
            output.append(line)

    if not replaced:
        output.append("GRUB_SAVEDEFAULT=false")

    grub_file.write_text("\n".join(output) + "\n")


def main():
    """Runs the GRUB_SAVEDEFAULT update.

    Args:
        None.

    Returns:
        None.
    """
    disable_save_default(sys.argv[1])


if __name__ == "__main__":
    main()
PY
    fi
}

main() {
    # Args:
    #     None.
    #
    # Returns:
    #     None.
    local current_default
    local selected_default
    local backup_path

    print_header
    require_root
    require_files_and_commands

    mapfile -t GRUB_ENTRIES < <(parse_grub_entries)

    if [[ "${#GRUB_ENTRIES[@]}" -eq 0 ]]; then
        echo "Error: no GRUB menu entries were detected."
        exit 1
    fi

    print_entries

    current_default="$(get_configured_grub_default)"

    echo "Current GRUB_DEFAULT value: ${current_default}"
    resolve_default "${current_default}"
    echo

    if ! prompt_yes_no "Change the default boot entry?" "y"; then
        echo "No changes made."
        exit 0
    fi

    selected_default="$(choose_entry)"

    echo
    echo "New selected default:"
    echo "${selected_default}"
    echo

    if ! prompt_yes_no "Apply this change?" "n"; then
        echo "No changes made."
        exit 0
    fi

    backup_path="$(backup_grub_default_file)"
    echo "Backup created: ${backup_path}"

    update_grub_default_value "${selected_default}"
    disable_save_default_if_requested

    echo
    echo "Running update-grub..."
    update-grub

    echo
    echo "Done."
    echo "New GRUB_DEFAULT value:"
    grep -E '^[[:space:]]*GRUB_DEFAULT=' "${GRUB_DEFAULT_FILE}"
    echo
    echo "Reboot to test the new default boot entry."
}

main "$@"