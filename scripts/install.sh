#!/usr/bin/env bash
# Install — or upgrade — sindri from an extracted release tarball, on Linux or macOS.
#
# Idempotent: to upgrade, download a newer tarball, extract it, and run this again.
# The binaries go next to each other (the hub finds sindri-worker/brokkr beside
# itself, and the CLI finds the hub the same way) into ~/.local/bin — the ONE
# install location, so nothing on PATH can shadow it with a different build.
# Override with PREFIX if you must.
#
# Two details keep the upgrade path clean:
#   1. Atomic replace: each binary is staged in PREFIX and rename(2)'d into place, so
#      a currently-running sindri/hub is swapped safely (the live process keeps its
#      old inode) instead of failing with "text file busy" — the same reason
#      `make install` uses mv rather than cp.
#   2. Gatekeeper (macOS): release binaries are unsigned, so the quarantine attribute
#      is cleared; without it macOS refuses to run a freshly-downloaded binary. The
#      call is a no-op elsewhere.
#
# After an upgrade, a hub from the previous version keeps running; the next `sindri`
# command detects the version mismatch and offers to restart it — no manual step.
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local/bin}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# brokkr-linux is the linux brokkr mounted into the (always-linux) agent pods so
# `brokkr` works inside agents on macOS too — installed beside the darwin brokkr.
bins="sindri sindri-hub sindri-worker brokkr brokkr-linux yq"

mkdir -p "$PREFIX"
for bin in $bins; do
	[ -f "$here/$bin" ] || { echo "error: $bin not found next to this script" >&2; exit 1; }
done

for bin in $bins; do
	tmp="$PREFIX/.$bin.new"
	cp "$here/$bin" "$tmp"
	chmod +x "$tmp"
	xattr -d com.apple.quarantine "$tmp" 2>/dev/null || true # clear Gatekeeper
	mv -f "$tmp" "$PREFIX/$bin"                              # atomic within PREFIX
done
echo "installed sindri + tools to $PREFIX"

case ":$PATH:" in
*":$PREFIX:"*) ;;
*) echo "note: $PREFIX is not on your PATH — add it, e.g.:  echo 'export PATH=\"$PREFIX:\$PATH\"' >> ~/.zshrc" ;;
esac

if pgrep -qf 'sindri-hub' 2>/dev/null; then
	echo "a hub from the previous version is running — your next 'sindri' command will offer to restart it."
fi
