#!/bin/sh
# tap.sh VERSION ZIP TAP
#
# Points the Homebrew cask in our tap at a release publish.sh has put on
# GitHub (milestone 7): its version and its zip's SHA-256, committed and
# pushed from TAP, the tap's clone on this Mac. Releases are published from
# here, so no token sits on GitHub for this.
#
# Before it commits, it checks that:
# - TAP's origin is alron/homebrew-llama-swap-mac-launcher, and its commits
#   use the noreply address (the global one would leak the real address);
# - the tap is on main, clean, and not behind GitHub;
# - the release's zip on GitHub is ZIP (the cask's URL names the tag);
# - the change is the version and SHA-256 lines alone, and passes brew style.
#
# With DRY_RUN set, it shows the change and the commits it would push, and
# puts the cask back. Any commits already waiting in the tap are pushed
# with it (0.3.3's change to how upgrades quit the app was one).
set -eu
version=$1
zip=$2
tap=$3
die() {
	echo "tap: $*" >&2
	exit 1
}

repo=alron/llama-swap-mac-launcher
noreply=566136+alron@users.noreply.github.com
zip=$(cd "$(dirname "$zip")" && pwd)/$(basename "$zip")
[ -f "$zip" ] || die "$zip doesn't exist"
cask=Casks/llama-swap-launcher.rb # within the tap, where git runs
[ -f "$tap/$cask" ] || die "no cask at $tap/$cask: set TAP to the tap's clone"
cd "$tap"

git remote get-url origin | grep -q 'alron/homebrew-llama-swap-mac-launcher' ||
	die "the tap's origin isn't alron/homebrew-llama-swap-mac-launcher"
[ "$(git config user.email)" = "$noreply" ] ||
	die "the tap's commits would use $(git config user.email), not $noreply: git config user.email $noreply"
[ "$(git branch --show-current)" = main ] || die "the tap isn't on main"
[ -z "$(git status --porcelain)" ] || die "the tap has uncommitted changes"
git fetch --quiet origin
git merge-base --is-ancestor origin/main HEAD || die "the tap's main is behind GitHub's; pull it first"

sha=$(shasum -a 256 "$zip" | awk '{ print $1 }')
asset=$(basename "$zip")
if published=$(gh release view "v$version" --repo "$repo" --json assets \
	--jq ".assets[] | select(.name == \"$asset\") | .digest" 2>/dev/null) && [ -n "$published" ]; then
	[ "$published" = "sha256:$sha" ] || die "GitHub's $asset is $published, not this zip's sha256:$sha"
elif [ -z "${DRY_RUN:-}" ]; then
	die "GitHub has no release v$version with $asset; publish it first"
else
	echo "tap: (v$version isn't on GitHub yet; it would check its zip is this one)"
fi

sed -i '' -E \
	-e "s/^  version \"[^\"]*\"\$/  version \"$version\"/" \
	-e "s/^  sha256 \"[0-9a-f]*\"\$/  sha256 \"$sha\"/" "$cask"
grep -q -x "  version \"$version\"" "$cask" && grep -q -x "  sha256 \"$sha\"" "$cask" ||
	{ git checkout -- "$cask"; die "the cask's version or sha256 line isn't where it's expected"; }
changed=$(git diff --numstat -- "$cask" | awk '{ print $1 + 0 }')
if [ -z "$(git diff --name-only)" ]; then
	echo "tap: the cask already has $version and this zip"
elif [ "$changed" != 2 ] || [ "$(git diff --name-only)" != "$cask" ]; then
	git checkout -- "$cask"
	die "the change isn't the version and sha256 lines alone"
fi
if command -v brew >/dev/null; then
	brew style "$cask" >/dev/null || { git checkout -- "$cask"; die "brew style fails on the cask"; }
fi

if [ -n "${DRY_RUN:-}" ]; then
	echo "tap: the checks passed. The cask would change:"
	git --no-pager diff -- "$cask"
	git checkout -- "$cask"
	pending=$(git log --oneline origin/main..HEAD)
	[ -z "$pending" ] || printf 'tap: and these commits, waiting in the tap, would be pushed too:\n%s\n' "$pending"
	exit 0
fi
if [ -n "$(git diff --name-only)" ]; then
	git commit -q -m "llama-swap-launcher $version" -- "$cask"
fi
git push --quiet origin main
echo "tap: the cask is at $version; brew upgrade picks it up"
