# Your own settings belong in ~/.bashrc.local (or ~/.config/shell/local.sh for
# bash and zsh). dotfiles never touches those files. Lines which installers
# (conda, nvm, sdkman, ...) append to this file are moved to ~/.bashrc.local
# by the next `dotfiles apply`.

# System wide settings (Fedora, RHEL. Debian and Ubuntu read
# /etc/bash.bashrc without help.)
if [ -f /etc/bashrc ]; then
    source /etc/bashrc
fi

# Source common configuration
if [[ -f "$HOME/.config/shell/common.sh" ]]; then
    source "$HOME/.config/shell/common.sh"
fi

if [[ $- == *i* ]]; then
    # Bash completion (from the system package: apt install bash-completion)
    if ! declare -F _init_completion >/dev/null; then
        if [ -f /usr/share/bash-completion/bash_completion ]; then
            source /usr/share/bash-completion/bash_completion
        elif [ -f /etc/bash_completion ]; then
            source /etc/bash_completion
        fi
    fi

    # Keep the history of all open terminals; Atuin (Ctrl-R) searches it.
    shopt -s histappend checkwinsize
    HISTSIZE=10000
    HISTFILESIZE=20000
    HISTCONTROL=ignoreboth

    # FZF Support
    if command -v fzf >/dev/null; then
        eval "$(fzf --bash)"

        # `fzf --bash` binds Ctrl-R to fzf's own history search, overwriting the
        # binding Atuin installed in ~/.config/shell/common.sh. Atuin is the
        # better history search, so hand Ctrl-R back to it (atuin-bind is
        # defined by `atuin init bash`; Zsh never loads fzf's key bindings).
        if declare -F atuin-bind >/dev/null; then
            atuin-bind -m emacs '\C-r' atuin-search-emacs
            atuin-bind -m vi-insert '\C-r' atuin-search-viins
        fi

        # Previews for `**<TAB>` completion
        _fzf_comprun() {
            local command=$1
            shift

            case "$command" in
                cd) fzf --preview 'eza --tree --color=always {} | head -200' "$@" ;;
                export | unset) fzf --preview "eval 'echo \$'{}" "$@" ;;
                ssh) fzf --preview 'dig {}' "$@" ;;
                *) fzf --preview 'bat -n --color=always {}' "$@" ;;
            esac
        }
    fi

    if command -v kubectl >/dev/null; then
        complete -o default -F __start_kubectl k 2>/dev/null || true
    fi

    # Interactive Tab Menu (Roughly the same as Zsh menu select)
    bind 'set show-all-if-ambiguous on'
    bind 'set menu-complete-display-prefix on'
    bind 'set colored-stats on'
    bind 'TAB:menu-complete'
    bind '"\e[Z": menu-complete-backward' # Shift-Tab cycles backward

    # Smart Matching
    bind 'set completion-ignore-case on' # Ignore case
    bind 'set mark-directories on'       # Add / to directories
    bind 'set visible-stats on'          # Append file type indicators
fi

# Your own settings (not managed by dotfiles).
if [ -f "$HOME/.bashrc.local" ]; then
    source "$HOME/.bashrc.local"
fi
