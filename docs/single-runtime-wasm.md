# Single Runtime WASM Track

Ambience's long-term rendering contract is one pixel simulation runtime across
server, browser, terminal, and generated images. The first browser-side bridge
uses the Go `sim` package compiled to WebAssembly, with JavaScript kept as the
UI, SSE, and canvas presentation layer.

The browser runtime now uses this path:

- `cmd/ambience-wasm` exports a small `window.ambienceWasm` API.
- `cmd/ambience/web/wasm_runtime.js` loads `wasm_exec.js` and `ambience.wasm`.
- `AmbienceSim.wasm.ready()` registers Go-backed constructors for every
  supported effect in `AmbienceSim.effects`.
- `/`, `/dev`, and `client.js` wait for the Go/WASM runtime before opening
  their SSE streams, so snapshots instantiate Go sim runtimes in the browser.
- The old browser-side effect implementations have been removed from the
  embedded bundle; `/sim.js` is shared browser plumbing, and effect pixels come
  from Go `GridCopy()` through WASM.

Build the browser artifact locally with:

```sh
./scripts/build-web-wasm.sh
```

Smoke-test the generated artifact with Node:

```sh
node scripts/smoke-web-wasm.mjs
```

The script writes generated files under `cmd/ambience/web/`:

- `ambience.wasm`
- `wasm_exec.js`

Those generated files are ignored by git. The Docker image build runs the same
script before compiling `cmd/ambience`, because the server embeds web assets at
Go compile time.

To experiment in a browser page:

```html
<script src="/sim.js"></script>
<script src="/wasm_runtime.js"></script>
<script>
await AmbienceSim.wasm.ready();
const Rain = AmbienceSim.effects.rain;
const rain = new Rain(160, 80, {});
</script>
```

Next steps:

- measure frame copy cost and WASM payload size
- consider replacing per-frame JS byte copies with a shared memory view if
  frame copy cost becomes visible
