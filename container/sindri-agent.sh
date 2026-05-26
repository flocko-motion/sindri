#!/usr/bin/env bash
# Sindri agent — starts Claude as a background daemon inside the container.
# Output streamed via: podman logs -f <container>
# Send messages via: podman exec <container> claude -c -p "message"
set -euo pipefail

PROMPT_FILE="/home/sindri/.claude/system-prompt.txt"
PROMPT="${1:-$(cat "$PROMPT_FILE" 2>/dev/null || echo 'You are a Sindri agent.')}"

echo "=== sindri agent starting ==="

# Start Claude as a background daemon, capture output to extract session ID
BG_OUTPUT=$(claude --bg --dangerously-skip-permissions \
    --verbose --output-format stream-json \
    -p "$PROMPT" 2>&1)
echo "$BG_OUTPUT"

# Extract short session ID from "backgrounded · XXXXXXXX" line
SESSION_ID=$(echo "$BG_OUTPUT" | grep -oP 'backgrounded · \K[a-f0-9]+' || true)
echo "=== session: ${SESSION_ID:-unknown} ==="

if [ -z "$SESSION_ID" ]; then
    echo "WARNING: could not determine session ID, falling back to sleep"
    exec sleep infinity
fi

# Stream daemon logs to container stdout (podman logs -f picks this up)
exec claude logs "$SESSION_ID"
