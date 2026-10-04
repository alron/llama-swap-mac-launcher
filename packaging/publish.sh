#!/bin/sh
# publish.sh VERSION ZIP APP_NAME EXE
#
# Puts a release that `make release` built on GitHub, as a GitHub Release
# with the zip attached. Signing and notarizing stay on this Mac: GitHub
# never holds the Developer ID or the notary key (CLAUDE.md, milestone 6).
#
# Before anything leaves this Mac, it checks that:
# - the zip still passes check-zip.sh;
# - the app inside is VERSION, built from a committed tree whose commit is
#   already on origin/main, so the release is source anyone can find;
# - CHANGELOG.md on origin/main has a section for VERSION (the notes);
# - the tag vVERSION, if it exists here or on GitHub, is on that commit,
#   and any SHA-256 in its message is this zip's (not an earlier build's);
# - GitHub has no release vVERSION yet.
#
# Then it creates the tag if there isn't one (annotated, with the zip's
# SHA-256), pushes it, and creates the release. With DRY_RUN set, it stops
# after the checks and shows the notes.
set -eu
version=$1
zip=$2
app=$3
exe=$4
tag=v$version
die() {
	echo "publish: $*" >&2
	exit 1
}

command -v gh >/dev/null || die "needs the GitHub CLI, gh"
[ -f "$zip" ] || die "$zip doesn't exist; run make release first"
"$(dirname "$0")/check-zip.sh" "$zip" "$app"

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
/usr/bin/unzip -q "$zip" -d "$dir"
bundle="$dir/$app.app"
got=$(plutil -extract CFBundleShortVersionString raw "$bundle/Contents/Info.plist")
[ "$got" = "$version" ] || die "the zip holds version $got, not $version"
build=$(go version -m "$bundle/Contents/MacOS/$exe")
rev=$(echo "$build" | sed -n 's/^[[:space:]]*build[[:space:]]*vcs.revision=//p')
[ -n "$rev" ] || die "the app doesn't say which commit it was built from"
echo "$build" | grep -q 'vcs.modified=false' ||
	die "the app was built from a tree with uncommitted changes"
team=$(codesign -dv "$bundle" 2>&1 | sed -n 's/^TeamIdentifier=//p')
sha=$(shasum -a 256 "$zip" | awk '{ print $1 }')

git fetch --quiet origin main
git merge-base --is-ancestor "$rev" origin/main ||
	die "the app was built from $rev, which isn't on origin/main; push it first"

# The tag: here, on GitHub, or neither yet.
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
	on=$(git rev-parse "$tag^{commit}")
	[ "$on" = "$rev" ] || die "$tag is on $on, but the app was built from $rev"
	hashes=$(git for-each-ref --format='%(contents)' "refs/tags/$tag" | grep -E -o '[0-9a-f]{64}' || true)
	if [ -n "$hashes" ] && ! echo "$hashes" | grep -q -x "$sha"; then
		die "$tag's message has a different SHA-256 from this zip's: is the zip a rebuild?"
	fi
	new_tag=
else
	new_tag=1
fi
# ls-remote lists an annotated tag's commit last, as tag^{}.
remote=$(git ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}" | awk '{ c = $1 } END { print c }')
if [ -n "$remote" ] && [ "$remote" != "$rev" ]; then
	die "GitHub's $tag is on $remote, but the app was built from $rev"
fi
if gh release view "$tag" >/dev/null 2>&1; then
	die "GitHub already has a release $tag"
fi

# The notes: CHANGELOG.md's section for this version, from origin/main
# (the version people can read), then how to check the download.
git show origin/main:CHANGELOG.md >"$dir/CHANGELOG.md" 2>/dev/null ||
	die "CHANGELOG.md isn't on origin/main"
awk -v v="$version" '/^## / { if (on) exit; on = ($2 == v); next } on' \
	"$dir/CHANGELOG.md" >"$dir/section.md"
grep -q '[^[:space:]]' "$dir/section.md" ||
	die "CHANGELOG.md on origin/main has no section for $version"
# GitHub shows a release's line breaks as they are, as in a comment, so the
# changelog's wrapped lines are joined: each paragraph or list item becomes
# one line. Code blocks are left alone.
awk '
	function flush() { if (buf != "") print buf; buf = "" }
	/^```/ { flush(); print; fence = !fence; next }
	fence { print; next }
	/^[[:space:]]*$/ { flush(); print; next }
	/^[[:space:]]*([-*+]|[0-9]+\.) / || /^#/ || /^>/ { flush(); buf = $0; next }
	{ if (buf == "") buf = $0; else { sub(/^[[:space:]]+/, ""); buf = buf " " $0 } }
	END { flush() }
' "$dir/section.md" >"$dir/notes.md"
cat >>"$dir/notes.md" <<EOF

### Checking the download

\`$(basename "$zip")\` has the SHA-256 \`$sha\`; compare it with \`shasum -a 256\`. After unzipping, \`spctl -a -vv "$app.app"\` should say \`source=Notarized Developer ID\`, and \`codesign -dv "$app.app"\` should show \`TeamIdentifier=$team\`.
EOF

if [ -n "${DRY_RUN:-}" ]; then
	echo "publish: the checks passed. The release notes would be:"
	echo
	cat "$dir/notes.md"
	echo
	[ -n "$new_tag" ] && echo "publish: it would create the tag $tag on $rev"
	[ -z "$remote" ] && echo "publish: it would push the tag $tag"
	echo "publish: it would create the release $tag with $(basename "$zip")"
	exit 0
fi

if [ -n "$new_tag" ]; then
	git tag -a "$tag" "$rev" -m "$app $version" -m "$(basename "$zip"): SHA-256 $sha"
fi
[ -n "$remote" ] || git push origin "refs/tags/$tag"
gh release create "$tag" "$zip" --verify-tag --title "$app $version" --notes-file "$dir/notes.md"
