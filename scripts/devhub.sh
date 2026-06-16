#!/usr/bin/env bash
# Dev harness for the hub: spin a throwaway repo, start a persistent hub, launch
# agents, then run a mode. Used by `make demo` / `make diag` / `make loop`.
#
# Usage: scripts/devhub.sh <demo|diag|loop>
set -euo pipefail

MODE="${1:-demo}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SINDRI="$ROOT/bin/sindri"

T="$(mktemp -d)"
HUB_PID=""
cleanup() {
	[ -n "$HUB_PID" ] && kill "$HUB_PID" 2>/dev/null || true
	podman rm -f sindri-brokkr sindri-rune >/dev/null 2>&1 || true
	rm -rf "$T"
}
trap cleanup EXIT

git -C "$T" init -q
git -C "$T" config user.email t@t
git -C "$T" config user.name t
git -C "$T" commit -q --allow-empty -m init

start_hub() {
	( cd "$T" && "$SINDRI" hub >/tmp/sindri-devhub.log 2>&1 & echo $! >/tmp/sindri-devhub.pid )
	HUB_PID="$(cat /tmp/sindri-devhub.pid)"
	sleep 0.6
}

case "$MODE" in
diag)
	start_hub
	( cd "$T" && "$SINDRI" new brokkr >/dev/null && "$SINDRI" launch brokkr ); sleep 2
	echo "== host socket =="; ls -ln "$T/.sindri/sockets/brokkr.sock"; echo "host uid: $(id -u)"
	echo "== in-pod id =="; podman exec sindri-brokkr id
	echo "== in-pod connect test =="
	podman exec sindri-brokkr python3 -c \
		"import socket; s=socket.socket(socket.AF_UNIX); s.connect('/run/sindri.sock'); print('CONNECTED')" 2>&1 || true
	;;
demo)
	start_hub
	( cd "$T" && "$SINDRI" new brokkr >/dev/null && "$SINDRI" launch brokkr ); sleep 2
	echo "== sindri-worker (menu) =="; podman exec sindri-brokkr sindri-worker || true
	echo "== sindri-worker status =="; podman exec sindri-brokkr sindri-worker status || true
	echo "== sindri-worker approve (invisible) =="; podman exec sindri-brokkr sindri-worker approve || true
	;;
loop)
	# A td task is the unit of work.
	( cd "$T" && td init >/dev/null 2>&1 || true )
	( cd "$T" && td create -t feature -p high -- "wire the doohickey" >/dev/null 2>&1 || true )
	start_hub
	( cd "$T" && "$SINDRI" new brokkr --role worker >/dev/null && "$SINDRI" launch brokkr )
	( cd "$T" && "$SINDRI" new rune --role reviewer >/dev/null && "$SINDRI" launch rune )
	sleep 2

	echo "== worker: next (claim a task) =="
	podman exec sindri-brokkr sindri-worker next
	echo "== worker edits /workspace (simulated) =="
	echo "the doohickey, wired" > "$T/.worktrees/brokkr/doohickey.txt"
	echo "== worker: submit =="
	podman exec sindri-brokkr sindri-worker submit "wired the doohickey"
	echo "== PRs =="
	( cd "$T" && "$SINDRI" prs )
	PR="$( cd "$T" && "$SINDRI" prs | awk 'NR==1{print $1}' )"
	echo "== reviewer: approve $PR =="
	podman exec sindri-rune sindri-worker approve "$PR"
	echo "== human: merge $PR =="
	( cd "$T" && "$SINDRI" merge "$PR" )
	echo "== worker pane (verdict routed back) =="
	podman exec sindri-brokkr tmux capture-pane -p -t brokkr | grep -v '^$' | tail -4
	echo "== final PRs =="
	( cd "$T" && "$SINDRI" prs )
	echo "== task status in td =="
	( cd "$T" && td list --all 2>/dev/null | tail -3 || true )
	;;
*)
	echo "unknown mode: $MODE" >&2
	exit 2
	;;
esac
