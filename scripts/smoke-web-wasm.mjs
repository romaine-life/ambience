import fs from 'node:fs';
import { webcrypto } from 'node:crypto';
import { performance } from 'node:perf_hooks';
import util from 'node:util';

if (!globalThis.crypto) {
	Object.defineProperty(globalThis, 'crypto', { value: webcrypto });
}
globalThis.performance = performance;
globalThis.TextEncoder = util.TextEncoder;
globalThis.TextDecoder = util.TextDecoder;

await import('../cmd/ambience/web/wasm_exec.js');

const go = new globalThis.Go();
const bytes = fs.readFileSync(new URL('../cmd/ambience/web/ambience.wasm', import.meta.url));
const result = await WebAssembly.instantiate(bytes, go.importObject);
go.run(result.instance);

await new Promise((resolve) => setTimeout(resolve, 0));

const id = globalThis.ambienceWasm.newRuntime('rain', 160, 80, '1', JSON.stringify({ spawn: 1, burst: 4 }));
if (id < 0) {
	throw new Error('newRuntime failed');
}
globalThis.ambienceWasm.step(id, 20);
const frame = globalThis.ambienceWasm.frame(id);
let nonzero = 0;
for (const b of frame) {
	if (b !== 0) nonzero++;
}
const resultSummary = {
	id,
	tick: globalThis.ambienceWasm.tick(id),
	width: globalThis.ambienceWasm.width(id),
	height: globalThis.ambienceWasm.height(id),
	bytes: frame.length,
	nonzero,
};
console.log(JSON.stringify(resultSummary));
if (resultSummary.tick !== 20 || resultSummary.bytes !== 160 * 80 * 3 || nonzero <= 0) {
	throw new Error('wasm rain smoke failed');
}
globalThis.ambienceWasm.destroy(id);
process.exit(0);
