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
| [direnv](https://direnv.net/) + [nix-direnv](https://github.com/nix-community/nix-direnv) | Per-directory env vars | Binary + Config |
| [tmux](https://github.com/tmux/tmux) | Terminal multiplexer | Binary + Config |

## Usage

The templates are embedded into the Go binary using `go:embed`, you can run the installer directly
from GitHub on any new machine.

### Prerequisites

You need [Go](https://go.dev/doc/install) installed. *(Note: Zsh is expected to be installed via your system package manager or existing Nix profile.)*

### 1. View Pending Changes (Diff)

To see what changes the tool *would* make to your machine without modifying anything (this will also list which Nix dependencies are missing), run:

```bash
go run github.com/guettli/dotfiles@latest diff
```

### 2. Apply Changes (Installation)

To safely install the required Nix dependencies and deploy your dotfiles to a machine, run:

```bash
go run github.com/guettli/dotfiles@latest apply
```

**Dependency Installation:** It will automatically check your `nix profile` and install any missing tools (Starship, Atuin, Direnv, Tmux, Antidote, xclip).

**Overwrite Protection:** The tool maintains a hidden cache of what it previously installed. If you have made un-tracked manual edits to a config file (e.g., you edited `~/.zshrc` directly), the `apply` command will **abort** and show you a diff, preventing accidental data loss.

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
