# dotfiles

A good default shell environment for Linux and macOS, installed with one command.

You get:

- **Ctrl-R** searches every command you ever typed ([Atuin](https://github.com/atuinsh/atuin))
- **a prompt that shows** the git branch, changes and Kubernetes context ([Starship](https://starship.rs/))
- **Tab completion with a menu**, and `**<Tab>` fuzzy file search ([fzf](https://github.com/junegunn/fzf))
- **`z proj`** jumps to `~/projects` ([zoxide](https://github.com/ajeetdsouza/zoxide))
- **friendly tools:** `micro` (an editor where Ctrl-S saves), `lazygit`, `bat`, `eza`,
  `rg`, `fd`, `tldr`
- **git set up for you:** your name and email, a separate work email for your company's
  repositories, and `prek` pre-commit hooks

It changes only your user account. It needs no root and no `sudo`, and you can undo it with
`dotfiles uninstall`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/guettli/dotfiles/main/install.sh | sh
```

The script asks for your name and email, then installs everything. Open a new terminal
afterwards, and run:

```bash
dotfiles tips     # what the new shell can do
dotfiles doctor   # check the setup; it prints the command for each problem it finds
```

You need `curl`, `git` and `tar`. On Ubuntu, `sudo apt-get install bash-completion` gives you
completion for more commands; `dotfiles doctor` tells you if it is missing.

### Try it first

Try it in a throwaway container. It does not touch your machine:

```bash
docker run --rm -it ubuntu:24.04 bash -c \
  'apt-get update && apt-get install -y curl git && curl -fsSL https://raw.githubusercontent.com/guettli/dotfiles/main/install.sh | sh && exec bash'
```

## Commands

| Command | What it does |
| --- | --- |
| `dotfiles init` | Create `~/.config/dotfiles/config.toml` (asks for name and email) |
| `dotfiles diff` | Show what `apply` would change |
| `dotfiles apply` | Install the tools and write the config files |
| `dotfiles doctor` | Check the setup and show how to fix problems |
| `dotfiles tips` | Show what the new shell can do |
| `dotfiles update` | Update dotfiles itself, then apply |
| `dotfiles uninstall` | Remove the managed files and restore the ones from before |

## Your own changes

Do not edit `~/.bashrc` or the other managed files: they start with `# DO NOT EDIT`, and the
next `apply` stops if they were edited. Put your own settings here instead. dotfiles never
touches these files:

| File | For |
| --- | --- |
| `~/.bashrc.local` | bash: aliases, exports, ... |
| `~/.zshrc.local` | zsh |
| `~/.config/shell/local.sh` | both bash and zsh |
| `~/.gitconfig` | git (wins over the managed git settings) |
| `mise use -g <tool>` | extra tools, e.g. `mise use -g node` |

Installers like conda, nvm or sdkman append lines to `~/.bashrc`. The next `apply` moves those
lines to `~/.bashrc.local` (or `~/.zshrc.local`), so they keep working.

When you first install, your old `~/.bashrc` is saved to
`~/.local/state/dotfiles/backup/` and `apply` prints the exact path. Copy what you need from it
to `~/.bashrc.local`.

## Configuration

`~/.config/dotfiles/config.toml` (written by `dotfiles init`):

```toml
name           = "Your Name"
personal_email = "you@example.com"
shell          = "bash"  # or "zsh"

# Optional: repositories of this organization use the work email.
[[orgs]]
url   = "github.com/your-company"
email = "you@your-company.com"
```

- **`shell`**: `bash` is the default. With `zsh` you also get `~/.zshrc` with
  autosuggestions and syntax highlighting (via [antidote](https://github.com/mattmc3/antidote)).
  If you leave it out, it is `zsh` when zsh is your login shell, otherwise `bash`.
- **`orgs`**: `apply` writes a `~/.gitconfig-org-<name>` for each org, so repositories under that
  org use the matching git email.
- **`profile`**: adds personal settings from
  [`templates/profiles/<name>/`](templates/profiles): `shell.sh` (sourced by the shell),
  `gitconfig` and `direnv.toml`. Example: `profile = "guettli"`.

## What gets installed

| Tool | Purpose |
| --- | --- |
| [mise](https://mise.jdx.dev/) | Installs and updates all the tools below |
| [Starship](https://starship.rs/) | Prompt |
| [Atuin](https://github.com/atuinsh/atuin) | Shell history search (Ctrl-R). History stays on your machine unless you run `atuin login` |
| [direnv](https://direnv.net/) | Per-project environment variables (`.envrc`, needs `direnv allow`) |
| [fzf](https://github.com/junegunn/fzf) | Fuzzy finder |
| [zoxide](https://github.com/ajeetdsouza/zoxide) | `z <dir>`: jump to directories |
| [micro](https://micro-editor.github.io/) | Simple terminal editor, the default `$EDITOR` |
| [lazygit](https://github.com/jesseduffield/lazygit) | Git in the terminal |
| [bat](https://github.com/sharkdp/bat), [eza](https://github.com/eza-community/eza) | `cat` and `ls` with colors |
| [ripgrep](https://github.com/BurntSushi/ripgrep), [fd](https://github.com/sharkdp/fd) | Search text and files |
| [tealdeer](https://github.com/tealdeer-rs/tealdeer) | `tldr <command>`: short examples |
| [prek](https://github.com/j178/prek) | Runs `.pre-commit-config.yaml` hooks on commit |
| [tmux](https://github.com/tmux/tmux) | Terminal multiplexer |

The base tools are declared in [`templates/mise/dotfiles.toml`](templates/mise/dotfiles.toml),
which `apply` deploys to `~/.config/mise/conf.d/dotfiles.toml` before it runs `mise install`.
Your own tools (`mise use -g`) go to `~/.config/mise/config.toml`, which dotfiles never touches.

The versions are pinned. [Renovate](renovate.json5) opens a weekly PR when new versions are out,
at least 14 days old. Every merge to main is released, and `dotfiles update` brings the new
versions to your machine.

### kubectl completion

If `kubectl` is installed (dotfiles doesn't install it), `apply` saves its completion script in
`~/.cache/dotfiles/completions/` and the shell loads it at startup, so `kubectl <Tab>` and the
alias `k <Tab>` complete. Loading the saved script takes about 10 ms; generating it takes
`kubectl` about 150 ms, too slow for every new terminal. After upgrading kubectl, run
`dotfiles apply`, or delete the file: the next shell creates it again.

## Git

dotfiles manages `~/.config/git/config`. `~/.gitconfig` is yours: git reads both, and settings in
`~/.gitconfig` win. Tools that run `git config --global` (e.g. `gh auth setup-git`) write there, so
they don't collide with `apply`. dotfiles creates an (almost empty) `~/.gitconfig` if it is
missing, because without it `git config --global` would write into the managed file.

Because `~/.gitconfig` wins, `git config --global user.email …` quietly overrides the per-org
emails. `dotfiles doctor` warns about it.

**Hooks.** `core.hooksPath` points to `~/.config/git/hooks`. On commit, the pre-commit hook runs
[prek](https://github.com/j178/prek) if the repository has a `.pre-commit-config.yaml`, and
every commit message shows `# Committing as: <email>`. git skips a repository's own
`.git/hooks` when `core.hooksPath` is set, so the managed `pre-commit` (in repositories without
a pre-commit config) and `prepare-commit-msg` hooks run the repository's own hook when there is
one. `git lfs install` puts its hooks into `~/.config/git/hooks` next to the managed ones;
dotfiles leaves them alone.

You don't need `prek install` or `pre-commit install`: the global hook already runs prek in
every repository with a `.pre-commit-config.yaml` (or `prek.toml`). Both commands refuse to run
while `core.hooksPath` is set globally, and they are right to: they
would overwrite the shared hooks of all your repositories. If a repository needs its own hooks
anyway, set the hooks path for that repository only; a repository setting wins over the global
one:

```bash
git config --local core.hooksPath .git/hooks   # then: prek install
```

Tests that create git repositories inherit the global setting. Run them with
`GIT_CONFIG_GLOBAL=/dev/null` if they must not.

Older versions managed `~/.gitconfig` itself. `apply` migrates once: the managed part is
removed, and lines appended later (like the `gh` credential helper) are kept. If the managed
part was edited, `apply` stops and asks you to move your settings to the end of the file, or
`--force` moves the file to `~/.gitconfig.bak` (never overwriting an existing `.bak`).

An existing `~/.config/git/config` that dotfiles did not write is an error (move its settings to
`~/.gitconfig`). A non-default `XDG_CONFIG_HOME` is an error too, because git would then not
read `~/.config/git/config`.

## Safety

- `apply` keeps a copy of what it wrote last time (`~/.local/state/dotfiles/installed_cache`).
  If a managed file was edited by hand, `apply` stops and shows the diff instead of overwriting
  it. `--force` overwrites, after saving your version to `~/.local/state/dotfiles/backup/`.
- Files that existed before dotfiles (your old `~/.bashrc`) are moved to the backup directory,
  not deleted. `dotfiles uninstall` restores them. Symlinks (GNU stow) are moved as links, never
  written through.
- `install.sh` and `dotfiles update` check the release archive against the release's
  `checksums.txt`.

## Without the installer

With [Go](https://go.dev/install) and [mise](https://mise.jdx.dev/getting-started.html)
installed, you can run it straight from GitHub:

```bash
go run github.com/guettli/dotfiles@latest diff
go run github.com/guettli/dotfiles@latest apply
```

## Developing

I tried [chezmoi](https://www.chezmoi.io/) and
[home-manager](https://github.com/nix-community/home-manager), but these tools felt too
complicated. This small Go program embeds the files in [`templates/`](templates) with
`go:embed`.

```bash
git clone git@github.com:guettli/dotfiles.git
cd dotfiles
# edit templates/ or the Go code
go test ./...
go run . diff
go run . apply
```

`test/e2e.sh` installs everything into a fresh machine (CI runs it in Ubuntu and Fedora
containers). Every push to main is released: it gets the next patch version and
[GoReleaser](https://goreleaser.com/) builds the binaries. Push a `v*` tag yourself for a
minor or major version.
