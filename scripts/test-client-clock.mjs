#!/usr/bin/env node
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';

const repoRoot = path.resolve(import.meta.dirname, '..');
const clientPath = path.join(repoRoot, 'cmd', 'ambience', 'web', 'client.js');
let now = 0;
const context = {
	console: {
		warn() {},
		error: console.error,
	},
	document: {
		querySelector() {
			return null;
		},
	},
	performance: {
		now() {
			return now;
		},
	},
	window: {},
};
context.window.window = context.window;

vm.createContext(context);
vm.runInContext(fs.readFileSync(clientPath, 'utf8'), context, { filename: clientPath });

const createPlaybackClock = context.window.AmbienceClientClock?.createPlaybackClock;
assert.equal(typeof createPlaybackClock, 'function');

function makeClock() {
	now = 0;
	return createPlaybackClock({
		tickMs: 100,
		delayTicks: 50,
		softCatchupDrift: 20,
		hardCatchupDrift: 100,
		maxCatchupSteps: 5,
		now: () => now,
	});
}

{
	const clock = makeClock();
	assert.equal(clock.estimatedAuthorityTick(12), 12);
	assert.equal(clock.targetPlaybackTick(12), 0);
	assert.equal(clock.stepsFor(12), 0);

	clock.noteAuthorityTick(100);
	assert.equal(clock.estimatedAuthorityTick(0), 100);
	assert.equal(clock.targetPlaybackTick(0), 50);

	now = 1000;
	assert.equal(clock.estimatedAuthorityTick(0), 110);
	assert.equal(clock.targetPlaybackTick(0), 60);
	assert.equal(clock.stepsFor(60), 0);
}

{
	const clock = makeClock();
	clock.noteAuthorityTick(55);
	assert.equal(clock.stepsFor(4), 1);
}

{
	const clock = makeClock();
	clock.noteAuthorityTick(80);
	assert.equal(clock.stepsFor(5), 2);
}

{
	const clock = makeClock();
	clock.noteAuthorityTick(200);
	assert.equal(clock.stepsFor(40), 5);
}

{
	const clock = makeClock();
	clock.noteAuthorityTick(100);
	const state = JSON.parse(JSON.stringify(clock.debugState(45, {
		queuedCommands: 3,
		nextQueuedCommandTick: 52,
		maxQueuedCommandTick: 75,
	})));
	assert.deepEqual(state, {
		authorityTick: 100,
		playbackTick: 50,
		simTick: 45,
		driftTicks: 5,
		delayTicks: 50,
		bufferedAheadTicks: 25,
		tickMs: 100,
		queuedCommands: 3,
		nextQueuedCommandTick: 52,
		maxQueuedCommandTick: 75,
		haveAuthoritySample: true,
	});
}

{
	const clock = makeClock();
	clock.noteAuthorityTick(120);
	const state = JSON.parse(JSON.stringify(clock.debugState(70, {
		queuedCommands: 1,
		nextQueuedCommandTick: 65,
		maxQueuedCommandTick: 65,
	})));
	assert.equal(state.playbackTick, 70);
	assert.equal(state.bufferedAheadTicks, 0, 'buffered-ahead is clamped once playback reaches the command horizon');
}

console.log('client playback clock ok');
