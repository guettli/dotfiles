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
| [mise](https://mise.jdx.dev/) | Installs and manages all the tools above | Binary |
| [tmux](https://github.com/tmux/tmux) | Terminal multiplexer | Binary + Config |

## How tools are installed

The base tools (Starship, Atuin, direnv, tmux) are declared in
[`templates/mise/dotfiles.toml`](templates/mise/dotfiles.toml). `apply` deploys that file to
`~/.config/mise/conf.d/dotfiles.toml` and then runs `mise install`, so mise itself decides what is
missing and installs it — the Go tool keeps no tool bookkeeping of its own.

To install **extra / machine-specific tools**, use mise directly:

```bash
mise use -g <tool>
```

That writes `~/.config/mise/config.toml`, a separate file dotfiles never touches — so your personal
tools and the managed base set never fight over the same file.

Antidote is not in the mise registry, so it is installed separately via `git clone` into
`~/.antidote`.

## Usage

The templates are embedded into the Go binary using `go:embed`, so you can run the installer
directly from GitHub on any new machine.

### Prerequisites

You need [Go](https://go.dev/install) and [mise](https://mise.jdx.dev/getting-started.html)
installed. *(Zsh is expected to be installed via your system package manager.)*

### Configuration

Create `~/.config/dotfiles/config.toml` (TOML) from
[`config.example.toml`](config.example.toml):

```toml
name = "Your Name"
personal_email = "you@example.com"

[[orgs]]
url   = "github.com/your-company"
email = "you@your-company.com"
```

`orgs` generates a per-org `~/.gitconfig-org-<name>` so repos under that org use the matching git
email.

> **Upgrading from the old YAML config?** The config format is now TOML and there is **no**
> backwards compatibility. If a `~/.config/dotfiles/config.yaml` is present, the tool prints a
> migration hint and refuses to run — convert it to `config.toml` (see above) and delete the old
> file. The old `mise_tools:` key was removed; install extra tools with `mise use -g <tool>`
> instead.

### 1. View Pending Changes (Diff)

To see what changes the tool *would* make to your machine without modifying anything (this also
lists which mise tools are missing), run:

```bash
go run github.com/guettli/dotfiles@latest diff
```

### 2. Apply Changes (Installation)

To install the tools (via mise) and deploy your dotfiles:

```bash
go run github.com/guettli/dotfiles@latest apply
```

To overwrite local modifications, use the `--force` flag:

```bash
go run github.com/guettli/dotfiles@latest apply --force
```

**Overwrite Protection:** The tool maintains a hidden cache of what it previously installed. If you
have made un-tracked manual edits to a config file (e.g., you edited `~/.zshrc` directly), the
`apply` command will **abort** and show you a diff, preventing accidental data loss. You can bypass
this with `--force`.

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
   go run . diff
   go run . apply
   ```
4. Commit and push. You can immediately run `go run github.com/guettli/dotfiles@latest apply` on
   your other machines to sync.
