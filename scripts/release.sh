#!/usr/bin/env bash
# Cut a release from the default branch: bump the latest semver tag, tag HEAD,
# push the tag. The pushed tag triggers the release workflow, which builds and
# attaches the .deb. Refuses unless the tree is clean and you're on the default
# branch in sync with origin — so a tag only ever points at merged, pushed code.
#
# Usage: make release <major|minor|patch>
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

# Releases are cut only from the default branch, in sync with origin — so a tag
# can only ever point at PR'd, merged, and pushed code (not a feature branch or
# an unpushed local commit).
default="$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')"
default="${default:-master}"
current="$(git rev-parse --abbrev-ref HEAD)"
if [ "$current" != "$default" ]; then
	echo "releases come from '$default', not '$current' — merge your branch first" >&2
	exit 1
fi
if git rev-parse --verify --quiet "origin/$default" >/dev/null; then
	if [ "$(git rev-parse HEAD)" != "$(git rev-parse "origin/$default")" ]; then
		echo "local $default differs from origin/$default — push (and merge) before releasing" >&2
		exit 1
	fi
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
