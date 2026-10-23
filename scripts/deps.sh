#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

export GOFLAGS=-mod=mod
readonly action=${1:?Usage: deps.sh lock|add|tool|update|upgrade|remove}
repository_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
readonly repository_dir
readonly module_dir=${MODULE_DIR:-.}
case "$module_dir" in
. | analyzer) ;;
*)
	log ERROR 'MODULE_DIR must be . or analyzer'
	exit 2
	;;
esac
cd "$repository_dir/$module_dir"

check_age() {
	local candidate=$1 module=${1%@*} version=${1##*@} metadata released now
	[[ "$candidate" =~ ^[A-Za-z0-9./_-]+@v[0-9][A-Za-z0-9.+-]*$ ]] || {
		log ERROR 'dependency requires an explicit module@version'
		return 1
	}
	[[ "$module" != *'/../'* ]] || return 1
	if [[ "$module" == github.com/psyb0t/* ]]; then
		log DEBUG 'first-party dependency is exempt from release age'
		return 0
	fi
	# A command's package path may be below its containing module.
	module=${module%/cmd/*}
	metadata=$(go list -m -json "$module@$version")
	[[ $(jq -r '.Version' <<<"$metadata") == "$version" ]] || return 1
	released=$(date -u -d "$(jq -er '.Time' <<<"$metadata")" +%s)
	now=$(date -u +%s)
	((released <= now - 7 * 86400)) || {
		log ERROR 'third-party dependency is newer than seven days'
		return 1
	}
}

lock_modules() {
	go mod tidy
	go mod vendor
	if [[ "$module_dir" == . && -f analyzer/go.mod ]]; then
		(cd analyzer && go mod tidy && go mod vendor)
	fi
}

case "$action" in
lock) lock_modules ;;
add | tool)
	read -r -a candidates <<<"${PKG:?Set PKG=module@version}"
	for candidate in "${candidates[@]}"; do check_age "$candidate"; done
	if [[ "$action" == tool ]]; then
		go get -tool "${candidates[@]}"
	else
		go get "${candidates[@]}"
	fi
	lock_modules
	;;
update)
	[[ "${PKG:-}" =~ ^[A-Za-z0-9./_-]+$ ]] || exit 2
	version=$(go list -m -f '{{.Version}}' "$PKG@latest")
	check_age "$PKG@$version"
	go get "$PKG@$version"
	lock_modules
	;;
upgrade)
	metadata=$(go list -m -json all)
	tool_modules=$(go list -f '{{.Module.Path}}' tool | sort -u)
	module_paths=$({
		jq -sr '.[] | select(.Main != true and .Indirect != true) | .Path' <<<"$metadata"
		printf '%s\n' "$tool_modules"
	} | sed '/^$/d' | sort -u)
	mapfile -t modules <<<"$module_paths"
	candidates=()
	for module in "${modules[@]}"; do
		version=$(go list -m -f '{{.Version}}' "$module@latest")
		check_age "$module@$version"
		candidates+=("$module@$version")
	done
	go get "${candidates[@]}"
	lock_modules
	;;
remove)
	[[ "${PKG:-}" =~ ^[A-Za-z0-9./_-]+$ ]] || exit 2
	go mod edit "-droprequire=$PKG"
	lock_modules
	;;
*)
	log ERROR 'unknown dependency action'
	exit 2
	;;
esac

cd "$repository_dir"
GOFLAGS=-mod=vendor bash scripts/sec.sh
log INFO 'dependency operation completed'
