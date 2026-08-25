#!/usr/bin/env bash
# Cut a release as a self-contained cycle: validate the bump arg, ensure the tree
# is clean, lint (the quality gate), then rebase your branch onto the default
# branch (catching any conflict locally, up front), force-push it, open + merge a
# PR into the default branch (so the tag points at merged code), tag the merged
# tip, and push the tag — which triggers the release workflow (build + attach the
# sindri tarballs and the standalone brokkr binaries). Once the PR has merged it
# rebases your branch onto the updated default and pushes it, so the release
# branch ends the cycle current with main (ready for the next round) rather than
# stranded behind. It then returns you to the branch you
# started on; it never leaves you on, or commits directly to, the default branch.
# The rebases rewrite history, so the branch pushes are lease-guarded forces.
# Every wait here is a wait with a verdict: a failed PR check (or a conflict) aborts
# at once, naming it, rather than sitting out the merge timeout on something that can
# never go green — the timeout then means only "still running", as it says.
# Finally, back on your branch, it blocks on the tag-triggered release workflow and
# reports success/failure, so the CLI shows when the release is actually done.
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
	# Rebase onto the default FIRST: a branch merely behind is brought current, and a
	# real conflict surfaces locally instead of after a 30-minute auto-merge wait that
	# can never complete. This rewrites history, so the push below is a leased force.
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
	# Branch protection refuses an immediate `--merge`, so --auto queues it for when
	# checks pass and we wait, letting the tag come off the truly-merged tip. Needs
	# auto-merge enabled (Settings → Pull Requests → Allow auto-merge).
	echo "enabling auto-merge (merges once CI passes)…"
	gh pr merge "$start" --merge --auto
	echo "waiting for the PR to merge — CI must pass first…"
	state=""
	for _ in $(seq 1 180); do # up to ~30 min for CI + merge
		# One request per poll, '|'-joined because a check name can contain spaces. Only
		# the listed verdicts count as failed; a null CheckRun conclusion is still running.
		IFS='|' read -r state mergeable failed <<<"$(gh pr view "$start" --json state,mergeable,statusCheckRollup --jq '
			[ .state, .mergeable,
			  ([ .statusCheckRollup[]?
			     | select((.conclusion // .state // "") | test("^(FAILURE|ERROR|TIMED_OUT|CANCELLED|ACTION_REQUIRED|STARTUP_FAILURE)$"))
			     | (.name // .context) ] | join(", "))
			] | join("|")' 2>/dev/null)"
		[ "$state" = "MERGED" ] && break
		[ "$state" = "CLOSED" ] && { echo "PR was closed without merging" >&2; exit 1; }
		# A failed check is THE common reason auto-merge never fires, and it will not go
		# green by itself — so say so now instead of sitting out the full 30 minutes and
		# then reporting a timeout, which reads like slow CI rather than a broken build.
		if [ -n "$failed" ]; then
			echo "PR check(s) failed: $failed — inspect with 'gh pr checks $start'" >&2
			echo "auto-merge stays armed, so pushing a fix (or re-running the job) merges it; then re-run 'make release $bump' to tag." >&2
			exit 1
		fi
		# A definitive CONFLICTING verdict means auto-merge is stuck, so bail with a fix
		# rather than spin the full 30 min. UNKNOWN is GitHub still computing it: wait.
		if [ "$mergeable" = "CONFLICTING" ]; then
			echo "PR conflicts with '$default' — run 'git rebase origin/$default', resolve, then re-run." >&2
			exit 1
		fi
		sleep 10
	done
	if [ "$state" != "MERGED" ]; then
		# Failed checks and conflicts bailed above, so reaching here means nothing ever
		# reached a verdict: checks still running after 30 min, or never reported at all.
		echo "PR hasn't merged after ~30 min (checks still running, or none reported) — check 'gh pr checks $start', then re-run to tag" >&2
		exit 1
	fi
	git fetch origin "$default" >/dev/null 2>&1
	# The branch is stranded at its pre-merge tip once the PR lands: rebase onto the
	# updated default and push, so it ends the cycle current. Best-effort — a hiccup
	# warns rather than failing an already-merged release.
	echo "rebasing '$start' onto the updated '$default'…"
	if git rebase "origin/$default"; then
		git push --force-with-lease origin "$start" >/dev/null 2>&1 ||
			echo "note: rebased '$start' locally but couldn't push it — push it yourself" >&2
	else
		git rebase --abort 2>/dev/null || true
		echo "note: couldn't auto-rebase '$start' onto '$default' — do it manually with 'git rebase origin/$default'" >&2
	fi
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
echo "pushed ${next} — triggering the release workflow (back on '$start')"

# 4. Wait for the tag-triggered workflow, so "done" means built with assets attached.
#    Needs gh, so guard. The run lags the push: poll, then `gh run watch` streams it,
#    matching on the tag name. The tag is already pushed — a failure means inspect it.
if command -v gh >/dev/null; then
	echo "waiting for the release workflow to finish…"
	run_id=""
	for _ in $(seq 1 30); do # ~1 min for GitHub to register the run
		run_id="$(gh run list --workflow release.yml --branch "$next" --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || true)"
		[ -n "$run_id" ] && break
		sleep 2
	done
	if [ -z "$run_id" ]; then
		echo "note: couldn't find the release workflow run — check it with 'gh run list --workflow release.yml'" >&2
	elif gh run watch "$run_id" --exit-status; then
		echo "release ${next} is live — workflow succeeded, assets attached."
	else
		echo "release workflow for ${next} FAILED — inspect it with 'gh run view ${run_id} --log-failed'" >&2
		exit 1
	fi
else
	echo "install gh to have future releases wait for the workflow; check it at the repo's Actions tab."
fi
