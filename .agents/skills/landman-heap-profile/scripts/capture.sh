#!/bin/sh
set -eu

if [ "$#" -ne 1 ] || [ ! -S "$1" ]; then
    printf 'Usage: %s HEAP_SOCKET\n' "$0" >&2
    exit 2
fi

umask 077
dir=$(mktemp -d)
profile=$dir/heap.pb.gz
trap 'rm -f -- "$profile"; rmdir -- "$dir"' EXIT
trap 'exit 1' HUP INT TERM
curl --disable --fail --silent --show-error --noproxy '*' --max-time 25 \
    --unix-socket "$1" --output "$profile" http://localhost/heap
go tool pprof -top -nodecount=1 "$profile" >/dev/null
trap - EXIT
printf '%s\n' "$profile"
