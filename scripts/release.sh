#!/usr/bin/env bash
# Cut a release. From a feature branch it HELPS get there: pushes the branch,
# opens a PR if needed (gh pr create), merges it, then continues from the default
# branch — so a tag only ever points at merged, pushed code. Then it bumps the
# latest semver tag, tags HEAD, and pushes the tag, which triggers the release
# workflow (build + attach the .deb). Refuses on a dirty tree.
#
# Usage: make release <major|minor|patch>  (needs gh when on a feature branch)
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

# Refuse to release a dirty tree — the tag must point at a clean, pushed commit.
if [ -n "$(git status --porcelain)" ]; then
	echo "working tree is dirty — commit or stash before releasing" >&2
	exit 1
fi

git fetch --tags --force origin >/dev/null 2>&1 || true

# A release is cut from the default branch — so a tag only ever points at merged,
# pushed code. If you're on a feature branch, don't just refuse: HELP get the work
# in — push it, open a PR if there isn't one, merge it, then continue from default.
default="$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')"
default="${default:-master}"
current="$(git rev-parse --abbrev-ref HEAD)"

if [ "$current" != "$default" ]; then
	if ! command -v gh >/dev/null; then
		echo "on '$current' — releasing needs it merged to '$default'. Install gh (https://cli.github.com) or merge manually, then re-run." >&2
		exit 1
	fi
	echo "on '$current' — pushing and merging into '$default' before releasing…"
	git push -u origin "$current"
	if [ -z "$(gh pr list --head "$current" --state open --json number --jq '.[0].number' 2>/dev/null)" ]; then
		echo "opening a pull request…"
		gh pr create --base "$default" --head "$current" --fill
	fi
	echo "merging the pull request…"
	gh pr merge "$current" --merge --delete-branch
	git checkout "$default"
fi

# On the default branch: fast-forward to origin so the tag points at the merged,
# pushed tip; refuse if it still has unpushed local commits.
git pull --ff-only origin "$default" || {
	echo "couldn't fast-forward '$default' from origin — reconcile it manually, then re-run" >&2
	exit 1
}
if [ "$(git rev-parse HEAD)" != "$(git rev-parse "origin/$default" 2>/dev/null || git rev-parse HEAD)" ]; then
	echo "'$default' has commits not on origin — push them first" >&2
	exit 1
fi

latest="$(git tag --list 'v*' --sort=-v:refname | head -n1)"
latest="${latest:-v0.0.0}"
IFS=. read -r maj min pat <<<"${latest#v}"
case "$bump" in
	major) maj=$((maj + 1)); min=0; pat=0 ;;
	minor) min=$((min + 1)); pat=0 ;;
	patch) pat=$((pat + 1)) ;;
esac
next="v${maj}.${min}.${pat}"

echo "releasing ${latest} -> ${next}"
git tag -a "$next" -m "release $next"
git push origin "$next"
echo "pushed ${next} — the release workflow will build and attach the .deb"
