#!/usr/bin/env node
// Embedding contract for client.js: the auto-init wiring (canvas attributes,
// EventSource, display frames, render, handshake), and compatibility of the
// server's wire format with the client copy chess-tactics has pinned.
//
// scripts/fixtures/client-chess-pinned.js is the client.js chess-tactics
// vendors into frontend/public/ambience/ (scripts/vendor-ambience.mjs there).
// It is deliberately frozen: it is what runs in chess until chess re-vendors,
// so the /chess world must keep working for it.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';

const repoRoot = path.resolve(import.meta.dirname, '..');
const TICK_MS = 1000 / 60;

class FakeEffect {
	constructor(w, h) { this.w = w; this.h = h; this.tick = 0; this.steps = 0; this.renders = 0; this.triggers = []; }
	restoreSnapshot(snap) { this.tick = (snap.state && snap.state.tick) || snap.tick || 0; }
	setConfig() {}
	triggerEvent(name) { this.triggers.push(name); return true; }
	step() { this.tick++; this.steps++; }
	render() { this.renders++; }
}

// mount runs a client.js source as a page would: a <canvas data-ambience>,
// sim.js's AmbienceSim registry, an EventSource, timers and display frames,
// all driven by hand.
async function mount(file, dataset) {
	const source = fs.readFileSync(file, 'utf8');
	let now = 0;
	const intervals = [];
	const frames = [];
	const streams = [];
	const canvas = {
		dataset: Object.assign({ ambienceGridW: '4', ambienceGridH: '3', ambienceInitialFadeMs: '0' }, dataset),
		style: {},
		getContext: () => ({ clearRect() {}, fillRect() {}, drawImage() {} }),
		insertAdjacentElement() {},
	};
	class FakeEventSource {
		constructor(url) { this.url = url; this.listeners = []; streams.push(this); }
		addEventListener(type, fn) { if (type === 'message') this.listeners.push(fn); }
		emit(cmd) { for (const fn of this.listeners) fn({ data: JSON.stringify(cmd) }); }
	}
	const effects = { rain: FakeEffect, aurora: FakeEffect };
	const context = {
		console,
		location: { hostname: 'example.test' },
		performance: { now: () => now },
		setInterval: (fn) => { intervals.push(fn); return intervals.length; },
		setTimeout: (fn) => { fn(); return 1; },
		requestAnimationFrame: (fn) => { frames.push(fn); return frames.length; },
		fetch: () => Promise.reject(new Error('no network in test')),
		EventSource: FakeEventSource,
		getComputedStyle: () => ({ zIndex: 'auto', backgroundColor: '#000' }),
		document: {
			currentScript: null,
			body: { classList: { add() {} } },
			querySelector: (sel) => (sel === 'canvas[data-ambience]' ? canvas : null),
			createElement: () => ({ style: {}, setAttribute() {}, remove() {} }),
			head: { appendChild(el) { if (el.onload) el.onload(); } },
			addEventListener() {},
		},
		window: {
			AmbienceSim: { effects, wasm: { ready: async () => {} } },
			addEventListener() {},
			devicePixelRatio: 1,
			innerWidth: 64,
			innerHeight: 48,
		},
	};
	context.window.window = context.window;
	context.AmbienceSim = context.window.AmbienceSim;
	for (const k of ['document', 'location', 'performance', 'setInterval', 'setTimeout', 'requestAnimationFrame', 'fetch', 'EventSource', 'getComputedStyle', 'innerWidth', 'innerHeight', 'devicePixelRatio']) {
		if (!(k in context.window)) context.window[k] = context[k];
	}
	context.innerWidth = 64;
	context.innerHeight = 48;
	context.devicePixelRatio = 1;
	vm.createContext(context);
	vm.runInContext(source, context, { filename: file });
	await new Promise((r) => setImmediate(r));
	await new Promise((r) => setImmediate(r));
	assert.equal(streams.length, 1, 'client opens one event stream');
	return {
		stream: streams[0],
		// advance runs one display frame and whatever timers the client set.
		advance(ms = TICK_MS) {
			now += ms;
			for (const fn of intervals) fn();
			const due = frames.splice(0);
			for (const fn of due) fn();
		},
		state: () => context.window.AmbienceClient.getDebugState(),
	};
}

// Wire format the server sends now (cmd/ambience/atmosphere.go): world tick on
// every command and in snapshot.worldTick, one suggested delay for every effect.
function snapshot(type, worldTick, extra = {}) {
	return {
		kind: 'snapshot', tick: worldTick,
		data: Object.assign({ type, tick: worldTick, worldTick, joinMode: type === 'rain' ? 'fresh' : 'restore', config: {}, state: {}, servedEffects: ['rain', 'aurora'] }, extra),
	};
}
function clock(worldTick) {
	return { kind: 'clock', tick: worldTick, data: { tick: worldTick, tickRateMs: TICK_MS, suggestedDelayTicks: 90 } };
}

// ── current client: auto-init plays and draws from display frames.
{
	const page = await mount(path.join(repoRoot, 'cmd', 'ambience', 'web', 'client.js'), {});
	page.stream.emit(snapshot('rain', 5000));
	page.stream.emit(clock(5000));
	for (let i = 0; i < 120; i++) page.advance();
	const s = page.state();
	assert.equal(s.ready, true);
	assert.equal(s.effectType, 'rain');
	assert.ok(s.simTick > 5000 + 100, `played from display frames (tick ${s.simTick})`);
	assert.equal(s.lastError, null);

	page.stream.emit(snapshot('missing-effect', s.simTick + 200, { servedEffects: undefined }));
	for (let i = 0; i < 250; i++) page.advance();
	assert.equal(page.state().effectType, 'rain', 'an unknown effect does not replace the running one');
	assert.match(page.state().lastError, /unknown effect type: missing-effect/);
}

// ── current client: the handshake refuses a world that serves effects this
// build cannot render.
{
	const page = await mount(path.join(repoRoot, 'cmd', 'ambience', 'web', 'client.js'), {});
	page.stream.emit(snapshot('rain', 10, { servedEffects: ['rain', 'volcano'] }));
	page.advance();
	assert.match(page.state().lastError, /client missing world effects: volcano/);
	assert.equal(page.state().ready, false);
}

// ── chess-tactics' pinned client against today's /chess wire format: rain
// joins with its intro and steps every frame without freezing. The /chess
// world serves only rain and never rotates.
{
	const page = await mount(path.join(repoRoot, 'scripts', 'fixtures', 'client-chess-pinned.js'), {});
	const start = 7000;
	page.stream.emit(snapshot('rain', start, { servedEffects: ['rain'] }));
	page.stream.emit(clock(start));
	assert.equal(page.state().ready, true, 'pinned client accepts the snapshot');
	assert.equal(page.state().lastError, null);
	let prev = page.state().simTick;
	for (let i = 0; i < 300; i++) {
		if (i % 60 === 59) page.stream.emit(clock(start + i + 1));
		if (i === 150) page.stream.emit({ kind: 'trigger', tick: start + 151, event: 'downpour' });
		page.advance();
		const t = page.state().simTick;
		assert.equal(t, prev + 1, `pinned client steps once per frame (frame ${i}: ${prev} -> ${t})`);
		prev = t;
	}
}

console.log('client embed contract ok');
