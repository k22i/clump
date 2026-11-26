#!/bin/sh
# Install clump from its GitHub releases: download the archive built for
# this platform, verify it against the release's checksums.txt, unpack the
# clump binary into a bin dir (on PATH whenever possible) and run it once.
#
#   curl -fsSL https://raw.githubusercontent.com/k22i/clump/main/scripts/install.sh | sh
#
# CLUMP_BIN_DIR overrides where the binary lands; CLUMP_RELEASE_BASE
# overrides where releases are downloaded from (for testing). After this,
# `clump upgrade` fetches the next release over the binary itself.
set -eu

repo=${CLUMP_REPO:-k22i/clump}
base=${CLUMP_RELEASE_BASE:-"https://github.com/$repo/releases/latest/download"}

log() { printf 'install: %s\n' "$*" >&2; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

# --- platform -----------------------------------------------------------------
os=$(uname -s)
arch=$(uname -m)
case "$os" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) die "no clump release for $(uname -s); build from source: go install github.com/k22i/clump/cmd/clump@latest" ;;
esac
case "$arch" in
    arm64 | aarch64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) die "no clump release for $arch; build from source: go install github.com/k22i/clump/cmd/clump@latest" ;;
esac
archive="clump_${os}_${arch}.tar.gz"

# --- tools ---------------------------------------------------------------------
command -v curl >/dev/null 2>&1 || die "curl is required"
if command -v sha256sum >/dev/null 2>&1; then
    sha=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    sha="shasum -a 256"
else
    die "sha256sum or shasum is required to verify the download"
fi

# --- install dir ---------------------------------------------------------------
# In place when clump is already on PATH (also a go install); else
# ~/.local/bin, which most systems have on PATH.
if [ -n "${CLUMP_BIN_DIR:-}" ]; then
    bin_dir=$CLUMP_BIN_DIR
elif command -v clump >/dev/null 2>&1; then
    bin_dir=$(cd "$(dirname "$(command -v clump)")" && pwd)
else
    bin_dir=$HOME/.local/bin
fi
mkdir -p "$bin_dir" 2>/dev/null || die "cannot create $bin_dir"
[ -w "$bin_dir" ] || die "cannot write to $bin_dir (set CLUMP_BIN_DIR or rerun as a user who can)"

# --- download, verify, install -------------------------------------------------
tmp=$(mktemp -d) || die "mktemp failed"
trap 'rm -rf "$tmp"' EXIT

log "downloading $archive from $repo"
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" ||
    die "no checksums.txt: a release has to be published for this to work (git tag vX.Y.Z)"

want=$(grep " $archive" "$tmp/checksums.txt" | head -1 | cut -d' ' -f1)
[ -n "$want" ] || die "checksums.txt lists nothing for $archive"
(cd "$tmp" && printf '%s  %s\n' "$want" "$archive" | $sha -c - >/dev/null) ||
    die "$archive: checksum mismatch"

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/clump" ] || die "$archive holds no clump binary"

mv "$tmp/clump" "$bin_dir/clump"
chmod 0755 "$bin_dir/clump"

case ":$PATH:" in
    *":$bin_dir:"*) ;;
    *) log "note: $bin_dir is not on your PATH; add it to use clump" ;;
esac

log "installed $($bin_dir/clump version) at $bin_dir/clump"
log "afterwards clump upgrade fetches the next release"
