#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

reports=$(mktemp -d)
readonly reports
trap 'rm -f "$reports/root.sarif" "$reports/secrets.sarif" "$reports/history.sarif" "$reports/trivy.sarif"; rmdir "$reports"' EXIT
status=0
log INFO 'scanning source and Git history for secrets'
go tool gitleaks dir . --redact --no-banner --report-format sarif --report-path "$reports/secrets.sarif" || status=$?
go tool gitleaks git . --redact --no-banner --report-format sarif --report-path "$reports/history.sarif" || status=$?
trivy fs --scanners vuln,misconfig --skip-dirs vendor,diag/app/node_modules,.pnpm-store,.cache,.research_files,.testing,.refactor,.anustimes,.brutal_reviews \
	--ignorefile .trivyignore.yaml --format sarif --output "$reports/trivy.sarif" --exit-code 1 --severity HIGH,CRITICAL . || status=$?
log INFO 'scanning reachable dependency vulnerabilities'
go tool govulncheck -format=sarif ./... >"$reports/root.sarif" || status=$?
go tool govulncheck ./... || status=$?
jq -s '{version:"2.1.0","$schema":"https://json.schemastore.org/sarif-2.1.0.json",runs:[.[].runs[]]}' \
	"$reports/root.sarif" "$reports/secrets.sarif" "$reports/history.sarif" "$reports/trivy.sarif" >sec.sarif
if ((status != 0)); then
	log ERROR 'security checks failed; inspect sec.sarif and scanner output'
	exit "$status"
fi
log INFO 'dependency security checks passed'
