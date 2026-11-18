# Shared by ~/.bashrc and ~/.zshrc.
#
# Your own settings belong in ~/.config/shell/local.sh (bash and zsh),
# ~/.bashrc.local or ~/.zshrc.local. dotfiles never touches those files.

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
pathadd "$HOME/go/bin"
pathadd "$HOME/bin"
pathadd "$HOME/.local/share/mise/shims"
pathadd "$HOME/.local/bin"
export PATH

# A beginner friendly editor (Ctrl-S saves, Ctrl-Q quits). Set EDITOR in
# ~/.config/shell/local.sh to use something else.
if [ "$TERM_PROGRAM" = "vscode" ]; then
    export EDITOR="code --wait"
elif command -v micro >/dev/null; then
    export EDITOR="micro"
else
    export EDITOR="nano"
fi
export LESS="-Ri"
export RIPGREP_CONFIG_PATH="$HOME/.config/ripgrep/ripgreprc"
# Same config location on macOS as on Linux.
export TEALDEER_CONFIG_DIR="$HOME/.config/tealdeer"

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

    if command -v mise >/dev/null; then
        eval "$(mise activate $shell_type)"
    fi

    # `z foo` jumps to the most used directory matching "foo".
    if command -v zoxide >/dev/null; then
        eval "$(zoxide init $shell_type)"
    fi

    # Aliases
    if ls --color=auto -d . >/dev/null 2>&1; then
        alias ls="ls --color=auto"
    fi
    alias ll="ls -alF"
    alias la="ls -A"
    alias grep="grep --color=auto"
    alias k="kubectl"

    # Copy to / paste from the clipboard: `cat file | toClipboard`
    toClipboard() {
        if [ -n "$WAYLAND_DISPLAY" ] && command -v wl-copy >/dev/null; then
            wl-copy
        elif command -v xclip >/dev/null; then
            xclip -sel clip
        elif command -v pbcopy >/dev/null; then
            pbcopy
        else
            echo "toClipboard: install wl-clipboard (Wayland) or xclip (X11)" >&2
            return 1
        fi
    }
    fromClipboard() {
        if [ -n "$WAYLAND_DISPLAY" ] && command -v wl-paste >/dev/null; then
            wl-paste
        elif command -v xclip >/dev/null; then
            xclip -sel clip -out
        elif command -v pbpaste >/dev/null; then
            pbpaste
        else
            echo "fromClipboard: install wl-clipboard (Wayland) or xclip (X11)" >&2
            return 1
        fi
    }

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

# Settings of the profile chosen in ~/.config/dotfiles/config.toml (managed,
# empty without a profile).
if [ -f "$HOME/.config/shell/profile.sh" ]; then
    . "$HOME/.config/shell/profile.sh"
fi

# Your own settings for bash and zsh (not managed).
if [ -f "$HOME/.config/shell/local.sh" ]; then
    . "$HOME/.config/shell/local.sh"
fi
