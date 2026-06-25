// client.js — shared auto-init ambience client.
//
// Any consumer page can opt in by dropping:
//
//   <canvas data-ambience></canvas>
//   <script src="https://ambience.romaine.life/sim.js"></script>
//   <script src="https://ambience.romaine.life/client.js"></script>
//
// and get a rain (or future effect) overlay plus live entropy contribution
// to the shared atmosphere. client.js loads the Go/WASM runtime itself; no
// per-consumer JS required.
//
// PLAYBACK MODEL: each client runs its OWN local sim replica and steps it once
// per render frame. The server does not stream frames and the client does not
// try to lock its tick to the server's — it just advances the local sim and
// applies the server's broadcast commands (config/trigger/scene) as they
// arrive. Frame-level sync across clients is intentionally not a goal (each
// replica's RNG drifts after the initial snapshot); only event timing stays in
// sync, because every client gets the same broadcasts. This is why there is no
// playback delay, jitter buffer, or authority-tick estimation here: those frame-
// lock the replica to the server and produce join freezes / judder. Keep it
// free-running.
//
// Configuration, via attributes on the <canvas>:
//   data-ambience-url="https://ambience.romaine.life"   — stream/server override
//   data-ambience-wasm-url / -wasm-exec-url / -runtime-url — load the runtime
//     from a vendored, version-pinned copy instead of the stream origin. Lets a
//     consumer bundle its own (effect-scoped) WASM while subscribing to a world.
//   data-ambience-grid-w="320" / data-ambience-grid-h="180" — sim grid size
//   data-ambience-transparent="false"  — paint solid bg (default: true)
//   data-ambience-entropy="off"        — disable keystroke entropy upload
//   data-ambience-initial-fade-ms="1200" — fade in after the first authority snapshot
//   data-ambience-initial-fade-color="#050505" — startup cover color
//   data-ambience-intro-on-join="off" — disable the join intro. By default a
//     freshly-connected client does NOT drop into a "fresh" world (e.g. rain)
//     mid-storm: it plays the effect's "intro" beat so the scene eases in (rain
//     starts spawning from a near-empty grid) instead of the full atmosphere
//     jolting on at once. Set to "off" to restore instant mid-simulation
//     replication. (Effects whose snapshot declares joinMode "restore" — a
//     persistent scene that's already there — always replay as-is.)
//
// Effect agnostic: the server's snapshot broadcasts the effect type; this
// file looks it up in AmbienceSim.effects[type]. Adding a new effect means
// registering a Go-backed constructor through wasm_runtime.js — no change
// needed here.

