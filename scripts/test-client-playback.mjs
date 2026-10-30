#!/usr/bin/env node
// Playback contract for the browser client (cmd/ambience/web/client.js).
//
// Drives the playback engine against a simulated authority with real-world
// imperfections: network delivery jitter, uneven display frames, stalls,
// hidden tabs, effect rotations and a stale edge snapshot. Asserts what a
// viewer would notice: playback never stops, never runs visibly fast or slow,
// and every screen applies each command on the same world tick.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';

const repoRoot = path.resolve(import.meta.dirname, '..');
const clientPath = path.join(repoRoot, 'cmd', 'ambience', 'web', 'client.js');
const clientSource = fs.readFileSync(clientPath, 'utf8');
const TICK_MS = 1000 / 60;
const DELAY = 90;

function loadLibrary() {
	const context = {
		console,
		document: { currentScript: { hasAttribute: (n) => n === 'data-ambience-library' } },
		performance: { now: () => 0 },
		window: {},
	};
	context.window.window = context.window;
	vm.createContext(context);
	vm.runInContext(clientSource, context, { filename: clientPath });
	assert.equal(typeof context.window.AmbiencePlayback?.createPlayback, 'function');
	return context.window.AmbiencePlayback;
}
const { createPlayback } = loadLibrary();

function rng(seed) {
	let s = seed >>> 0;
	return () => {
		s = (s + 0x6d2b79f5) | 0;
		let t = Math.imul(s ^ (s >>> 15), s | 1);
		t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

class FakeEffect {
	constructor() { this.kind = 'fake'; this.steps = 0; this.triggers = []; this.config = null; this.restoredFrom = null; }
	restoreSnapshot(snap) { this.restoredFrom = snap.worldTick; this.config = snap.config; }
	setConfig(cfg) { this.config = cfg; }
	triggerEvent(name) { this.triggers.push(name); return true; }
	step() { this.steps++; }
	render() {}
}
const effects = { alpha: class extends FakeEffect {}, beta: class extends FakeEffect {} };
class Transition {
	constructor(outgoing, incoming) { this.isTransition = true; this.outgoing = outgoing; this.incoming = incoming; this.n = 0; }
	step() { this.outgoing.step(); this.incoming.step(); this.n++; }
	done() { return this.n >= 30; }
	setConfig(c) { this.incoming.setConfig(c); }
	triggerEvent(e) { return this.incoming.triggerEvent(e); }
	restoreSnapshot(s) { this.incoming.restoreSnapshot(s); }
	render() {}
}

// World is the simulated authority plus any number of subscribed pages. Time
// is in ms; the authority ticks at 60 Hz; every command reaches each page after
// its own random latency.
class World {
	constructor(seed) {
		this.t = 0;
		this.tick = 1000;
		this.type = 'alpha';
		this.pages = [];
		this.rand = rng(seed);
		this.snapshotStaleTicks = 0;
		this.fetchCount = 0;
	}
	snapshot(atTick = this.tick) {
		return { type: this.type, tick: 0, worldTick: atTick, joinMode: 'restore', config: { at: atTick }, state: {} };
	}
	addPage(name, opts = {}) {
		const page = { name, inbox: [], frames: [], applied: [], latency: opts.latency || [20, 150], hiddenUntil: 0 };
		page.pb = createPlayback({
			effects,
			EffectTransition: Transition,
			gridW: 4,
			gridH: 3,
			introOnJoin: opts.introOnJoin ?? true,
			pinnedDelayTicks: opts.pinnedDelayTicks,
			now: () => this.t,
			fetchSnapshot: async () => { this.fetchCount++; return this.snapshot(this.tick - this.snapshotStaleTicks); },
			onApplied: (cmd) => { if (cmd.kind !== 'snapshot') page.applied.push([cmd.kind, cmd.event || '', page.pb.debugState().simTick]); },
			onSnapshot: (data, info) => { if (!info.first) page.applied.push(['snapshot', data.type, page.pb.debugState().simTick]); },
		});
		page.nextFrame = this.t;
		this.pages.push(page);
		const snapTick = this.tick - (opts.staleTicks || 0);
		this.deliver(page, { kind: 'snapshot', tick: snapTick, data: this.snapshot(snapTick) });
		return page;
	}
	deliver(page, cmd) {
		const [lo, hi] = page.latency;
		page.inbox.push({ at: this.t + lo + this.rand() * (hi - lo), cmd });
		page.inbox.sort((a, b) => a.at - b.at);
	}
	broadcast(cmd) { for (const p of this.pages) this.deliver(p, cmd); }
	// run advances simulated time, ticking the authority, delivering commands
	// and running each visible page's display frames (60 Hz with jitter and an
	// occasional long frame).
	run(ms, hooks = {}) {
		const end = this.t + ms;
		let nextTickAt = this.t;
		while (this.t < end) {
			this.t += 1;
			while (nextTickAt <= this.t) {
				nextTickAt += TICK_MS;
				this.tick++;
				if (this.tick % 60 === 0) this.broadcast({ kind: 'clock', tick: this.tick, data: { tick: this.tick, tickRateMs: TICK_MS, suggestedDelayTicks: DELAY } });
				if (hooks.onTick) hooks.onTick(this.tick);
			}
			for (const page of this.pages) {
				while (page.inbox.length && page.inbox[0].at <= this.t) {
					const { cmd } = page.inbox.shift();
					page.pb.onCommand(cmd, cmd.data);
				}
				if (this.t < page.hiddenUntil) { page.nextFrame = this.t; continue; }
				if (this.t >= page.nextFrame) {
					const steps = page.pb.frame(this.t);
					page.frames.push({ t: this.t, steps, tick: page.pb.debugState().simTick });
					const r = this.rand();
					page.nextFrame = this.t + (r < 0.01 ? 33 : TICK_MS + (r - 0.5) * 4);
				}
			}
		}
	}
}

async function settle() { await new Promise((r) => setImmediate(r)); }

// stats over a page's frames since `from`: the longest run of frames that
// showed no new tick, the most ticks shown by one frame, and ticks advanced per
// second (playback speed).
function stats(page, from) {
	const frames = page.frames.filter((f) => f.t >= from);
	let stall = 0, maxStall = 0, maxSteps = 0;
	for (const f of frames) {
		stall = f.steps === 0 ? stall + 1 : 0;
		maxStall = Math.max(maxStall, stall);
		maxSteps = Math.max(maxSteps, f.steps);
	}
	const perSec = [];
	for (let s = from; s + 1000 <= frames.at(-1).t; s += 1000) {
		const inSec = frames.filter((f) => f.t >= s && f.t < s + 1000);
		perSec.push(inSec.reduce((n, f) => n + f.steps, 0));
	}
	return { maxStall, maxSteps, minRate: Math.min(...perSec), maxRate: Math.max(...perSec) };
}

// A frame is 16.7 ms and playback runs at 60 ticks/s, so a single frame with
// no new tick is normal frame-to-tick beat; two in a row is a visible hitch.
// 60 ticks/s ± the 8% correction ceiling, plus a tick of counting slop.
function assertSmooth(page, from, label) {
	const s = stats(page, from);
	assert.ok(s.maxStall <= 2, `${label}: playback stalled for ${s.maxStall} frames`);
	assert.ok(s.maxSteps <= 2, `${label}: one frame jumped ${s.maxSteps} ticks`);
	assert.ok(s.minRate >= 54 && s.maxRate <= 66, `${label}: speed ranged ${s.minRate}-${s.maxRate} ticks/s`);
	return s;
}

function drift(page) {
	return page.pb.debugState().driftTicks;
}

// ── join: a page joining at the live edge starts immediately and eases back
// to the shared delay without stopping.
{
	const w = new World(1);
	const a = w.addPage('a');
	w.run(200);
	assert.ok(a.pb.ready, 'page ready after the first snapshot');
	const startTick = a.pb.debugState().simTick;
	w.run(500);
	assert.ok(a.pb.debugState().simTick > startTick + 20, 'page plays from the moment it joins, no freeze');
	w.run(90000);
	assertSmooth(a, 300, 'join at live edge');
	assert.ok(Math.abs(drift(a)) <= 3, `page settles on the shared delay (drift ${drift(a)})`);
}

// ── join from a stale edge snapshot: once the first clock sample shows how
// far behind the snapshot was, the page simulates forward to the shared moment
// in one undrawn burst (under the join fade) instead of fast-forwarding on
// screen.
{
	const w = new World(2);
	const a = w.addPage('a', { staleTicks: 300 });
	w.run(1200);
	assert.ok(Math.abs(drift(a)) <= 10, `stale join lands near the target (drift ${drift(a)})`);
	assert.ok(a.pb.debugState().jumps <= 1, 'one catch-up burst at most');
	w.run(20000);
	assertSmooth(a, 1200, 'stale join');
}

// ── steady state with network jitter and uneven frames, two pages with very
// different latency: smooth, and every command lands on the same tick.
{
	const w = new World(3);
	const a = w.addPage('a', { latency: [5, 30] });
	const b = w.addPage('b', { latency: [100, 900] });
	w.run(60000);
	const from = w.t;
	w.run(60000, {
		onTick: (tick) => {
			if (tick % 97 === 0) w.broadcast({ kind: 'trigger', tick, event: 'pulse', data: null });
			if (tick % 401 === 0) w.broadcast({ kind: 'config', tick, data: { v: tick } });
		},
	});
	assertSmooth(a, from, 'steady a');
	assertSmooth(b, from, 'steady b');
	assert.ok(a.applied.length > 40, 'commands were applied');
	assert.deepEqual(a.applied, b.applied, 'both pages apply every command on the same world tick');
	assert.equal(a.pb.debugState().lateCommands, 0, 'no command arrived after playback passed it');
}

// ── effect rotation: the new effect starts on the same world tick on every
// page, the world tick keeps counting, and playback does not stop around it.
{
	const w = new World(4);
	const a = w.addPage('a', { latency: [5, 30] });
	const b = w.addPage('b', { latency: [100, 600] });
	w.run(60000);
	const from = w.t;
	let rotatedAt = null;
	w.run(30000, {
		onTick: (tick) => {
			if (rotatedAt === null && w.t > from + 5000) {
				rotatedAt = tick;
				w.type = 'beta';
				w.broadcast({ kind: 'snapshot', tick, data: { type: 'beta', tick: 0, worldTick: tick, joinMode: 'restore', config: {}, state: {} } });
			}
		},
	});
	for (const p of [a, b]) {
		assert.equal(p.pb.effectType, 'beta', `${p.name} switched effect`);
		assert.deepEqual(p.applied.filter((x) => x[0] === 'snapshot'), [['snapshot', 'beta', rotatedAt]], `${p.name} switched on the rotation tick`);
		assertSmooth(p, from, `rotation ${p.name}`);
	}
}

// ── a short stall (a busy main thread): closed in one undrawn burst, then
// smooth again; no snapshot needed.
{
	const w = new World(5);
	const a = w.addPage('a');
	w.run(60000);
	a.hiddenUntil = w.t + 3000;
	w.run(3000);
	const fetches = w.fetchCount;
	w.run(100);
	assert.equal(a.pb.debugState().jumps, 1, 'the gap was closed in one jump');
	assert.equal(w.fetchCount, fetches, 'a short gap does not refetch a snapshot');
	assert.ok(Math.abs(drift(a)) <= 3, `back on the shared moment (drift ${drift(a)})`);
	const from = w.t;
	w.run(20000);
	assertSmooth(a, from, 'after short stall');
}

// ── a long-hidden tab: restores a fresh snapshot rather than replaying minutes.
{
	const w = new World(6);
	const a = w.addPage('a');
	w.snapshotStaleTicks = 200; // served by an edge whose cached state trails
	w.run(60000);
	a.hiddenUntil = w.t + 120000;
	w.run(120000);
	w.run(500);
	await settle();
	w.run(500);
	assert.ok(w.fetchCount >= 1, 'a long gap fetches a fresh snapshot');
	assert.ok(a.pb.debugState().resyncs >= 1, 'the snapshot was applied');
	assert.ok(Math.abs(drift(a)) <= 3, `back on the shared moment (drift ${drift(a)})`);
	const from = w.t;
	w.run(20000);
	assertSmooth(a, from, 'after long hide');
}

// ── the authority restarted without its saved state, so the world tick went
// backwards: the reconnect snapshot resets the page's clock instead of the
// page waiting for the old tick to come around again.
{
	const w = new World(7);
	const a = w.addPage('a');
	w.run(30000);
	w.tick = 500;
	w.broadcast({ kind: 'snapshot', tick: w.tick, data: w.snapshot() });
	w.run(3000);
	await settle();
	w.run(3000);
	// It rejoined at the live edge, so it is up to one delay ahead and easing back.
	assert.ok(drift(a) >= -DELAY - 5 && drift(a) <= 5, `following the restarted clock (drift ${drift(a)})`);
	assert.ok(a.pb.debugState().simTick < 1000, 'playing the restarted world');
	const from = w.t;
	w.run(90000);
	assertSmooth(a, from, 'after authority restart');
	assert.ok(Math.abs(drift(a)) <= 3, `settled on the shared delay (drift ${drift(a)})`);
}

// ── join intro: a fresh effect (rain) eases in on join; a restore effect does not.
{
	const w = new World(8);
	w.snapshot = (atTick = w.tick) => ({ type: 'alpha', tick: 0, worldTick: atTick, joinMode: 'fresh', config: {}, state: {} });
	const a = w.addPage('a');
	const b = w.addPage('b', { introOnJoin: false });
	w.run(300);
	assert.deepEqual(a.pb.sim.triggers, ['intro'], 'fresh effect plays its intro on join');
	assert.deepEqual(b.pb.sim.triggers, [], 'intro can be turned off');
	const w2 = new World(9);
	const c = w2.addPage('c');
	w2.run(300);
	assert.deepEqual(c.pb.sim.triggers, [], 'restore effect keeps its snapshot as-is');
}

// ── a pinned delay attribute overrides the world's suggestion.
{
	const w = new World(10);
	const a = w.addPage('a', { pinnedDelayTicks: 10 });
	w.run(60000);
	assert.equal(a.pb.debugState().delayTicks, 10);
	assert.ok(Math.abs(drift(a)) <= 3);
}

console.log('client playback contract ok');
