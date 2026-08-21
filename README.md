# dotfiles

Shell environment managed with a custom embedded Go tool.

I tried [chezmoi](https://www.chezmoi.io/) and
[home-manager](https://github.com/nix-community/home-manager), but somehow these tools felt too
complicated. This small Go application works fine for me.

## Software and Configuration

| Tool | Purpose | Managed |
| --- | --- | --- |
| [Zsh](https://www.zsh.org/) | Shell | **Config only** |
| [Antidote](https://github.com/mattmc3/antidote) | Zsh plugin manager | Binary + Config |
| [Starship](https://starship.rs/) | Prompt | Binary + Config |
| [Atuin](https://github.com/atuinsh/atuin) | Shell history | Binary + Config |
| [direnv](https://direnv.net/) | Per-directory env vars | Binary + Config |
| [mise](https://mise.jdx.dev/) | Per-project tool versions/env vars **and the installer for the tools above** | Binary |
| [tmux](https://github.com/tmux/tmux) | Terminal multiplexer | Binary + Config |

## Usage

The templates are embedded into the Go binary using `go:embed`, you can run the installer directly
from GitHub on any new machine.

### Prerequisites

You need [Go](https://go.dev/doc/install) and [mise](https://mise.jdx.dev/) installed. The installer uses mise to install the required tools. *(Note: Zsh is expected to be installed via your system package manager.)*

### 1. View Pending Changes (Diff)

To see what changes the tool *would* make to your machine without modifying anything (this will also list which mise dependencies are missing), run:

```bash
go run github.com/guettli/dotfiles@latest diff
```

### 2. Apply Changes (Installation)

To safely install the required dependencies (via mise) and deploy your dotfiles to a machine, run:

```bash
go run github.com/guettli/dotfiles@latest apply
```

To overwrite local modifications, use the `--force` flag:

```bash
go run github.com/guettli/dotfiles@latest apply --force
```

**Dependency Installation:** It will automatically check your mise tools (via `mise which`) and install any missing ones (Starship, Atuin, direnv, tmux) with `mise use -g`. Antidote is installed separately via `git clone` into `~/.antidote`, since it is not in the mise registry.

**Overwrite Protection:** The tool maintains a hidden cache of what it previously installed. If you have made un-tracked manual edits to a config file (e.g., you edited `~/.zshrc` directly), the `apply` command will **abort** and show you a diff, preventing accidental data loss. You can bypass this with `--force`.

---

## Developing

If you want to edit the configurations:

1. Clone the repository locally:
   ```bash
   git clone git@github.com:guettli/dotfiles.git
   cd dotfiles
   ```
2. Modify the files inside the `templates/` directory.
3. Test your changes locally before committing:
   ```bash
   go run main.go diff
   go run main.go apply
   ```
4. Commit and push. You can immediately run `go run github.com/guettli/dotfiles@latest apply` on your other machines to sync.
