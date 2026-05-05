#!/bin/sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
web_dir="$root/cmd/ambience/web"
goroot="$(go env GOROOT)"

cp "$goroot/lib/wasm/wasm_exec.js" "$web_dir/wasm_exec.js"
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o "$web_dir/ambience.wasm" "$root/cmd/ambience-wasm"
