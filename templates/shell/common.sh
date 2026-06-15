# Detect shell type
if [ -n "$ZSH_VERSION" ]; then
    shell_type="zsh"
elif [ -n "$BASH_VERSION" ]; then
    shell_type="bash"
else
    shell_type="sh"
fi

# Path deduplication and preparation (compatible with both bash and zsh)
pathadd() {
    # If directory is not already in PATH, prepend it
    case ":$PATH:" in
        *":$1:"*) ;;
        *) PATH="$1:$PATH" ;;
    esac
}
pathadd "$HOME/projects/git-tips/scripts"
pathadd "$HOME/syself/app-catalog/scripts"
pathadd "$HOME/go/bin"
pathadd "$HOME/.local/bin"
pathadd "$HOME/syself/dotfiles/bin"
pathadd "$HOME/scripts"
pathadd "$HOME/bin"
pathadd "$HOME/.nix-profile/bin"
unset -f pathadd
export PATH

export K9S_FEATURE_GATE_NODE_SHELL=true
if [[ "$TERM_PROGRAM" == "vscode" ]]; then
    export EDITOR="code --wait"
else
    export EDITOR="vim"
fi
export LESS="-Ri"
export RIPGREP_CONFIG_PATH="$HOME/.config/ripgrep/ripgreprc"

# Check if shell is interactive
is_interactive=false
case $- in
    *i*) is_interactive=true ;;
esac

if [ "$is_interactive" = true ]; then
    # External Tool Initializations
    if command -v atuin >/dev/null; then
        eval "$(atuin init $shell_type --disable-up-arrow)"
    fi
    if command -v starship >/dev/null; then
        eval "$(starship init $shell_type)"
    fi

    # Aliases
    alias k="kubectl"
    alias toClipboard="xclip -sel clip"
    alias fromClipboard="xclip -sel clip -out"

    # Direnv (Load last)
    if command -v direnv >/dev/null; then
        eval "$(direnv hook $shell_type)"
    fi
fi

# NVM (optional, only active if nvm is installed)
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
if [ "$shell_type" = "bash" ]; then
    [ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"
fi

# Dart (optional, only active if dart is installed)
if [ "$shell_type" = "zsh" ]; then
    [[ -f $HOME/.dart-cli-completion/zsh-config.zsh ]] && . $HOME/.dart-cli-completion/zsh-config.zsh || true
elif [ "$shell_type" = "bash" ]; then
    [[ -f $HOME/.dart-cli-completion/bash-config.bash ]] && . $HOME/.dart-cli-completion/bash-config.bash || true
fi
