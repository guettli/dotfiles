# Source common configuration
if [[ -f "$HOME/.config/shell/common.sh" ]]; then
    source "$HOME/.config/shell/common.sh"
fi

if [[ $- == *i* ]]; then
    # Bash completion (from the system package: apt install bash-completion)
    if [ -f /usr/share/bash-completion/bash_completion ]; then
        source /usr/share/bash-completion/bash_completion
    elif [ -f /etc/bash_completion ]; then
        source /etc/bash_completion
    fi

    # FZF Support
    if command -v fzf >/dev/null; then
        eval "$(fzf --bash)"
        
        # Intercept TAB to use FZF for file completion
        _fzf_comprun() {
            local command=$1
            shift

            case "$command" in
                cd) fzf --preview 'tree -C {} | head -200' "$@" ;;
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
