#!/usr/bin/env bash
set -euo pipefail
export COLUMNS=80

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_root=$(mktemp -d)
repo="$test_root/repo"
remote="$test_root/origin.git"
fake_bin="$test_root/bin"
implementation_count="$test_root/implementation-count"
review_count="$test_root/review-count"
review_specification="$test_root/review-specification"
mkdir -p "$repo/scripts" "$repo/agent/specs" "$fake_bin"

cleanup() {
	rm -rf -- "$test_root"
}
trap cleanup EXIT

cp "$script_dir/codex-implement.pl" "$repo/scripts/codex-implement.pl"
printf '%s\n' '# Domain types' >"$repo/agent/specs/000-domain-types.md"
printf '%s\n' '# Failing change' >"$repo/agent/specs/001-failing-change.md"
printf '%s\n' '# Invalid name' >"$repo/agent/specs/plain.md"

cat >"$repo/scripts/codex-review-loop.pl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ $# -eq 1 ]]
[[ "$1" == 'agent/specs/000-domain-types.md' ]]
[[ $(git branch --show-current) == 'change/000-domain-types' ]]
[[ -z $(git status --short) ]]
count=0
[[ ! -f "$CODEX_TEST_REVIEW_COUNT" ]] || count=$(<"$CODEX_TEST_REVIEW_COUNT")
((count += 1))
printf '%s\n' "$count" >"$CODEX_TEST_REVIEW_COUNT"
printf '%s\n' "$1" >"$CODEX_TEST_REVIEW_SPECIFICATION"
EOF
chmod +x "$repo/scripts/codex-review-loop.pl"

cat >"$fake_bin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

[[ ${1-} == exec ]]
[[ ${2-} == --json ]]
[[ ${3-} == -o ]]
result=${4-}
prompt=${5-}
[[ $# -eq 5 ]]

printf '%s\n' '{"type":"started"}'
if [[ ${CODEX_TEST_FAIL-} == 1 ]]; then
	printf '%s\n' 'simulated implementation failure' >&2
	exit 42
fi

[[ "$prompt" == "\$change-code $CODEX_TEST_EXPECTED_SPECIFICATION" ]]
count=0
[[ ! -f "$CODEX_TEST_IMPLEMENTATION_COUNT" ]] || count=$(<"$CODEX_TEST_IMPLEMENTATION_COUNT")
((count += 1))
printf '%s\n' "$count" >"$CODEX_TEST_IMPLEMENTATION_COUNT"
sleep 1.1
printf '%s\n' '{"type":"completed"}'
printf '%s\n' 'Implementation complete.' >"$result"
printf '%s\n' "$count" >"implemented-$count.txt"
EOF
chmod +x "$fake_bin/codex"

git init --bare -q "$remote"
git -C "$repo" init -q -b master
git -C "$repo" config user.name 'Implement Test'
git -C "$repo" config user.email 'implement@example.invalid'
git -C "$repo" remote add origin "$remote"
git -C "$repo" add .
git -C "$repo" commit -q -m initial
git -C "$repo" push -q -u origin master
git -C "$remote" symbolic-ref HEAD refs/heads/master
git -C "$repo" remote set-head origin --auto >/dev/null

run_implementation() {
	local output=$1
	(
		cd "$repo"
		PATH="$fake_bin:$PATH" \
			CODEX_TEST_EXPECTED_SPECIFICATION='agent/specs/000-domain-types.md' \
			CODEX_TEST_IMPLEMENTATION_COUNT="$implementation_count" \
			CODEX_TEST_REVIEW_COUNT="$review_count" \
			CODEX_TEST_REVIEW_SPECIFICATION="$review_specification" \
			./scripts/codex-implement.pl agent/specs/000-domain-types.md
	) >"$output"
}

first_output="$test_root/first-output"
run_implementation "$first_output"

[[ $(git -C "$repo" branch --show-current) == 'change/000-domain-types' ]]
[[ $(git -C "$repo" log -1 --format=%s) == 'Implement change 000-domain-types' ]]
[[ $(git -C "$repo" rev-parse change/000-domain-types) == \
	$(git -C "$repo" rev-parse origin/change/000-domain-types) ]]
[[ $(<"$implementation_count") == 1 ]]
[[ $(<"$review_count") == 1 ]]
[[ $(<"$review_specification") == 'agent/specs/000-domain-types.md' ]]
grep -Fxq 'Branch: change/000-domain-types' "$first_output"
grep -Eq '^codex exec --json -o /.+/implementation-result.md ' "$first_output"
grep -Fq "'\$change-code agent/specs/000-domain-types.md'" "$first_output"
grep -Eq '^Implement 00:0[1-9] \.\. ✅$' "$first_output"
grep -Fxq 'Commit: Implement change 000-domain-types' "$first_output"
grep -Fxq "Review: $repo/scripts/codex-review-loop.pl agent/specs/000-domain-types.md" "$first_output"
[[ -z $(git -C "$repo" status --short) ]]

git -C "$repo" checkout -q master
git -C "$repo" branch -D change/000-domain-types >/dev/null
second_output="$test_root/second-output"
run_implementation "$second_output"

[[ $(git -C "$repo" branch --show-current) == 'change/000-domain-types' ]]
[[ $(git -C "$repo" branch --list change/000-domain-types | wc -l) -eq 1 ]]
[[ $(git -C "$repo" rev-parse '@{upstream}') == \
	$(git -C "$repo" rev-parse origin/change/000-domain-types) ]]
[[ $(<"$implementation_count") == 2 ]]
[[ $(<"$review_count") == 2 ]]
git -C "$repo" show HEAD:implemented-1.txt | grep -Fxq '1'
git -C "$repo" show HEAD:implemented-2.txt | grep -Fxq '2'

git -C "$repo" checkout -q master
third_output="$test_root/third-output"
run_implementation "$third_output"

[[ $(git -C "$repo" branch --show-current) == 'change/000-domain-types' ]]
[[ $(git -C "$repo" branch --list change/000-domain-types | wc -l) -eq 1 ]]
[[ $(<"$implementation_count") == 3 ]]
[[ $(<"$review_count") == 3 ]]
git -C "$repo" show HEAD:implemented-3.txt | grep -Fxq '3'

git -C "$repo" checkout -q master
failed_output="$test_root/failed-output"
failed_error="$test_root/failed-error"
set +e
(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		CODEX_TEST_FAIL=1 \
		./scripts/codex-implement.pl agent/specs/001-failing-change.md
) >"$failed_output" 2>"$failed_error"
failed_status=$?
set -e
[[ $failed_status -eq 42 ]]
[[ $(git -C "$repo" branch --show-current) == 'change/001-failing-change' ]]
[[ $(git -C "$repo" rev-parse HEAD) == $(git -C "$repo" rev-parse master) ]]
[[ $(<"$review_count") == 3 ]]
grep -Eq '^Implement 00:00 \. ❌$' "$failed_output"
grep -Fxq 'codex-implement: Codex command failed with exit code 42' "$failed_error"
grep -Fq 'simulated implementation failure' "$failed_error"

git -C "$repo" checkout -q master
invalid_error="$test_root/invalid-error"
set +e
(
	cd "$repo"
	PATH="$fake_bin:$PATH" ./scripts/codex-implement.pl agent/specs/plain.md
) >/dev/null 2>"$invalid_error"
invalid_status=$?
set -e
[[ $invalid_status -eq 1 ]]
grep -Fxq 'codex-implement: specification path must end with /<number>-<name>.md' "$invalid_error"

printf '%s\n' 'codex-implement tests passed'
