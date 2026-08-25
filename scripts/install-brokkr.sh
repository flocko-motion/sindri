#!/usr/bin/env bash
# Install — or upgrade — the standalone `brokkr` binary, without the rest of sindri.
# Auto-detects OS/arch (linux/darwin, amd64/arm64), and if a `brokkr` already sits at
# the destination, compares its version against the latest release and skips the
# download when it's already current.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/flocko-motion/sindri/master/scripts/install-brokkr.sh | bash
#   curl -fsSL .../install-brokkr.sh | bash -s -- /custom/path/brokkr   # custom dest
#
# Default destination is ~/.local/bin/brokkr — sindri's own tarball install location,
# so a standalone `brokkr` and a later full sindri install land on top of each other
# rather than drifting apart as two builds on PATH.
set -euo pipefail

dest="${1:-$HOME/.local/bin/brokkr}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
x86_64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*)
	echo "error: unsupported arch '$arch' — brokkr ships linux/darwin, amd64/arm64 only" >&2
	exit 1
	;;
esac

release_json="$(curl -fsSL https://api.github.com/repos/flocko-motion/sindri/releases/latest)"
tag="$(echo "$release_json" | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -n "$tag" ] || { echo "error: couldn't resolve the latest release tag" >&2; exit 1; }
latest="${tag#v}"

# Skip the download if what's already at $dest reports the same version. `brokkr
# version`'s first line is "brokkr X.Y.Z" — a binary too old to have the command, or
# no binary at all, falls through to installing.
if [ -x "$dest" ]; then
	installed="$("$dest" version 2>/dev/null | head -1 | awk '{print $2}')"
	if [ "$installed" = "$latest" ]; then
		echo "brokkr $installed is already current at $dest"
		exit 0
	fi
	[ -n "$installed" ] && echo "upgrading brokkr $installed -> $latest"
fi

url="https://github.com/flocko-motion/sindri/releases/download/${tag}/brokkr_${latest}_${os}_${arch}"
mkdir -p "$(dirname "$dest")"
tmp="${dest}.new"
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"
xattr -d com.apple.quarantine "$tmp" 2>/dev/null || true # clear Gatekeeper; no-op elsewhere
mv -f "$tmp" "$dest"                                      # atomic within the dest dir
echo "installed brokkr $latest to $dest"

case ":$PATH:" in
*":$(dirname "$dest"):"*) ;;
*) echo "note: $(dirname "$dest") is not on your PATH — add it to run 'brokkr' directly" ;;
esac
