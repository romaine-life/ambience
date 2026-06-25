#!/usr/bin/env node
// Harness for cmd/ambience/web/client.js. The client runs a LOCAL sim replica
// that free-runs (one step per render frame) and applies the server's
// broadcasts on arrival — no delayed playback, no jitter buffer, no authority-
// tick tracking. These tests pin that contract.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';

const repoRoot = path.resolve(import.meta.dirname, '..');
const clientPath = path.join(repoRoot, 'cmd', 'ambience', 'web', 'client.js');
const clientSource = fs.readFileSync(clientPath, 'utf8');

function makeEffectClass(kind) {
	return class ScriptedEffect {
		constructor(w, h) {
			this.kind = kind;
			this.w = w;
			this.h = h;
			this.tick = 0;
			this.configVersion = 0;
			this.triggers = [];
			this.grid = new Uint8ClampedArray(w * h * 3);
		}
		restoreSnapshot(snap) {
			this.tick = snap.tick || 0;
			this.configVersion = snap.config?.version || 0;
			this.triggers = Array.isArray(snap.state?.triggers) ? snap.state.triggers.slice() : [];
		}
		setConfig(cfg) {
			this.configVersion = cfg?.version || 0;
		}
		triggerEvent(event) {
			this.triggers.push(`${event}@${this.tick}`);
		}
		step() {
			this.tick++;
		}
		render() {}
		getDebugState() {
			return {
				kind: this.kind,
				configVersion: this.configVersion,
				triggers: this.triggers.slice(),
			};
		}
	};
}

function makeConsumer(name, effects) {
	let now = 0;
	const intervals = [];
	const streams = [];
	const canvas = {
		dataset: {
			ambienceGridW: '4',
			ambienceGridH: '3',
			ambienceInitialFadeMs: '0',
		},
		getContext() {
			return {
				clearRect() {},
				fillRect() {},
				drawImage() {},
				save() {},
				restore() {},
				set fillStyle(_) {},
				set globalAlpha(_) {},
			};
		},
		insertAdjacentElement() {},
	};

	class FakeEventSource {
		constructor(url) {
			this.url = url;
			this.listeners = new Map();
			streams.push(this);
		}
		addEventListener(type, fn) {
			const list = this.listeners.get(type) || [];
			list.push(fn);
			this.listeners.set(type, list);
		}
		emit(kind, tick, data, extra = {}) {
			const cmd = Object.assign({ kind, tick, data }, extra);
			const event = { data: JSON.stringify(cmd) };
			for (const fn of this.listeners.get('message') || []) fn(event);
		}
		close() {}
	}

	const context = {
		console,
		location: { hostname: 'example.test' },
		performance: { now: () => now },
		setInterval(fn) {
			intervals.push(fn);
			return intervals.length;
		},
		setTimeout(fn) {
			fn();
			return 1;
		},
		EventSource: FakeEventSource,
		getComputedStyle() {
			return { zIndex: 'auto', backgroundColor: '#000' };
		},
		document: {
			body: { classList: { add() {} } },
			querySelector(selector) {
				return selector === 'canvas[data-ambience]' ? canvas : null;
			},
			createElement() {
				return {
					style: {},
					setAttribute() {},
					remove() {},
				};
			},
			head: {
				appendChild(el) {
					if (el.onload) el.onload();
				},
			},
			addEventListener() {},
		},
		window: {
			AmbienceSim: {
				effects,
				EffectTransition: class EffectTransition {
					constructor(outgoing, incoming) {
						this.isTransition = true;
						this.outgoing = outgoing;
						this.incoming = incoming;
						this.elapsed = 0;
						this.duration = 3;
					}
					step() {
						this.outgoing.step();
						this.incoming.step();
						this.elapsed++;
					}
					done() {
						return this.elapsed >= this.duration;
					}
					render() {}
					setConfig(cfg) {
						this.incoming.setConfig(cfg);
					}
					triggerEvent(event) {
						this.incoming.triggerEvent(event);
					}
					restoreSnapshot(snap) {
						this.incoming.restoreSnapshot(snap);
					}
				},
				wasm: { ready: async () => {} },
			},
			addEventListener() {},
			devicePixelRatio: 1,
		},
	};
	context.window.window = context.window;
	context.window.document = context.document;
	context.window.location = context.location;
	context.window.performance = context.performance;
	context.window.setInterval = context.setInterval;
	context.window.setTimeout = context.setTimeout;
	context.window.EventSource = context.EventSource;
	context.window.getComputedStyle = context.getComputedStyle;
	context.AmbienceSim = context.window.AmbienceSim;
	context.EventSource = FakeEventSource;

	vm.createContext(context);
	vm.runInContext(clientSource, context, { filename: clientPath });

	return {
		name,
		get stream() {
			assert.equal(streams.length, 1, `${name} should create exactly one EventSource`);
			return streams[0];
		},
		// One render frame: steps the local sim once and processes timers.
		advance() {
			now += 16;
			for (const fn of intervals) fn();
		},
		state() {
			return context.window.AmbienceClient.getDebugState();
		},
	};
}

