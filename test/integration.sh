#!/usr/bin/env bash
set -euo pipefail

# ── Fully automated integration test for Sindri ─────────────────────────────
#
# Builds the Sindri container image, then runs a real Claude Code agent
# inside it through the full workflow:
#
#   1. Agent picks up td task, discovers missing info, blocks → SINDRI_BLOCKED
#   2. Script (host) answers the question, unblocks
#   3. Agent implements, tests, creates PR → SINDRI_PR_CREATED
#   4. Script (host) approves PR
#   5. Agent merges → SINDRI_MERGED
#
# The agent runs inside a Podman container (the real Sindri path).
# The "human" runs on the host using td and gh-local directly.
# Auth: host's Claude session is mounted into the container.
#
# Usage:  ./test/integration.sh
# Requires: git, td, go, jq, podman

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SINDRI_ROOT="$(dirname "$SCRIPT_DIR")"

CYAN='\033[0;36m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[2m'
RESET='\033[0m'

section() { echo ""; echo -e "${CYAN}══ $* ══${RESET}"; echo ""; }
log()     { echo -e "${YELLOW}▸${RESET} $*"; }
dim()     { echo -e "${DIM}$*${RESET}"; }
ok()      { echo -e "${GREEN}✓${RESET} $*"; }
err()     { echo -e "${RED}✗${RESET} $*"; }

# ── Preflight ───────────────────────────────────────────────────────────────
for cmd in git td go jq podman; do
    command -v "$cmd" >/dev/null || { echo "FATAL: $cmd not found"; exit 1; }
done

CLAUDE_CREDS="$HOME/.claude/.credentials.json"
if [[ ! -f "$CLAUDE_CREDS" ]]; then
    echo "FATAL: no Claude credentials at $CLAUDE_CREDS"
    echo "Run 'claude /login' first."
    exit 1
fi

# ── Build gh-local (for host-side human commands) ───────────────────────────
GH_BIN="$SINDRI_ROOT/gh-local/gh"
if [[ ! -x "$GH_BIN" ]]; then
    log "Building gh-local..."
    (cd "$SINDRI_ROOT/gh-local" && go build -o gh .)
fi

# ── Stage host binaries for container build ─────────────────────────────────
mkdir -p "$SINDRI_ROOT/bin"
cp "$(which td)" "$SINDRI_ROOT/bin/td"
cp "$(which yq)" "$SINDRI_ROOT/bin/yq"

# ── Build container image ──────────────────────────────────────────────────
IMAGE="sindri-agent:test"
section "BUILD"
log "Building container image: $IMAGE"
podman build -t "$IMAGE" -f "$SINDRI_ROOT/dockerfiles/Dockerfile" "$SINDRI_ROOT"

# ── Create throwaway workspace ──────────────────────────────────────────────
WORKDIR=$(mktemp -d /tmp/sindri-test-XXXX)
trap 'echo ""; log "Workspace preserved at: $WORKDIR"' EXIT

REPO="$WORKDIR/repo"
WORKTREE="$WORKDIR/worktree"
CLAUDE_HOME="$WORKDIR/claude-home"

section "SETUP"

log "repo:     $REPO"
log "worktree: $WORKTREE"

# Container's .claude dir — writable, with host credentials copied in
mkdir -p "$CLAUDE_HOME"
cp "$CLAUDE_CREDS" "$CLAUDE_HOME/.credentials.json"
echo '{}' > "$WORKDIR/.claude.json"

# ── Init main repo ─────────────────────────────────────────────────────────
mkdir -p "$REPO"
git -C "$REPO" init -b main
git -C "$REPO" config user.name "test"
git -C "$REPO" config user.email "test@test"

cat > "$REPO/calculator.py" << 'PYEOF'
"""A simple calculator module."""


def add(a: float, b: float) -> float:
    return a + b


def subtract(a: float, b: float) -> float:
    return a - b


def multiply(a: float, b: float) -> float:
    return a * b


def divide(a: float, b: float) -> float:
    if b == 0:
        raise ValueError("Cannot divide by zero")
    return a / b
PYEOF

cat > "$REPO/test_calculator.py" << 'PYEOF'
"""Tests for the calculator module."""

from calculator import add, subtract, multiply, divide
import pytest


def test_add():
    assert add(2, 3) == 5

def test_subtract():
    assert subtract(5, 3) == 2

def test_multiply():
    assert multiply(4, 3) == 12

def test_divide():
    assert divide(10, 2) == 5.0

def test_divide_by_zero():
    with pytest.raises(ValueError):
        divide(1, 0)
PYEOF

git -C "$REPO" add -A
git -C "$REPO" commit -m "initial: calculator module with tests"

# ── Create worktree ─────────────────────────────────────────────────────────
git -C "$REPO" branch agent-work
git -C "$REPO" worktree add "$WORKTREE" agent-work

# ── Init td ─────────────────────────────────────────────────────────────────
log "Initializing td..."
(cd "$REPO" && td init)

