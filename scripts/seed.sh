#!/usr/bin/env bash
# Seed a mock task hierarchy into the CURRENT repo's td store so the TUI/CLI has
# something to show. Every title is prefixed "Mock:" so they're easy to spot and
# close later. Safe to re-run (it just adds more).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
mk() { td -w "$ROOT" create "$@" 2>/dev/null | grep -oE 'td-[a-f0-9]+' | head -1; }

ep=$(mk -t epic -p P1 "Mock: dashboard polish epic")
f1=$(mk -t feature -p P1 --parent "$ep" "Mock: tabbed TUI feature")
mk -t task -p P2 --parent "$f1" "Mock: scroll viewport primitive" >/dev/null
mk -t task -p P3 --parent "$f1" "Mock: hierarchical task tree view" >/dev/null
f2=$(mk -t feature -p P2 --parent "$ep" "Mock: action wiring feature")
mk -t task -p P2 --parent "$f2" "Mock: merge from the PRs tab" >/dev/null
mk -t bug -p P0 "Mock: crash on empty user input" >/dev/null
mk -t task "Mock: standalone chore with no parent" >/dev/null

echo "Seeded mock tasks under $ROOT/.todos"
echo "In the TUI press 'r' to refresh (or restart 'sindri hub')."
