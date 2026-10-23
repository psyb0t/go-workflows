#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

readonly release_tag=${RELEASE_TAG:-${GITHUB_REF_NAME:-}}
module_file=go.mod
version=$release_tag
case "$release_tag" in
analyzer/v*)
	module_file=analyzer/go.mod
	version=${release_tag#analyzer/}
	;;
v*) ;;
*)
	log ERROR 'release tag must select the root or analyzer module'
	exit 2
	;;
esac

readonly semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if ! [[ "$version" =~ $semver ]]; then
	log ERROR 'release tag is not a complete SemVer version'
	exit 2
fi
major=${BASH_REMATCH[1]}
base=${version%%+*}
if [[ "$base" == *-* ]]; then
	IFS=. read -r -a identifiers <<<"${base#*-}"
	for identifier in "${identifiers[@]}"; do
		if [[ "$identifier" =~ ^[0-9]+$ && "$identifier" == 0* && "$identifier" != 0 ]]; then
			log ERROR 'numeric prerelease identifiers cannot have leading zeroes'
			exit 2
		fi
	done
fi

repository_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
module_path=$(awk '$1 == "module" {print $2}' "$repository_dir/$module_file")
required_suffix=
if [[ "$major" != 0 && "$major" != 1 ]]; then
	required_suffix=/v$major
fi
module_suffix=
if [[ "$module_path" =~ /v([0-9]+)$ ]]; then
	module_suffix=/v${BASH_REMATCH[1]}
fi
if [[ "$module_suffix" != "$required_suffix" ]]; then
	log ERROR 'release major version does not match the Go module path'
	exit 2
fi
log INFO 'release tag matches SemVer and its Go module path'
