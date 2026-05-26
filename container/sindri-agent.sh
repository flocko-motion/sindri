#!/usr/bin/env bash
# Sindri agent wrapper — runs Claude in a loop, waiting for tasks between turns.
# All output goes to stdout (captured by podman logs).
# The TUI sends follow-up messages via:
#   podman exec <container> claude -c -p "message"
set -euo pipefail

PROMPT_FILE="/home/sindri/.claude/system-prompt.txt"
PROMPT="${1:-$(cat "$PROMPT_FILE" 2>/dev/null || echo 'You are a Sindri agent.')}"

echo "=== sindri agent starting ==="
echo "=== prompt: ${PROMPT:0:80}... ==="

# First turn with streaming output
claude --dangerously-skip-permissions \
    --verbose --output-format stream-json \
    -p "$PROMPT" 2>&1 || true

echo "=== first turn complete ==="

# Keep container alive for follow-up messages via:
#   podman exec <container> claude -c -p "next instruction"
# Each exec continues the most recent session in /workspace.
echo "=== agent idle, waiting for input ==="
echo "=== send messages with: podman exec <container> claude -c -p 'your message' ==="
sleep infinity
