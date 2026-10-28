#!/usr/bin/env bash
# Merges upstream Alloy into the current branch.
#
#   sync-upstream.sh                    merge the newest stable upstream release tag
#   sync-upstream.sh <ref>              merge ref: a release tag, upstream/main or a commit
#   sync-upstream.sh --prune-workflows  only prune upstream workflows
#
# After the merge every upstream path is identical to ref and only fork files
# keep their content, upstream workflows are pruned and the overlay is
# verified. The merge is left uncommitted for review.
set -euo pipefail

usage="usage: sync-upstream.sh [<ref> | --prune-workflows]"
manifest=promtail/overlay/manifest.txt
fork_paths=(':(exclude)promtail' ':(exclude).github/README.md' ':(exclude).github/workflows/promtail-*')

die() {
	echo "sync-upstream.sh: $*" >&2
	exit 1
}

# Removes every path below .github/workflows except promtail-* from the index,
# including unmerged entries, and from the working tree.
prune_workflows() {
	echo "pruned upstream workflows: $(git rm -r -f --ignore-unmatch -- .github/workflows ':(exclude).github/workflows/promtail-*' | uniq | wc -l | tr -d ' ')"
}

# Replaces every upstream path of the index and the working tree with its content in ref.
#
# Release branches modify the same lines (versions, changelog, cherry-picks),
# so a 3-way merge between release lines conflicts on files the fork never changes.
take_upstream_tree() {
	git rm -r -q -f --ignore-unmatch -- . "${fork_paths[@]}"
	git checkout "$1" -- . "${fork_paths[@]}"
}

# Prints the newest upstream release tag of the form vX.Y.Z.
latest_release() {
	git tag -l 'v[0-9]*' | grep -E '^v[0-9]+[.][0-9]+[.][0-9]+$' | sort -V | tail -n 1
}

# Prints the merged upstream range and which overlay manifest paths it changes.
# Keep entries only consider the top-level Go files of their directory and list
# the files upstream added there, which the overlay drops.
print_summary() {
	local op path spec commits

	echo
	echo "merged $ref ($(git rev-parse --short MERGE_HEAD)), upstream commits: $(git rev-list --count HEAD..MERGE_HEAD)"
	echo "overlay manifest paths:"
	sed 's/#.*//' "$manifest" | awk 'NF >= 2 { print $1, $2 }' | while read -r op path; do
		spec=$path
		if [ "$op" = keep ]; then
			spec=":(glob)$path/*.go"
		fi
		if git diff --quiet HEAD MERGE_HEAD -- "$spec"; then
			printf '  unchanged  %s\n' "$path"
			continue
		fi
		commits=$(git rev-list --count HEAD..MERGE_HEAD -- "$spec")
		if [ "$commits" -ne 0 ]; then
			printf '  changed    %s (upstream commits: %s)\n' "$path" "$commits"
		else
			printf '  changed    %s\n' "$path"
		fi
		if [ "$op" = keep ]; then
			git diff --name-only --diff-filter=A HEAD MERGE_HEAD -- "$spec" ":(exclude,glob)$path/*_test.go" |
				while read -r added; do
					printf '             dropped by keep: %s\n' "$added"
				done
		fi
	done
}

[ $# -le 1 ] || die "$usage"
cd "$(git rev-parse --show-toplevel)"

if [ "${1:-}" = --prune-workflows ]; then
	prune_workflows
	exit 0
fi
ref=${1:-}

git remote get-url upstream >/dev/null 2>&1 || die "remote upstream is missing, add it with:
  git remote add upstream https://github.com/grafana/alloy.git && git remote set-url --push upstream DISABLED"
! git rev-parse -q --verify MERGE_HEAD >/dev/null || die "a merge is already in progress"
[ -z "$(git status --porcelain --untracked-files=no)" ] || die "the working tree has uncommitted changes"

echo "fetching upstream"
git fetch --quiet upstream --tags
if [ -z "$ref" ]; then
	ref=$(latest_release)
	[ -n "$ref" ] || die "no upstream release tags found"
fi
upstream=$(git rev-parse -q --verify "$ref^{commit}") || die "unknown ref $ref"
if git merge-base --is-ancestor "$upstream" HEAD; then
	echo "already up to date with $ref"
	exit 0
fi

echo "merging $ref"
if ! out=$(git merge --no-ff --no-commit "$ref" 2>&1); then
	git rev-parse -q --verify MERGE_HEAD >/dev/null || die "git merge $ref failed: $out"
fi
take_upstream_tree "$ref"
prune_workflows
git diff --cached --quiet HEAD -- .github/workflows || die "the merge changes fork workflows: $(git diff --cached --name-only HEAD -- .github/workflows | tr '\n' ' ')"

if [ -n "$(git diff --name-only --diff-filter=U)" ]; then
	echo
	echo "unmerged paths:"
	git diff --name-only --diff-filter=U | sed 's/^/  /'
	cat <<EOF

Upstream paths are taken from $ref; resolve the conflicts of these fork files.
Then run:
  make -C promtail check
  git commit
EOF
	exit 1
fi

status=0
make -C promtail --no-print-directory overlay || status=$?
print_summary

echo
if [ $status -ne 0 ]; then
	cat <<EOF
The fork no longer builds against the merged upstream sources:
  patch:       make -C promtail patch-start FILE=<file>, fix it, make -C promtail patch-save FILE=<file>
  replace:     port the upstream change to promtail/overlay/_files/<file> and update its blob= pin
Then run:
  make -C promtail check
  git commit
EOF
	exit 1
fi
cat <<EOF
The overlay applies. The merge is not committed, next steps:
  make -C promtail check
  git commit
EOF
