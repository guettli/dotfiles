{ config, pkgs, ... }:
let
  user = builtins.getEnv "USER";
in {
  home.username = user;
  home.homeDirectory = "/home/${user}";

  # Match your installed home-manager channel version. Do not change after first switch.
  home.stateVersion = "25.11";

  programs.home-manager.enable = true;

  home.packages = with pkgs; [
    antidote
    k9s
    xclip
  ];

  # ── Zsh ─────────────────────────────────────────────────────────────────────

  programs.zsh = {
    enable = true;
    enableCompletion = false; # handled manually below for -u flag

    shellAliases = {
      k = "kubectl";
      toClipboard = "xclip -sel clip";
      fromClipboard = "xclip -sel clip -out";
    };

    envExtra = ''
      skip_global_compinit=1
    '';

    initExtra = ''
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
      export EDITOR="code -w"
      export LESS="-Ri"

      if [ -d "$HOME/.nix-profile/share/zsh/site-functions" ]; then
          fpath=("$HOME/.nix-profile/share/zsh/site-functions" $fpath)
      fi

      # Antidote plugin management
      source ${pkgs.antidote}/share/antidote/antidote.zsh
      zdir="$HOME/.config/zsh"
      zplugins="$zdir/plugins.txt"
      zstatic="$zdir/plugins.zsh"
      if [[ ! -f "$zstatic" || "$zplugins" -nt "$zstatic" ]]; then
          antidote bundle <"$zplugins" >"$zstatic"
      fi
      source "$zstatic"

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
    '';
  };

  home.file.".config/zsh/plugins.txt".text = ''
    zsh-users/zsh-autosuggestions
    zsh-users/zsh-syntax-highlighting
    zsh-users/zsh-completions
  '';

  # ── Starship ─────────────────────────────────────────────────────────────────

  programs.starship = {
    enable = true;
    settings = {
      kubernetes.disabled = false;
      golang.disabled = true;
      gcloud.disabled = true;
      python.disabled = true;
      nodejs.disabled = true;
      direnv = {
        disabled = false;
        symbol = "";
        allowed_msg = "";
        loaded_msg = "";
        not_allowed_msg = " 🚫 not allowed";
        denied_msg = " 🔒 denied";
        unloaded_msg = " 🌪 not loaded";
        format = "[$symbol$loaded$allowed]($style) ";
        style = "bold yellow";
      };
      time = {
        disabled = false;
        format = "[$time]($style)\\n";
        time_format = "%H:%M";
      };
      git_status = {
        conflicted = " conflicted";
        ahead = " ahead";
        behind = " behind";
        diverged = " diverged";
        up_to_date = "";
        stashed = " stashes";
        untracked = " untracked";
        modified = "m";
      };
    };
  };

  # ── Atuin ────────────────────────────────────────────────────────────────────

  programs.atuin = {
    enable = true;
    flags = [ "--disable-up-arrow" ];
    settings = {
      enter_accept = true;
      sync.records = true;
      search.filters = [ "directory" "global" ];
    };
  };

  # ── Direnv ───────────────────────────────────────────────────────────────────

  programs.direnv = {
    enable = true;
    nix-direnv.enable = true;
    config = {
      global.hide_env_diff = true;
      whitelist.prefix = [ "/" ];
    };
  };

  # ── Tmux ─────────────────────────────────────────────────────────────────────

  programs.tmux = {
    enable = true;
    mouse = true;
    extraConfig = ''
      # Shift+click to copy with mouse (plain click just selects)
      unbind -T copy-mode-vi MouseDragEnd1Pane
    '';
  };
}
