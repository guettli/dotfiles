typeset -U path
path=(
    $HOME/.nix-profile/bin
    $HOME/bin
    $HOME/scripts
    $HOME/syself/dotfiles/bin
    $HOME/.local/bin
    $HOME/go/bin
    $HOME/syself/app-catalog/scripts
    $HOME/projects/git-tips/scripts
    $path
)

export K9S_FEATURE_GATE_NODE_SHELL=true
if [[ "$TERM_PROGRAM" == "vscode" ]]; then
    export EDITOR="code --wait"
else
    export EDITOR="vim"
fi
export LESS="-Ri"

if [[ -o interactive ]]; then
    setopt interactive_comments

    # External Tool Initializations
    eval "$(atuin init zsh --disable-up-arrow)"
    eval "$(starship init zsh)"

    if [ -d "$HOME/.nix-profile/share/zsh/site-functions" ]; then
        fpath=("$HOME/.nix-profile/share/zsh/site-functions" $fpath)
    fi

    # Antidote Plugin Management
    if [[ -f "$HOME/.nix-profile/share/antidote/antidote.zsh" ]]; then
        source "$HOME/.nix-profile/share/antidote/antidote.zsh"
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

    # Aliases
    alias k="kubectl"
    alias toClipboard="xclip -sel clip"
    alias fromClipboard="xclip -sel clip -out"

    if [[ "$TERM_PROGRAM" == "vscode" ]]; then
        alias rg="rg --no-heading --column --hidden --glob '!.git/*'"
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

    # Direnv (Load last)
    eval "$(direnv hook zsh)"
fi

# NVM (optional, only active if nvm is installed)
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"

# Dart (optional, only active if dart is installed)
[[ -f $HOME/.dart-cli-completion/zsh-config.zsh ]] && . $HOME/.dart-cli-completion/zsh-config.zsh || true

skip_global_compinit=1
