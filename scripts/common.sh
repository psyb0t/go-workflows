#!/usr/bin/env bash
set -euo pipefail

log() {
	local level=$1
	shift
	jq -cn --arg time "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
		--arg level "$level" --arg file "${BASH_SOURCE[1]##*/}" \
		--argjson line "${BASH_LINENO[0]}" --arg func "${FUNCNAME[1]:-main}" \
		--arg msg "$*" '{time:$time,level:$level,file:$file,line:$line,func:$func,msg:$msg}' >&2
}

on_error() {
	local status=$?
	log ERROR "command failed exit=$status"
	exit "$status"
}

trap on_error ERR
readonly LOG_FILE="${LOG_FILE:-/tmp/go-workflows-${0##*/}.log}"
exec > >(tee -a "$LOG_FILE") 2>&1