(function () {
	'use strict';

	function loadScript(src) {
		return new Promise((resolve, reject) => {
			const el = document.createElement('script');
			el.src = src;
			el.async = true;
			el.onload = resolve;
			el.onerror = () => reject(new Error('failed to load ' + src));
			document.head.appendChild(el);
		});
	}

	const canvas = document.querySelector('canvas[data-ambience]');
	if (!canvas) {
		console.warn('ambience-client: no <canvas data-ambience> found');
		return;
	}
	if (!window.AmbienceSim) {
		console.warn('ambience-client: AmbienceSim missing — load sim.js first');
		return;
	}

	const isLocalhost =
		location.hostname === 'localhost' || location.hostname === '127.0.0.1';
	const SERVER =
		canvas.dataset.ambienceUrl ||
		window.AMBIENCE_URL ||
		(isLocalhost ? 'http://127.0.0.1:8080' : 'https://ambience.romaine.life');
	const trimSlashes = (s) => s.replace(/\/+$/, '');
	// Asset URLs default to SERVER (load the runtime from the same origin as the
	// stream). A vendoring consumer overrides them so the WASM/runtime load from
	// its OWN bundled, version-pinned copy while the stream still points at the
	// world. This is what lets the chess menu ship a vendored rain-only WASM yet
	// subscribe to ambience's /chess world.
	const WASM_URL =
		canvas.dataset.ambienceWasmUrl || trimSlashes(SERVER) + '/ambience.wasm';
	const WASM_EXEC_URL =
		canvas.dataset.ambienceWasmExecUrl || trimSlashes(SERVER) + '/wasm_exec.js';
	const RUNTIME_URL =
		canvas.dataset.ambienceRuntimeUrl || trimSlashes(SERVER) + '/wasm_runtime.js';
	const GRID_W = parseInt(canvas.dataset.ambienceGridW || '320', 10);
	const GRID_H = parseInt(canvas.dataset.ambienceGridH || '180', 10);
	const TRANSPARENT = canvas.dataset.ambienceTransparent !== 'false';
	const ENTROPY_ENABLED = canvas.dataset.ambienceEntropy !== 'off';
	const TICK_MS = 1000 / 60;
	const INITIAL_FADE_MS = Math.max(0, parseInt(canvas.dataset.ambienceInitialFadeMs || '1200', 10) || 0);
	const INTRO_ON_JOIN = canvas.dataset.ambienceIntroOnJoin !== 'off';

	const ctx = canvas.getContext('2d');
	if (canvas.style) canvas.style.imageRendering = canvas.style.imageRendering || 'pixelated';
	ctx.imageSmoothingEnabled = false;
	let initialFadeCover = null;

	// Mark body so consumer CSS can conditionally adapt (e.g. make terminal
	// backgrounds transparent only when ambience is actually running).
	document.body.classList.add('ambience-on');

	function resize() {
		const dpr = window.devicePixelRatio || 1;
		canvas.width = Math.floor(window.innerWidth * dpr);
		canvas.height = Math.floor(window.innerHeight * dpr);
	}
	resize();
	window.addEventListener('resize', resize);

	// The first snapshot tells us what effect is running. Until then, render
	// nothing; creating a local fallback effect makes subscribers visibly
	// diverge and can crossfade two worlds together during startup.
	let effectType = null;
	let sim = null;
	let ready = false;
	let initialFadePending = false;
	let initialFadeStarted = false;
	let lastError = null;
	// Capability handshake: on the first snapshot we verify this client's
	// runtime supports every effect the world advertises in servedEffects. If
	// one is missing, the client was not built for this world — fail loudly
	// (log + refuse to render) instead of silently mis-rendering.
	let handshakeChecked = false;
	let handshakeOK = true;
	const sceneState = {
		currentName: null,
		nextName: null,
		sceneRemaining: null,
		durationTicks: null,
		startedAtTick: null,
	};

	function makeInitialFadeCover() {
		if (INITIAL_FADE_MS <= 0) return null;
		const cover = document.createElement('div');
		const canvasStyle = getComputedStyle(canvas);
		const bodyStyle = getComputedStyle(document.body);
		const color =
			canvas.dataset.ambienceInitialFadeColor ||
			bodyStyle.backgroundColor ||
			'#000';
		cover.setAttribute('aria-hidden', 'true');
		cover.style.position = 'fixed';
		cover.style.inset = '0';
		cover.style.pointerEvents = 'none';
		cover.style.background = color;
		cover.style.opacity = '1';
		cover.style.zIndex = canvasStyle.zIndex === 'auto' ? 'auto' : canvasStyle.zIndex;
		cover.style.willChange = 'opacity';
		canvas.insertAdjacentElement('afterend', cover);
		return cover;
	}

	initialFadeCover = makeInitialFadeCover();

	function revealInitialScene() {
		if (initialFadeStarted) return;
		initialFadeStarted = true;
		if (!initialFadeCover || INITIAL_FADE_MS <= 0) {
			if (initialFadeCover) initialFadeCover.remove();
			initialFadeCover = null;
			return;
		}
		const cover = initialFadeCover;
		if (cover.animate) {
			const fade = cover.animate(
				[{ opacity: 1 }, { opacity: 0 }],
				{ duration: INITIAL_FADE_MS, easing: 'ease', fill: 'both' },
			);
			fade.finished
				.then(() => { cover.remove(); })
				.catch(() => { cover.remove(); });
			initialFadeCover = null;
			return;
		}
		cover.style.transition = `opacity ${INITIAL_FADE_MS}ms ease`;
		cover.offsetWidth;
		cover.style.opacity = '0';
		setTimeout(() => { cover.remove(); }, INITIAL_FADE_MS + 50);
		initialFadeCover = null;
	}

	function getSimTick(s) {
		if (!s) return 0;
		if (s.isTransition && s.incoming) return getSimTick(s.incoming);
		return Number.isFinite(s.tick) ? s.tick : 0;
	}

	function getSimDebug(s) {
		if (!s) return null;
		if (s.isTransition && s.incoming) return getSimDebug(s.incoming);
		if (typeof s.getDebugState === 'function') return s.getDebugState();
		return null;
	}

	function updateSceneFromSnapshot(data) {
		if (!data) return;
		if (data.currentScene) {
			sceneState.currentName = data.currentScene.name || sceneState.currentName;
			sceneState.durationTicks = Number.isFinite(data.currentScene.durationTicks)
				? data.currentScene.durationTicks
				: sceneState.durationTicks;
			sceneState.startedAtTick = Number.isFinite(data.currentScene.startedAtTick)
				? data.currentScene.startedAtTick
				: sceneState.startedAtTick;
		}
		if (data.nextScene) sceneState.nextName = data.nextScene.name || sceneState.nextName;
		if (Number.isFinite(data.sceneRemaining)) sceneState.sceneRemaining = data.sceneRemaining;
	}

	function applySceneData(data) {
		if (!data) return;
		sceneState.currentName = data.name || data.currentName || sceneState.currentName;
		sceneState.nextName = data.nextName || sceneState.nextName;
		sceneState.durationTicks = Number.isFinite(data.durationTicks) ? data.durationTicks : sceneState.durationTicks;
		sceneState.startedAtTick = Number.isFinite(data.startedAtTick) ? data.startedAtTick : sceneState.startedAtTick;
		if (Number.isFinite(data.sceneRemaining)) sceneState.sceneRemaining = data.sceneRemaining;
	}

	// runHandshake verifies the client supports every effect the world may
	// broadcast. Returns false (and logs) when an advertised effect is missing
	// from this build. An older authority that advertises nothing passes.
	function runHandshake(served) {
		if (!Array.isArray(served) || served.length === 0) return true;
		const missing = served.filter((name) => !AmbienceSim.effects[name]);
		if (missing.length === 0) return true;
		lastError = `client missing world effects: ${missing.join(', ')}`;
		console.error(
			'[ambience] handshake failed — this client was not built to render ' +
			`effect(s) [${missing.join(', ')}] served by ${SERVER}. Update the ` +
			'ambience client to a version that includes them.',
		);
		return false;
	}

	// applyCommand applies one server broadcast to the local replica immediately
	// on arrival. There is no per-tick command queue: the replica free-runs and
	// events land when the SSE message does (network jitter is imperceptible for
	// downpour/calm/scene beats).
	function applyCommand(cmd, data) {
		switch (cmd.kind) {
			case 'snapshot': {
				if (!handshakeChecked) {
					handshakeChecked = true;
					handshakeOK = runHandshake(data && data.servedEffects);
				}
				if (!handshakeOK) break;
				const newType = (data && data.type) || 'rain';
				const ctor = AmbienceSim.effects[newType];
				if (!ctor) {
					lastError = `unknown effect type: ${newType}`;
					console.warn('ambience-client: unknown effect type', newType);
					break;
				}
				lastError = null;
				// joinMode is the effect's own declaration of whether its current
				// frame is coupled to a past we missed. "fresh" effects (rain) are
				// steady-state, so on first connect we start them from their intro
				// (ease in from near-empty) instead of replaying the mid-storm
				// snapshot. "restore" effects (the default) carry persistent state —
				// a structure that's already there — so we replay the snapshot
				// as-is. Unknown/legacy snapshots default to restore.
				const fresh = !!(data && data.joinMode === 'fresh');
				if (!sim) {
					sim = new ctor(GRID_W, GRID_H, {});
					try { sim.restoreSnapshot(data); } catch (err) { console.error('bad snapshot', err); }
					if (fresh && INTRO_ON_JOIN && sim.triggerEvent) {
						try { sim.triggerEvent('intro'); } catch (err) { console.error('join intro failed', err); }
					}
					effectType = newType;
					initialFadePending = true;
				} else if (newType !== effectType) {
					const incoming = new ctor(GRID_W, GRID_H, {});
					try { incoming.restoreSnapshot(data); } catch (err) { console.error('bad snapshot', err); }
					sim = AmbienceSim.EffectTransition
						? new AmbienceSim.EffectTransition(sim, incoming)
						: incoming;
					effectType = newType;
				} else {
					// Reconnect / re-snapshot of the same effect: resync to the live
					// state as-is (no re-intro — the world is already established).
					try { sim.restoreSnapshot(data); } catch (err) { console.error('bad snapshot', err); }
				}
				updateSceneFromSnapshot(data);
				ready = true;
				break;
			}
			case 'config':
				if (!sim) break;
				try { sim.setConfig(data); } catch (err) { console.error('bad config', err); }
				break;
			case 'trigger':
				if (sim && sim.triggerEvent) sim.triggerEvent(cmd.event);
				break;
			case 'scene':
			case 'metric':
				applySceneData(data);
				break;
			// 'clock' and any unknown kinds are ignored: the replica free-runs and
			// does not track the authority tick.
		}
	}

	window.AmbienceClient = {
		getDebugState: () => ({
			effectType,
			ready,
			initialFadeStarted,
			simTick: getSimTick(sim),
			scene: Object.assign({}, sceneState),
			sim: getSimDebug(sim),
			lastError,
		}),
	};

	async function start() {
		if (!AmbienceSim.wasm) {
			await loadScript(RUNTIME_URL);
		}
		if (!AmbienceSim.wasm || !AmbienceSim.wasm.ready) {
			throw new Error('ambience-client: Go WASM runtime missing');
		}
		await AmbienceSim.wasm.ready({
			wasmExecURL: WASM_EXEC_URL,
			wasmURL: WASM_URL,
		});

		const es = new EventSource(SERVER.replace(/\/+$/, '') + '/events');
		es.addEventListener('message', (e) => {
			let cmd;
			try { cmd = JSON.parse(e.data); } catch (_) { return; }
			let data = cmd.data;
			if (typeof data === 'string') {
				try { data = JSON.parse(data); } catch (_) { /* leave as string */ }
			}
			applyCommand(cmd, data);
		});

		// Free-run: step the local sim once per frame and render. rAF stays out of
		// it so background-tab throttling just slows the local replica (which is
		// fine — it's decorative and resyncs on the next snapshot), rather than
		// silently changing any tick math.
		setInterval(() => {
			if (ready && sim) sim.step();
			if (!sim) return;
			// Unwrap a finished crossfade so we drop the outgoing sim and stop
			// paying its render cost.
			if (sim.isTransition && sim.done()) {
				if (sim.outgoing && typeof sim.outgoing.destroy === 'function') sim.outgoing.destroy();
				sim = sim.incoming;
			}
			sim.render(ctx, canvas.width, canvas.height, { transparent: TRANSPARENT });
			if (initialFadePending) {
				initialFadePending = false;
				revealInitialScene();
			}
		}, Math.max(1, Math.round(TICK_MS)));
	}

	function startEntropy() {
		const buf = [];
		const FLUSH_INTERVAL_MS = 2000;
		const MAX_BUFFERED = 256;

		document.addEventListener('keydown', (e) => {
			// Hash: low-byte of key charCode ^ low-byte of milliseconds since
			// epoch. Cheap, plenty of variance for entropy purposes.
			const k = (e.key && e.key.charCodeAt(0)) || 0;
			const t = Date.now() & 0xff;
			buf.push((k ^ t) & 0xff);
			if (buf.length > MAX_BUFFERED) buf.splice(0, buf.length - MAX_BUFFERED);
		}, { passive: true });

		setInterval(() => {
			if (buf.length === 0) return;
			const bytes = buf.splice(0, buf.length);
			// Fire-and-forget; keepalive=true so it completes on unload
			try {
				fetch(SERVER.replace(/\/+$/, '') + '/entropy', {
					method: 'POST',
					body: new Uint8Array(bytes),
					keepalive: true,
				}).catch(() => {});
			} catch (_) { /* swallow */ }
		}, FLUSH_INTERVAL_MS);
	}

	start()
		.then(() => {
			if (ENTROPY_ENABLED) startEntropy();
		})
		.catch((err) => {
			console.error(err);
		});
})();
