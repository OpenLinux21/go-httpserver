#!/usr/bin/env bash

set -euo pipefail

supported_os=(linux windows darwin)

choose() {
    local prompt=$1
    shift
    local options=("$@")
    local choice

    printf '%s\n' "$prompt" >&2
    select choice in "${options[@]}"; do
        if [[ -n "$choice" ]]; then
            printf '%s' "$choice"
            return
        fi
        printf 'Invalid selection.\n' >&2
    done
}

target_os=${1:-}
target_arch=${2:-}

if [[ -z "$target_os" ]]; then
    target_os=$(choose "Select target operating system:" "${supported_os[@]}")
fi

case "$target_os" in
    linux) supported_arch=(amd64 arm64 arm 386 s390x) ;;
    windows) supported_arch=(amd64 arm64 386) ;;
    darwin) supported_arch=(amd64 arm64) ;;
    *) printf 'Unsupported operating system: %s\n' "$target_os" >&2; exit 1 ;;
esac

if [[ -z "$target_arch" ]]; then
    target_arch=$(choose "Select target architecture:" "${supported_arch[@]}")
fi

valid_arch=false
for arch in "${supported_arch[@]}"; do
    if [[ "$target_arch" == "$arch" ]]; then
        valid_arch=true
        break
    fi
done
if [[ "$valid_arch" != true ]]; then
    printf 'Unsupported architecture %s for %s\n' "$target_arch" "$target_os" >&2
    exit 1
fi

output=web_server
if [[ "$target_os" == windows ]]; then
    output+=.exe
fi

printf 'Building %s/%s -> %s\n' "$target_os" "$target_arch" "$output"
CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
    go build -trimpath -ldflags='-s -w' -o "$output" ./cmd/server
