# Single Runtime WASM Track

Ambience's long-term rendering contract is one pixel simulation runtime across
server, browser, terminal, and generated images. The first browser-side bridge
uses the Go `sim` package compiled to WebAssembly, with JavaScript kept as the
UI, SSE, and canvas presentation layer.

The initial slice is intentionally narrow:

- `cmd/ambience-wasm` exports a small `window.ambienceWasm` API.
- `cmd/ambience/web/wasm_runtime.js` loads `wasm_exec.js` and `ambience.wasm`.
- `AmbienceSim.wasm.Rain` wraps the Go `sim.Rain` runtime and renders its
  `GridCopy()` output through the existing pixel-grid canvas renderer.
- The live client still uses the existing JavaScript effects until the bridge
  is reviewed and migrated effect by effect.

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
await AmbienceSim.wasm.load();
const Rain = AmbienceSim.wasm.registerRain('rain-wasm');
const rain = new Rain(160, 80, {});
</script>
```

Next steps:

- add a small dev harness that can switch between `rain` and `rain-wasm`
- measure frame copy cost and WASM payload size
- expand the bridge beyond rain only after the ergonomics are acceptable
- migrate browser effects by deleting JS effect logic once the Go runtime path
  is equivalent enough for the live client
