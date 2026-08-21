# Source common configuration
if [[ -f "$HOME/.config/shell/common.sh" ]]; then
    source "$HOME/.config/shell/common.sh"
fi

# Ensure unique path variable for Zsh
typeset -U path

if [[ -o interactive ]]; then
    setopt interactive_comments

    # Antidote Plugin Management (installed via git clone into ~/.antidote)
    if [[ -f "$HOME/.antidote/antidote.zsh" ]]; then
        source "$HOME/.antidote/antidote.zsh"
    fi

    zdir="$HOME/.config/zsh"
    zplugins="$zdir/plugins.txt"
    zstatic="$zdir/plugins.zsh"

    if [[ ! -f "$zstatic" || "$zplugins" -nt "$zstatic" ]]; then
        if command -v antidote >/dev/null; then
            antidote bundle <"$zplugins" >"$zstatic"
        fi
    fi
    if [[ -f "$zstatic" ]]; then
        source "$zstatic"
    fi

    zstyle ':completion:*' menu select

    # Make autocomplete fuzzy (matching *foo*, not just foo*)
    zstyle ':completion:*' matcher-list \
        'm:{a-zA-Z}={A-Za-z}' \
        'r:|[._-]=* r:|=*' \
        'l:|=* r:|=*'

    # Sort file completions by modification time (newest first)
    zstyle ':completion:*' file-sort modification

    # This forces Zsh to never ask 'do you wish to see all...'
    zstyle ':completion:*:default' list-prompt ""

    # This ensures the menu selection starts immediately even for huge lists
    zstyle ':completion:*' menu select=long-list

    LISTMAX=500

    autoload -Uz compinit && compinit -u

    # Keybindings
    bindkey '^ ' autosuggest-accept
    bindkey -e

    # Do not show "%" marker when command output has no trailing newline.
    PROMPT_EOL_MARK=""
fi

skip_global_compinit=1
