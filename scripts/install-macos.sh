#!/usr/bin/env bash
# Install — or upgrade — sindri on macOS from an extracted release tarball.
#
# Idempotent: to upgrade, download a newer tarball, extract it, and run this again.
# It installs the binaries next to each other (the hub finds sindri-worker/brokkr
# beside the sindri binary) into ~/.local/bin by default — override with PREFIX,
# e.g. `PREFIX=/usr/local/bin ./install.sh`.
#
# Two macOS specifics it handles so the upgrade path is clean:
#   1. Atomic replace: each binary is staged in PREFIX and rename(2)'d into place, so
#      a currently-running sindri/hub is swapped safely (the live process keeps its
#      old inode) instead of failing with "text file busy" — the same reason
#      `make install` uses mv rather than cp.
#   2. Gatekeeper: the release binaries are unsigned, so their quarantine attribute
#      is cleared; otherwise macOS refuses to run a freshly-downloaded binary.
#
# After an upgrade, a hub from the previous version keeps running; the next `sindri`
# command detects the version mismatch and offers to restart it — no manual step.
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local/bin}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bins="sindri sindri-worker brokkr td yq"

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

if pgrep -qf 'sindri hub' 2>/dev/null; then
	echo "a hub from the previous version is running — your next 'sindri' command will offer to restart it."
fi
