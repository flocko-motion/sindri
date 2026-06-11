#!/usr/bin/env bash
# Sindri agent entrypoint (hub architecture).
#
# The agent runs INTERACTIVE inside a tmux session named after the agent. The
# hub delivers all inbound messages by `tmux send-keys` into this session
# ("as if the user typed"), and a human can `tmux attach` to dial in. The
# container's PID 1 is a sleep that keeps the pod alive; the tmux server runs
# the real session independently, so a hub crash never touches it.
#
# Phase 1 runs an interactive shell to prove the inbound channel end to end.
# Later phases set SINDRI_AGENT_CMD to launch interactive Claude here instead.
set -euo pipefail

AGENT="${1:-${SINDRI_AGENT:-agent}}"
SESSION="$AGENT"
CMD="${SINDRI_AGENT_CMD:-bash}"

echo "=== sindri agent '$AGENT' starting (session: $SESSION, cmd: $CMD) ==="

# Start the detached interactive session. The tmux server daemonises; PID 1
# below keeps the container alive so the session persists.
tmux new-session -d -s "$SESSION" "$CMD"

echo "=== session ready — hub injects via 'tmux send-keys -t $SESSION' ==="

exec sleep infinity
