#!/bin/sh
# Install dotfiles: a good default shell environment.
#
#   curl -fsSL https://raw.githubusercontent.com/guettli/dotfiles/main/install.sh | sh
#
# Arguments are passed to `dotfiles init`, for example:
#
#   curl -fsSL .../install.sh | sh -s -- --name "Jane Doe" --email jane@example.com
#
# It installs the dotfiles program to ~/.local/bin, installs mise
# (https://mise.jdx.dev) if missing, creates ~/.config/dotfiles/config.toml
# (asks for your name and email) and runs `dotfiles apply`.
set -eu

repo="https://github.com/guettli/dotfiles"
bin_dir="${DOTFILES_BIN_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf '❌ %s\n' "$*" >&2; exit 1; }

for cmd in curl tar git; do
    command -v "$cmd" >/dev/null 2>&1 || die "$cmd is needed. Install it first, for example: sudo apt-get install $cmd"
done

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux | darwin) ;;
*) die "unsupported operating system: $os" ;;
esac
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "unsupported CPU: $arch" ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -n "${DOTFILES_BINARY:-}" ]; then
    # For testing: use a locally built binary.
    cp "$DOTFILES_BINARY" "$tmp/dotfiles"
else
    archive="dotfiles_${os}_${arch}.tar.gz"
    say "⬇️  Downloading $archive"
    curl -fsSL -o "$tmp/$archive" "$repo/releases/latest/download/$archive"
    curl -fsSL -o "$tmp/checksums.txt" "$repo/releases/latest/download/checksums.txt"
    (
        cd "$tmp"
        grep " $archive\$" checksums.txt >"$archive.sha256" || die "$archive is not in checksums.txt"
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum -c --quiet "$archive.sha256"
        else
            shasum -a 256 -c --quiet "$archive.sha256"
        fi
    ) || die "checksum of $archive does not match"
    tar -xzf "$tmp/$archive" -C "$tmp" dotfiles
fi

mkdir -p "$bin_dir"
cp "$tmp/dotfiles" "$bin_dir/dotfiles.new"
chmod 755 "$bin_dir/dotfiles.new"
mv "$bin_dir/dotfiles.new" "$bin_dir/dotfiles"
say "✅ Installed $bin_dir/dotfiles"

export PATH="$bin_dir:$HOME/.local/bin:$PATH"

if ! command -v mise >/dev/null 2>&1; then
    say "⬇️  Installing mise (https://mise.jdx.dev)"
    curl -fsSL https://mise.run | sh
fi

# Ask for name and email on the terminal, also when this script is piped
# into sh (then stdin is the script, not the keyboard).
if [ ! -f "$HOME/.config/dotfiles/config.toml" ]; then
    if [ $# -eq 0 ] && [ ! -t 0 ] && (: </dev/tty) 2>/dev/null; then
        dotfiles init </dev/tty
    else
        dotfiles init "$@"
    fi
fi

if (: </dev/tty) 2>/dev/null; then
    dotfiles apply </dev/tty
else
    dotfiles apply
fi

case ":$PATH:" in
*":$bin_dir:"*) ;;
*) say "Note: $bin_dir is on PATH in new terminals (set by the new shell config)." ;;
esac
