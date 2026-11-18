# Your own settings belong in ~/.zshrc.local (or ~/.config/shell/local.sh for
# bash and zsh). dotfiles never touches those files. Lines which installers
# append to this file are moved to ~/.zshrc.local by the next `dotfiles apply`.

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

    # kubectl completion (also for the `k` alias), from the copy `dotfiles
    # apply` caches; created here once if kubectl came later.
    if command -v kubectl >/dev/null; then
        _kubectl_comp="$HOME/.cache/dotfiles/completions/kubectl.zsh"
        if [[ ! -s "$_kubectl_comp" ]]; then
            mkdir -p "${_kubectl_comp:h}"
            kubectl completion zsh >"$_kubectl_comp.tmp" 2>/dev/null &&
                mv "$_kubectl_comp.tmp" "$_kubectl_comp" || rm -f "$_kubectl_comp.tmp"
        fi
        if [[ -s "$_kubectl_comp" ]]; then
            source "$_kubectl_comp"
            compdef k=kubectl
        fi
        unset _kubectl_comp
    fi

    # Keybindings
    bindkey '^ ' autosuggest-accept
    bindkey -e

    # Do not show "%" marker when command output has no trailing newline.
    PROMPT_EOL_MARK=""
fi

skip_global_compinit=1

# Your own settings (not managed by dotfiles).
if [[ -f "$HOME/.zshrc.local" ]]; then
    source "$HOME/.zshrc.local"
fi
