#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

repo="$test_root/repo"
remote="$test_root/origin.git"
fake_bin="$test_root/bin"
mkdir -p "$repo/scripts" "$fake_bin"
cp "$script_dir/codex-review-loop.sh" "$repo/scripts/codex-review-loop.sh"

cat >"$repo/scripts/commit-agent.pl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$repo/scripts/commit-agent.pl"

git init --bare -q "$remote"
git -C "$repo" init -q -b master
git -C "$repo" config user.name 'Review Loop Test'
git -C "$repo" config user.email 'review-loop@example.invalid'
git -C "$repo" remote add origin "$remote"
printf '%s\n' 'initial' >"$repo/initial.txt"
git -C "$repo" add initial.txt scripts/codex-review-loop.sh scripts/commit-agent.pl
git -C "$repo" commit -q -m initial
git -C "$repo" push -q -u origin master
git -C "$repo" symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/master
git -C "$repo" branch develop
printf '%s\n' 'stale review output' >"$repo/comments.md"

cat >"$fake_bin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ ${1-} == exec && ${2-} == review && ${3-} == --json ]]; then
	shift 3
	base=
	output=
	parse_options=true
	while [[ $# -gt 0 ]]; do
		if [[ "$parse_options" == false ]]; then
			shift
			continue
		fi
		case "$1" in
		--)
			parse_options=false
			shift
			;;
		--base)
			base=${2-}
			shift 2
			;;
		-o)
			output=${2-}
			shift 2
			;;
		*)
			shift
			;;
		esac
	done
	[[ "$base" == "$CODEX_TEST_EXPECTED_BASE" ]]
	[[ -n "$output" ]]
	if [[ ${CODEX_TEST_HANG-} == 1 ]]; then
		trap 'exit 0' INT TERM
		printf '%s\n' 'waiting'
		sh -c '
trap "printf \"%s\\n\" \"\$\$\" >\"\$CODEX_TEST_CHILD_TERMINATED\"; exit 0" INT TERM
printf "%s\n" "$$" >"$CODEX_TEST_CHILD_PID"
while true; do sleep 1; done
' &
		wait "$!"
	fi
	count=0
	[[ ! -f "$CODEX_TEST_REVIEW_COUNT" ]] || count=$(<"$CODEX_TEST_REVIEW_COUNT")
	((count += 1))
	printf '%s\n' "$count" >"$CODEX_TEST_REVIEW_COUNT"
	printf '%s' 'r'
	if [[ $count -eq 1 ]]; then
		sleep 2
	fi
	printf '%s\n' 'eview output one'
	printf '%s\n' 'review output two'
	if [[ $count -eq 1 ]]; then
		printf '%s\n%s' 'No findings.' 'Extra text means this is not the exact message.' >"$output"
	else
		printf '%s\n' 'No findings.' >"$output"
	fi
	exit 0
fi

