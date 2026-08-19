#!/usr/bin/env bash
set -euo pipefail

readonly no_findings_message='No findings.'
readonly -a activity_frames=('·' '•' '●' '•')
readonly activity_interval='0.25'
readonly activity_interval_us=250000
readonly progress_read_timeout=1
readonly output_dot_interval_us=1000000

active_codex_pid=''
active_output_dir=''
progress_ticker_pid=''
requested_exit_code=0
progress_active=false
progress_has_output=false
progress_dots=0
progress_last_dot_us=0
progress_label=''
progress_started_us=0

fail() {
	printf 'codex-review-loop: %s\n' "$*" >&2
	exit 1
}

shell_join() {
	local destination=$1
	shift
	local argument
	local escaped
	local joined

	printf -v joined '%q' "$1"
	shift
	for argument in "$@"; do
		printf -v escaped '%q' "$argument"
		joined+=" $escaped"
	done
	printf -v "$destination" '%s' "$joined"
}

print_command() {
	local command
	local separator

	shell_join command "$@"
	printf -v separator '%*s' "${#command}" ''
	separator=${separator// /-}
	printf '%s\n%s\n%s\n' "$separator" "$command" "$separator"
}

print_initial_context() {
	local label_color=''
	local repository_color=''
	local branch_color=''
	local base_color=''
	local options_color=''
	local reset=''

	if [[ -t 1 && -z ${NO_COLOR-} ]]; then
		label_color=$'\033[1;36m'
		repository_color=$'\033[34m'
		branch_color=$'\033[32m'
		base_color=$'\033[33m'
		options_color=$'\033[35m'
		reset=$'\033[0m'
	fi

	printf '%sRepository:%s %s%s%s\n' "$label_color" "$reset" "$repository_color" "$repo_root" "$reset"
	printf '%sBranch:%s %s%s%s\n' "$label_color" "$reset" "$branch_color" "$branch" "$reset"
	printf '%sBase:%s %s%s%s\n' "$label_color" "$reset" "$base_color" "$review_base" "$reset"
	printf '%sPinned base:%s %s%s%s\n' "$label_color" "$reset" "$base_color" "$review_base_commit" "$reset"
	printf '%sReview options:%s %s%s%s\n' "$label_color" "$reset" "$options_color" "$review_options" "$reset"
	printf '%sComments:%s %s%s%s\n' "$label_color" "$reset" "$repository_color" "$comments_file" "$reset"
}

clock_us() {
	local destination=$1
	local now=${EPOCHREALTIME-}
	local seconds
	local fraction
	local value

	if [[ -n "$now" ]]; then
		seconds=${now%%.*}
		fraction=${now#*.}000000
		fraction=${fraction:0:6}
		value=$((10#$seconds * 1000000 + 10#$fraction))
	else
		value=$((SECONDS * 1000000))
	fi
	printf -v "$destination" '%d' "$value"
}

progress_text() {
	local now_us
	local elapsed
	local dots
	clock_us now_us
	elapsed=$(((now_us - progress_started_us) / 1000000))
	printf -v dots '%*s' "$progress_dots" ''
	dots=${dots// /.}
	printf '%s %02d:%02d %s' "$progress_label" "$((elapsed / 60))" "$((elapsed % 60))" "$dots"
}

record_output() {
	local now_us
	clock_us now_us
	progress_has_output=true
	if [[ $progress_last_dot_us -eq 0 ]] || ((now_us - progress_last_dot_us >= output_dot_interval_us)); then
		progress_dots=$((progress_dots + 1))
		progress_last_dot_us=$now_us
	fi
}

render_progress() {
	[[ -t 1 ]] || return 0

	local now_us
	local elapsed_us
	local frame_index
	clock_us now_us
	elapsed_us=$((now_us - progress_started_us))
	frame_index=$(((elapsed_us / activity_interval_us) % ${#activity_frames[@]}))
	local frame=${activity_frames[frame_index]}
	printf '\r\033[2K%s %s' "$(progress_text)" "$frame"
}

progress_tick() {
	[[ "$progress_active" == true ]] || return 0
	render_progress
}

start_progress_ticker() {
	local progress_owner_pid=$$
	(
		while true; do
			sleep "$activity_interval"
			kill -USR1 -- "$progress_owner_pid" 2>/dev/null || exit 0
		done
	) &
	progress_ticker_pid=$!
}

stop_progress_ticker() {
	[[ -n "$progress_ticker_pid" ]] || return 0
	kill "$progress_ticker_pid" 2>/dev/null || true
	wait "$progress_ticker_pid" 2>/dev/null || true
	progress_ticker_pid=''
}

finish_progress() {
	local result=${1:-success}
	[[ "$progress_active" == true ]] || return 0

	local symbol='✅'
	if [[ "$result" != success ]]; then
		symbol='❌'
	fi

	if [[ -t 1 ]]; then
		printf '\r\033[2K%s %s\n' "$(progress_text)" "$symbol"
	else
		printf '%s %s\n' "$(progress_text)" "$symbol"
	fi
	progress_active=false
}

stop() {
	requested_exit_code=$1
	trap - INT TERM

	if [[ -n "$active_codex_pid" ]]; then
		kill -TERM -- "-$active_codex_pid" 2>/dev/null || true
		return 0
	fi

	printf '%s\n' 'codex-review-loop: interrupted' >&2
	exit "$requested_exit_code"
}

run_codex() {
	progress_label=$1
	shift
	print_command "$@"
	clock_us progress_started_us
	progress_has_output=false
	progress_dots=0
	progress_last_dot_us=0
	progress_active=true

	active_output_dir=$(mktemp -d)
	local output_fifo="$active_output_dir/output"
	mkfifo "$output_fifo"
	# Monitor mode gives the background command a dedicated process group so
	# interruption can also terminate descendants that inherited the FIFO.
	set -m
	"$@" >"$output_fifo" 2>&1 &
	active_codex_pid=$!
	set +m
	local command_pid=$active_codex_pid
	local output_fd=3
	exec 3<"$output_fifo"
	start_progress_ticker
	local line
	local read_status
	local exit_code

	render_progress
	while true; do
		line=''
		if [[ "$progress_has_output" == false ]]; then
			if IFS= read -r -n 1 -t "$progress_read_timeout" -u "$output_fd" line; then
				read_status=0
			else
				read_status=$?
			fi
		elif IFS= read -r -t "$progress_read_timeout" -u "$output_fd" line; then
			read_status=0
		else
			read_status=$?
		fi
		if [[ $read_status -eq 0 ]]; then
			record_output
			render_progress
			continue
		fi
		if [[ -n "$line" ]]; then
			record_output
		fi
		if ((read_status > 128)); then
			render_progress
			continue
		fi
		break
	done

	if wait "$command_pid"; then
		exit_code=0
	else
		exit_code=$?
	fi
	stop_progress_ticker
	active_codex_pid=''
	exec 3<&-
	rm -f -- "$output_fifo"
	rmdir -- "$active_output_dir"
	active_output_dir=''
	if [[ $requested_exit_code -ne 0 ]]; then
		finish_progress failure
		printf '%s\n' 'codex-review-loop: interrupted' >&2
		exit "$requested_exit_code"
	elif [[ $exit_code -eq 0 ]]; then
		finish_progress success
	else
		finish_progress failure
	fi
	return "$exit_code"
}

# Keep the commit operation in this already-loaded shell. The fixer is allowed
# to edit repository helpers, so executing one after a fix would let those
# edits escape the Codex sandbox with this process's permissions.
commit_review_fix() {
	local message=$1

	git add -A
	git commit -m "$message"
	git push origin "$branch"
}

trap 'stop 130' INT
trap 'stop 143' TERM
trap 'progress_tick' USR1

command -v codex >/dev/null 2>&1 || fail "codex is not installed or is not on PATH"
command -v git >/dev/null 2>&1 || fail "git is not installed or is not on PATH"

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(git -C "$script_dir" rev-parse --show-toplevel 2>/dev/null) || \
	fail "scripts directory is not inside a git repository"
cd "$repo_root"

branch=$(git branch --show-current)
[[ -n "$branch" ]] || fail "cannot commit review fixes from detached HEAD"

comments_file="$repo_root/comments.md"
if git ls-files --error-unmatch -- comments.md >/dev/null 2>&1; then
	fail "comments.md is tracked; review output must remain an untracked work file"
fi

worktree_changes=$(git status --porcelain=v1 --untracked-files=all -- . ':(exclude)comments.md')
[[ -z "$worktree_changes" ]] || \
	fail "working tree must be clean except for an untracked comments.md"

default_base=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD || true)
review_args=("$@")
review_base=''
review_base_index=-1
review_base_equals=false
parse_review_options=true
for ((index = 0; index < ${#review_args[@]}; index++)); do
	argument=${review_args[index]}
	if [[ "$parse_review_options" == false ]]; then
		continue
	fi
	case "$argument" in
	--)
		parse_review_options=false
		;;
	-o | -o?* | --output-last-message | --output-last-message=*)
		fail "-o/--output-last-message is managed by this script and always writes comments.md"
		;;
	--uncommitted | --uncommitted=*)
		fail "--uncommitted cannot include committed fixes; use --base BRANCH"
		;;
	--commit | --commit=*)
		fail "--commit cannot include later fix commits; use --base BRANCH"
		;;
	--base)
		((index + 1 < ${#review_args[@]})) || fail "--base requires a branch"
		[[ -z "$review_base" ]] || fail "multiple --base options are not supported"
		((index += 1))
		review_base=${review_args[index]}
		review_base_index=$index
		[[ -n "$review_base" && "$review_base" != --* ]] || fail "--base requires a branch"
		;;
	--base=*)
		[[ -z "$review_base" ]] || fail "multiple --base options are not supported"
		review_base=${argument#--base=}
		review_base_index=$index
		review_base_equals=true
		[[ -n "$review_base" ]] || fail "--base requires a branch"
		;;
	esac
done

if [[ -z "$review_base" ]]; then
	[[ -n "$default_base" ]] || fail "cannot resolve origin's default branch; supply --base explicitly"
	review_base=$default_base
fi

review_base_commit=$(git rev-parse --verify --end-of-options "${review_base}^{commit}" 2>/dev/null) || \
	fail "cannot resolve review base $review_base to a commit"
if ((review_base_index < 0)); then
	review_args=(--base "$review_base_commit" "${review_args[@]}")
elif [[ "$review_base_equals" == true ]]; then
	review_args[review_base_index]="--base=$review_base_commit"
else
	review_args[review_base_index]=$review_base_commit
fi

review_options=''
shell_join review_options "${review_args[@]}"
print_initial_context

pass=1
fix_number=1

while true; do
	printf '\n=== Review pass %02d ===\n' "$pass"

	# `codex exec review` is the non-interactive review form that supports -o.
	rm -f -- "$comments_file"
	review_command=(codex exec review --json)
	review_output_added=false
	for argument in "${review_args[@]}"; do
		if [[ "$review_output_added" == false && "$argument" == -- ]]; then
			review_command+=(-o "$comments_file")
			review_output_added=true
		fi
		review_command+=("$argument")
	done
	if [[ "$review_output_added" == false ]]; then
		review_command+=(-o "$comments_file")
	fi
	run_codex Review "${review_command[@]}"
	[[ -f "$comments_file" ]] || fail "review did not write comments.md"

	printf '%s\n' 'Findings:'
	cat -- "$comments_file"
	printf '\n\n'

	if [[ $(<"$comments_file") == "$no_findings_message" ]]; then
		printf '%s\n' 'Changed files:' '  (none)'
		printf 'Review complete: exact message "%s" found.\n' "$no_findings_message"
		rm -f -- "$comments_file"
		exit 0
	fi

	before_fix=$(git rev-parse HEAD)
	run_codex Fix codex exec --json "fix all comments"
	after_fix=$(git rev-parse HEAD)
	[[ "$after_fix" == "$before_fix" ]] || \
		fail "codex created a commit; expected this script to create review fixes $(printf '%02d' "$fix_number")"

	changed_files=$(git status --short --untracked-files=all -- . ':(exclude)comments.md')
	printf '%s\n' 'Changed files:'
	if [[ -n "$changed_files" ]]; then
		printf '%s\n' "$changed_files"
	else
		printf '%s\n' '  (none)'
		fail "codex reported fixes but changed no files"
	fi

	commit_message=$(printf 'review fixes %02d' "$fix_number")
	printf 'Commit: %s\n' "$commit_message"
	rm -f -- "$comments_file"
	commit_review_fix "$commit_message"

	((pass += 1))
	((fix_number += 1))
done
