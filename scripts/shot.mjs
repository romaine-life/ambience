#!/usr/bin/env node
// Headless-Chrome screenshot helper for ambience.
//
// Why this exists: the in-editor preview/screenshot tool's *capture* step hangs
// on this machine — every grab times out at ~30s, even on a blank page. The dev
// server is fine; only the pixel grab is broken. Do NOT retry that tool, and do
// not tell the user screenshots are impossible. Use this instead, then Read the
// PNG to view it. (Same machine-wide bug chess-tactics documents.)
//
// Two ambience-specific gotchas this handles that a naive `chrome --screenshot`
// does not:
//   1. The user's normal Chrome is usually already running. A plain launch just
//      hands the URL to that instance and exits without grabbing — so we use a
//      throwaway --user-data-dir to get an independent headless instance.
//   2. ambience pages hold a persistent SSE stream (/events), so the page never
//      reaches network-idle and --virtual-time-budget hangs forever. Instead we
//      drive the DevTools Protocol: navigate, wait a fixed real-time delay for
//      the WASM + snapshot to load and the sim to accumulate a frame, then grab.
//
// Dependency-free: uses Node's built-in fetch + WebSocket (Node 18+; tested on 24).
//
// Usage:
//   node scripts/shot.mjs <url> [outPath] [WxH] [waitMs]
//
// Examples:
//   node scripts/shot.mjs "http://127.0.0.1:8080/dev?effect=rain&render=vector"
//   node scripts/shot.mjs "http://127.0.0.1:8080/dev?effect=rain" tmp-shots/grid.png 1280x720 5000
//
// /dev takes effect + knobs as query params (docs/dev-endpoints.md), so any
// effect/config is reachable as a deep link — no clicking needed for a shot.

import { existsSync, mkdirSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { spawn } from 'node:child_process';
import { tmpdir } from 'node:os';

const BROWSERS = [
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  'C:/Program Files/Microsoft/Edge/Application/msedge.exe',
];

const url = process.argv[2];
const outArg = process.argv[3] ?? 'tmp-shots/shot.png';
const sizeArg = process.argv[4] ?? '1280x720';
const waitMs = Number(process.argv[5] ?? 4500);

if (!url) {
  console.error('Usage: node scripts/shot.mjs <url> [outPath] [WxH] [waitMs]');
  process.exit(2);
}
const browser = BROWSERS.find((p) => existsSync(p));
if (!browser) {
  console.error('No Chrome/Edge binary found. Checked:\n' + BROWSERS.join('\n'));
  process.exit(1);
}

const [w, h] = sizeArg.split('x').map(Number);
const out = resolve(process.cwd(), outArg);
mkdirSync(dirname(out), { recursive: true });
const profile = join(tmpdir(), `ambience-shot-${process.pid}-${Date.now()}`);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const fail = (msg) => { console.error(msg); cleanup(); process.exit(1); };

let chrome;
function cleanup() {
  try { chrome?.kill(); } catch {}
  try { rmSync(profile, { recursive: true, force: true }); } catch {}
}
// Watchdog: never let the helper itself hang — that's the whole point.
const watchdog = setTimeout(() => fail(`Timed out after ${waitMs + 20000}ms.`), waitMs + 20000);

chrome = spawn(browser, [
  '--headless=new',
  '--no-sandbox',
  '--disable-gpu',
  '--hide-scrollbars',
  '--no-first-run',
  '--no-default-browser-check',
  '--force-device-scale-factor=1',
  `--user-data-dir=${profile}`,
  `--window-size=${w},${h}`,
  '--remote-debugging-port=0',
  'about:blank',
], { stdio: 'ignore' });

// Chrome writes the chosen debug port to DevToolsActivePort in the profile dir.
const portFile = join(profile, 'DevToolsActivePort');
let port;
for (let i = 0; i < 100; i++) {
  if (existsSync(portFile)) { port = readFileSync(portFile, 'utf8').split('\n')[0].trim(); break; }
  await sleep(100);
}
if (!port) fail('Chrome did not expose a DevTools port.');

const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
const target = targets.find((t) => t.type === 'page') ?? targets[0];
if (!target?.webSocketDebuggerUrl) fail('No DevTools page target.');

const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((res, rej) => { ws.addEventListener('open', res, { once: true }); ws.addEventListener('error', rej, { once: true }); });

let msgId = 0;
const pending = new Map();
ws.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m.result); pending.delete(m.id); }
});
const cmd = (method, params = {}) => {
  const id = ++msgId;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((r) => pending.set(id, r));
};

await cmd('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false });
await cmd('Page.enable');
await cmd('Page.navigate', { url });
await sleep(waitMs); // let WASM load, SSE snapshot arrive, sim accumulate a frame
const shot = await cmd('Page.captureScreenshot', { format: 'png' });
if (!shot?.data) fail('captureScreenshot returned no data.');
writeFileSync(out, Buffer.from(shot.data, 'base64'));

clearTimeout(watchdog);
cleanup();
console.log(`Wrote ${out}`);
process.exit(0);
