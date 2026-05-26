#!/usr/bin/env bash
# Sindri agent — starts Claude as a background daemon inside the container.
# The daemon runs independently of this script.
# The script's only job is to start the daemon and keep the container alive.
#
# Output: podman exec <container> claude logs <session-id>
# Input:  podman exec <container> claude -c -p "message"
# Status: podman exec <container> claude agents --json
set -euo pipefail

PROMPT_FILE="/home/sindri/.claude/system-prompt.txt"
PROMPT="${1:-$(cat "$PROMPT_FILE" 2>/dev/null || echo 'You are a Sindri agent.')}"

echo "=== sindri agent starting ==="

# Start Claude as a background daemon
BG_OUTPUT=$(claude --bg --dangerously-skip-permissions \
    --verbose --output-format stream-json \
    --disallowedTools "EnterWorktree,ExitWorktree" \
    -p "$PROMPT" 2>&1)
echo "$BG_OUTPUT"

# Extract and persist session ID for external tools
SESSION_ID=$(echo "$BG_OUTPUT" | grep -oP 'backgrounded · \K[a-f0-9]+' || true)
echo "$SESSION_ID" > /tmp/sindri-session-id
echo "=== session: ${SESSION_ID:-unknown} ==="
echo "=== daemon running ==="

# Keep container alive. The daemon runs in the background independently.
exec sleep infinity
