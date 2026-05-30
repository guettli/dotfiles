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
| [k9s](https://k9scli.io/) | Kubernetes TUI |

## Setup

### Prerequisites

[Nix](https://github.com/DeterminateSystems/nix-installer) must be installed.

### First time on a new account

```bash
git clone git@github.com:guettli/dotfiles.git ~/.config/home-manager
nix run nixpkgs#home-manager -- switch
```

> If you have existing `~/.zshrc` or `~/.zshenv`, back them up first —
> Home Manager will not overwrite files it does not manage:
> ```bash
> mv ~/.zshrc ~/.zshrc.bak
> mv ~/.zshenv ~/.zshenv.bak
> ```

### Applying changes

Edit `home.nix`, then:

```bash
home-manager switch
```
