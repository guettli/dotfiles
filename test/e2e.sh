#!/bin/sh
# End to end test on a fresh machine (CI runs it in Ubuntu and Fedora
# containers and on macOS). Needs DOTFILES_BINARY: a built dotfiles binary.
# E2E_SHELL: bash (default) or zsh.
set -eux

: "${DOTFILES_BINARY:?set DOTFILES_BINARY to a built dotfiles binary}"
sh="${E2E_SHELL:-bash}"
rc="$HOME/.${sh}rc"
cd "$(dirname "$0")/.."

printf '# my old rc file\nalias old=yes\n' >"$rc"

# Fresh install, non-interactive.
DOTFILES_BINARY="$DOTFILES_BINARY" sh ./install.sh --name "CI User" --email ci@example.com --shell "$sh" </dev/null
export PATH="$HOME/.local/bin:$PATH"

grep -q "shell = \"$sh\"" "$HOME/.config/dotfiles/config.toml"
# bash is always configured; zsh only when chosen.
test -e "$HOME/.bashrc"
if [ "$sh" = bash ]; then test ! -e "$HOME/.zshrc"; fi
grep -q 'alias old=yes' "$HOME/.local/state/dotfiles/backup$rc"

# The new interactive shell has the tools (eza has no macOS builds).
tools="starship atuin zoxide fzf bat micro rg fd lazygit tldr prek"
if [ "$(uname)" = Linux ]; then tools="$tools eza"; fi
"$sh" -ic "command -v $tools && type z && alias ll" </dev/null

# A second apply changes nothing.
dotfiles apply </dev/null | tee "$HOME/apply2.log"
if grep -q '✅ Applied' "$HOME/apply2.log"; then exit 1; fi

# An installer appends to the rc file: apply moves it to ~/.<shell>rc.local.
echo 'export FROM_INSTALLER=1' >>"$rc"
dotfiles apply </dev/null
grep -q FROM_INSTALLER "$rc.local"
if grep -q FROM_INSTALLER "$rc"; then exit 1; fi
test "$("$sh" -ic 'echo $FROM_INSTALLER' </dev/null 2>/dev/null | tail -n1)" = 1

# git uses the configured identity.
test "$(git config user.email)" = ci@example.com

dotfiles doctor || true
dotfiles tips >/dev/null

# Uninstall restores the old rc file.
dotfiles uninstall --yes
grep -q 'alias old=yes' "$rc"
test ! -e "$HOME/.config/shell/common.sh"
test ! -e "$HOME/.config/git/config"
echo "e2e OK"