ISSUE_OUTPUT=$(cd "$REPO" && td create \
    --title "Implement discount calculation" \
    --body "$(cat <<'BODY'
Add a calculate_discount(price, quantity) function to calculator.py.

The function should apply tiered discounts based on quantity.
The exact discount tiers are NOT specified — you MUST ask before implementing.
Also add tests in test_calculator.py.
BODY
)" 2>&1)

ISSUE_ID=$(echo "$ISSUE_OUTPUT" | grep -oP 'td-[0-9a-f]+' | head -1)
log "Created issue: $ISSUE_ID"
echo ""
(cd "$REPO" && td show "$ISSUE_ID")

# ── Host environment (for human-side commands) ──────────────────────────────
export PATH="$(dirname "$GH_BIN"):$PATH"
export GH_LOCAL_BASE=main
export TD_ROOT="$REPO/.todos"

SESSION_ID=$(uuidgen)

# ── Helper: run an agent turn inside the container ──────────────────────────
#
# Mounts the workspace at the SAME absolute path so git worktree refs resolve.
# Mounts a prepared Claude home for auth + session persistence across turns.
#
agent_turn() {
    local prompt="$1"
    local label="$2"
    local expected_sentinel="$3"
    local is_resume="${4:-false}"

    section "AGENT: $label"

    log "Prompt:"
    dim "$prompt"
    echo ""

    local claude_args=(
        -p
        --dangerously-skip-permissions
        --verbose
        --output-format stream-json
        --model sonnet
    )

    if [[ "$is_resume" == "true" ]]; then
        claude_args+=(--resume "$SESSION_ID")
    else
        claude_args+=(--session-id "$SESSION_ID")
    fi

    log "Running in container..."
    echo ""

    local tmpfile
    tmpfile=$(mktemp)

    podman run --rm \
        --userns=keep-id \
        -v "$WORKDIR:$WORKDIR:rw,z" \
        -v "$CLAUDE_HOME:/home/sindri/.claude:rw,z" \
        -v "$WORKDIR/.claude.json:/home/sindri/.claude.json:rw,z" \
        -e GH_LOCAL_BASE=main \
        -e TD_ROOT="$REPO/.todos" \
        -w "$WORKTREE" \
        "$IMAGE" \
        claude "${claude_args[@]}" "$prompt" \
    2>&1 | tee "$tmpfile" | jq --unbuffered -r '
        if .type == "assistant" then
            (.message.content[]? |
                if .type == "tool_use" then
                    "  🤖🔨 " + .name + " " + (
                        if .name == "Bash" then (.input.command // "" | .[:120])
                        elif .name == "Read" then (.input.file_path // "")
                        elif .name == "Edit" then (.input.file_path // "")
                        elif .name == "Write" then (.input.file_path // "")
                        else (.input | tostring | .[:80])
                        end)
                elif .type == "text" then
                    "  🤖💬 " + .text
                else empty end)
        elif .type == "user" and .tool_use_result then
            if (.tool_use_result | type) == "object" and (.tool_use_result.stdout // null) then
                "     ← " + (.tool_use_result.stdout | tostring | .[:200])
            elif (.tool_use_result | type) == "object" and .tool_use_result.file then
                "     ← (file content)"
            else empty end
        elif .type == "result" then
            "\n  ▸ Result: " + (.result // "done") + "\n  ▸ Cost: $" + (.total_cost_usd | tostring) + " | Turns: " + (.num_turns | tostring)
        else empty end
    ' || true

    echo ""

    LAST_RESPONSE=$(cat "$tmpfile")
    rm -f "$tmpfile"

    if echo "$LAST_RESPONSE" | grep -q "$expected_sentinel"; then
        ok "Sentinel found: $expected_sentinel"
    else
        err "Sentinel NOT found: $expected_sentinel"
        log "Agent may have deviated from the expected flow. Continuing anyway."
    fi
}

# ── Helper: show td + gh state (runs on host) ──────────────────────────────
show_state() {
    local label="${1:-state}"
    section "STATE: $label"

    log "td show $ISSUE_ID:"
    (cd "$REPO" && td show "$ISSUE_ID" 2>/dev/null) || dim "(no issue)"
    echo ""

    log "td comments $ISSUE_ID:"
    (cd "$REPO" && td comments "$ISSUE_ID" 2>/dev/null) || dim "(no comments)"
    echo ""

    log "gh pr list:"
    (cd "$WORKTREE" && gh pr list 2>/dev/null) || dim "(no PRs)"
    echo ""

    log "git log (worktree):"
    git -C "$WORKTREE" log --oneline -5 2>/dev/null || dim "(no commits)"
    echo ""
}

# ════════════════════════════════════════════════════════════════════════════
#  TURN 1 — Agent discovers task, asks a question, blocks
# ════════════════════════════════════════════════════════════════════════════

agent_turn "$(cat <<'PROMPT'
You are a software agent working in a git worktree.

TOOLS:
- td: task tracker
    td next              — get next open issue
    td start <id>        — claim it
    td show <id>         — read full details
    td comment <id> "m"  — post a comment (visible to reviewer)
    td block <id> --reason "r" — mark yourself as blocked
    td handoff <id> --done "m" — record completed work
    td review <id>       — submit for review
- gh: PR management
    gh pr create --title "t" --body "b"
    gh pr view [id]
    gh pr merge [id]
- Standard: git, python3, pytest

WORKFLOW FOR THIS TURN:
1. Run `td next` to find the next task.
2. Run `td start <id>` to claim it.
3. Read the full task with `td show <id>`.
4. The task is deliberately incomplete — information is missing.
   You MUST ask before guessing:
   a. Post your question: td comment <id> "your question here"
   b. Block yourself: td block <id> --reason "waiting for answer"
5. STOP. Do not implement anything.

IMPORTANT: When you are done and have blocked yourself, your final line must be:
SINDRI_BLOCKED

Begin now.
PROMPT
)" "discover task and ask question" "SINDRI_BLOCKED"

show_state "after turn 1"

# ════════════════════════════════════════════════════════════════════════════
#  HUMAN ACTION — Answer the question, unblock (runs on host)
# ════════════════════════════════════════════════════════════════════════════

section "HUMAN: answering question"

log "Posting answer..."
(cd "$REPO" && td comment "$ISSUE_ID" \
    "Use these discount tiers: 1-9 items = 0%, 10-49 items = 10%, 50-99 items = 20%, 100+ items = 30%. Apply the discount percentage to the unit price. Return the total price (price * quantity * (1 - discount)).")

log "Unblocking..."
(cd "$REPO" && td unblock "$ISSUE_ID" 2>/dev/null) || dim "(was not blocked)"

show_state "after human answer"

# ════════════════════════════════════════════════════════════════════════════
#  TURN 2 — Agent reads answer, implements, tests, creates PR
# ════════════════════════════════════════════════════════════════════════════

agent_turn "$(cat <<PROMPT
The issue $ISSUE_ID has been unblocked. The reviewer answered your question.

DO THESE STEPS IN ORDER:
1. Read the answer: td comments $ISSUE_ID
2. Implement the calculate_discount(price, quantity) function in calculator.py.
3. Add tests for it in test_calculator.py.
4. Run tests: python3 -m pytest test_calculator.py -v
5. Commit all changes: git add -A && git commit -m "feat: add tiered discount calculation"
6. Create a PR: gh pr create --title "Add discount calculation" --body "Implements tiered discount per reviewer specs"
7. Record work: td handoff $ISSUE_ID --done "implemented tiered discount calculation"
8. Submit: td review $ISSUE_ID
9. STOP.

IMPORTANT: When you are done and the PR is created, your final line must be:
SINDRI_PR_CREATED
PROMPT
)" "implement and create PR" "SINDRI_PR_CREATED" true

show_state "after turn 2"

# ── Verify implementation ──────────────────────────────────────────────────
section "VERIFY: implementation"

log "calculator.py:"
cat "$WORKTREE/calculator.py"
echo ""

log "Running tests:"
(cd "$WORKTREE" && python3 -m pytest test_calculator.py -v 2>&1) || true
echo ""

# ════════════════════════════════════════════════════════════════════════════
#  HUMAN ACTION — Approve the PR (runs on host)
# ════════════════════════════════════════════════════════════════════════════

section "HUMAN: approving PR"

PR_ID=$(cd "$WORKTREE" && gh pr list 2>/dev/null | awk '{print $1}' | head -1 || true)
if [[ -z "$PR_ID" ]]; then
    PR_ID=$(ls "$REPO/.git/pr/" 2>/dev/null | sed 's/\.json$//' | head -1 || true)
fi

if [[ -n "$PR_ID" ]]; then
    log "Approving PR: $PR_ID"
    (cd "$WORKTREE" && gh pr review "$PR_ID" --approve)
    log "PR state:"
    (cd "$WORKTREE" && gh pr view "$PR_ID" 2>/dev/null) || true
else
    err "No PR found to approve"
    log ".git/pr/ contents:"
    ls -la "$REPO/.git/pr/" 2>/dev/null || dim "(directory doesn't exist)"
fi

# ════════════════════════════════════════════════════════════════════════════
#  TURN 3 — Agent merges
# ════════════════════════════════════════════════════════════════════════════

agent_turn "$(cat <<PROMPT
The PR ${PR_ID:-pr-agent-work} has been approved.

DO THESE STEPS:
1. Merge the PR: gh pr merge ${PR_ID:-pr-agent-work}
2. Verify the merge succeeded.

IMPORTANT: When the merge is complete, your final line must be:
SINDRI_MERGED
PROMPT
)" "merge approved PR" "SINDRI_MERGED" true

# ── Verify merge ───────────────────────────────────────────────────────────
section "VERIFY: merge result"

log "calculator.py on main:"
git -C "$REPO" show main:calculator.py 2>/dev/null || dim "(not on main)"
echo ""

log "git log --all --graph:"
git -C "$REPO" log --oneline --all --graph -15
echo ""

if [[ -n "${PR_ID:-}" ]] && [[ -f "$REPO/.git/pr/${PR_ID}.json" ]]; then
    log "PR final state:"
    jq . "$REPO/.git/pr/${PR_ID}.json"
fi

# ════════════════════════════════════════════════════════════════════════════
section "DONE"
log "Workspace: $WORKDIR"
