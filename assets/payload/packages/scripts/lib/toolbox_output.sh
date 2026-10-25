#!/bin/sh

# Shared terminal output and prompt helpers for toolbox shell tools.

tb_output_init() {
    TB_OUTPUT_TOOL=$1
    TB_OUTPUT_STARTED=0
    if [ -n "${NO_COLOR+x}" ]; then
        TB_OUTPUT_MODE=never
    elif [ -n "${TB_OUTPUT_COLOR:-}" ]; then
        TB_OUTPUT_MODE=$TB_OUTPUT_COLOR
    elif [ -t 1 ] && [ "${TERM:-}" != dumb ]; then
        case "${TERM:-}" in
            *256color*) TB_OUTPUT_MODE=256 ;;
            *) TB_OUTPUT_MODE=basic ;;
        esac
    else
        TB_OUTPUT_MODE=never
    fi
}

tb_output_time() {
    if [ -n "${TB_OUTPUT_CLOCK:-}" ]; then
        printf '%s' "$TB_OUTPUT_CLOCK"
    else
        date '+%H:%M:%S'
    fi
}

tb_output_line() {
    tb_level=$1
    tb_message=$2
    tb_color=$3
    if [ "$TB_OUTPUT_STARTED" -eq 0 ]; then
        printf '\n'
        TB_OUTPUT_STARTED=1
    fi
    tb_prefix="[$(tb_output_time) $TB_OUTPUT_TOOL $tb_level] $tb_message"
    if [ "$TB_OUTPUT_MODE" != never ] && [ -n "$tb_color" ]; then
        printf '\033[%sm%s\033[0m\n' "$tb_color" "$tb_prefix"
    else
        printf '%s\n' "$tb_prefix"
    fi
}

tb_info() {
    tb_output_line INFO "$1" ''
}

tb_prompt() {
    tb_output_line PROMPT "$1" 36
}

tb_menu() {
    tb_prompt "$1"
    shift
    for tb_menu_option in "$@"; do
        if [ "$TB_OUTPUT_MODE" != never ]; then
            printf '\033[36m  %s\033[0m\n' "$tb_menu_option"
        else
            printf '  %s\n' "$tb_menu_option"
        fi
    done
}

tb_warn() {
    tb_warning_color=33
    if [ "$TB_OUTPUT_MODE" = 256 ]; then
        tb_warning_color='38;5;208'
    fi
    tb_output_line WARN "$1" "$tb_warning_color"
}

tb_error() {
    tb_output_line ERROR "$1" 31
}

tb_success() {
    tb_output_line SUCCESS "$1" 32
}

tb_ask_yes_no() {
    tb_question=$1
    tb_default=$2
    while true; do
        if [ "$tb_default" = yes ] || [ "$tb_default" = y ]; then
            tb_marker='Y/n'
            tb_default_name=Y
        else
            tb_marker='y/N'
            tb_default_name=n
        fi
        tb_prompt "$tb_question [$tb_marker] (default: $tb_default_name):"
        if ! IFS= read -r TB_ANSWER; then
            return 1
        fi
        case "$TB_ANSWER" in
            '') TB_ANSWER=$tb_default; return 0 ;;
            y|Y|yes|YES|Yes) TB_ANSWER=yes; return 0 ;;
            n|N|no|NO|No) TB_ANSWER=no; return 0 ;;
            *) tb_warn "Enter y or n, or press Enter for $tb_default_name." ;;
        esac
    done
}

tb_ask_required() {
    tb_question=$1
    while true; do
        tb_prompt "$tb_question: "
        if ! IFS= read -r TB_ANSWER; then
            return 1
        fi
        if [ -n "$TB_ANSWER" ]; then
            return 0
        fi
        tb_warn "A value is required; enter a value to continue."
    done
}
