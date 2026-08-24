#!/usr/bin/env bash
set -euo pipefail

usage() {
	echo "usage: scripts/create-change-branch.sh <spec-path>" >&2
}

fail() {
	echo "create-change-branch: $*" >&2
	exit 1
}

ensure_clean_worktree() {
	local changes
	changes=$(git status --porcelain=v1 --untracked-files=all -- .)
	[[ -z "$changes" ]] || fail "working tree must be clean"
}

if [[ $# -ne 1 ]]; then
	usage
	exit 2
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(git -C "$script_dir" rev-parse --show-toplevel 2>/dev/null) || \
	fail "scripts directory is not inside a git repository"
cd "$repo_root"

spec_path=$1
[[ -f "$spec_path" ]] || fail "specification file not found: $spec_path"
if [[ "$spec_path" =~ ^agent/specs/([^/]+)\.md$ ]]; then
	spec_slug=${BASH_REMATCH[1]}
else
	fail "specification path must match agent/specs/<spec-slug>.md"
fi

branch="change/$spec_slug"
git check-ref-format --branch "$branch" >/dev/null 2>&1 || \
	fail "invalid change branch derived from specification: $branch"

ensure_clean_worktree
git checkout master 1>&2
git fetch 1>&2
ensure_clean_worktree

if git show-ref --verify --quiet "refs/heads/$branch"; then
	fail "branch already exists: $branch"
else
	status=$?
	[[ $status -eq 1 ]] || fail "cannot inspect branch $branch"
fi

if git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
	fail "branch already exists: origin/$branch"
else
	status=$?
	[[ $status -eq 1 ]] || fail "cannot inspect origin/$branch"
fi

git checkout -b "$branch" 1>&2
ensure_clean_worktree
printf '%s\n' "$branch"
