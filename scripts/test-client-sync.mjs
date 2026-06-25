#!/usr/bin/env node
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

function makeConsumer(name, effects, opts = {}) {
	let now = 0;
	const intervals = [];
	const streams = [];
	const dataset = {
		ambienceGridW: '4',
		ambienceGridH: '3',
		ambienceInitialFadeMs: '0',
	};
	// Default consumers pin a delay so the alignment assertions below exercise
	// the delayed-playback machinery. Pass {delayTicks: null} to omit the attr
	// and exercise the live-edge default (no playback delay, no join freeze).
	if (opts.delayTicks !== null) dataset.ambienceDelayTicks = opts.delayTicks ?? '5';
	const canvas = {
		dataset,
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
		advance(ms = 100) {
			now += ms;
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

function assertDeepField(left, right, field, label) {
	assert.deepEqual(plain(left[field]), plain(right[field]), `${label}: ${field}`);
}

function assertAligned(a, b, label) {
	const left = a.state();
	const right = b.state();
	assert.equal(left.effectType, right.effectType, `${label}: effect type`);
	assert.equal(left.simTick, right.simTick, `${label}: sim tick`);
	assert.equal(left.playbackTick, right.playbackTick, `${label}: playback tick`);
	assert.equal(left.authorityTick, right.authorityTick, `${label}: authority tick`);
	assert.equal(left.queuedCommands, right.queuedCommands, `${label}: queued commands`);
	assertDeepField(left, right, 'scene', label);
	assertDeepField(left, right, 'sim', label);
	assert.equal(left.lastError, right.lastError, `${label}: last error`);
}

const effects = {
	scripted: makeEffectClass('scripted'),
	rotated: makeEffectClass('rotated'),
};
for (const required of ['scripted', 'rotated']) {
	assert.equal(typeof effects[required], 'function', `test registry missing ${required}`);
}

const a = makeConsumer('consumer-a', effects);
const b = makeConsumer('consumer-b', effects);
await new Promise((resolve) => setImmediate(resolve));

for (const consumer of [a, b]) {
	send(consumer.stream, 'snapshot', 0, {
		type: 'scripted',
		tick: 0,
		joinMode: 'fresh',
		config: { version: 1 },
		state: { triggers: [] },
		seed: 42,
		gridW: 4,
		gridH: 3,
		currentScene: { name: 'scene-a', durationTicks: 80, startedAtTick: 0 },
		nextScene: { name: 'scene-b', durationTicks: 90, startedAtTick: 80 },
		sceneRemaining: 80,
	});
	send(consumer.stream, 'clock', 20, { tick: 20, tickRateMs: 100, suggestedDelayTicks: 5 });
	send(consumer.stream, 'scene', 8, {
		name: 'scene-c',
		nextName: 'scene-d',
		durationTicks: 70,
		startedAtTick: 8,
	});
	send(consumer.stream, 'metric', 9, {
		currentName: 'scene-c',
		nextName: 'scene-d',
		sceneRemaining: 61,
	});
	send(consumer.stream, 'config', 10, { version: 2 });
	send(consumer.stream, 'trigger', 12, {}, { event: 'gust' });
}
assert.equal(a.state().queuedCommands, 2, 'future commands are queued before delayed playback reaches them');
assert.equal(a.state().nextQueuedCommandTick, 10, 'debug exposes next queued command tick');
assert.equal(a.state().maxQueuedCommandTick, 12, 'debug exposes command horizon');
assert.equal(
	a.state().bufferedAheadTicks,
	Math.max(0, a.state().maxQueuedCommandTick - a.state().playbackTick),
	'buffered ahead is command horizon minus playback tick',
);

for (let i = 0; i < 20; i++) {
	a.advance();
	b.advance();
}
assertAligned(a, b, 'steady buffered playback');
assert.ok(Math.abs(a.state().driftTicks) <= 1, 'clients converge to the delayed authority tick');
assert.equal(a.state().queuedCommands, 0, 'queued authority commands were applied');
assert.equal(a.state().sim.configVersion, 2, 'config command applied at playback tick');
assert.deepEqual(plain(a.state().sim.triggers), ['intro@0', 'gust@11'], 'join plays the intro beat, then the queued trigger applies at its tick');
assert.deepEqual(plain(a.state().scene), {
	currentName: 'scene-c',
	nextName: 'scene-d',
	sceneRemaining: 61,
	durationTicks: 70,
	startedAtTick: 8,
}, 'scene and metric commands update observable scene metadata');

// Simulate hidden-tab or scheduling delay. Both clients receive the same later
// authority sample and converge to the same delayed playback tick.
for (const consumer of [a, b]) {
	send(consumer.stream, 'clock', 60, { tick: 60, tickRateMs: 100, suggestedDelayTicks: 5 });
}
a.advance(3000);
b.advance(3000);
for (let i = 0; i < 20; i++) {
	a.advance();
	b.advance();
}
assertAligned(a, b, 'resume catch-up');
assert.ok(a.state().simTick > 15, 'clients catch up after a delayed authority sample');

// Effect rotation arrives as an authority snapshot with a new type. Both
// clients should instantiate the same incoming effect and converge after the
// transition wrapper finishes.
for (const consumer of [a, b]) {
	send(consumer.stream, 'snapshot', 70, {
		type: 'rotated',
		tick: 70,
		config: { version: 7 },
		state: { triggers: ['rotate@70'] },
		seed: 77,
		gridW: 4,
		gridH: 3,
		currentScene: { name: 'rotated-scene', durationTicks: 100, startedAtTick: 70 },
		nextScene: { name: 'rotated-next', durationTicks: 100, startedAtTick: 170 },
		sceneRemaining: 100,
	});
}
for (let i = 0; i < 5; i++) {
	a.advance();
	b.advance();
}
assertAligned(a, b, 'effect rotation');
assert.equal(a.state().effectType, 'rotated', 'effect rotation changes type');
assert.equal(a.state().sim.kind, 'rotated', 'debug state follows incoming rotated effect');
assert.equal(a.state().sim.configVersion, 7, 'rotated snapshot config restored');
assert.deepEqual(plain(a.state().sim.triggers), ['rotate@70'], 'rotated snapshot state restored');

// A reconnect-style fresh snapshot should bring both clients to the same
// visible phase immediately, without waiting for the next effect rotation.
for (const consumer of [a, b]) {
	send(consumer.stream, 'snapshot', 90, {
		type: 'rotated',
		tick: 90,
		config: { version: 3 },
		state: { triggers: ['resume@90'] },
		seed: 42,
		gridW: 4,
		gridH: 3,
		currentScene: { name: 'resume-scene', durationTicks: 40, startedAtTick: 90 },
		nextScene: { name: 'resume-next', durationTicks: 40, startedAtTick: 130 },
		sceneRemaining: 40,
	});
}
a.advance();
b.advance();
assertAligned(a, b, 'fresh snapshot convergence');
assert.equal(a.state().simTick, 90, 'fresh snapshot restores the visible phase');
assert.equal(a.state().sim.configVersion, 3, 'fresh snapshot replaces prior config state');
assert.deepEqual(plain(a.state().sim.triggers), ['resume@90'], 'fresh snapshot replaces prior trigger history');

// Unsupported live effect types should be visible as registry failures instead
// of silently switching consumers into different worlds.
for (const consumer of [a, b]) {
	send(consumer.stream, 'snapshot', 100, {
		type: 'missing-effect',
		tick: 100,
		config: {},
		state: {},
	});
}
assertAligned(a, b, 'unsupported effect handling');
assert.equal(a.state().effectType, 'rotated', 'unsupported effect does not replace active effect');
assert.match(a.state().lastError, /unknown effect type: missing-effect/);

// A "fresh" effect (joinMode: 'fresh', e.g. rain) with no delay attr must NOT
// freeze on join: it plays its intro, renders at the live edge (delay forced to
// 0), and starts stepping on the very first tick. Regression guard for the rain
// freezing on load.
const live = makeConsumer('consumer-live', effects, { delayTicks: null });
await new Promise((resolve) => setImmediate(resolve));
send(live.stream, 'snapshot', 500, {
	type: 'scripted',
	tick: 500,
	joinMode: 'fresh',
	config: { version: 1 },
	state: { triggers: [] },
	seed: 7,
	gridW: 4,
	gridH: 3,
	currentScene: { name: 'live-scene', durationTicks: 100, startedAtTick: 500 },
	nextScene: { name: 'live-next', durationTicks: 100, startedAtTick: 600 },
	sceneRemaining: 100,
});
assert.equal(live.state().delayTicks, 0, 'fresh consumer renders at the live edge (delay forced to 0)');
assert.equal(live.state().simTick, 500, 'fresh consumer restores at the live tick');
assert.deepEqual(plain(live.state().sim.triggers), ['intro@500'], 'fresh consumer plays the join intro');
const before = live.state().simTick;
live.advance(); // a single tick of wall-clock
assert.ok(live.state().simTick > before,
	`fresh consumer must step immediately, not freeze (was ${before}, now ${live.state().simTick})`);

// Free-run regression for the rain stutter: a fresh consumer steps EXACTLY one
// tick per frame and never freezes, even as authority clock samples jitter
// around the live edge. Under the old clock-chasing playback, a backward nudge
// in the estimate made stepsFor() return 0 and the frame froze — the "freezing
// every few frames" symptom. Free-run is immune by construction.
let prev = live.state().simTick;
for (let i = 0; i < 30; i++) {
	// Jitter the authority sample backward and forward; free-run must ignore it.
	const jitter = [0, -4, 2, -2, 3][i % 5];
	send(live.stream, 'clock', 500 + i, {
		tick: 500 + i + jitter,
		tickRateMs: 1000 / 60,
		suggestedDelayTicks: 0,
	});
	live.advance(16);
	const now = live.state().simTick;
	assert.equal(now, prev + 1,
		`free-run steps exactly one tick per frame, never freezes (frame ${i}: ${prev} -> ${now})`);
	prev = now;
}

// A "restore" effect (the default — a tree that's already there) must do the
// opposite: NO intro, and it honors the world's playback delay, so on join it
// holds the restored frame (the acceptable, intended freeze) rather than
// re-animating from scratch.
const persist = makeConsumer('consumer-persist', effects, { delayTicks: null });
await new Promise((resolve) => setImmediate(resolve));
send(persist.stream, 'snapshot', 500, {
	type: 'scripted',
	tick: 500,
	joinMode: 'restore',
	config: { version: 1 },
	state: { triggers: ['planted@500'] },
	seed: 7,
	gridW: 4,
	gridH: 3,
	currentScene: { name: 'persist-scene', durationTicks: 100, startedAtTick: 500 },
	nextScene: { name: 'persist-next', durationTicks: 100, startedAtTick: 600 },
	sceneRemaining: 100,
});
send(persist.stream, 'clock', 500, { tick: 500, tickRateMs: 100, suggestedDelayTicks: 30 });
assert.equal(persist.state().delayTicks, 30, 'restore consumer honors the world playback delay');
assert.deepEqual(plain(persist.state().sim.triggers), ['planted@500'],
	'restore consumer keeps its snapshot as-is and does NOT play an intro');
const persistBefore = persist.state().simTick;
persist.advance();
assert.equal(persist.state().simTick, persistBefore,
	'restore consumer holds the restored frame while delayed (the intended freeze), does not step yet');

console.log('browser-client sync harness ok');
