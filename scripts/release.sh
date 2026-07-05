#!/usr/bin/env bash
# Cut a release as a self-contained cycle: validate the bump arg, ensure the tree
# is clean, lint (the quality gate), then rebase your branch onto the default
# branch (catching any conflict locally, up front), force-push it, open + merge a
# PR into the default branch (so the tag points at merged code), tag the merged
# tip, and push the tag — which triggers the release workflow (build + attach the
# .deb). It then returns you to the branch you started on; it never leaves you on,
# or commits directly to, the default branch. The rebase rewrites history, so the
# branch push is a lease-guarded force.
#
# Usage: make release <major|minor|patch>   (aliases: breaking|feature|fix;
#        needs gh when run from a feature branch)
set -euo pipefail

bump="${1:-}"
case "$bump" in
	major | breaking) bump=major ;; # incompatible change
	minor | feature) bump=minor ;;  # backwards-compatible feature
	patch | fix) bump=patch ;;      # backwards-compatible fix
	*)
		echo "usage: make release <major|breaking | minor|feature | patch|fix>" >&2
		exit 1
		;;
esac

# 1. Everything committed — the release must capture a clean, committed state.
if [ -n "$(git status --porcelain)" ]; then
	echo "working tree is dirty — commit or stash before releasing" >&2
	exit 1
fi

# 2. Quality gate (after the arg + clean checks, so a bad arg / dirty tree fails
#    fast): the active Go toolchain must be current and the linters must pass.
echo "verifying (toolchain + linters)…"
if ! make verify; then
	echo "verify failed — fix it before releasing" >&2
	exit 1
fi

git fetch --tags --force origin >/dev/null 2>&1 || true
default="$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')"
default="${default:-master}"
start="$(git rev-parse --abbrev-ref HEAD)"

# Always end up back where we started — a release must not park you elsewhere.
trap 'git checkout --quiet "$start" 2>/dev/null || true' EXIT

if [ "$start" != "$default" ]; then
	# 2. Feature branch: push it, open a PR if there isn't one, and merge it into
	#    the default branch — without switching this checkout or deleting the
	#    branch (so we can return to it). The tag then comes off the merged tip.
	if ! command -v gh >/dev/null; then
		echo "on '$start' — releasing needs it merged to '$default'. Install gh (https://cli.github.com) or merge manually, then re-run." >&2
		exit 1
	fi
	# Rebase onto the current default branch FIRST. A branch that has merely fallen
	# behind is brought current (so the PR merges cleanly), and a real conflict is
	# caught here — locally, where you can fix it — instead of after a push and a
	# 30-minute auto-merge wait that can never complete. Rebasing rewrites history,
	# so the push below must be a (lease-guarded) force.
	echo "rebasing '$start' onto '$default'…"
	git fetch origin "$default" >/dev/null 2>&1 || true
	if ! git rebase "origin/$default"; then
		git rebase --abort 2>/dev/null || true
		echo "'$start' conflicts with '$default' — run 'git rebase origin/$default', resolve the conflicts, then re-run." >&2
		exit 1
	fi
	echo "pushing '$start' and merging it into '$default'…"
	# --force-with-lease: the rebase above rewrote history, so a plain push would be
	# rejected as non-fast-forward. The lease still refuses to clobber the branch if
	# someone else pushed to it since our fetch.
	git push --force-with-lease -u origin "$start"
	if [ -z "$(gh pr list --head "$start" --state open --json number --jq '.[0].number' 2>/dev/null)" ]; then
		echo "opening a pull request…"
		gh pr create --base "$default" --head "$start" --fill
	fi
	# master's branch protection requires the CI check to pass before a merge, so an
	# immediate `--merge` is refused. --auto queues the merge for when checks pass;
	# we then wait for it, so the tag below comes off the truly-merged tip (not the
	# pre-merge commit). Needs auto-merge enabled on the repo (Settings → Pull
	# Requests → Allow auto-merge).
	echo "enabling auto-merge (merges once CI passes)…"
	gh pr merge "$start" --merge --auto
	echo "waiting for the PR to merge — CI must pass first…"
	state=""
	for _ in $(seq 1 180); do # up to ~30 min for CI + merge
		read -r state mergeable <<<"$(gh pr view "$start" --json state,mergeable --jq '.state + " " + .mergeable' 2>/dev/null)"
		[ "$state" = "MERGED" ] && break
		[ "$state" = "CLOSED" ] && { echo "PR was closed without merging" >&2; exit 1; }
		# Don't spin the full 30 min on a PR that can never merge: a definitive
		# CONFLICTING verdict (rare here, since we rebased above, but master can move)
		# means auto-merge is stuck. Bail now with a fix. UNKNOWN = GitHub still
		# computing mergeability, so we keep waiting.
		if [ "$mergeable" = "CONFLICTING" ]; then
			echo "PR conflicts with '$default' — run 'git rebase origin/$default', resolve, then re-run." >&2
			exit 1
		fi
		sleep 10
	done
	if [ "$state" != "MERGED" ]; then
		echo "PR hasn't merged yet (CI still running or failing) — merge it once green, then re-run to tag" >&2
		exit 1
	fi
	git fetch origin "$default" >/dev/null 2>&1
	target="origin/$default"
else
	# Already on the default branch: it must be in sync with origin so the tag
	# points at pushed code (never release unpushed local commits).
	if [ "$(git rev-parse HEAD)" != "$(git rev-parse "origin/$default" 2>/dev/null || git rev-parse HEAD)" ]; then
		echo "'$default' has commits not on origin — push them first" >&2
		exit 1
	fi
	target="HEAD"
fi

# 3. Tag the merged default-branch tip (without checking it out) and push the tag.
latest="$(git tag --list 'v*' --sort=-v:refname | head -n1)"
latest="${latest:-v0.0.0}"
IFS=. read -r maj min pat <<<"${latest#v}"
case "$bump" in
	major) maj=$((maj + 1)); min=0; pat=0 ;;
	minor) min=$((min + 1)); pat=0 ;;
	patch) pat=$((pat + 1)) ;;
esac
next="v${maj}.${min}.${pat}"

echo "tagging ${latest} -> ${next} on ${default}"
git tag -a "$next" "$target" -m "release $next"
git push origin "$next"
echo "pushed ${next} — the release workflow will build and attach the .deb (back on '$start')"
