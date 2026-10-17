#!/usr/bin/env bash
# Merges upstream Alloy into the current branch.
#
#   sync-upstream.sh [ref]              merge ref (default upstream/main) without committing,
#                                       prune upstream workflows and verify the overlay
#   sync-upstream.sh --prune-workflows  only prune upstream workflows
#
# The merge is left uncommitted for review. The fork runs only its own
# .github/workflows/promtail-* workflows.
set -euo pipefail

usage="usage: sync-upstream.sh [ref] | --prune-workflows"
manifest=promtail/overlay/manifest.txt

die() {
	echo "sync-upstream.sh: $*" >&2
	exit 1
}

# Removes every path below .github/workflows except promtail-* from the index,
# including unmerged entries, and from the working tree.
prune_workflows() {
	echo "pruning upstream workflows"
	git rm -r -f --ignore-unmatch -- .github/workflows ':(exclude).github/workflows/promtail-*' | uniq
}

# Prints the merged upstream range and which overlay manifest paths it changes.
# Keep entries only consider the top-level Go files of their directory and list
# the files upstream added there, which the overlay drops.
print_summary() {
	local base op path spec commits

	base=$(git merge-base HEAD MERGE_HEAD)
	echo
	echo "merged $ref $(git rev-parse --short "$base")..$(git rev-parse --short MERGE_HEAD), upstream commits: $(git rev-list --count HEAD..MERGE_HEAD)"
	echo "overlay manifest paths:"
	sed 's/#.*//' "$manifest" | awk 'NF >= 2 { print $1, $2 }' | while read -r op path; do
		spec=$path
		if [ "$op" = keep ]; then
			spec=":(glob)$path/*.go"
		fi
		if git diff --quiet "$base" MERGE_HEAD -- "$spec"; then
			printf '  unchanged  %s\n' "$path"
			continue
		fi
		commits=$(git rev-list --count HEAD..MERGE_HEAD -- "$spec")
		printf '  changed    %s (upstream commits: %s)\n' "$path" "$commits"
		if [ "$op" = keep ]; then
			git diff --name-only --diff-filter=A "$base" MERGE_HEAD -- "$spec" ":(exclude,glob)$path/*_test.go" |
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
ref=${1:-upstream/main}

git remote get-url upstream >/dev/null 2>&1 || die "remote upstream is missing, add it with:
  git remote add upstream https://github.com/grafana/alloy.git && git remote set-url --push upstream DISABLED"
! git rev-parse -q --verify MERGE_HEAD >/dev/null || die "a merge is already in progress"
[ -z "$(git status --porcelain --untracked-files=no)" ] || die "the working tree has uncommitted changes"

echo "fetching upstream"
git fetch --quiet upstream --tags
upstream=$(git rev-parse -q --verify "$ref^{commit}") || die "unknown ref $ref"
if git merge-base --is-ancestor "$upstream" HEAD; then
	echo "already up to date with $ref"
	exit 0
fi

if ! git merge --no-ff --no-commit "$ref"; then
	git rev-parse -q --verify MERGE_HEAD >/dev/null || die "git merge $ref failed"
fi
prune_workflows
git diff --cached --quiet HEAD -- .github/workflows || die "the merge changes fork workflows: $(git diff --cached --name-only HEAD -- .github/workflows | tr '\n' ' ')"

if [ -n "$(git diff --name-only --diff-filter=U)" ]; then
	echo
	echo "unmerged paths:"
	git diff --name-only --diff-filter=U | sed 's/^/  /'
	cat <<EOF

Fork-owned files live only in promtail/ and .github/: resolve their conflicts.
Any other path must stay identical to upstream: git checkout --theirs -- <path>
Then run:
  make -C promtail check
  git commit
EOF
	exit 1
fi

status=0
make -C promtail --no-print-directory overlay go-version || status=$?
print_summary

echo
if [ $status -ne 0 ]; then
	cat <<EOF
The fork no longer builds against the merged upstream sources:
  patch:       make -C promtail patch-start FILE=<file>, fix it, make -C promtail patch-save FILE=<file>
  replace:     port the upstream change to promtail/overlay/_files/<file> and update its blob= pin
  go-version:  update the golang image tag and digest in promtail/Dockerfile
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
