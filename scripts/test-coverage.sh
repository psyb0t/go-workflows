#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

readonly threshold=${MIN_TEST_COVERAGE:-90}
if ! [[ "$threshold" =~ ^[0-9]+([.][0-9]+)?$ ]] || ! awk -v n="$threshold" 'BEGIN {exit !(n >= 90 && n <= 100)}'; then
	log ERROR 'coverage threshold must be between 90 and 100'
	exit 2
fi
profile=$(mktemp)
readonly profile
covdata=$(mktemp -d)
readonly covdata
child_profile=$(mktemp)
readonly child_profile
trap 'rm -f "$profile" "$child_profile" "$profile.merged"; rm -r "$covdata"' EXIT
export GO_WORKFLOWS_COVDATA_DIR="$covdata"
export GOCOVERDIR="$covdata"
export GO_WORKFLOWS_COVERAGE_WORKER="$covdata/worker"
go build -race -cover -covermode=atomic -coverpkg=./... -o "$GO_WORKFLOWS_COVERAGE_WORKER" ./tests/testinfra/recovery/cmd/worker
module=$(go list -m)
readonly module
go test -race -count=1 -timeout="${TEST_TIMEOUT:-10m}" -covermode=atomic -coverpkg=./... -coverprofile="$profile" ./...
shopt -s nullglob
counters=("$covdata"/covcounters.*)
if ((${#counters[@]})); then
	go tool covdata textfmt -i="$covdata" -o="$child_profile"
fi
awk -v root="$module/" '
	/^mode:/ {next}
	$1 ~ /\.gen\.go:/ || $1 ~ /(^|\/)gen\.go:/ {next}
	index($1, root "samples/") == 1 || index($1, root "bench/") == 1 {next}
	{statements[$1] = $2; if ($3 > counts[$1]) counts[$1] = $3}
	END {for (block in statements) print block, statements[block], counts[block]+0}
' "$profile" "$child_profile" | LC_ALL=C sort >"$profile.merged"
printf 'mode: atomic\n' >coverage.out
cat "$profile.merged" >>coverage.out
go tool cover -func=coverage.out
passed=0
if pct=$(awk -v minimum="$threshold" 'FNR == 1 {next} {total += $2; if ($3 > 0) covered += $2} END {if (total == 0) exit 1; printf "%.2f", covered * 100 / total; exit !(covered * 100 >= minimum * total)}' coverage.out); then
	passed=1
fi
[[ -n "$pct" ]] || {
	log ERROR 'coverage profile contains no eligible statements'
	exit 1
}
printf '%s\n' "$pct" >coverage-percent.txt
((passed)) || {
	log ERROR "coverage $pct is below $threshold before rounding"
	exit 1
}
log INFO "coverage $pct meets $threshold"
