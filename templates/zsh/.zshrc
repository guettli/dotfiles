# Managed by custom Go dotfile manager

typeset -U path
path=(
    $HOME/.nix-profile/bin
    $HOME/bin
    $HOME/scripts
    $HOME/.local/bin
    $HOME/go/bin
    $path
)

export K9S_FEATURE_GATE_NODE_SHELL=true
if [[ "$TERM_PROGRAM" == "vscode" ]]; then
    export EDITOR="code --wait"
else
    export EDITOR="vim"
fi
export LESS="-Ri"

if [ -d "$HOME/.nix-profile/share/zsh/site-functions" ]; then
    fpath=("$HOME/.nix-profile/share/zsh/site-functions" $fpath)
fi

# Antidote plugin management
# We assume antidote is installed via Nix profile or package manager now
if [[ -f "$HOME/.nix-profile/share/antidote/antidote.zsh" ]]; then
  source "$HOME/.nix-profile/share/antidote/antidote.zsh"
else
  # Fallback if installed elsewhere, e.g., brew or manual clone
  # source /path/to/antidote.zsh
  echo "Warning: antidote not found at ~/.nix-profile/share/antidote/antidote.zsh"
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
zstyle ':completion:*' matcher-list \
    'm:{a-zA-Z}={A-Za-z}' \
    'r:|[._-]=* r:|=*' \
    'l:|=* r:|=*'
zstyle ':completion:*' file-sort modification
zstyle ':completion:*:default' list-prompt ""
zstyle ':completion:*' menu select=long-list

LISTMAX=500
autoload -Uz compinit && compinit -u

bindkey '^ ' autosuggest-accept
PROMPT_EOL_MARK=""

if [[ "$TERM_PROGRAM" == "vscode" ]]; then
    alias rg="rg --no-heading --column --hidden --glob '!.git/*'"
fi

# NVM (optional, only active if nvm is installed)
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"

# Dart (optional, only active if dart is installed)
[[ -f $HOME/.dart-cli-completion/zsh-config.zsh ]] && . $HOME/.dart-cli-completion/zsh-config.zsh || true

# Aliases
alias k="kubectl"
alias toClipboard="xclip -sel clip"
alias fromClipboard="xclip -sel clip -out"

skip_global_compinit=1
