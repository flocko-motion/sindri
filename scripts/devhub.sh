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
	( cd "$T" && "$SINDRI" agent new brokkr >/dev/null && "$SINDRI" agent launch brokkr --shell ); sleep 2
	echo "== host socket =="; ls -ln "$T/.sindri/sockets/brokkr.sock"; echo "host uid: $(id -u)"
	echo "== in-pod id =="; podman exec sindri-brokkr id
	echo "== in-pod connect test =="
	podman exec sindri-brokkr python3 -c \
		"import socket; s=socket.socket(socket.AF_UNIX); s.connect('/run/sindri.sock'); print('CONNECTED')" 2>&1 || true
	;;
demo)
	start_hub
	( cd "$T" && "$SINDRI" agent new brokkr >/dev/null && "$SINDRI" agent launch brokkr --shell ); sleep 2
	echo "== sindri-worker (menu) =="; podman exec sindri-brokkr sindri-worker || true
	echo "== sindri-worker status =="; podman exec sindri-brokkr sindri-worker status || true
	echo "== sindri-worker approve (invisible) =="; podman exec sindri-brokkr sindri-worker approve || true
	;;
loop)
	# A td task is the unit of work.
	( cd "$T" && td init >/dev/null 2>&1 || true )
	( cd "$T" && td create -t feature -p high -- "wire the doohickey" >/dev/null 2>&1 || true )
	start_hub
	( cd "$T" && "$SINDRI" agent new brokkr --role worker >/dev/null && "$SINDRI" agent launch brokkr --shell )
	( cd "$T" && "$SINDRI" agent new rune --role reviewer >/dev/null && "$SINDRI" agent launch rune --shell )
	sleep 2

	echo "== worker: next (claim a task) =="
	podman exec sindri-brokkr sindri-worker next
	echo "== worker edits /workspace (simulated) =="
	echo "the doohickey, wired" > "$T/.worktrees/brokkr/doohickey.txt"
	echo "== worker: submit =="
	podman exec sindri-brokkr sindri-worker submit "wired the doohickey"
	echo "== PRs =="
	( cd "$T" && "$SINDRI" pr list )
	PR="$( cd "$T" && "$SINDRI" pr list | awk 'NR==1{print $1}' )"
	echo "== reviewer: approve $PR =="
	podman exec sindri-rune sindri-worker approve "$PR"
	echo "== human: merge $PR =="
	( cd "$T" && "$SINDRI" pr merge "$PR" )
	echo "== worker pane (verdict routed back) =="
	podman exec sindri-brokkr tmux capture-pane -p -t brokkr | grep -v '^$' | tail -4
	echo "== final PRs =="
	( cd "$T" && "$SINDRI" pr list )
	echo "== task status in td =="
	( cd "$T" && td list --all 2>/dev/null | tail -3 || true )
	;;
claude)
	# Launch a REAL Claude worker (uses your ~/.claude credentials + API tokens).
	# Boots Claude, lets it receive the hub kickoff, then captures the pane.
	( cd "$T" && td init >/dev/null 2>&1 || true )
	( cd "$T" && td create -t feature -p high -- "add a GREETING file saying hello" >/dev/null 2>&1 || true )
	start_hub
	( cd "$T" && "$SINDRI" agent new brokkr --role worker >/dev/null && "$SINDRI" agent launch brokkr )
	echo "waiting ~25s for Claude to boot and act on the kickoff..."
	sleep 25
	echo "== brokkr pane =="
	podman exec sindri-brokkr tmux capture-pane -p -t brokkr | tail -30
	echo "== activity log =="
	python3 - "$T/.sindri/hub.db" <<'PY'
import sqlite3, sys
for t, p in sqlite3.connect(sys.argv[1]).execute("SELECT type,payload FROM events WHERE agent='brokkr' ORDER BY id"):
    print(f" • {t} — {p}")
PY
	;;
fullloop)
	# Full autonomous loop with TWO real Claude agents (worker + reviewer).
	# Uses ~/.claude credentials + API tokens. The human only merges.
	( cd "$T" && td init >/dev/null 2>&1 || true )
	( cd "$T" && td create -t feature -p high -- "add a file named GREETING containing the word hello" >/dev/null 2>&1 || true )
	start_hub
	( cd "$T" && "$SINDRI" agent new brokkr --role worker >/dev/null && "$SINDRI" agent launch brokkr )
	( cd "$T" && "$SINDRI" agent new rune --role reviewer >/dev/null && "$SINDRI" agent launch rune )
	echo "two real Claude agents live; polling for the reviewer's verdict (up to ~180s)..."
	PR=""
	for i in $(seq 1 30); do
		sleep 6
		line="$( cd "$T" && "$SINDRI" pr list 2>/dev/null | head -1 )"
		[ -n "$line" ] && echo "  [$((i*6))s] $line"
		status="$(echo "$line" | awk '{print $2}')"
		if [ "$status" = "approved" ]; then PR="$(echo "$line" | awk '{print $1}')"; break; fi
	done
	if [ -n "$PR" ]; then
		echo "== human: merge $PR =="
		( cd "$T" && "$SINDRI" pr merge "$PR" )
	else
		echo "(no approved PR within the window — see panes below)"
	fi
	sleep 6
	echo "== worker pane =="; podman exec sindri-brokkr tmux capture-pane -p -t brokkr | grep -v '^$' | tail -18
	echo "== reviewer pane =="; podman exec sindri-rune tmux capture-pane -p -t rune | grep -v '^$' | tail -18
	echo "== brokkr activity log =="
	python3 - "$T/.sindri/hub.db" <<'PY'
import sqlite3, sys
for t, p in sqlite3.connect(sys.argv[1]).execute("SELECT type,payload FROM events WHERE agent='brokkr' ORDER BY id"):
    print(f" • {t} — {p}")
PY
	echo "== final PRs / task =="; ( cd "$T" && "$SINDRI" pr list; td list --all 2>/dev/null | tail -2 )
	;;
*)
	echo "unknown mode: $MODE" >&2
	exit 2
	;;
esac
