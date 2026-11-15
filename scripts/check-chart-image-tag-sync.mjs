import assert from 'node:assert/strict';
import fs from 'node:fs';

const chart = new URL('../chart/ambience/', import.meta.url);
const base = fs.readFileSync(new URL('values.yaml', chart), 'utf8');
assert.match(base, /^  reference: romainecr\.azurecr\.io\/ambience:[\w.+-]+@sha256:[0-9a-f]{64}$/m);
for (const file of ['values-prod.yaml', 'values-dev.yaml']) {
  const values = fs.readFileSync(new URL(file, chart), 'utf8');
  assert.doesNotMatch(values, /^image:/m, `${file} must inherit the canonical image pin`);
}
console.log('Chart environments inherit the canonical image digest');
