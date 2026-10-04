#!/bin/sh
# check-build.sh APP MIN_MACOS
#
# Checks the build settings that only show once the app runs somewhere
# else. Both binaries target MIN_MACOS or older (without the Makefile's
# flags, cgo targets the build machine's macOS, and the app won't launch on
# anything older), each has a Mach-O UUID (Local Network privacy misbehaves
# without one), and neither contains a path from the build machine
# (-trimpath). The bundle macro in the Makefile runs it after every build.
set -eu
app=$1
min=$2
root=$(cd "$(dirname "$0")/.." && pwd)
fail=0
for bin in "$app"/Contents/MacOS/* "$app/Contents/Helpers/llsl"; do
	name=$(basename "$bin")
	minos=$(otool -l "$bin" | awk '$1 == "minos" { print $2; exit }')
	# Compares major.minor: the binary's must not be newer than MIN_MACOS.
	if ! awk -v a="$minos" -v b="$min" 'BEGIN {
		split(a, x, "."); split(b, y, ".")
		exit !(x[1] < y[1] || (x[1] == y[1] && x[2] + 0 <= y[2] + 0)) }'; then
		echo "check-build: $name targets macOS ${minos:-?}, newer than $min" >&2
		fail=1
	fi
	if ! otool -l "$bin" | grep -q LC_UUID; then
		echo "check-build: $name has no LC_UUID" >&2
		fail=1
	fi
	paths=$(strings -a "$bin" | grep -c -F -e "$HOME/" -e "$root" || true)
	if [ "$paths" != 0 ]; then
		echo "check-build: $name contains $paths path(s) from this machine" >&2
		fail=1
	fi
done
[ "$fail" = 0 ] && echo "check-build: minimum macOS, UUIDs and paths are as they should be"
exit "$fail"
