#!/bin/sh
set -eu

repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM
mkdir -p "$test_root/bin" "$test_root/zsh"

cat > "$test_root/bin/tb" <<'SH'
#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ "$1" != "__complete" ]; then
    exit 2
fi
printf '%s\n' help install-codex install-gh list uninstall update version
SH
chmod 755 "$test_root/bin/tb"

bash_first=$(PATH="$test_root/bin:$PATH" bash -c '
    . "$1/completions/tb.bash"
    COMP_WORDS=(tb in)
    COMP_CWORD=1
    _tb_completion
    printf "%s\n" "${COMPREPLY[@]}"
' bash "$repository_root")
if [ "$bash_first" != "install-codex
install-gh" ]; then
    printf 'Bash first-argument completion = %s\n' "$bash_first" >&2
    exit 1
fi

bash_later=$(PATH="$test_root/bin:$PATH" bash -c '
    . "$1/completions/tb.bash"
    COMP_WORDS=(tb install-codex --)
    COMP_CWORD=2
    COMPREPLY=(toolbox-suggestion)
    _tb_completion
    printf "%s" "${COMPREPLY[*]}"
' bash "$repository_root")
if [ -n "$bash_later" ]; then
    printf 'Bash later-argument completion returned %s.\n' "$bash_later" >&2
    exit 1
fi

zsh_first=$(PATH="$test_root/bin:$PATH" ZDOTDIR="$test_root/zsh" zsh -f -c '
    source "$1/completions/_tb"
    function compadd {
        if [[ "$1" == -- ]]; then
            shift
        fi
        local candidate
        for candidate in "$@"; do
            if [[ "$candidate" == ${words[CURRENT]}* ]]; then
                print -r -- "$candidate"
            fi
        done
    }
    words=(tb in)
    CURRENT=2
    _tb_completion
' zsh "$repository_root")
if [ "$zsh_first" != "install-codex
install-gh" ]; then
    printf 'Zsh first-argument completion = %s\n' "$zsh_first" >&2
    exit 1
fi

zsh_later=$(PATH="$test_root/bin:$PATH" ZDOTDIR="$test_root/zsh" zsh -f -c '
    source "$1/completions/_tb"
    function compadd {
        print -rl -- "$@"
    }
    words=(tb install-codex --)
    CURRENT=3
    _tb_completion
' zsh "$repository_root")
if [ -n "$zsh_later" ]; then
    printf 'Zsh later-argument completion returned %s.\n' "$zsh_later" >&2
    exit 1
fi
