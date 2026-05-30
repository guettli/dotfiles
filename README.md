# dotfiles

Shell environment managed with [Home Manager](https://github.com/nix-community/home-manager).

## What's included

| Tool | Purpose |
|---|---|
| [Zsh](https://www.zsh.org/) | Shell |
| [Antidote](https://github.com/mattmc3/antidote) | Zsh plugin manager |
| [Starship](https://starship.rs/) | Prompt |
| [Atuin](https://github.com/atuinsh/atuin) | Shell history |
| [direnv](https://direnv.net/) + [nix-direnv](https://github.com/nix-community/nix-direnv) | Per-directory env vars |
| [tmux](https://github.com/tmux/tmux) | Terminal multiplexer |

## Setup

### Prerequisites

[Nix](https://github.com/DeterminateSystems/nix-installer) must be installed.

### First time on a new account

Back up any existing shell config files that Home Manager will manage:

```bash
for f in ~/.zshrc ~/.zshenv ~/.config/zsh/plugins.txt ~/.config/starship.toml \
          ~/.config/atuin/config.toml ~/.config/direnv/direnv.toml; do
  [ -f "$f" ] && mv "$f" "${f}.bak"
done
```

Clone and activate:

```bash
git clone git@github.com:guettli/dotfiles.git ~/.config/home-manager
NIXPKGS=$(nix --extra-experimental-features 'nix-command flakes' flake metadata nixpkgs --json | python3 -c "import sys,json; print(json.load(sys.stdin)['path'])")
nix --extra-experimental-features 'nix-command flakes' run nixpkgs#home-manager -- switch -I nixpkgs=$NIXPKGS
```

> If `~/.config/home-manager` already exists, replace the clone with:
> ```bash
> git -C ~/.config/home-manager pull
> ```

### Applying changes

Edit `home.nix`, then:

```bash
NIXPKGS=$(nix flake metadata nixpkgs --json | python3 -c "import sys,json; print(json.load(sys.stdin)['path'])")
nix run nixpkgs#home-manager -- switch -I nixpkgs=$NIXPKGS
```
