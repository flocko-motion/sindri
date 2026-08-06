#!/usr/bin/env bash
# This project's own submit gate: what CI runs, run before a PR exists rather than after.
#
# `verify:` in .sindri/config.yaml points the hub here, so an agent cannot submit work that fails
# the build, the tests or the architecture tests (the front-end import guard among them) — those
# rules are this repository's, and a generic toolbelt has no place knowing them.
#
# A script rather than a command line because the gate takes a path it can validate before running.
set -euo pipefail

cd "$(dirname "$0")/.."
make verify
