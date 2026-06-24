#!/bin/sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
web_dir="$root/cmd/ambience/web"
goroot="$(go env GOROOT)"

for generated in "$web_dir/wasm_exec.js" "$web_dir/ambience.wasm" "$web_dir/ambience-rain.wasm"; do
  if [ -e "$generated" ]; then
    chmod u+w "$generated"
  fi
done

cp "$goroot/lib/wasm/wasm_exec.js" "$web_dir/wasm_exec.js"
# Full all-effects bundle — the ambience site's own runtime at /ambience.wasm.
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o "$web_dir/ambience.wasm" "$root/cmd/ambience-wasm"
# Rain-scoped client artifact (-tags rainonly) — served at /ambience-rain.wasm
# for single-effect consumers to vendor (e.g. the chess /chess world). The
# linker drops every non-rain effect, so it is much smaller than the full
# bundle. See cmd/ambience-wasm/effects_rainonly.go.
GOOS=js GOARCH=wasm go build -trimpath -tags rainonly -ldflags="-s -w" -o "$web_dir/ambience-rain.wasm" "$root/cmd/ambience-wasm"
