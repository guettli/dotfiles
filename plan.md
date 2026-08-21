# Plan: convert guettli/dotfiles from nix → mise, ship PR, clone to tc, register in AgentLoop

## Goal (from user)
1. Convert `guettli/dotfiles` so it installs its tools via **mise** instead of **nix** — I write the PR now.
2. Clone the repo **locally** into the `code1..code4` users on **tc** (which has mise, no nix — so the conversion is what makes it runnable there).
3. **Add the repo to AgentLoop** (register it as a managed repo in the running instance).

## Resume context (survives /clear)
- Working clone: `~/dotfiles` on p16 (this host), branch `main` @ `1fdb2c7`.
- Git/GitHub auth: git over SSH via the **guettlibot** key `~/.ssh/id_ed25519` (authenticates as guettlibot). `gh` is logged in as guettlibot.
- tc access: `ssh root@tc` (Ubuntu 26.04). Users `code1..code4` exist, key-login only, each already has the other 7 AgentLoop repos cloned + `~/.env` (guettlibot token, sourced on login via `~/.profile`) + mise tools (`node@lts`, `claude`). System mise = `/usr/bin/mise`; per-user tools via `mise use -g`.
- tc gotcha: Ubuntu 26.04 ships **uutils coreutils** — `head`/`tail` reject `-1`/`-2`; use `-n1`. Also `mise use -g ... >/dev/null` can exit 1 (progress writer); don't redirect to /dev/null.
- AgentLoop source: `~/agentloop`. Running instance: `agentloop.thomas-guettler.de`, k8s ns `agentloop`, `deploy/agentloop` (KUBECONFIG=~/.kube/config). Daemon args include `--repo-data /data/agentloop`.

## The change — what uses nix today
### main.go (installer logic)
- L134-143 `requiredPackages`: `nixpkgs#{starship,atuin,direnv,nix-direnv,mise,tmux,antidote,bash-completion}`.
- L151-179 apply/diff block: installs via `nix ... profile add`.
- L291-313 `getMissingPackages`: detects via `nix ... profile list`.

### templates (hardcode ~/.nix-profile paths)
- `templates/shell/common.sh` L24: `pathadd "$HOME/.nix-profile/bin"`. (mise activate + direnv hook already present L54-65.)
- `templates/zsh/.zshrc` L12-13: zsh site-functions from `.nix-profile/share/zsh/site-functions`; L17-18: antidote from `.nix-profile/share/antidote/antidote.zsh`.
- `templates/bash/.bashrc` L8-9: bash-completion from `.nix-profile/etc/profile.d/bash_completion.sh`.

### README.md
- "installs required Nix dependencies", `nix profile`, prerequisites mention Nix — update to mise.

## Tool mapping (verified against mise registry on tc)
| tool | in mise registry? | new install path |
| --- | --- | --- |
| starship | ✅ aqua:starship/starship | `mise use -g starship` |
| atuin | ✅ aqua:atuinsh/atuin | `mise use -g atuin` |
| direnv | ✅ aqua:direnv/direnv | `mise use -g direnv` |
| tmux | ✅ aqua:tmux/tmux-builds | `mise use -g tmux` |
| mise | (is the installer) | drop — already present |
| nix-direnv | ❌ nix-only | **drop** — plain direnv + mise needs no nix-direnv |
| antidote | ❌ not in registry | `git clone --depth=1 https://github.com/mattmc3/antidote ~/.antidote`; source `~/.antidote/antidote.zsh` |
| bash-completion | ❌ not in registry | system pkg (`apt install bash-completion`), source `/usr/share/bash-completion/bash_completion` |

## Implementation steps
1. Branch: `git -C ~/dotfiles switch -c nix-to-mise`.
2. **main.go**:
   - Replace `requiredPackages` with mise tool list `[]string{"starship","atuin","direnv","tmux"}`.
   - Rewrite `getMissingPackages` to detect via `mise ls -g` / `mise which <tool>` (tool missing if not on shims). 
   - Rewrite apply branch: `mise use -g <missing...>` (idempotent; don't redirect stdout to /dev/null). Keep the diff branch listing "+ <tool>".
   - Add antidote step: if `~/.antidote` absent → git clone (apply) / report (diff).
   - bash-completion: don't try to install via mise; rely on system (note in output) — or `apt-get install -y bash-completion` guarded by `command -v apt-get` + root. Keep minimal: just stop referencing nix.
   - Update user-facing strings ("via Nix" → "via mise", "nix installed" → "mise installed").
3. **templates/shell/common.sh**: drop `.nix-profile/bin` pathadd; ensure `~/.local/share/mise/shims` on PATH (mise activate covers interactive; add shim path for non-interactive login) and `~/.antidote` not needed on PATH.
4. **templates/zsh/.zshrc**: antidote source path → `~/.antidote/antidote.zsh`; drop/replace nix-profile zsh site-functions block (mise completions instead, optional).
5. **templates/bash/.bashrc**: bash-completion source → `/usr/share/bash-completion/bash_completion` (guard with `-f`).
6. **README.md**: replace Nix wording with mise; update prerequisites (Go + mise, drop nix).
7. Build/verify: `cd ~/dotfiles && go build ./... && go vet ./...`. Optionally `go run . diff` in a scratch HOME to smoke-test (no nix required).
8. Commit (Co-Authored-By + Claude-Session trailers), push branch, open PR to `guettli/dotfiles` via `gh pr create`. NOTE: guettlibot's shared GitHub REST limit — gh may 403 if the loop is busy; retry.

## Then (tasks 2 & 3)
9. Clone into tc users on the feature branch (so it runs there pre-merge; switch to main after merge):
   `for u in code1 code2 code3 code4; do ssh root@tc "sudo -u $u -H git -C /home/$u clone -b nix-to-mise git@github.com:guettli/dotfiles.git"; done`
   (matches the existing 7-repo layout; known_hosts + key already in place.)
10. Register in AgentLoop — `POST /api/v1/repos` on the running instance for `guettli/dotfiles`.
    - TODO on resume: find request schema + auth. `~/agentloop` openapi (locate the file; not at repo root) is source of truth; `onboarder.go` / `serve.go` reference `POST /api/v1/repos` (issue #700) and the onboarder installs webhooks. Confirm auth (instance uses webauthn/rp `agentloop.thomas-guettler.de`) — may need an API token/session or a direct store write.

## Open decisions / notes
- PR target branch = `main`. Keep change minimal & reviewable; call out dropped `nix-direnv` and the antidote(git)/bash-completion(apt) relocations in the PR body.
- Do NOT run `dotfiles apply` as the code users until PR logic is verified — it rewrites ~/.zshrc, ~/.bashrc, ~/.gitconfig etc. (has .bak backup + cache-baseline guard, but still).
- Cloning the feature branch means a later `git pull` after merge needs a switch back to main.
