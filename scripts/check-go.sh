#!/usr/bin/env bash
# Check the active Go toolchain is not OLDER than the latest release. brokkr's linters
# (notably deadcode, via go/packages) should run on current Go, so an outdated
# toolchain is a hard fail. The go.dev lookup is best-effort: if it can't be
# reached we warn and pass rather than block offline work.
#
# A FLOOR, NOT AN EQUALITY. Requiring the exact latest breaks every developer and every
# agent the moment Go ships, and it cannot be satisfied at all when go.dev serves a
# release candidate: `go get go@latest` installs the stable release, which then fails
# the check again — observed as go1.27.0 being refused for not being go1.27rc3.
#
# GOVERSION is the toolchain actually executing, which under GOTOOLCHAIN=auto is
# the one go.mod's `go` directive selects — not necessarily the base install in
# /usr/local/go. So this effectively checks that the `go` directive is current,
# and `make upgrade-go` (which raises it) is the fix, not reinstalling Go.
set -euo pipefail

have="$(go env GOVERSION 2>/dev/null || true)" # e.g. go1.26.4
if [ -z "$have" ]; then
	echo "go: toolchain not found on PATH" >&2
	exit 1
fi

latest="$(curl -fsSL --max-time 3 'https://go.dev/VERSION?m=text' 2>/dev/null | head -n1 || true)"
if [ -z "$latest" ]; then
	echo "go: couldn't reach go.dev — skipping the latest-toolchain check (have $have)" >&2
	exit 0
fi

# A pre-release is never something to require: it is not what `go get go@latest` installs,
# so demanding it is a loop with no exit.
case "$latest" in
*rc* | *beta*)
	echo "go: go.dev reports $latest, a pre-release — skipping the check (have $have)" >&2
	exit 0
	;;
esac

# Oldest first, so if `have` sorts first and differs, it is behind. Being AHEAD passes:
# the linters need a floor, and a newer toolchain clears it.
oldest="$(printf '%s\n%s\n' "$have" "$latest" | sort -V | head -n1)"
if [ "$have" != "$latest" ] && [ "$oldest" = "$have" ]; then
	echo "go: toolchain is $have but the latest is $latest — run 'make upgrade-go' (the linters must run on current Go)" >&2
	exit 1
fi
echo "go: toolchain $have is current"
