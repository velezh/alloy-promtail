#!/usr/bin/env bash
# Edits an overlay patch of an upstream file.
#
#   patch.sh start <file>   apply promtail/overlay/patches/<file>.patch to the working tree,
#                           falling back to a 3-way merge with conflict markers
#   patch.sh save <file>    store the working tree changes of <file> as its patch and
#                           restore the upstream content
#
# The index keeps the upstream content of <file> the whole time, so the script
# also works in the middle of an upstream merge.
set -euo pipefail

die() {
	echo "patch.sh: $*" >&2
	exit 1
}

[ $# -eq 2 ] || die "usage: patch.sh start|save <upstream file>"
cmd=$1
file=$2

cd "$(git rev-parse --show-toplevel)"
patch="promtail/overlay/patches/$file.patch"

entry=$(git ls-files --stage -- "$file")
[ -n "$entry" ] || die "$file is not tracked"
[ "$(echo "$entry" | wc -l)" -eq 1 ] && [ "$(echo "$entry" | awk '{ print $3 }')" = 0 ] || die "$file has unresolved merge conflicts"
mode=$(echo "$entry" | awk '{ print $1 }')
blob=$(echo "$entry" | awk '{ print $2 }')

case $cmd in
start)
	git diff --quiet -- "$file" || die "$file has unstaged changes"
	if [ ! -f "$patch" ]; then
		echo "edit $file, then run: make -C promtail patch-save FILE=$file"
		exit 0
	fi
	status=0
	git apply --3way "$patch" || status=$?
	git update-index --cacheinfo "$mode,$blob,$file"
	if [ $status -ne 0 ]; then
		echo "resolve the conflict markers in $file, then run: make -C promtail patch-save FILE=$file"
	else
		echo "edit $file, then run: make -C promtail patch-save FILE=$file"
	fi
	;;
save)
	! git diff --quiet -- "$file" || die "$file has no changes"
	! grep -qE '^(<<<<<<<|>>>>>>>)' "$file" || die "$file still contains conflict markers"
	mkdir -p "$(dirname "$patch")"
	git diff --no-color --no-ext-diff --src-prefix=a/ --dst-prefix=b/ -U"${CONTEXT:-3}" -- "$file" >"$patch"
	git checkout -- "$file"
	echo "saved $patch"
	;;
*)
	die "unknown command $cmd"
	;;
esac
