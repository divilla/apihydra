#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_root=$(mktemp -d)
repo="$test_root/repo with spaces"
remote="$test_root/origin.git"
mkdir -p "$repo/scripts" "$repo/agent/specs" "$repo/other"
trap 'rm -rf -- "$test_root"' EXIT

cp "$script_dir/create-change-branch.sh" "$repo/scripts/create-change-branch.sh"
printf '%s\n' '# Domain types' >"$repo/agent/specs/000-domain-types.md"
printf '%s\n' '# Remote branch' >"$repo/agent/specs/001-remote-branch.md"
printf '%s\n' '# Invalid path' >"$repo/other/plain.md"

git init --bare -q "$remote"
git -C "$repo" init -q -b master
git -C "$repo" config user.name 'Create Change Branch Test'
git -C "$repo" config user.email 'create-change-branch@example.invalid'
git -C "$repo" remote add origin "$remote"
git -C "$repo" add .
git -C "$repo" commit -q -m initial
git -C "$repo" push -q -u origin master
git -C "$remote" symbolic-ref HEAD refs/heads/master
git -C "$repo" remote set-head origin --auto >/dev/null
git -C "$repo" checkout -q -b source-branch
printf '%s\n' 'source branch only' >"$repo/source-branch-only.txt"
git -C "$repo" add source-branch-only.txt
git -C "$repo" commit -q -m 'source branch commit'

usage_error="$test_root/usage-error"
set +e
"$repo/scripts/create-change-branch.sh" > /dev/null 2>"$usage_error"
usage_status=$?
set -e
[[ $usage_status -eq 2 ]]
grep -Fxq 'usage: scripts/create-change-branch.sh <spec-path>' "$usage_error"

printf '%s\n' uncommitted >"$repo/uncommitted.txt"
dirty_error="$test_root/dirty-error"
set +e
"$repo/scripts/create-change-branch.sh" agent/specs/000-domain-types.md > /dev/null 2>"$dirty_error"
dirty_status=$?
set -e
[[ $dirty_status -eq 1 ]]
[[ $(git -C "$repo" branch --show-current) == source-branch ]]
[[ -z $(git -C "$repo" branch --list change/000-domain-types) ]]
grep -Fxq 'create-change-branch: working tree must be clean' "$dirty_error"
rm "$repo/uncommitted.txt"

branch=$(cd "$test_root" && "$repo/scripts/create-change-branch.sh" agent/specs/000-domain-types.md)
[[ "$branch" == change/000-domain-types ]]
[[ $(git -C "$repo" branch --show-current) == change/000-domain-types ]]
[[ $(git -C "$repo" rev-parse HEAD) == $(git -C "$repo" rev-parse master) ]]
[[ $(git -C "$repo" rev-parse HEAD) != $(git -C "$repo" rev-parse source-branch) ]]
! git -C "$repo" cat-file -e HEAD:source-branch-only.txt 2>/dev/null

local_error="$test_root/local-error"
set +e
"$repo/scripts/create-change-branch.sh" agent/specs/000-domain-types.md > /dev/null 2>"$local_error"
local_status=$?
set -e
[[ $local_status -eq 1 ]]
[[ $(git -C "$repo" branch --show-current) == master ]]
grep -Fxq 'create-change-branch: branch already exists: change/000-domain-types' "$local_error"

git -C "$repo" checkout -q -b change/001-remote-branch
git -C "$repo" push -q -u origin change/001-remote-branch
git -C "$repo" checkout -q master
git -C "$repo" branch -D change/001-remote-branch >/dev/null
git -C "$repo" branch -dr origin/change/001-remote-branch >/dev/null
remote_error="$test_root/remote-error"
set +e
"$repo/scripts/create-change-branch.sh" agent/specs/001-remote-branch.md > /dev/null 2>"$remote_error"
remote_status=$?
set -e
[[ $remote_status -eq 1 ]]
[[ -z $(git -C "$repo" branch --list change/001-remote-branch) ]]
grep -Fxq 'create-change-branch: branch already exists: origin/change/001-remote-branch' "$remote_error"

invalid_error="$test_root/invalid-error"
set +e
"$repo/scripts/create-change-branch.sh" other/plain.md > /dev/null 2>"$invalid_error"
invalid_status=$?
set -e
[[ $invalid_status -eq 1 ]]
grep -Fxq 'create-change-branch: specification path must match agent/specs/<spec-slug>.md' "$invalid_error"

printf '%s\n' 'create-change-branch tests passed'