if [[ ${1-} == exec && ${2-} == --json && ${3-} == 'fix all comments' && $# -eq 3 ]]; then
	printf '%s\n' 'fix output'
	printf '%s\n' 'fixed' >reviewed.txt
	cat >scripts/commit-agent.pl <<'HELPER'
#!/usr/bin/env bash
printf '%s\n' 'helper was executed' >"$CODEX_TEST_HELPER_EXECUTED"
exit 99
HELPER
	chmod +x scripts/commit-agent.pl
	printf '%s\n' '1' >"$CODEX_TEST_FIX_COUNT"
	exit 0
fi

printf 'unexpected codex arguments: %q' "$1" >&2
printf ' %q' "${@:2}" >&2
printf '\n' >&2
exit 1
EOF
chmod +x "$fake_bin/codex"

review_count="$test_root/review-count"
fix_count="$test_root/fix-count"
helper_executed="$test_root/helper-executed"
output="$test_root/output"
pinned_base=$(git -C "$repo" rev-parse origin/master)
(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		CODEX_TEST_REVIEW_COUNT="$review_count" \
		CODEX_TEST_FIX_COUNT="$fix_count" \
		CODEX_TEST_HELPER_EXECUTED="$helper_executed" \
		CODEX_TEST_EXPECTED_BASE="$pinned_base" \
		./scripts/codex-review-loop.sh
) >"$output"

grep -Fxq "Repository: $repo" "$output"
grep -Fxq 'Branch: master' "$output"
grep -Fxq 'Base: origin/master' "$output"
grep -Fxq "Pinned base: $pinned_base" "$output"
grep -Fxq "Review options: --base $pinned_base" "$output"
grep -Fxq "Comments: $repo/comments.md" "$output"
grep -Fxq '=== Review pass 01 ===' "$output"
grep -Fxq '=== Review pass 02 ===' "$output"
awk '
/^-+$/ {
	separator = $0
	if ((getline command) <= 0 || command !~ /^codex /) exit 1
	if ((getline closing) <= 0 || closing !~ /^-+$/) exit 1
	if (length(separator) != length(command) || length(closing) != length(command)) exit 1
	commands++
}
END { if (commands != 3) exit 1 }
' "$output"
grep -Eq "^codex exec review --json --base $pinned_base -o /tmp/.*/comments\\.md$" "$output"
grep -Fxq 'codex exec --json fix\ all\ comments' "$output"
grep -Eq '^Review [0-9]+:[0-9]{2} \.+ ✅$' "$output"
grep -Eq '^Review 00:0[2-9] \.\. ✅$' "$output"
grep -Eq '^Fix [0-9]+:[0-9]{2} \. ✅$' "$output"
! grep -Fq 'review output one' "$output"
! grep -Fq 'fix output' "$output"
grep -Fxq 'Extra text means this is not the exact message.' "$output"
[[ $(<"$output") == *$'Extra text means this is not the exact message.\n\n-'* ]]
grep -Fxq 'No findings.' "$output"
grep -Fxq '?? reviewed.txt' "$output"
grep -Fxq 'Commit: review fixes 01' "$output"
grep -Fxq 'Review complete: exact message "No findings." found.' "$output"

[[ $(<"$review_count") == 2 ]]
[[ $(<"$fix_count") == 1 ]]
[[ $(git -C "$repo" log -1 --pretty=%s) == 'review fixes 01' ]]
[[ ! -e "$helper_executed" ]]
git -C "$repo" show HEAD:scripts/commit-agent.pl | grep -Fq 'helper was executed'
[[ ! -e "$repo/comments.md" ]]
[[ -z $(git -C "$repo" status --short) ]]
git -C "$repo" add -A
! git -C "$repo" ls-files --error-unmatch -- comments.md >/dev/null 2>&1
[[ $(git -C "$repo" rev-parse origin/master) != "$pinned_base" ]]

explicit_base_output="$test_root/explicit-base-output"
develop_base=$(git -C "$repo" rev-parse develop)
(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		CODEX_TEST_REVIEW_COUNT="$review_count" \
		CODEX_TEST_EXPECTED_BASE="$develop_base" \
		./scripts/codex-review-loop.sh --base develop
) >"$explicit_base_output"

grep -Fxq 'Base: develop' "$explicit_base_output"
grep -Fxq "Pinned base: $develop_base" "$explicit_base_output"
grep -Fxq "Review options: --base $develop_base" "$explicit_base_output"
grep -Eq "^codex exec review --json --base $develop_base -o /tmp/.*/comments\\.md$" "$explicit_base_output"
[[ $(<"$review_count") == 3 ]]
[[ ! -e "$repo/comments.md" ]]

terminator_output="$test_root/terminator-output"
(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		CODEX_TEST_REVIEW_COUNT="$review_count" \
		CODEX_TEST_EXPECTED_BASE="$develop_base" \
		./scripts/codex-review-loop.sh --base develop -- -review-prompt
) >"$terminator_output"

grep -Fxq "Review options: --base $develop_base -- -review-prompt" "$terminator_output"
grep -Eq "^codex exec review --json --base $develop_base -o /tmp/.*/comments\\.md -- -review-prompt$" "$terminator_output"
[[ $(<"$review_count") == 4 ]]
[[ ! -e "$repo/comments.md" ]]

assert_rejected_target() {
	local name=$1
	local expected=$2
	shift 2
	local rejected_output="$test_root/$name-output"
	local rejected_error="$test_root/$name-error"
	local status

	set +e
	(
		cd "$repo"
		PATH="$fake_bin:$PATH" ./scripts/codex-review-loop.sh "$@"
	) >"$rejected_output" 2>"$rejected_error"
	status=$?
	set -e

	if [[ $status -ne 1 ]]; then
		printf '%s status = %d, want 1\n' "$name" "$status" >&2
		cat "$rejected_output" >&2
		cat "$rejected_error" >&2
	fi
	[[ $status -eq 1 ]]
	grep -Fxq "codex-review-loop: $expected" "$rejected_error"
}

assert_rejected_target uncommitted \
	'--uncommitted cannot include committed fixes; use --base BRANCH' \
	--uncommitted
assert_rejected_target commit \
	'--commit cannot include later fix commits; use --base BRANCH' \
	--commit HEAD
assert_rejected_target short_output \
	'-o/--output-last-message is managed by this script and always writes comments.md' \
	-o elsewhere.md
assert_rejected_target long_output \
	'-o/--output-last-message is managed by this script and always writes comments.md' \
	--output-last-message elsewhere.md
assert_rejected_target long_output_equals \
	'-o/--output-last-message is managed by this script and always writes comments.md' \
	--output-last-message=elsewhere.md

interrupt_output="$test_root/interrupt-output"
interrupt_error="$test_root/interrupt-error"
interrupt_child_pid="$test_root/interrupt-child-pid"
interrupt_child_terminated="$test_root/interrupt-child-terminated"
interrupt_base=$(git -C "$repo" rev-parse origin/master)
set +e
(
	cd "$repo"
	export PATH="$fake_bin:$PATH"
	export CODEX_TEST_HANG=1
	export CODEX_TEST_REVIEW_COUNT="$review_count"
	export CODEX_TEST_EXPECTED_BASE="$interrupt_base"
	export CODEX_TEST_CHILD_PID="$interrupt_child_pid"
	export CODEX_TEST_CHILD_TERMINATED="$interrupt_child_terminated"
	exec ./scripts/codex-review-loop.sh
) >"$interrupt_output" 2>"$interrupt_error" &
interrupt_pid=$!
sleep 1
kill -TERM "$interrupt_pid"
(
	sleep 5
	kill -KILL "$interrupt_pid" 2>/dev/null || true
) &
watchdog_pid=$!
wait "$interrupt_pid"
interrupt_status=$?
kill "$watchdog_pid" 2>/dev/null || true
wait "$watchdog_pid" 2>/dev/null || true
set -e

if [[ $interrupt_status -ne 143 ]]; then
	printf 'interrupt status = %d, want 143\n' "$interrupt_status" >&2
	cat "$interrupt_output" >&2
	cat "$interrupt_error" >&2
fi
[[ $interrupt_status -eq 143 ]]
grep -Eq '^Review [0-9]+:[0-9]{2} \. ❌$' "$interrupt_output"
grep -Fxq 'codex-review-loop: interrupted' "$interrupt_error"
[[ -s "$interrupt_child_pid" ]]
[[ -s "$interrupt_child_terminated" ]]
if kill -0 "$(<"$interrupt_child_pid")" 2>/dev/null; then
	printf 'Codex descendant %s survived interruption\n' "$(<"$interrupt_child_pid")" >&2
	exit 1
fi

if grep -Fq 'exec {' "$repo/scripts/codex-review-loop.sh"; then
	printf '%s\n' 'review loop uses Bash 4 dynamic file descriptors' >&2
	exit 1
fi
progress_read_timeout=$(sed -n 's/^readonly progress_read_timeout=\([0-9][0-9]*\)$/\1/p' "$repo/scripts/codex-review-loop.sh")
if [[ -z "$progress_read_timeout" ]] ||
	grep -Eq 'read .* -t "\$activity_interval"' "$repo/scripts/codex-review-loop.sh"; then
	printf '%s\n' 'review loop uses a non-integer Bash read timeout' >&2
	exit 1
fi
grep -Fxq "readonly -a activity_frames=('·' '•' '●' '•')" "$repo/scripts/codex-review-loop.sh"
grep -Fxq "readonly activity_interval='0.25'" "$repo/scripts/codex-review-loop.sh"
grep -Fxq 'readonly activity_interval_us=250000' "$repo/scripts/codex-review-loop.sh"
grep -Fq 'kill -USR1 -- "$progress_owner_pid"' "$repo/scripts/codex-review-loop.sh"
if grep -Eq '(^|[[:space:]])timeout[[:space:]]' "$0"; then
	printf '%s\n' 'review-loop test depends on an external timing utility' >&2
	exit 1
fi

printf '%s\n' 'codex-review-loop tests passed'
