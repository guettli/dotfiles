#!/bin/sh
# End to end test on a fresh machine (CI runs it in Ubuntu and Fedora
# containers). Needs DOTFILES_BINARY: a built dotfiles binary.
set -eux

: "${DOTFILES_BINARY:?set DOTFILES_BINARY to a built dotfiles binary}"
cd "$(dirname "$0")/.."

printf '# my old bashrc\nalias old=yes\n' >"$HOME/.bashrc"

# Fresh install, non-interactive.
DOTFILES_BINARY="$DOTFILES_BINARY" sh ./install.sh --name "CI User" --email ci@example.com </dev/null
export PATH="$HOME/.local/bin:$PATH"

grep -q 'shell = "bash"' "$HOME/.config/dotfiles/config.toml"
test ! -e "$HOME/.zshrc"
grep -q 'alias old=yes' "$HOME/.local/state/dotfiles/backup$HOME/.bashrc"

# The new interactive bash has the tools.
bash -ic 'command -v starship atuin zoxide fzf bat eza micro rg fd lazygit tldr prek && type z && alias ll' </dev/null

# A second apply changes nothing.
dotfiles apply </dev/null | tee "$HOME/apply2.log"
if grep -q '✅ Applied' "$HOME/apply2.log"; then exit 1; fi

# An installer appends to ~/.bashrc: apply moves it to ~/.bashrc.local.
echo 'export FROM_INSTALLER=1' >>"$HOME/.bashrc"
dotfiles apply </dev/null
grep -q FROM_INSTALLER "$HOME/.bashrc.local"
if grep -q FROM_INSTALLER "$HOME/.bashrc"; then exit 1; fi
test "$(bash -ic 'echo $FROM_INSTALLER' </dev/null 2>/dev/null | tail -n1)" = 1

# git uses the configured identity.
test "$(git config user.email)" = ci@example.com

dotfiles doctor || true
dotfiles tips >/dev/null

# Uninstall restores the old ~/.bashrc.
dotfiles uninstall --yes
grep -q 'alias old=yes' "$HOME/.bashrc"
test ! -e "$HOME/.config/shell/common.sh"
test ! -e "$HOME/.config/git/config"
echo "e2e OK"
