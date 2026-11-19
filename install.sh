#!/bin/sh
# Install or update Finux: download the binary of a release, check it,
# set up the database and put the binary in a folder of the PATH. Before
# an update changes the database, the script keeps a copy of it.
#
#   curl -fsSL https://raw.githubusercontent.com/johnvilela/finux/main/install.sh | sh
#
# Environment variables:
#   FINUX_VERSION       Release to install, for example v0.10.0. Default: the newest.
#   FINUX_INSTALL_DIR   Folder of the binary. Default: ~/.local/bin.
#   FINUX_DB            Database file. Default: ~/.finux/finux.db.
set -eu

releases="${FINUX_RELEASES_URL:-https://github.com/johnvilela/finux/releases}"
directory="${FINUX_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
  echo "finux install: $1" >&2
  exit 1
}

# The release has one binary for each system it supports.
system="${FINUX_SYSTEM:-$(uname -s) $(uname -m)}"
case "$system" in
  "Linux x86_64" | "Linux amd64") asset="finux-linux-amd64" ;;
  *) fail "no finux release for $system. Build from source: https://github.com/johnvilela/finux#build-and-run" ;;
esac

if [ -n "${FINUX_VERSION:-}" ]; then
  url="$releases/download/$FINUX_VERSION"
else
  url="$releases/latest/download"
fi

if command -v curl >/dev/null 2>&1; then
  download() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  download() { wget -q "$1" -O "$2"; }
else
  fail "curl or wget is necessary"
fi

if command -v sha256sum >/dev/null 2>&1; then
  checksum() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
  checksum() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
  fail "sha256sum or shasum is necessary"
fi

# Download to a temporary folder, so a bad download changes nothing.
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT

echo "Downloading $asset from $url"
download "$url/$asset" "$temporary/$asset" || fail "cannot download $url/$asset"
download "$url/checksums.txt" "$temporary/checksums.txt" || fail "cannot download $url/checksums.txt"

expected="$(grep " $asset\$" "$temporary/checksums.txt" | cut -d ' ' -f 1)"
actual="$(checksum "$temporary/$asset")"
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  fail "the checksum of $asset is wrong: nothing was installed"
fi

chmod +x "$temporary/$asset"

# Keep a copy of a database that exists, then let the new version create
# the database or apply its migrations. The binary that is installed now
# stays until this works.
database="${FINUX_DB:-$HOME/.finux/finux.db}"
backup=""
if [ -f "$database" ]; then
  backup="$database.backup-$(date +%Y%m%d-%H%M%S)"
  cp -p "$database" "$backup" || fail "cannot copy the database to $backup: nothing was installed"
fi
if ! "$temporary/$asset" init; then
  if [ -n "$backup" ]; then
    cp -p "$backup" "$database"
    fail "the new version could not update the database. The database is as it was (copy: $backup) and nothing was installed"
  fi
  fail "the new version could not create the database: nothing was installed"
fi

# Move the binary in one step: a finux that runs now keeps its old file.
mkdir -p "$directory"
mv "$temporary/$asset" "$directory/.finux.new"
mv "$directory/.finux.new" "$directory/finux"

echo "✓ $("$directory/finux" version) installed in $directory"
if [ -n "$backup" ]; then
  echo "Copy of the database: $backup"
  echo "Delete it when the new version works for you."
fi

case ":$PATH:" in
  *":$directory:"*) ;;
  *)
    echo
    echo "$directory is not in your PATH. Add this line to ~/.bashrc or ~/.zshrc:"
    echo
    echo "  export PATH=\"$directory:\$PATH\""
    echo
    echo "With fish, run this one time:"
    echo
    echo "  fish_add_path $directory"
    ;;
esac
