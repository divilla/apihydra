#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

repo="$test_root/skeleton/repo with spaces"
fake_bin="$test_root/bin"
log="$test_root/tool.log"
mkdir -p "$repo/pkg/runner" "$repo/scripts" "$repo/agent/specs" "$fake_bin"
cp "$script_dir/../Makefile" "$repo/Makefile"
printf '%s\n' '# Domain types' >"$repo/agent/specs/000-domain-types.md"

cat >"$fake_bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

[[ ${1-} == list ]]
if [[ $# -eq 2 && $2 == apih/... ]]; then
	if [[ ${MAKEFILE_TEST_EMPTY-} == 1 ]]; then
		printf '%s\n' apih/skeleton apih/skeleton/internal/domain
		exit 0
	fi
	printf '%s\n' apih/pkg/runner apih/skeleton apih/skeleton/internal/domain
	exit 0
fi

[[ $# -eq 4 && $2 == -f && $3 == '{{.Dir}}' ]]
printf 'go-list-dir:%s\n' "$4" >>"$MAKEFILE_TEST_LOG"
case "$4" in
apih/pkg/runner)
	printf '%s\n' "$MAKEFILE_TEST_REPO/pkg/runner"
	;;
apih/...)
	printf '%s\n' \
		"$MAKEFILE_TEST_REPO/pkg/runner" \
		"$MAKEFILE_TEST_REPO/skeleton" \
		"$MAKEFILE_TEST_REPO/skeleton/internal/domain"
	;;
*)
	exit 1
	;;
esac
EOF

cat >"$fake_bin/goimports" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 2 && $1 == -w ]]
printf 'goimports:%s\n' "$2" >>"$MAKEFILE_TEST_LOG"
EOF

cat >"$fake_bin/staticcheck" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf 'staticcheck:%s\n' "$*" >>"$MAKEFILE_TEST_LOG"
EOF

cat >"$fake_bin/golint" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf 'golint:%s\n' "$*" >>"$MAKEFILE_TEST_LOG"
EOF

chmod +x "$fake_bin/go" "$fake_bin/goimports" "$fake_bin/staticcheck" "$fake_bin/golint"

for tool in create-change-branch.sh codex-code-spec.pl codex-review-loop.pl; do
	printf '%s\n' \
		'#!/usr/bin/env bash' \
		'set -euo pipefail' \
		'printf "%s:%s\n" "$(basename "$0")" "$1" >>"$MAKEFILE_TEST_LOG"' \
		>"$repo/scripts/$tool"
	chmod +x "$repo/scripts/$tool"
done

(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		MAKEFILE_TEST_LOG="$log" \
		MAKEFILE_TEST_REPO="$repo" \
		make lint
)

[[ $(grep -c '^goimports:' "$log") -eq 1 ]]
grep -Fxq "go-list-dir:apih/pkg/runner" "$log"
grep -Fxq "goimports:$repo/pkg/runner" "$log"
grep -Fxq 'staticcheck:apih/pkg/runner' "$log"
grep -Fxq 'golint:apih/pkg/runner' "$log"
! grep -Fq 'go-list-dir:apih/skeleton' "$log"

: >"$log"
(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		MAKEFILE_TEST_EMPTY=1 \
		MAKEFILE_TEST_LOG="$log" \
		MAKEFILE_TEST_REPO="$repo" \
		make lint vet race
)
[[ ! -s "$log" ]]

: >"$log"
(
	cd "$repo"
	PATH="$fake_bin:$PATH" \
		MAKEFILE_TEST_LOG="$log" \
		make implement agent/specs/000-domain-types.md
)
expected_implement_log=$'create-change-branch.sh:agent/specs/000-domain-types.md\ncodex-code-spec.pl:agent/specs/000-domain-types.md\ncodex-review-loop.pl:agent/specs/000-domain-types.md'
[[ $(<"$log") == "$expected_implement_log" ]]

usage_error="$test_root/implement-usage-error"
set +e
(
	cd "$repo"
	PATH="$fake_bin:$PATH" make implement
) > /dev/null 2>"$usage_error"
usage_status=$?
set -e
[[ $usage_status -eq 2 ]]
grep -Fxq 'usage: make implement <spec-path>' "$usage_error"

printf '%s\n' 'Makefile tests passed'
