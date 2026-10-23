#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

readonly action=${1:?Usage: checks.sh format|lint|lint-fix|build|generate|test|unit|analyzer}
readonly timeout=${TEST_TIMEOUT:-10m}
readonly packages=${PKG:-./...}
readonly count=${TEST_COUNT:-1}
[[ "$count" =~ ^[1-9][0-9]*$ ]] || {
	log ERROR 'TEST_COUNT must be a positive integer'
	exit 2
}
export GOFLAGS=${GOFLAGS:--mod=vendor}

test_module() {
	go test -race -count="$count" -timeout="$timeout" "$@"
}

case "$action" in
format)
	find . -type d \( -name vendor -o -name '.git' -o -name '.cache' -o -name '.research_files' -o -name '.testing' -o -name '.refactor' -o -name '.anustimes' -o -name '.brutal_reviews' \) -prune \
		-o -type f -name '*.go' ! -name 'mock_Backend.go' -print0 | xargs -0 -r go tool gofumpt -w
	shfmt -w scripts/*.sh
	go tool golangci-lint fmt
	;;
lint)
	go tool golangci-lint run --timeout=30m ./...
	go tool actionlint
	hadolint Dockerfile.dev Dockerfile.ui
	shellcheck --source-path=SCRIPTDIR -x scripts/*.sh
	shfmt -d scripts/*.sh
	;;
lint-fix)
	bash "$0" format
	go tool golangci-lint run --fix --timeout=30m ./...
	;;
build) go build "$packages" ;;
generate) go generate ./... ;;
test) test_module "$packages" ;;
unit) test_module -short "$packages" ;;
analyzer) (cd analyzer && test_module ./...) ;;
*)
	log ERROR 'unknown check action'
	exit 2
	;;
esac

if [[ -f analyzer/go.mod ]]; then
	case "$action" in
	build) (cd analyzer && go build ./...) ;;
	test | unit) (cd analyzer && test_module ./...) ;;
	generate) (cd analyzer && go generate ./...) ;;
	lint | lint-fix)
		lint_binary=$(go tool -n golangci-lint)
		options=()
		if [[ "$action" == lint-fix ]]; then options+=(--fix); fi
		(cd analyzer && "$lint_binary" run --config ../.golangci.yml --timeout=30m "${options[@]}" ./...)
		;;
	esac
fi

log INFO 'checks completed'
