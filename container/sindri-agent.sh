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
HOME="${HOME:-/home/sindri}"

echo "=== sindri agent '$AGENT' starting ==="

# Reserve the bottom row as a help/status line so the Claude pane no longer
# fills the whole screen: it shows the hotkeys a dialed-in human needs — chiefly
# how to detach again (C-b d leaves the agent running; do NOT C-c or `exit`).
# Written as ~/.tmux.conf (global options) so the server adopts it at start and
# nothing — including Claude's pane title — shadows our status-right.
cat > "$HOME/.tmux.conf" <<'TMUXCONF'
set -g status on
set -g status-interval 5
set -g status-justify left
set -g status-style "bg=colour63,fg=colour231"
set -g status-left "#[bold] sindri · #S #[default] "
set -g status-left-length 40
set -g status-right "detach: C-b d "
set -g status-right-length 50
set -g window-status-current-format ""
set -g window-status-format ""
set -g allow-rename off
set -g automatic-rename off
set -g set-titles off
# Mouse: drag to select (copy-mode), wheel to scroll; copy to the system
# clipboard via OSC52 so a dialed-in human can mark/copy. (Hold Shift to fall
# back to the terminal's own native selection.)
set -g mouse on
set -g set-clipboard on
TMUXCONF

if [ -n "${SINDRI_SHELL:-}" ]; then
	tmux new-session -d -s "$SESSION" bash
else
	# Single-quote the command so $(cat ...) is evaluated by tmux's shell at
	# session start, not here — the system prompt is multi-line. When Claude
	# exits, drop into an interactive shell (exec bash) so the session lives on
	# and a dialed-in human lands at a prompt instead of the pane dying.
	tmux new-session -d -s "$SESSION" \
		'claude --dangerously-skip-permissions --append-system-prompt "$(cat /home/sindri/.claude/system-prompt.txt)"; exec bash -i'
fi

# Belt-and-suspenders: re-source in case the server was already running.
tmux source-file "$HOME/.tmux.conf" 2>/dev/null || true

echo "=== session ready — hub injects via 'tmux send-keys -t $SESSION' ==="

exec sleep infinity
