#!/usr/bin/env bash
# Cut a release: fetch tags, bump the latest semver tag, tag HEAD, push the tag.
# The pushed tag triggers the release workflow, which builds and attaches the .deb.
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

git fetch --tags --force >/dev/null 2>&1 || true

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
