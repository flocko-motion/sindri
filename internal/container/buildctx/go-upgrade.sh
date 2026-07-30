#!/usr/bin/env bash
# Install the Go toolchain this project's go.mod asks for, from inside the pod.
#
# The golang base image pins GOTOOLCHAIN=local, so when go.mod's `go` directive is
# newer than the image's Go the go command refuses to work ("go.mod requires go >=
# X (running go Y; GOTOOLCHAIN=local)") instead of fetching what it needs — and
# /usr/local/go is root-owned, so the agent cannot replace it in place. That breaks
# `go build`, `go test` and brokkr's deadcode linter at once, for a reason that
# looks nothing like the code the agent is working on.
#
# This fetches the toolchain through go's own module path (proxy-served and
# checksum-verified — not curl-a-tarball) and links it into ~/.local/bin, which the
# image puts first on PATH, so plain `go` is the new version in this shell and every
# later one. Re-running is cheap: an already-cached toolchain costs only the link.
# Nothing here needs root, and /usr/local/go is left untouched.
#
# Usage:
#   go-upgrade              # the version go.mod requires (default)
#   go-upgrade latest       # the latest released Go
#   go-upgrade 1.26.5       # that exact version
set -euo pipefail

bindir="$HOME/.local/bin"

die() {
	echo "go-upgrade: $*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
go-upgrade — install the Go toolchain this project needs (no root, /usr/local/go untouched)

  go-upgrade              the version go.mod requires (default)
  go-upgrade latest       the latest released Go
  go-upgrade 1.26.5       that exact version

The toolchain is fetched through go's own module path (checksum-verified) and
linked into ~/.local/bin, which precedes the base install on PATH.
EOF
}

# normalize turns 1.26, go1.26 or 1.26.5 into a toolchain name (go1.26.0, go1.26.5).
# Every release since 1.21 carries a patch component, so a two-part version needs one.
normalize() {
	local v="${1#go}"
	case "$v" in
	*.*.*) : ;;
	*.*) v="$v.0" ;;
	*) die "unrecognized Go version '$1'" ;;
	esac
	echo "go$v"
}

# gomodVersion prints the version the nearest go.mod requires: the `toolchain` directive
# if it has one (that is what go would select), else the `go` directive.
gomodVersion() {
	local dir="$PWD" mod=""
	while :; do
		if [ -f "$dir/go.mod" ]; then
			mod="$dir/go.mod"
			break
		fi
		[ "$dir" = "/" ] && break
		dir="$(dirname "$dir")"
	done
	[ -n "$mod" ] || die "no go.mod here or above $PWD — pass a version, or 'latest'"
	local want
	want="$(awk '$1 == "toolchain" { print $2; exit }' "$mod")"
	[ -n "$want" ] || want="$(awk '$1 == "go" { print $2; exit }' "$mod")"
	[ -n "$want" ] || die "$mod has no go directive — pass a version, or 'latest'"
	echo "$want"
}

# latestVersion asks go.dev, the same source scripts/check-go.sh uses.
latestVersion() {
	local v
	v="$(curl -fsSL --max-time 10 'https://go.dev/VERSION?m=text' | head -n1)" ||
		die "couldn't reach go.dev for the latest version — pass one explicitly"
	[ -n "$v" ] || die "go.dev returned no version — pass one explicitly"
	echo "$v"
}

# atLeast reports whether $1 is a version at least as new as $2.
atLeast() {
	[ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" = "$2" ]
}

# The links point into the module cache, so `go clean -modcache` can orphan them and
# leave a broken `go` first on PATH. Since this command is where you'd come to fix a
# broken go, clear a dangling link before anything (including this script) trips on it.
for stale in "$bindir/go" "$bindir/gofmt"; do
	if [ -L "$stale" ] && [ ! -e "$stale" ]; then
		rm -f "$stale"
		hash -r 2>/dev/null || true
	fi
done

command -v go >/dev/null || die "no go on PATH — this needs a base Go install to bootstrap from"

# An explicit version is a pin: install exactly it. A version read from go.mod is a
# floor, so a toolchain already past it is not downgraded.
pinned=false
case "${1-}" in
"") want="$(gomodVersion)" ;;
latest) want="$(latestVersion)" ;;
-h | --help)
	usage
	exit 0
	;;
*)
	want="$1"
	pinned=true
	;;
esac
want="$(normalize "$want")"

# GOVERSION is the toolchain actually running, which is what go.mod is checked against.
have="$(go env GOVERSION 2>/dev/null || true)"
[ -n "$have" ] || die "'go env GOVERSION' failed — the go install looks broken"
if [ "$have" = "$want" ] || { [ "$pinned" = false ] && atLeast "$have" "$want"; }; then
	echo "go-upgrade: $have already satisfies $want — nothing to do"
	exit 0
fi

# The toolchain download is verified against the checksum database, whose cache lives
# under GOPATH (not GOMODCACHE). The image's GOPATH is root-owned, so a writable one
# has to be substituted or the fetch dies on "permission denied" mid-verify.
gopath="$(go env GOPATH)"
if ! mkdir -p "$gopath/pkg" 2>/dev/null || [ ! -w "$gopath/pkg" ]; then
	export GOPATH="$HOME/go"
	mkdir -p "$GOPATH/pkg"
fi

echo "go-upgrade: $have -> $want (fetching)"
# GOTOOLCHAIN=<version> makes this go command run that toolchain, downloading it first;
# printing its GOROOT is the cheapest way to both trigger the fetch and locate the result.
root="$(GOTOOLCHAIN="$want" go env GOROOT)" || die "fetching $want failed (network? version doesn't exist?)"
[ -x "$root/bin/go" ] || die "fetched $want but $root/bin/go is not executable"

mkdir -p "$bindir"
ln -sfn "$root/bin/go" "$bindir/go"
ln -sfn "$root/bin/gofmt" "$bindir/gofmt"

# Report what a later command will actually get, not what was installed — a $bindir that
# isn't ahead on PATH would leave the old toolchain in charge, and silence would hide it.
hash -r 2>/dev/null || true
active="$(command -v go)"
echo "go-upgrade: installed $("$bindir/go" env GOVERSION) at $root"
echo "go-upgrade: linked $bindir/go, $bindir/gofmt"
if [ "$active" != "$bindir/go" ]; then
	echo "go-upgrade: WARNING $bindir is not ahead of $(dirname "$active") on PATH — plain 'go' is still $have" >&2
	echo "go-upgrade: fix it for this shell with: export PATH=\"$bindir:\$PATH\"" >&2
	exit 1
fi
echo "go-upgrade: 'go' is now $(go env GOVERSION)"