function send(stream, kind, tick, data, extra) {
	stream.emit(kind, tick, data, extra);
}

function plain(value) {
	return JSON.parse(JSON.stringify(value));
}

const effects = {
	scripted: makeEffectClass('scripted'),
	rotated: makeEffectClass('rotated'),
};
for (const required of ['scripted', 'rotated']) {
	assert.equal(typeof effects[required], 'function', `test registry missing ${required}`);
}

// --- Fresh join: play the intro, then free-run (one local step per frame) ---
const a = makeConsumer('consumer-a', effects);
await new Promise((resolve) => setImmediate(resolve));
send(a.stream, 'snapshot', 0, {
	type: 'scripted',
	tick: 0,
	joinMode: 'fresh',
	config: { version: 1 },
	state: { triggers: [] },
	servedEffects: ['scripted'],
	currentScene: { name: 'scene-a', durationTicks: 80, startedAtTick: 0 },
	nextScene: { name: 'scene-b', durationTicks: 90, startedAtTick: 80 },
	sceneRemaining: 80,
});
assert.equal(a.state().effectType, 'scripted', 'first snapshot selects the effect');
assert.equal(a.state().ready, true, 'client is ready after the first snapshot');
assert.deepEqual(plain(a.state().sim.triggers), ['intro@0'], 'a fresh effect plays its intro on join');
assert.equal(a.state().simTick, 0, 'replica starts at the snapshot tick');

// Free-run: each frame advances the local sim by exactly one tick — no authority
// gating, so no skipped/doubled frames.
for (let i = 0; i < 5; i++) a.advance();
assert.equal(a.state().simTick, 5, 'replica free-runs one step per frame');

// --- config + trigger apply immediately on arrival (no per-tick queue) ---
send(a.stream, 'config', 10, { version: 2 });
assert.equal(a.state().sim.configVersion, 2, 'config applies on arrival');
send(a.stream, 'trigger', 12, {}, { event: 'gust' });
assert.deepEqual(plain(a.state().sim.triggers), ['intro@0', 'gust@5'],
	'trigger applies on arrival at the replica current tick, not a queued authority tick');

// --- scene + metric update observable scene metadata ---
send(a.stream, 'scene', 8, { name: 'scene-c', nextName: 'scene-d', durationTicks: 70, startedAtTick: 8 });
send(a.stream, 'metric', 9, { currentName: 'scene-c', nextName: 'scene-d', sceneRemaining: 61 });
assert.deepEqual(plain(a.state().scene), {
	currentName: 'scene-c', nextName: 'scene-d', sceneRemaining: 61, durationTicks: 70, startedAtTick: 8,
}, 'scene + metric update the observable scene metadata');

// --- 'clock' commands are ignored: the replica free-runs regardless ---
const beforeClock = a.state().simTick;
send(a.stream, 'clock', 999, { tick: 999, tickRateMs: 16.6, suggestedDelayTicks: 300 });
a.advance();
assert.equal(a.state().simTick, beforeClock + 1, 'a clock command does not jump or gate the replica');

// --- Restore join: NO intro, replay the snapshot as-is, then free-run ---
const r = makeConsumer('consumer-restore', effects);
await new Promise((resolve) => setImmediate(resolve));
send(r.stream, 'snapshot', 500, {
	type: 'scripted',
	tick: 500,
	joinMode: 'restore',
	config: { version: 1 },
	state: { triggers: ['planted@500'] },
	servedEffects: ['scripted'],
	currentScene: { name: 'persist', durationTicks: 100, startedAtTick: 500 },
});
assert.deepEqual(plain(r.state().sim.triggers), ['planted@500'],
	'a restore effect keeps its snapshot as-is and does NOT play an intro');
assert.equal(r.state().simTick, 500, 'restore replica starts at the restored tick');
for (let i = 0; i < 3; i++) r.advance();
assert.equal(r.state().simTick, 503, 'restore replica also free-runs one step per frame');

// --- Effect rotation arrives as a typed snapshot → crossfade to the incoming ---
send(a.stream, 'snapshot', 70, {
	type: 'rotated',
	tick: 70,
	joinMode: 'restore',
	config: { version: 7 },
	state: { triggers: ['rotate@70'] },
	servedEffects: ['rotated'],
	currentScene: { name: 'rotated-scene', durationTicks: 100, startedAtTick: 70 },
});
assert.equal(a.state().effectType, 'rotated', 'rotation switches the effect type');
for (let i = 0; i < 6; i++) a.advance(); // step past the crossfade duration
assert.equal(a.state().sim.kind, 'rotated', 'crossfade unwraps to the incoming effect');
assert.equal(a.state().sim.configVersion, 7, 'incoming effect carries the rotated config');

// --- Unsupported effect type surfaces as a registry error, not a silent swap ---
send(a.stream, 'snapshot', 100, { type: 'missing-effect', tick: 100, config: {}, state: {} });
assert.equal(a.state().effectType, 'rotated', 'unsupported effect does not replace the active effect');
assert.match(a.state().lastError, /unknown effect type: missing-effect/);

console.log('browser-client free-run harness ok');
