#!/usr/bin/env node
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';

const repoRoot = path.resolve(import.meta.dirname, '..');
const clientPath = path.join(repoRoot, 'cmd', 'ambience', 'web', 'client.js');
const clientSource = fs.readFileSync(clientPath, 'utf8');

function makeConsumer(name) {
	let now = 0;
	const intervals = [];
	const streams = [];
	const canvas = {
		dataset: {
			ambienceGridW: '4',
			ambienceGridH: '3',
			ambienceDelayTicks: '5',
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

	class ScriptedEffect {
		constructor(w, h) {
			this.kind = 'scripted';
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
	}

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
				effects: { scripted: ScriptedEffect },
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

function assertAligned(a, b, label) {
	const left = a.state();
	const right = b.state();
	assert.equal(left.effectType, right.effectType, `${label}: effect type`);
	assert.equal(left.simTick, right.simTick, `${label}: sim tick`);
	assert.equal(left.playbackTick, right.playbackTick, `${label}: playback tick`);
	assert.equal(left.authorityTick, right.authorityTick, `${label}: authority tick`);
	assert.equal(left.queuedCommands, right.queuedCommands, `${label}: queued commands`);
}

const a = makeConsumer('consumer-a');
const b = makeConsumer('consumer-b');
await new Promise((resolve) => setImmediate(resolve));

for (const consumer of [a, b]) {
	send(consumer.stream, 'snapshot', 0, {
		type: 'scripted',
		tick: 0,
		config: { version: 1 },
		state: { triggers: [] },
		seed: 42,
		gridW: 4,
		gridH: 3,
	});
	send(consumer.stream, 'clock', 20, { tick: 20, tickRateMs: 100, suggestedDelayTicks: 5 });
	send(consumer.stream, 'config', 10, { version: 2 });
	send(consumer.stream, 'trigger', 12, {}, { event: 'gust' });
}

for (let i = 0; i < 20; i++) {
	a.advance();
	b.advance();
}
assertAligned(a, b, 'steady buffered playback');
assert.ok(Math.abs(a.state().driftTicks) <= 1, 'clients converge to the delayed authority tick');
assert.equal(a.state().queuedCommands, 0, 'queued authority commands were applied');

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

// A reconnect-style fresh snapshot should bring both clients to the same
// visible phase immediately, without waiting for the next effect rotation.
for (const consumer of [a, b]) {
	send(consumer.stream, 'snapshot', 90, {
		type: 'scripted',
		tick: 90,
		config: { version: 3 },
		state: { triggers: ['resume@90'] },
		seed: 42,
		gridW: 4,
		gridH: 3,
	});
}
a.advance();
b.advance();
assertAligned(a, b, 'fresh snapshot convergence');
assert.equal(a.state().simTick, 90, 'fresh snapshot restores the visible phase');

console.log('cross-consumer client sync harness ok');
