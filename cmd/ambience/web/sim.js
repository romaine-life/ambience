// sim.js — JS port of ambience/sim/rain.go.
//
// Clients run their own Rain sim locally. The server broadcasts config
// changes + trigger events; clients apply those via setConfig / triggerEvent.
// Step() advances the local sim one tick; drops + splashes are rendered into
// an internal grid that render() paints to a canvas.
//
// Clients do NOT roll for discrete events — that's the server's job. Clients
// only advance timers and physics. This keeps all clients in rough agreement
// on when events happen (frame-level sync is not guaranteed in v1).
//
// This file holds shared infrastructure only: the AmbienceSim namespace,
// the helper functions used by every effect (makeRNG, jitterInt, clamp01,
// hslToRGB, positiveMod), the EffectTransition crossfade wrapper, and the
// SSE subscribe() helper. Each effect's own class lives in its own file
// under web/effects/. The server bundles sim.js with every web/effects/*.js
// file when serving GET /sim.js, so dropping a new effect file Just Works —
// no shared registry to edit.

'use strict';

window.AmbienceSim = window.AmbienceSim || { effects: {}, presets: {} };

(function (api) {
	function makeRNG(seed) {
		let state = seed >>> 0;
		const rng = () => {
			state = (state + 0x6D2B79F5) | 0;
			let t = state;
			t = Math.imul(t ^ (t >>> 15), t | 1);
			t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
			return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
		};
		rng.intn = (n) => (n <= 0 ? 0 : Math.floor(rng() * n));
		return rng;
	}

	function jitterInt(rng, base, spread) {
		const f = base * (1 + spread * (rng() * 2 - 1));
		return Math.max(1, Math.round(f));
	}

	function clamp01(v) {
		return Math.max(0, Math.min(1, v));
	}

	function hslToRGB(h, s, l) {
		const c = (1 - Math.abs(2 * l - 1)) * s;
		const hp = h / 60;
		const x = c * (1 - Math.abs((hp % 2) - 1));
		let rp = 0, gp = 0, bp = 0;
		if (hp < 1)      { rp = c; gp = x; bp = 0; }
		else if (hp < 2) { rp = x; gp = c; bp = 0; }
		else if (hp < 3) { rp = 0; gp = c; bp = x; }
		else if (hp < 4) { rp = 0; gp = x; bp = c; }
		else if (hp < 5) { rp = x; gp = 0; bp = c; }
		else             { rp = c; gp = 0; bp = x; }
		const m = l - c / 2;
		const clamp = (v) => Math.max(0, Math.min(1, v));
		return {
			r: Math.round(clamp(rp + m) * 255),
			g: Math.round(clamp(gp + m) * 255),
			b: Math.round(clamp(bp + m) * 255),
		};
	}

	function positiveMod(value, mod) {
		if (mod === 0) return 0;
		return ((value % mod) + mod) % mod;
	}

	function ensurePixelGrid(effect) {
		const w = Math.max(1, effect.w | 0);
		const h = Math.max(1, effect.h | 0);
		const need = w * h * 3;
		if (!(effect.grid instanceof Uint8ClampedArray) || effect.grid.length !== need) {
			effect.grid = new Uint8ClampedArray(need);
		}
		return effect.grid;
	}

	function paintPixel(grid, w, h, x, y, color) {
		x = Math.round(x);
		y = Math.round(y);
		if (x < 0 || y < 0 || x >= w || y >= h) return;
		const i = (y * w + x) * 3;
		grid[i] = Math.max(grid[i], color.r | 0);
		grid[i + 1] = Math.max(grid[i + 1], color.g | 0);
		grid[i + 2] = Math.max(grid[i + 2], color.b | 0);
	}

	function paintProceduralGrid(effect) {
		const grid = ensurePixelGrid(effect);
		const w = Math.max(1, effect.w | 0);
		const h = Math.max(1, effect.h | 0);
		const cfg = effect.cfg || {};
		const seed = Number(effect.seed || 1) >>> 0;
		const tick = Number(effect.tick || 0);
		const kind = String(effect.kind || (effect.constructor && effect.constructor.name) || 'effect');
		let kindHash = 0;
		for (let i = 0; i < kind.length; i++) {
			kindHash = ((kindHash << 5) - kindHash + kind.charCodeAt(i)) >>> 0;
		}
		const hue = Number.isFinite(cfg.hue) ? cfg.hue : ((seed % 360) || 210);
		const sat = Number.isFinite(cfg.sat) ? cfg.sat : 0.45;
		const lmin = Number.isFinite(cfg.lmin) ? cfg.lmin : 0.12;
		const lmax = Number.isFinite(cfg.lmax) ? cfg.lmax : 0.72;
		grid.fill(0);
		for (let y = 0; y < h; y++) {
			const yr = y / Math.max(1, h - 1);
			for (let x = 0; x < w; x++) {
				const wave =
					Math.sin((x + tick * (0.12 + (kindHash & 7) * 0.018)) * (0.09 + ((kindHash >> 4) & 7) * 0.01) + seed * 0.001) +
					Math.sin((y - tick * (0.09 + ((kindHash >> 8) & 7) * 0.018)) * (0.13 + ((kindHash >> 12) & 7) * 0.012) + seed * 0.0007);
				const sparkle = Math.sin((x * (13 + (kindHash & 5)) + y * (23 + ((kindHash >> 3) & 7)) + tick + seed) * 0.071);
				const band = Math.sin((x + y + tick * 0.2) * (0.04 + ((kindHash >> 16) & 7) * 0.006));
				const light = clamp01(lmin + (lmax - lmin) * (0.25 + 0.42 * yr + 0.16 * wave + 0.08 * band + 0.1 * Math.max(0, sparkle)));
				const c = hslToRGB((hue + (kindHash % 70) - 35 + wave * 16 + sparkle * 8 + 360) % 360, clamp01(sat), light);
				const i = (y * w + x) * 3;
				grid[i] = c.r;
				grid[i + 1] = c.g;
				grid[i + 2] = c.b;
			}
		}
		const count = Math.max(8, Math.floor(w * h * 0.012));
		for (let i = 0; i < count; i++) {
			const rng = makeRNG((seed ^ (i * 2654435761) ^ ((tick / 6) | 0)) >>> 0);
			const x = rng.intn(w);
			const y = rng.intn(h);
			const c = hslToRGB((hue + rng() * 60 - 30 + 360) % 360, clamp01(sat * 1.1), clamp01(lmax));
			paintPixel(grid, w, h, x, y, c);
		}
		return grid;
	}

	function renderPixelGridEffect(effect, ctx, canvasW, canvasH, opts) {
		opts = opts || {};
		const grid = ensurePixelGrid(effect);
		if (opts.transparent) {
			ctx.clearRect(0, 0, canvasW, canvasH);
		} else {
			ctx.fillStyle = opts.bg || '#0a0a0a';
			ctx.fillRect(0, 0, canvasW, canvasH);
		}
		const w = Math.max(1, effect.w | 0);
		const h = Math.max(1, effect.h | 0);
		const sx = canvasW / w;
		const sy = canvasH / h;
		const ceilSx = Math.ceil(sx);
		const ceilSy = Math.ceil(sy);
		for (let y = 0; y < h; y++) {
			for (let x = 0; x < w; x++) {
				const i = (y * w + x) * 3;
				const r = grid[i], g = grid[i + 1], b = grid[i + 2];
				if (r === 0 && g === 0 && b === 0) continue;
				ctx.fillStyle = `rgb(${r},${g},${b})`;
				ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
			}
		}
	}

	// EffectTransition wraps two sims (an outgoing one and an incoming one)
	// behind the same step / render / setConfig / triggerEvent / restoreSnapshot
	// surface, smoothly crossfading the visual output across `durationTicks`.
	// Both sims keep stepping during the window so neither freezes mid-fade;
	// config and trigger commands flow to the incoming sim because they
	// describe the new effect, not the one we're leaving. Callers unwrap the
	// transition once `done()` returns true to drop the outgoing sim.
	class EffectTransition {
		constructor(outgoing, incoming, opts) {
			opts = opts || {};
			this.outgoing = outgoing;
			this.incoming = incoming;
			this.duration = Math.max(1, (opts.durationTicks | 0) || 50);
			this.elapsed = 0;
			this._scratch = null;
		}
		step() {
			if (this.outgoing && typeof this.outgoing.step === 'function') this.outgoing.step();
			if (this.incoming && typeof this.incoming.step === 'function') this.incoming.step();
			this.elapsed++;
		}
		// Smoothstep so the alpha curve isn't a hard linear ramp.
		progress() {
			const t = clamp01(this.elapsed / this.duration);
			return t * t * (3 - 2 * t);
		}
		done() { return this.elapsed >= this.duration; }
		setConfig(cfg) {
			if (this.incoming && typeof this.incoming.setConfig === 'function') {
				this.incoming.setConfig(cfg);
			}
		}
		triggerEvent(name) {
			if (this.incoming && typeof this.incoming.triggerEvent === 'function') {
				this.incoming.triggerEvent(name);
			}
		}
		restoreSnapshot(snap) {
			if (this.incoming && typeof this.incoming.restoreSnapshot === 'function') {
				this.incoming.restoreSnapshot(snap);
			}
		}
		render(ctx, w, h, opts) {
			opts = opts || {};
			const t = this.progress();
			// Force the inner renders to skip painting their own backgrounds —
			// we paint the shared bg ourselves so both layers can be alpha-
			// composited on top without each one stomping the other.
			const transparentOpts = Object.assign({}, opts, { transparent: true });

			if (opts.transparent) {
				ctx.clearRect(0, 0, w, h);
			} else {
				ctx.fillStyle = opts.bg || '#0a0a0a';
				ctx.fillRect(0, 0, w, h);
			}

			if (!this._scratch || this._scratch.width !== w || this._scratch.height !== h) {
				this._scratch = (typeof OffscreenCanvas !== 'undefined')
					? new OffscreenCanvas(w, h)
					: document.createElement('canvas');
				this._scratch.width = w;
				this._scratch.height = h;
			}
			const sctx = this._scratch.getContext('2d');
			sctx.clearRect(0, 0, w, h);
			this.outgoing.render(sctx, w, h, transparentOpts);

			ctx.save();
			ctx.globalAlpha = t;
			this.incoming.render(ctx, w, h, transparentOpts);
			ctx.restore();

			ctx.save();
			ctx.globalAlpha = 1 - t;
			ctx.drawImage(this._scratch, 0, 0);
			ctx.restore();
		}
	}
	EffectTransition.prototype.isTransition = true;

	function subscribe(url, rain, onReady) {
		const es = new EventSource(url);
		es.addEventListener('message', (e) => {
			let cmd;
			try { cmd = JSON.parse(e.data); } catch (_) { return; }
			switch (cmd.kind) {
				case 'snapshot':
					try {
						const snap = typeof cmd.data === 'string' ? JSON.parse(cmd.data) : cmd.data;
						rain.restoreSnapshot(snap);
					} catch (err) { console.error('bad snapshot', err); }
					if (onReady) onReady();
					break;
				case 'config':
					try {
						const cfg = typeof cmd.data === 'string' ? JSON.parse(cmd.data) : cmd.data;
						rain.setConfig(cfg);
					} catch (err) { console.error('bad config', err); }
					break;
				case 'trigger':
					rain.triggerEvent(cmd.event);
					break;
			}
		});
		es.addEventListener('error', () => { /* auto-reconnect is built in */ });
		return es;
	}

	// Expose helpers on the namespace so per-effect files can pull them out
	// of api._helpers at the top of their own IIFE. positiveMod is included
	// because Burning-Trees and several procedural effects use it.
	api._helpers = { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod, ensurePixelGrid, paintPixel, paintProceduralGrid, renderPixelGridEffect };
	api.subscribe = subscribe;
	api.EffectTransition = EffectTransition;

	// Back-compat: hslToRGB used to be a top-level field on the
	// AmbienceSim export object. Keep it reachable so any external caller
	// using the old name still works.
	api.hslToRGB = hslToRGB;
})(window.AmbienceSim);
