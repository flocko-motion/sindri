#!/usr/bin/env bash
# Dev harness for the hub: spin a throwaway repo, start a persistent hub, launch
# an agent, then run a mode against it. Used by `make demo` / `make diag`.
#
# Usage: scripts/devhub.sh <demo|diag>
set -euo pipefail

MODE="${1:-demo}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SINDRI="$ROOT/bin/sindri"
AGENT="brokkr"

T="$(mktemp -d)"
HUB_PID=""
cleanup() {
	[ -n "$HUB_PID" ] && kill "$HUB_PID" 2>/dev/null || true
	podman rm -f "sindri-$AGENT" >/dev/null 2>&1 || true
	rm -rf "$T"
}
trap cleanup EXIT

git -C "$T" init -q
git -C "$T" config user.email t@t
git -C "$T" config user.name t
git -C "$T" commit -q --allow-empty -m init

( cd "$T" && "$SINDRI" hub >/tmp/sindri-devhub.log 2>&1 & echo $! >/tmp/sindri-devhub.pid )
HUB_PID="$(cat /tmp/sindri-devhub.pid)"
sleep 0.6

( cd "$T" && "$SINDRI" new "$AGENT" >/dev/null && "$SINDRI" launch "$AGENT" )
sleep 2

POD="sindri-$AGENT"

case "$MODE" in
diag)
	echo "== host socket =="
	ls -ln "$T/.sindri/sockets/$AGENT.sock"
	echo "host uid: $(id -u)"
	echo "== in-pod id =="
	podman exec "$POD" id
	echo "== in-pod socket perms =="
	podman exec "$POD" ls -ln /run/sindri.sock
	echo "== in-pod connect test =="
	podman exec "$POD" python3 -c \
		"import socket; s=socket.socket(socket.AF_UNIX); s.connect('/run/sindri.sock'); print('CONNECTED')" 2>&1 || true
	;;
demo)
	echo "== browser: sindri-worker (no args -> menu) =="
	podman exec "$POD" sindri-worker || true
	echo "== browser: sindri-worker status =="
	podman exec "$POD" sindri-worker status || true
	echo "== browser: sindri-worker log 'tightened the bolt' =="
	podman exec "$POD" sindri-worker log tightened the bolt || true
	echo "== browser: sindri-worker approve (reviewer-only -> invisible) =="
	podman exec "$POD" sindri-worker approve || true
	echo "== activity log =="
	podman exec "$POD" true # ensure pod alive
	"$ROOT/scripts/dump-events.sh" "$T/.sindri/hub.db" "$AGENT" || true
	;;
*)
	echo "unknown mode: $MODE" >&2
	exit 2
	;;
esac
