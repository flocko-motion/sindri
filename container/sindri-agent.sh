#!/usr/bin/env bash
# Sindri agent entrypoint (hub architecture).
#
# The agent runs INTERACTIVE inside a tmux session named after the agent. The
# hub delivers all inbound messages by `tmux send-keys` into this session
# ("as if the user typed"), and a human can `tmux attach` to dial in. The
# container's PID 1 is a sleep that keeps the pod alive; the tmux server runs
# the real session independently, so a hub crash never touches it.
#
# Default: launch interactive Claude with the hub-provided system prompt
# (/home/sindri/.claude/system-prompt.txt). SINDRI_SHELL=1 runs a bare shell
# instead — used for deterministic demos and debugging.
set -euo pipefail

AGENT="${1:-${SINDRI_AGENT:-agent}}"
SESSION="$AGENT"

echo "=== sindri agent '$AGENT' starting ==="

if [ -n "${SINDRI_SHELL:-}" ]; then
	tmux new-session -d -s "$SESSION" bash
else
	# Single-quote the command so $(cat ...) is evaluated by tmux's shell at
	# session start, not here — the system prompt is multi-line.
	tmux new-session -d -s "$SESSION" \
		'claude --dangerously-skip-permissions --append-system-prompt "$(cat /home/sindri/.claude/system-prompt.txt)"'
fi

# Reserve the bottom row as a help/status line: the Claude pane gets every row
# above it, and we print the hotkeys a dialed-in human needs — chiefly how to
# detach again (C-b d leaves the agent running; do NOT C-c or `exit`).
tmux set-option -t "$SESSION" status on
tmux set-option -t "$SESSION" status-style "bg=colour63,fg=colour231"
tmux set-option -t "$SESSION" status-justify left
tmux set-option -t "$SESSION" status-left "#[bold] sindri · $AGENT #[default] "
tmux set-option -t "$SESSION" status-left-length 40
tmux set-option -t "$SESSION" status-right "detach: C-b d · scroll: C-b [ (q to exit) "
tmux set-option -t "$SESSION" status-right-length 60
tmux set-option -t "$SESSION" window-status-current-format ""
tmux set-option -t "$SESSION" window-status-format ""

echo "=== session ready — hub injects via 'tmux send-keys -t $SESSION' ==="

exec sleep infinity
