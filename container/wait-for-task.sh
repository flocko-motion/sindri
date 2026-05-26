#!/usr/bin/env bash
# Polls td for the next open task, waiting up to TIMEOUT seconds.
# Exits 0 with the task ID on stdout, or exits 1 if no task appears.

TIMEOUT="${1:-300}"  # default 5 minutes
INTERVAL="${2:-10}"  # poll every 10 seconds
ELAPSED=0

while [ "$ELAPSED" -lt "$TIMEOUT" ]; do
    TASK=$(td next 2>/dev/null | grep -oP 'td-[0-9a-f]+' | head -1)
    if [ -n "$TASK" ]; then
        echo "$TASK"
        exit 0
    fi
    sleep "$INTERVAL"
    ELAPSED=$((ELAPSED + INTERVAL))
done

exit 1
