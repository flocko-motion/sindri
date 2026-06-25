#!/usr/bin/env bash
# Check the active Go toolchain is the latest released version. brokkr's linters
# (notably deadcode, via go/packages) should run on current Go, so an outdated
# toolchain is a hard fail. The go.dev lookup is best-effort: if it can't be
# reached we warn and pass rather than block offline work.
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

if [ "$have" != "$latest" ]; then
	echo "go: toolchain is $have but the latest is $latest — update Go (the linters must run on the latest)" >&2
	exit 1
fi
echo "go: toolchain $have is current"
