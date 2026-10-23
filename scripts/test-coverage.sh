#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

readonly threshold=${MIN_TEST_COVERAGE:-62}
readonly analyzer_threshold=${MIN_ANALYZER_COVERAGE:-90}
if ! [[ "$threshold" =~ ^[0-9]+([.][0-9]+)?$ ]] || ! awk -v n="$threshold" 'BEGIN {exit !(n > 0 && n <= 100)}'; then
	log ERROR 'coverage threshold must be greater than zero and at most 100'
	exit 2
fi
profile=$(mktemp)
readonly profile
analyzer_profile=$(mktemp)
readonly analyzer_profile
trap 'rm -f "$profile" "$analyzer_profile"' EXIT
go test -race -count=1 -timeout="${TEST_TIMEOUT:-10m}" -covermode=atomic -coverpkg=./... -coverprofile="$profile" ./...
profiles=("$profile")
if [[ -f analyzer/go.mod ]]; then
	(cd analyzer && go test -race -count=1 -timeout="${TEST_TIMEOUT:-10m}" -covermode=atomic -coverpkg=./... -coverprofile="$analyzer_profile" ./...)
	profiles+=("$analyzer_profile")
fi
for current in "${profiles[@]}"; do
	minimum=$threshold
	if [[ "$current" == "$analyzer_profile" ]]; then minimum=$analyzer_threshold; fi
	[[ "$minimum" =~ ^[0-9]+([.][0-9]+)?$ ]] || {
		log ERROR 'invalid analyzer coverage threshold'
		exit 2
	}
	if [[ "$current" == "$analyzer_profile" ]]; then
		actual=$(cd analyzer && go tool cover -func="$current" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')
	else
		actual=$(go tool cover -func="$current" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')
	fi
	log INFO "module coverage $actual; minimum $minimum"
	awk -v actual="$actual" -v minimum="$minimum" 'BEGIN {exit !(minimum > 0 && minimum <= 100 && actual >= minimum)}' || {
		log ERROR 'a module has missing or insufficient coverage'
		exit 1
	}
done
pct=$(awk 'FNR == 1 {next} {statements[$1] = $2; if ($3 > 0) hits[$1] = 1} END {for (block in statements) {total += statements[block]; if (hits[block]) covered += statements[block]} if (total == 0) exit 1; printf "%.2f", covered * 100 / total}' "${profiles[@]}")
printf '%s\n' "$pct" >coverage-percent.txt
awk -v actual="$pct" -v minimum="$threshold" 'BEGIN {exit !(actual >= minimum)}' || {
	log ERROR "coverage $pct is below $threshold"
	exit 1
}
log INFO "coverage $pct meets $threshold"
