#!/usr/bin/env bash
# Print an agent's activity log from a hub.db. Usage: dump-events.sh <db> <agent>
set -euo pipefail
DB="$1"
AGENT="$2"
python3 - "$DB" "$AGENT" <<'PY'
import sqlite3, sys
db, agent = sys.argv[1], sys.argv[2]
c = sqlite3.connect(db)
for t, p in c.execute("SELECT type,payload FROM events WHERE agent=? ORDER BY id", (agent,)):
    print(f" • {t} — {p}")
PY
