#!/usr/bin/env bash
# Sindri agent entrypoint (hub architecture).
#
# The agent runs INTERACTIVE inside a tmux session named after the agent. The
# hub delivers all inbound messages by `tmux send-keys` into this session
# ("as if the user typed"), and a human can `tmux attach` to dial in. The
# container's PID 1 is an init that keeps the pod alive and reaps; the tmux
# server runs the real session independently, so a hub crash never touches it.
#
# Default: launch interactive Claude with the hub-provided system prompt
# (/home/sindri/.claude/system-prompt.txt). SINDRI_SHELL=1 runs a bare shell
# instead — used for deterministic demos and debugging.
set -euo pipefail

AGENT="${1:-${SINDRI_AGENT:-agent}}"
SESSION="$AGENT"
HOME="${HOME:-/home/sindri}"

echo "=== sindri agent '$AGENT' starting ==="

# Reserve the bottom row for the hotkeys a dialed-in human needs — chiefly detach (C-b d; do
# NOT C-c or `exit`). Global options in ~/.tmux.conf, adopted at server start so nothing later
# shadows status-right.
cat > "$HOME/.tmux.conf" <<'TMUXCONF'
# Truecolor: the pane must be 256-colour AND the client flagged RGB-capable, or tmux
# downsamples and Claude's orange goes red — tmux 3.5 needs terminal-features for it.
set -g default-terminal "tmux-256color"
set -as terminal-features ",*:RGB"
set -g status on
set -g status-interval 5
set -g status-justify left
set -g status-style "bg=colour63,fg=colour231"
set -g status-left "#[bold] sindri · #S #[default] "
set -g status-left-length 40
set -g status-right "scrollback: Ctrl-O (Claude) · detach: C-b d "
set -g status-right-length 60
set -g window-status-current-format ""
set -g window-status-format ""
set -g allow-rename off
set -g automatic-rename off
set -g set-titles off
# Mouse: drag to select (copy-mode), wheel to scroll, copy to the system clipboard via
# OSC52. (Hold Shift for the terminal's own native selection instead.)
set -g mouse on
set -g set-clipboard on
# Scrollback: vi keys in copy-mode so `prefix [` then C-u/C-d (half-page), C-b/C-f (page), g/G
# and `/` search all work — the default emacs mode leaves C-u/C-d unbound, so scrollback
# appeared stuck. Generous history for a chatty agent.
set -g mode-keys vi
set -g history-limit 50000
TMUXCONF

# Overrides status-left above, once the hub has a model to name (nothing on a first launch).
# Stripped of '#'/'"': tmux's status format and this line's own quoting would read them as syntax.
if [ -n "${SINDRI_MODEL:-}" ]; then
	MODEL_SAFE="${SINDRI_MODEL//[\"#]/}"
	cat >> "$HOME/.tmux.conf" <<TMUXMODEL
set -g status-left "#[bold] sindri · #S ($MODEL_SAFE) #[default] "
TMUXMODEL
fi

# A detached new-session with no -x/-y sticks at tmux's own 80x24 until attached; the hub passes
# the caller's preview size (TUI only) so a fresh session isn't cramped from the start.
SIZE_ARGS=()
if [ -n "${SINDRI_COLS:-}" ] && [ -n "${SINDRI_LINES:-}" ]; then
	SIZE_ARGS=(-x "$SINDRI_COLS" -y "$SINDRI_LINES")
fi

if [ -n "${SINDRI_SHELL:-}" ]; then
	tmux new-session -d -s "$SESSION" "${SIZE_ARGS[@]}" bash
else
	# --continue resumes this workspace's session across a restart, but EXITS NON-ZERO with
	# nothing to resume, so `||` falls back to a fresh `claude`, not bash.
	# --append-system-prompt every launch: dropping it costs the agent its role, even on --continue.
	# Single-quoted so tmux's shell evaluates the multi-line $() at session start, not this script's.
	# `stty sane` undoes Claude's raw, echo-off terminal so a dial-in lands at a prompt.
	tmux new-session -d -s "$SESSION" "${SIZE_ARGS[@]}" \
		'SP="$(cat /home/sindri/.claude/system-prompt.txt)"; claude --continue --dangerously-skip-permissions --append-system-prompt "$SP" || claude --dangerously-skip-permissions --append-system-prompt "$SP"; stty sane; exec bash -i'
fi

# Belt-and-suspenders: re-source in case the server was already running.
tmux source-file "$HOME/.tmux.conf" 2>/dev/null || true

echo "=== session ready — hub injects via 'tmux send-keys -t $SESSION' ==="

# PID 1 must reap: tmux double-forks, so orphans land here, and a `sleep` never calls wait() —
# one session reached ~2000 defunct [git] entries and pids.max, after which nothing could fork.
exec tini -s -- sleep infinity
