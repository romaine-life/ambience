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
// hslToRGB, positiveMod), the ProceduralScene base class shared by the 12
// procedural-backed effects, and the SSE subscribe() helper. Each effect's
// own class lives in its own file under web/effects/. The server bundles
// sim.js with every web/effects/*.js file when serving GET /sim.js, so
// dropping a new effect file Just Works — no shared registry to edit.

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

	const PROCEDURAL_DEFAULTS = {
		aurora: {
			intro_dur: 70,
			intro_glow: 0.18,
			ending_dur: 80,
			ending_linger: 20,
			ending_glow: 0.05,
			intensity: 0.56,
			speed: 0.11,
			drift: 0.08,
			bands: 3,
			thickness: 9,
			wave_amp: 6,
			wave_freq: 0.16,
			curtain_len: 15,
			hue: 138,
			hue_sp: 26,
			sat: 0.72,
			lmin: 0.2,
			lmax: 0.74,
			brighten_p: 0,
			shift_p: 0,
			fade_p: 0,
			brighten_dur: 42,
			brighten_mult: 1.45,
			shift_dur: 64,
			shift_amt: 1.1,
			fade_dur: 58,
			fade_mult: 0.6,
		},
		'wheat-field': {
			intro_dur: 60,
			intro_breeze: 0.16,
			ending_dur: 70,
			ending_linger: 20,
			ending_sway: 0.08,
			density: 0.48,
			speed: 0.12,
			drift: 0.16,
			sway: 0.68,
			wave_freq: 0.18,
			field_top: 0.62,
			stalk_h: 18,
			layers: 3,
			hue: 46,
			hue_sp: 18,
			sat: 0.64,
			lmin: 0.3,
			lmax: 0.76,
			gust_p: 0,
			calm_p: 0,
			gust_dur: 50,
			gust_mult: 1.85,
			calm_dur: 72,
			calm_mult: 0.4,
		},
		beach: {
			intro_dur: 55,
			intro_tide: 0.18,
			ending_dur: 65,
			ending_linger: 18,
			ending_wet: 0.1,
			shoreline: 0.58,
			tide_amp: 6,
			wave_amp: 2.4,
			wave_freq: 0.18,
			speed: 0.1,
			slope: 0.16,
			foam: 0.36,
			shimmer: 0.22,
			hue: 198,
			hue_sp: 16,
			sat: 0.5,
			lmin: 0.28,
			lmax: 0.82,
			high_tide_p: 0,
			low_tide_p: 0,
			foam_burst_p: 0,
			high_tide_dur: 60,
			high_tide_push: 1.4,
			low_tide_dur: 58,
			low_tide_pull: 1.2,
			foam_burst_dur: 34,
			foam_burst_mult: 1.9,
		},
		campfire: {
			intro_dur: 45,
			intro_glow: 0.14,
			ending_dur: 60,
			ending_linger: 24,
			ending_glow: 0.08,
			flame_height: 14,
			flame_width: 10,
			flame_speed: 0.12,
			flicker: 0.72,
			ember_rate: 0.26,
			ember_speed: 0.62,
			glow: 0.54,
			hue: 24,
			hue_sp: 18,
			sat: 0.82,
			lmin: 0.32,
			lmax: 0.94,
			crackle_p: 0,
			lull_p: 0,
			crackle_dur: 36,
			crackle_mult: 1.85,
			lull_dur: 68,
			lull_mult: 0.55,
		},
		windmill: {
			intro_dur: 45,
			intro_turn: 0.12,
			ending_dur: 60,
			ending_linger: 20,
			ending_turn: 0.05,
			turn_speed: 0.08,
			blade_len: 14,
			blade_width: 1.8,
			tower_height: 20,
			tower_width: 6,
			horizon: 0.72,
			glow: 0.18,
			hue: 28,
			hue_sp: 18,
			sat: 0.42,
			lmin: 0.18,
			lmax: 0.82,
			gust_p: 0,
			lull_p: 0,
			gust_dur: 50,
			gust_mult: 1.9,
			lull_dur: 72,
			lull_mult: 0.45,
		},
		lighthouse: {
			intro_dur: 50,
			intro_beam: 0.16,
			ending_dur: 65,
			ending_linger: 18,
			ending_beam: 0.08,
			sweep_speed: 0.08,
			beam_width: 0.22,
			beam_softness: 0.42,
			tower_height: 22,
			tower_width: 6.5,
			horizon: 0.74,
			haze: 0.14,
			glow: 0.22,
			hue: 214,
			hue_sp: 18,
			sat: 0.34,
			lmin: 0.12,
			lmax: 0.84,
			bright_pass_p: 0,
			fog_thicken_p: 0,
			calm_p: 0,
			bright_pass_dur: 42,
			bright_pass_mult: 1.75,
			fog_thicken_dur: 72,
			fog_thicken_mult: 1.85,
			calm_dur: 64,
			calm_mult: 0.55,
		},
		rowboat: {
			intro_dur: 50,
			intro_drift: 0.18,
			ending_dur: 65,
			ending_linger: 18,
			ending_ripple: 0.08,
			waterline: 0.58,
			drift_speed: 0.08,
			bob_amp: 1.2,
			wave_amp: 1.6,
			wave_freq: 0.16,
			ripple: 0.24,
			reflection: 0.22,
			boat_len: 14,
			boat_height: 3.5,
			hue: 206,
			hue_sp: 16,
			sat: 0.36,
			lmin: 0.16,
			lmax: 0.82,
			wake_p: 0,
			drift_p: 0,
			calm_p: 0,
			wake_dur: 40,
			wake_mult: 1.85,
			drift_dur: 58,
			drift_push: 1.3,
			calm_dur: 72,
			calm_mult: 0.5,
		},
		underwater: {
			intro_dur: 55,
			intro_reveal: 0.14,
			ending_dur: 70,
			ending_linger: 22,
			ending_murk: 0.08,
			density: 0.28,
			rise_speed: 0.42,
			drift: 0.1,
			sway: 0.54,
			weed_height: 20,
			weed_count: 11,
			caustics: 0.3,
			depth: 0.56,
			hue: 192,
			hue_sp: 18,
			sat: 0.42,
			lmin: 0.12,
			lmax: 0.82,
			bubble_burst_p: 0,
			current_shift_p: 0,
			calm_p: 0,
			bubble_burst_dur: 38,
			bubble_burst_mult: 1.9,
			current_shift_dur: 62,
			current_shift_push: 1.2,
			calm_dur: 74,
			calm_mult: 0.55,
		},
		starfield: {
			intro_dur: 50,
			intro_density: 0.08,
			ending_dur: 60,
			ending_linger: 16,
			ending_density: 0.03,
			density: 0.22,
			speed: 0.12,
			drift: 0.04,
			layers: 3,
			size: 1,
			hue: 218,
			hue_sp: 18,
			sat: 0.18,
			lmin: 0.55,
			lmax: 0.95,
			shooting_star_p: 0,
			twinkle_burst_p: 0,
			shooting_star_dur: 26,
			shooting_star_mult: 1.8,
			twinkle_burst_dur: 42,
			twinkle_burst_mult: 1.7,
		},
		'autumn-leaves': {
			intro_dur: 55,
			intro_density: 0.12,
			ending_dur: 60,
			ending_linger: 18,
			ending_density: 0.04,
			density: 0.24,
			speed: 0.44,
			drift: 0.18,
			sway: 0.86,
			layers: 2,
			size: 1.2,
			hue: 28,
			hue_sp: 24,
			sat: 0.62,
			lmin: 0.38,
			lmax: 0.78,
			gust_p: 0,
			lull_p: 0,
			swirl_p: 0,
			gust_dur: 48,
			gust_mult: 1.9,
			lull_dur: 72,
			lull_mult: 0.35,
			swirl_dur: 52,
			swirl_pull: 1.15,
		},
		snow: {
			intro_dur: 60,
			intro_density: 0.16,
			ending_dur: 70,
			ending_linger: 22,
			ending_density: 0.08,
			density: 0.32,
			speed: 0.48,
			drift: 0.08,
			sway: 0.42,
			layers: 3,
			size: 1,
			hue: 210,
			hue_sp: 12,
			sat: 0.16,
			lmin: 0.74,
			lmax: 0.98,
			gust_p: 0,
			calm_p: 0,
			gust_dur: 55,
			gust_mult: 1.85,
			calm_dur: 80,
			calm_mult: 0.42,
		},
		volcano: {
			intro_dur: 55,
			intro_glow: 0.16,
			ending_dur: 70,
			ending_linger: 22,
			ending_glow: 0.10,
			horizon: 0.86,
			cone_height: 28,
			cone_width: 46,
			crater_width: 8,
			slope_jitter: 1.6,
			glow: 0.55,
			smoke: 0.32,
			smoke_height: 18,
			hue: 18,
			hue_sp: 16,
			sat: 0.78,
			lmin: 0.18,
			lmax: 0.92,
			eruption_p: 0,
			smolder_p: 0,
			flare_p: 0,
			eruption_dur: 80,
			eruption_height: 28,
			eruption_mult: 2.4,
			smolder_dur: 80,
			smolder_mult: 0.55,
			flare_dur: 24,
			flare_mult: 1.85,
		},
	};

	function applyProceduralDefaults(kind, cfg) {
		const base = PROCEDURAL_DEFAULTS[kind] || {};
		const c = Object.assign({}, base, cfg || {});
		switch (kind) {
			case 'wheat-field':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_breeze = clamp01(c.intro_breeze);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_sway = clamp01(c.ending_sway);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.sway <= 0) c.sway = base.sway;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.field_top <= 0) c.field_top = base.field_top;
				if (c.stalk_h <= 0) c.stalk_h = base.stalk_h;
				if (c.layers < 1) c.layers = base.layers;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'beach':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_tide = clamp01(c.intro_tide);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_wet = clamp01(c.ending_wet);
				if (c.shoreline <= 0) c.shoreline = base.shoreline;
				if (c.tide_amp <= 0) c.tide_amp = base.tide_amp;
				if (c.wave_amp <= 0) c.wave_amp = base.wave_amp;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.foam <= 0) c.foam = base.foam;
				if (c.shimmer <= 0) c.shimmer = base.shimmer;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.high_tide_dur <= 0) c.high_tide_dur = base.high_tide_dur;
				if (c.high_tide_push <= 0) c.high_tide_push = base.high_tide_push;
				if (c.low_tide_dur <= 0) c.low_tide_dur = base.low_tide_dur;
				if (c.low_tide_pull <= 0) c.low_tide_pull = base.low_tide_pull;
				if (c.foam_burst_dur <= 0) c.foam_burst_dur = base.foam_burst_dur;
				if (c.foam_burst_mult <= 0) c.foam_burst_mult = base.foam_burst_mult;
				break;
			case 'campfire':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_glow = clamp01(c.intro_glow);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_glow = clamp01(c.ending_glow);
				if (c.flame_height <= 0) c.flame_height = base.flame_height;
				if (c.flame_width <= 0) c.flame_width = base.flame_width;
				if (c.flame_speed <= 0) c.flame_speed = base.flame_speed;
				if (c.flicker <= 0) c.flicker = base.flicker;
				if (c.ember_rate <= 0) c.ember_rate = base.ember_rate;
				if (c.ember_speed <= 0) c.ember_speed = base.ember_speed;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.crackle_dur <= 0) c.crackle_dur = base.crackle_dur;
				if (c.crackle_mult <= 0) c.crackle_mult = base.crackle_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				break;
			case 'windmill':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_turn = clamp01(c.intro_turn);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_turn = clamp01(c.ending_turn);
				if (c.turn_speed <= 0) c.turn_speed = base.turn_speed;
				if (c.blade_len <= 0) c.blade_len = base.blade_len;
				if (c.blade_width <= 0) c.blade_width = base.blade_width;
				if (c.tower_height <= 0) c.tower_height = base.tower_height;
				if (c.tower_width <= 0) c.tower_width = base.tower_width;
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				break;
			case 'lighthouse':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_beam = clamp01(c.intro_beam);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_beam = clamp01(c.ending_beam);
				if (c.sweep_speed <= 0) c.sweep_speed = base.sweep_speed;
				if (c.beam_width <= 0) c.beam_width = base.beam_width;
				if (c.beam_softness <= 0) c.beam_softness = base.beam_softness;
				if (c.tower_height <= 0) c.tower_height = base.tower_height;
				if (c.tower_width <= 0) c.tower_width = base.tower_width;
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.haze <= 0) c.haze = base.haze;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.bright_pass_dur <= 0) c.bright_pass_dur = base.bright_pass_dur;
				if (c.bright_pass_mult <= 0) c.bright_pass_mult = base.bright_pass_mult;
				if (c.fog_thicken_dur <= 0) c.fog_thicken_dur = base.fog_thicken_dur;
				if (c.fog_thicken_mult <= 0) c.fog_thicken_mult = base.fog_thicken_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'rowboat':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_drift = clamp01(c.intro_drift);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_ripple = clamp01(c.ending_ripple);
				if (c.waterline <= 0) c.waterline = base.waterline;
				if (c.drift_speed <= 0) c.drift_speed = base.drift_speed;
				if (c.bob_amp <= 0) c.bob_amp = base.bob_amp;
				if (c.wave_amp <= 0) c.wave_amp = base.wave_amp;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.ripple <= 0) c.ripple = base.ripple;
				if (c.reflection <= 0) c.reflection = base.reflection;
				if (c.boat_len <= 0) c.boat_len = base.boat_len;
				if (c.boat_height <= 0) c.boat_height = base.boat_height;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.wake_dur <= 0) c.wake_dur = base.wake_dur;
				if (c.wake_mult <= 0) c.wake_mult = base.wake_mult;
				if (c.drift_dur <= 0) c.drift_dur = base.drift_dur;
				if (c.drift_push <= 0) c.drift_push = base.drift_push;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'underwater':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_reveal = clamp01(c.intro_reveal);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_murk = clamp01(c.ending_murk);
				if (c.density <= 0) c.density = base.density;
				if (c.rise_speed <= 0) c.rise_speed = base.rise_speed;
				if (c.sway <= 0) c.sway = base.sway;
				if (c.weed_height <= 0) c.weed_height = base.weed_height;
				if (c.weed_count < 1) c.weed_count = base.weed_count;
				if (c.caustics <= 0) c.caustics = base.caustics;
				if (c.depth <= 0) c.depth = base.depth;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.bubble_burst_dur <= 0) c.bubble_burst_dur = base.bubble_burst_dur;
				if (c.bubble_burst_mult <= 0) c.bubble_burst_mult = base.bubble_burst_mult;
				if (c.current_shift_dur <= 0) c.current_shift_dur = base.current_shift_dur;
				if (c.current_shift_push <= 0) c.current_shift_push = base.current_shift_push;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'aurora':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_glow = clamp01(c.intro_glow);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_glow = clamp01(c.ending_glow);
				if (c.intensity <= 0) c.intensity = base.intensity;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.bands < 1) c.bands = base.bands;
				if (c.thickness <= 0) c.thickness = base.thickness;
				if (c.wave_amp <= 0) c.wave_amp = base.wave_amp;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.curtain_len <= 0) c.curtain_len = base.curtain_len;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.brighten_dur <= 0) c.brighten_dur = base.brighten_dur;
				if (c.brighten_mult <= 0) c.brighten_mult = base.brighten_mult;
				if (c.shift_dur <= 0) c.shift_dur = base.shift_dur;
				if (c.shift_amt <= 0) c.shift_amt = base.shift_amt;
				if (c.fade_dur <= 0) c.fade_dur = base.fade_dur;
				if (c.fade_mult <= 0) c.fade_mult = base.fade_mult;
				break;
			case 'starfield':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_density = clamp01(c.intro_density);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_density = clamp01(c.ending_density);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.layers < 1) c.layers = base.layers;
				if (c.size <= 0) c.size = base.size;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.shooting_star_dur <= 0) c.shooting_star_dur = base.shooting_star_dur;
				if (c.shooting_star_mult <= 0) c.shooting_star_mult = base.shooting_star_mult;
				if (c.twinkle_burst_dur <= 0) c.twinkle_burst_dur = base.twinkle_burst_dur;
				if (c.twinkle_burst_mult <= 0) c.twinkle_burst_mult = base.twinkle_burst_mult;
				break;
			case 'autumn-leaves':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_density = clamp01(c.intro_density);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_density = clamp01(c.ending_density);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.layers < 1) c.layers = base.layers;
				if (c.size <= 0) c.size = base.size;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				if (c.swirl_dur <= 0) c.swirl_dur = base.swirl_dur;
				if (c.swirl_pull <= 0) c.swirl_pull = base.swirl_pull;
				break;
			case 'snow':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_density = clamp01(c.intro_density);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_density = clamp01(c.ending_density);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.layers < 1) c.layers = base.layers;
				if (c.size <= 0) c.size = base.size;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'volcano':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_glow = clamp01(c.intro_glow);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_glow = clamp01(c.ending_glow);
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.cone_height <= 0) c.cone_height = base.cone_height;
				if (c.cone_width <= 0) c.cone_width = base.cone_width;
				if (c.crater_width <= 0) c.crater_width = base.crater_width;
				if (c.slope_jitter < 0) c.slope_jitter = 0;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.smoke < 0) c.smoke = 0;
				if (c.smoke_height <= 0) c.smoke_height = base.smoke_height;
				if (c.hue < 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.eruption_dur <= 0) c.eruption_dur = base.eruption_dur;
				if (c.eruption_height <= 0) c.eruption_height = base.eruption_height;
				if (c.eruption_mult <= 0) c.eruption_mult = base.eruption_mult;
				if (c.smolder_dur <= 0) c.smolder_dur = base.smolder_dur;
				if (c.smolder_mult <= 0) c.smolder_mult = base.smolder_mult;
				if (c.flare_dur <= 0) c.flare_dur = base.flare_dur;
				if (c.flare_mult <= 0) c.flare_mult = base.flare_mult;
				break;
		}
		return c;
	}

	class ProceduralScene {
		constructor(kind, w, h, cfg, seed) {
			this.kind = kind;
			this.w = w;
			this.h = h;
			this.seed = Number(seed || Date.now());
			this.tick = 0;
			this.timers = {};
			this.values = {};
			this.cfg = applyProceduralDefaults(kind, cfg);
		}

		setConfig(cfg) {
			this.cfg = applyProceduralDefaults(this.kind, Object.assign({}, this.cfg, cfg));
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.timers = Object.assign({}, state.timers || {});
			this.values = Object.assign({}, state.values || {});
			if (typeof snap.seed === 'number') this.seed = snap.seed;
			if (snap.gridW > 0 && snap.gridH > 0) {
				this.w = snap.gridW;
				this.h = snap.gridH;
			}
		}

		_eventRng(salt) {
			return makeRNG(((this.seed >>> 0) ^ (((this.tick + salt) * 2654435761) >>> 0)) >>> 0);
		}

		triggerEvent(name) {
			switch (this.kind) {
				case 'wheat-field':
					return this._triggerWheatField(name);
				case 'beach':
					return this._triggerBeach(name);
				case 'campfire':
					return this._triggerCampfire(name);
				case 'windmill':
					return this._triggerWindmill(name);
				case 'lighthouse':
					return this._triggerLighthouse(name);
				case 'rowboat':
					return this._triggerRowboat(name);
				case 'underwater':
					return this._triggerUnderwater(name);
				case 'aurora':
					return this._triggerAurora(name);
				case 'starfield':
					return this._triggerStarfield(name);
				case 'autumn-leaves':
					return this._triggerAutumnLeaves(name);
				case 'snow':
					return this._triggerSnow(name);
				case 'volcano':
					return this._triggerVolcano(name);
			}
			return false;
		}

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (this.kind === 'snow' && (!this.timers.gust || this.timers.gust <= 0)) {
				this.values.gust_push = 0;
			}
			if (this.kind === 'autumn-leaves') {
				if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_push = 0;
				if (!this.timers.swirl || this.timers.swirl <= 0) this.values.swirl_spin = 0;
			}
			if (this.kind === 'aurora') {
				if (!this.timers.brighten || this.timers.brighten <= 0) this.values.brighten_gain = 0;
				if (!this.timers.shift || this.timers.shift <= 0) {
					this.values.shift_push = 0;
					this.values.shift_seed = 0;
				}
			}
			if (this.kind === 'wheat-field' && (!this.timers.gust || this.timers.gust <= 0)) {
				this.values.gust_push = 0;
			}
			if (this.kind === 'beach') {
				if ((!this.timers['high-tide'] || this.timers['high-tide'] <= 0) && (!this.timers['low-tide'] || this.timers['low-tide'] <= 0)) {
					this.values.tide_bias = 0;
				}
				if (!this.timers['foam-burst'] || this.timers['foam-burst'] <= 0) {
					this.values.foam_gain = 1;
				}
			}
			if (this.kind === 'campfire' && (!this.timers.crackle || this.timers.crackle <= 0)) {
				this.values.crackle_gain = 1;
			}
			if (this.kind === 'windmill' && (!this.timers.gust || this.timers.gust <= 0)) {
				this.values.gust_gain = 1;
			}
			if (this.kind === 'lighthouse') {
				if (!this.timers['bright-pass'] || this.timers['bright-pass'] <= 0) {
					this.values.bright_gain = 1;
				}
				if (!this.timers['fog-thicken'] || this.timers['fog-thicken'] <= 0) {
					this.values.fog_gain = 1;
				}
			}
			if (this.kind === 'rowboat') {
				if (!this.timers.wake || this.timers.wake <= 0) {
					this.values.wake_gain = 1;
				}
				if (!this.timers.drift || this.timers.drift <= 0) {
					this.values.drift_push = 0;
				}
			}
			if (this.kind === 'underwater') {
				if (!this.timers['bubble-burst'] || this.timers['bubble-burst'] <= 0) {
					this.values.bubble_gain = 1;
				}
				if (!this.timers['current-shift'] || this.timers['current-shift'] <= 0) {
					this.values.current_push = 0;
				}
			}
			if (this.kind === 'volcano') {
				if (!this.timers.eruption || this.timers.eruption <= 0) {
					this.values.eruption_gain = 1;
				}
				if (!this.timers.flare || this.timers.flare <= 0) {
					this.values.flare_gain = 1;
				}
			}
		}

		render(ctx, canvasW, canvasH, opts) {
			switch (this.kind) {
				case 'wheat-field':
					return this._renderWheatField(ctx, canvasW, canvasH, opts);
				case 'beach':
					return this._renderBeach(ctx, canvasW, canvasH, opts);
				case 'campfire':
					return this._renderCampfire(ctx, canvasW, canvasH, opts);
				case 'windmill':
					return this._renderWindmill(ctx, canvasW, canvasH, opts);
				case 'lighthouse':
					return this._renderLighthouse(ctx, canvasW, canvasH, opts);
				case 'rowboat':
					return this._renderRowboat(ctx, canvasW, canvasH, opts);
				case 'underwater':
					return this._renderUnderwater(ctx, canvasW, canvasH, opts);
				case 'aurora':
					return this._renderAurora(ctx, canvasW, canvasH, opts);
				case 'starfield':
					return this._renderStarfield(ctx, canvasW, canvasH, opts);
				case 'autumn-leaves':
					return this._renderAutumnLeaves(ctx, canvasW, canvasH, opts);
				case 'snow':
					return this._renderSnow(ctx, canvasW, canvasH, opts);
				case 'volcano':
					return this._renderVolcano(ctx, canvasW, canvasH, opts);
			}
		}

		_hash(index) {
			const x = Math.sin((this.seed * 0.000001 + index * 12.9898) * 43758.5453);
			return x - Math.floor(x);
		}

		_phaseProgress(total, left) {
			if (left <= 1 || total <= 1) return 1;
			const elapsed = total - left;
			if (elapsed <= 0) return 0;
			return clamp01(elapsed / Math.max(1, total - 1));
		}

		_fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, w, h, color, alpha) {
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), Math.max(1, Math.ceil(w * sx || ceilSx)), Math.max(1, Math.ceil(h * sy || ceilSy)));
			ctx.globalAlpha = 1;
		}

		_triggerSnow(name) {
			const rng = this._eventRng(name.length + 17);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.5 ? -1 : 1) * this.cfg.gust_mult * (0.45 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.values.gust_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerAutumnLeaves(name) {
			const rng = this._eventRng(name.length + 29);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.5 ? -1 : 1) * this.cfg.gust_mult * (0.5 + rng() * 0.7);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'swirl':
					this.timers.swirl = jitterInt(rng, this.cfg.swirl_dur, 0.3);
					this.values.swirl_spin = (rng() < 0.5 ? -1 : 1) * this.cfg.swirl_pull * (0.65 + rng() * 0.45);
					this.values.swirl_row = Math.max(8, this.h / 3) + rng() * Math.max(1, this.h / 2);
					this.values.swirl_col = rng() * this.w;
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.swirl = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.values.swirl_spin = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.swirl = 0;
					this.values.gust_push = 0;
					this.values.swirl_spin = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerWheatField(name) {
			const rng = this._eventRng(name.length + 47);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.35 ? -1 : 1) * this.cfg.gust_mult * (0.55 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.values.gust_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerBeach(name) {
			const rng = this._eventRng(name.length + 61);
			switch (name) {
				case 'high-tide':
					this.timers['high-tide'] = jitterInt(rng, this.cfg.high_tide_dur, 0.3);
					this.timers['low-tide'] = 0;
					this.values.tide_bias = this.cfg.high_tide_push * (0.65 + rng() * 0.55);
					return true;
				case 'low-tide':
					this.timers['low-tide'] = jitterInt(rng, this.cfg.low_tide_dur, 0.3);
					this.timers['high-tide'] = 0;
					this.values.tide_bias = -this.cfg.low_tide_pull * (0.65 + rng() * 0.55);
					return true;
				case 'foam-burst':
					this.timers['foam-burst'] = jitterInt(rng, this.cfg.foam_burst_dur, 0.3);
					this.values.foam_gain = this.cfg.foam_burst_mult * (0.85 + rng() * 0.35);
					return true;
				case 'intro':
					this.timers['high-tide'] = 0;
					this.timers['low-tide'] = 0;
					this.timers['foam-burst'] = 0;
					this.timers.ending = 0;
					this.values.tide_bias = 0;
					this.values.foam_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['high-tide'] = 0;
					this.timers['low-tide'] = 0;
					this.timers['foam-burst'] = 0;
					this.values.tide_bias = 0;
					this.values.foam_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerCampfire(name) {
			const rng = this._eventRng(name.length + 67);
			switch (name) {
				case 'crackle':
					this.timers.crackle = jitterInt(rng, this.cfg.crackle_dur, 0.3);
					this.values.crackle_gain = this.cfg.crackle_mult * (0.75 + rng() * 0.5);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'intro':
					this.timers.crackle = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.crackle_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.crackle = 0;
					this.timers.lull = 0;
					this.values.crackle_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerWindmill(name) {
			const rng = this._eventRng(name.length + 71);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_gain = this.cfg.gust_mult * (0.75 + rng() * 0.45);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.gust_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.values.gust_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerLighthouse(name) {
			const rng = this._eventRng(name.length + 79);
			switch (name) {
				case 'bright-pass':
					this.timers['bright-pass'] = jitterInt(rng, this.cfg.bright_pass_dur, 0.3);
					this.values.bright_gain = this.cfg.bright_pass_mult * (0.8 + rng() * 0.4);
					return true;
				case 'fog-thicken':
					this.timers['fog-thicken'] = jitterInt(rng, this.cfg.fog_thicken_dur, 0.3);
					this.timers.calm = 0;
					this.values.fog_gain = this.cfg.fog_thicken_mult * (0.8 + rng() * 0.45);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.timers['fog-thicken'] = 0;
					this.values.fog_gain = 1;
					return true;
				case 'intro':
					this.timers['bright-pass'] = 0;
					this.timers['fog-thicken'] = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.bright_gain = 1;
					this.values.fog_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['bright-pass'] = 0;
					this.timers['fog-thicken'] = 0;
					this.timers.calm = 0;
					this.values.bright_gain = 1;
					this.values.fog_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerRowboat(name) {
			const rng = this._eventRng(name.length + 83);
			switch (name) {
				case 'wake':
					this.timers.wake = jitterInt(rng, this.cfg.wake_dur, 0.3);
					this.values.wake_gain = this.cfg.wake_mult * (0.8 + rng() * 0.45);
					return true;
				case 'drift':
					this.timers.drift = jitterInt(rng, this.cfg.drift_dur, 0.3);
					this.values.drift_push = (rng() < 0.5 ? -1 : 1) * this.cfg.drift_push * (0.65 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.wake = 0;
					this.timers.drift = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.wake_gain = 1;
					this.values.drift_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.wake = 0;
					this.timers.drift = 0;
					this.timers.calm = 0;
					this.values.wake_gain = 1;
					this.values.drift_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerUnderwater(name) {
			const rng = this._eventRng(name.length + 89);
			switch (name) {
				case 'bubble-burst':
					this.timers['bubble-burst'] = jitterInt(rng, this.cfg.bubble_burst_dur, 0.3);
					this.values.bubble_gain = this.cfg.bubble_burst_mult * (0.8 + rng() * 0.45);
					return true;
				case 'current-shift':
					this.timers['current-shift'] = jitterInt(rng, this.cfg.current_shift_dur, 0.3);
					this.timers.calm = 0;
					this.values.current_push = (rng() < 0.5 ? -1 : 1) * this.cfg.current_shift_push * (0.55 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.timers['current-shift'] = 0;
					this.values.current_push = 0;
					return true;
				case 'intro':
					this.timers['bubble-burst'] = 0;
					this.timers['current-shift'] = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.bubble_gain = 1;
					this.values.current_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['bubble-burst'] = 0;
					this.timers['current-shift'] = 0;
					this.timers.calm = 0;
					this.values.bubble_gain = 1;
					this.values.current_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerAurora(name) {
			const rng = this._eventRng(name.length + 53);
			switch (name) {
				case 'brighten':
					this.timers.brighten = jitterInt(rng, this.cfg.brighten_dur, 0.3);
					this.values.brighten_gain = this.cfg.brighten_mult * (0.85 + rng() * 0.35);
					return true;
				case 'shift':
					this.timers.shift = jitterInt(rng, this.cfg.shift_dur, 0.3);
					this.values.shift_push = (rng() < 0.5 ? -1 : 1) * this.cfg.shift_amt * (0.55 + rng() * 0.55);
					this.values.shift_seed = rng() * Math.PI * 2;
					return true;
				case 'fade':
					this.timers.fade = jitterInt(rng, this.cfg.fade_dur, 0.3);
					return true;
				case 'intro':
					this.timers.brighten = 0;
					this.timers.shift = 0;
					this.timers.fade = 0;
					this.timers.ending = 0;
					this.values.brighten_gain = 0;
					this.values.shift_push = 0;
					this.values.shift_seed = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.brighten = 0;
					this.timers.shift = 0;
					this.timers.fade = 0;
					this.values.brighten_gain = 0;
					this.values.shift_push = 0;
					this.values.shift_seed = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerStarfield(name) {
			const rng = this._eventRng(name.length + 41);
			switch (name) {
				case 'shooting-star':
					this.timers['shooting-star'] = jitterInt(rng, this.cfg.shooting_star_dur, 0.3);
					this.values['shooting-star_total'] = this.timers['shooting-star'];
					this.values.shooting_dir = rng() < 0.5 ? -1 : 1;
					this.values.shooting_row = 6 + rng() * Math.max(4, this.h / 3);
					this.values.shooting_start = rng() * this.w;
					return true;
				case 'twinkle-burst':
					this.timers['twinkle-burst'] = jitterInt(rng, this.cfg.twinkle_burst_dur, 0.3);
					return true;
				case 'intro':
					this.timers.ending = 0;
					this.timers['shooting-star'] = 0;
					this.timers['twinkle-burst'] = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['shooting-star'] = 0;
					this.timers['twinkle-burst'] = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerVolcano(name) {
			const rng = this._eventRng(name.length + 97);
			switch (name) {
				case 'eruption':
					this.timers.eruption = jitterInt(rng, this.cfg.eruption_dur, 0.3);
					this.timers.smolder = 0;
					this.values.eruption_gain = this.cfg.eruption_mult * (0.8 + rng() * 0.45);
					this.values.eruption_seed = rng() * 1024;
					return true;
				case 'smolder':
					this.timers.smolder = jitterInt(rng, this.cfg.smolder_dur, 0.3);
					this.timers.eruption = 0;
					this.values.eruption_gain = 1;
					return true;
				case 'flare':
					this.timers.flare = jitterInt(rng, this.cfg.flare_dur, 0.3);
					this.values.flare_gain = this.cfg.flare_mult * (0.85 + rng() * 0.3);
					return true;
				case 'intro':
					this.timers.eruption = 0;
					this.timers.smolder = 0;
					this.timers.flare = 0;
					this.timers.ending = 0;
					this.values.eruption_gain = 1;
					this.values.flare_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.eruption = 0;
					this.timers.smolder = 0;
					this.timers.flare = 0;
					this.values.eruption_gain = 1;
					this.values.flare_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_densityLevelSnow() {
			let level = this.cfg.density;
			if (this.timers.gust > 0) level *= 1.28;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.02, level);
		}

		_densityLevelAutumnLeaves() {
			let level = this.cfg.density;
			if (this.timers.gust > 0) level *= 1.22;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.015, level);
		}

		_densityLevelStarfield() {
			let level = this.cfg.density;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.02, level);
		}

		_intensityLevelAurora() {
			let level = this.cfg.intensity;
			if (this.timers.brighten > 0) level *= this.values.brighten_gain || this.cfg.brighten_mult;
			if (this.timers.fade > 0) level *= this.cfg.fade_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_glow + (1 - this.cfg.intro_glow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_glow) * progress;
			}
			return Math.max(0.02, level);
		}

		_motionLevelWheatField() {
			let level = this.cfg.sway;
			if (this.timers.gust > 0) level *= 1 + Math.abs(this.values.gust_push || this.cfg.gust_mult) * 0.35;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_breeze + (1 - this.cfg.intro_breeze) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_sway) * progress;
			}
			return Math.max(0.05, level);
		}

		_tideLevelBeach() {
			let level = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_tide + (1 - this.cfg.intro_tide) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_wet) * progress;
			}
			return Math.max(0.05, level);
		}

		_flameLevelCampfire() {
			let level = 1;
			if (this.timers.crackle > 0) level *= this.values.crackle_gain || this.cfg.crackle_mult;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_glow + (1 - this.cfg.intro_glow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_glow) * progress;
			}
			return Math.max(0.05, level);
		}

		_pressureLevelVolcano() {
			let level = 1;
			if (this.timers.eruption > 0) level *= this.values.eruption_gain || this.cfg.eruption_mult;
			if (this.timers.smolder > 0) level *= this.cfg.smolder_mult;
			if (this.timers.flare > 0) level *= 1 + ((this.values.flare_gain || this.cfg.flare_mult) - 1) * 0.5;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_glow + (1 - this.cfg.intro_glow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_glow) * progress;
			}
			return Math.max(0.05, level);
		}

		_rotationLevelWindmill() {
			let level = 1;
			if (this.timers.gust > 0) level *= this.values.gust_gain || this.cfg.gust_mult;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_turn + (1 - this.cfg.intro_turn) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_turn) * progress;
			}
			return Math.max(0.03, level);
		}

		_beamLevelLighthouse() {
			let level = 1;
			if (this.timers['bright-pass'] > 0) level *= this.values.bright_gain || this.cfg.bright_pass_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_beam + (1 - this.cfg.intro_beam) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_beam) * progress;
			}
			return Math.max(0.05, level);
		}

		_rippleLevelRowboat() {
			let level = 1;
			if (this.timers.wake > 0) level *= this.values.wake_gain || this.cfg.wake_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_drift + (1 - this.cfg.intro_drift) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_ripple) * progress;
			}
			if (this.timers.drift > 0) {
				level *= 1 + Math.abs(this.values.drift_push || this.cfg.drift_push) * 0.22;
			}
			return Math.max(0.04, level);
		}

		_sceneLevelUnderwater() {
			let level = 1;
			if (this.timers['bubble-burst'] > 0) level *= this.values.bubble_gain || this.cfg.bubble_burst_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_reveal + (1 - this.cfg.intro_reveal) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_murk) * progress;
			}
			if (this.timers['current-shift'] > 0) {
				level *= 1 + Math.abs(this.values.current_push || this.cfg.current_shift_push) * 0.18;
			}
			return Math.max(0.04, level);
		}

		_renderSnow(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#09111d');
				sky.addColorStop(0.58, '#102033');
				sky.addColorStop(1, '#17263a');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.8);

			const moonX = canvasW * (0.16 + this._hash(401) * 0.18);
			const moonY = canvasH * (0.14 + this._hash(402) * 0.08);
			const moonR = Math.max(12, Math.min(canvasW, canvasH) * 0.035);
			const moon = ctx.createRadialGradient(moonX, moonY, 0, moonX, moonY, moonR * 2.6);
			moon.addColorStop(0, 'rgba(225, 234, 255, 0.18)');
			moon.addColorStop(1, 'rgba(225, 234, 255, 0)');
			ctx.fillStyle = moon;
			ctx.fillRect(0, 0, canvasW, canvasH);

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const hue = ((this.cfg.hue - 8) % 360 + 360) % 360;
				const light = clamp01(0.06 + 0.2 * ratio);
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.55), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
			}

			const treeCount = 13;
			for (let i = 0; i < treeCount; i++) {
				const center = Math.floor((i + 0.5) * this.w / treeCount + (this._hash(500 + i) - 0.5) * 6);
				const trunkH = 1 + Math.floor(this._hash(530 + i) * 2);
				const crownH = 8 + Math.floor(this._hash(560 + i) * 9);
				const maxHalf = 2 + Math.floor(this._hash(590 + i) * 4);
				const hue = ((this.cfg.hue - 26) % 360 + 360) % 360;
				const treeColor = hslToRGB(hue, clamp01(this.cfg.sat * 0.6), 0.18 + this._hash(620 + i) * 0.08);
				for (let row = 0; row < crownH; row++) {
					const width = Math.max(1, maxHalf - Math.floor(row / 2));
					const y = groundRow - crownH + row;
					for (let dx = -width; dx <= width; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center + dx, y, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
					}
				}
				for (let row = 0; row < trunkH; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center, groundRow + row - trunkH, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
				}
			}

			const density = this._densityLevelSnow();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(8, Math.round(this.w * density * (0.35 + layerRatio * 0.8)));
				const baseSpeed = this.cfg.speed * (0.4 + layerRatio * 0.85);
				const drift = this.cfg.drift * (0.35 + layerRatio * 0.65) + (this.values.gust_push || 0) * 0.035 * (0.5 + layerRatio * 0.65);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1000 + i;
					const baseX = this._hash(1000 + idx) * this.w;
					const baseY = this._hash(2000 + idx) * Math.max(1, groundRow - 2);
					const sway = (this._hash(3000 + idx) * 2 - 1) * this.cfg.sway * (1.4 + layerRatio * 2.4);
					const fall = baseY + this.tick * baseSpeed * (0.75 + this._hash(4000 + idx) * 0.5);
					const row = positiveMod(fall, Math.max(1, groundRow - 2));
					const col = positiveMod(baseX + this.tick * drift + Math.sin(this.tick * 0.035 + idx * 0.19) * sway, this.w);
					const hue = ((this.cfg.hue + (this._hash(5000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.35 + 0.55 * (0.3 + layerRatio * 0.7)));
					const alpha = clamp01(0.35 + 0.55 * (0.25 + layerRatio * 0.75));
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}
		}

		_renderAutumnLeaves(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#20170f');
				sky.addColorStop(0.52, '#58422b');
				sky.addColorStop(1, '#7c6042');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.82);

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const ground = hslToRGB(38, 0.35, 0.1 + ratio * 0.16);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${ground.r},${ground.g},${ground.b})`, 1);
			}

			for (let x = 0; x < this.w; x++) {
				const canopyDepth = 4 + Math.floor(this._hash(6100 + x) * 6);
				const shade = hslToRGB(24 + this._hash(6200 + x) * 18, 0.45, 0.14 + this._hash(6300 + x) * 0.08);
				for (let y = 0; y < canopyDepth; y++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${shade.r},${shade.g},${shade.b})`, 0.75);
				}
			}

			const density = this._densityLevelAutumnLeaves();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(6, Math.round(this.w * density * (0.28 + layerRatio * 0.62)));
				const baseSpeed = this.cfg.speed * (0.34 + layerRatio * 0.72);
				const drift = this.cfg.drift * (0.45 + layerRatio * 0.65) + (this.values.gust_push || 0) * 0.04 * (0.5 + layerRatio * 0.7);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio * 0.8));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1000 + i;
					const baseX = this._hash(7000 + idx) * this.w;
					const baseY = this._hash(8000 + idx) * Math.max(1, groundRow - 2);
					const flutter = (this._hash(9000 + idx) * 2 - 1) * this.cfg.sway * (2.4 + layerRatio * 2.8);
					let row = positiveMod(baseY + this.tick * baseSpeed * (0.7 + this._hash(10000 + idx) * 0.55), Math.max(1, groundRow - 2));
					let col = positiveMod(baseX + this.tick * drift + Math.sin(this.tick * 0.04 + idx * 0.23) * flutter, this.w);
					if (this.timers.swirl > 0) {
						const sr = this.values.swirl_row || this.h * 0.55;
						const sc = this.values.swirl_col || this.w * 0.5;
						const angle = Math.atan2(row - sr, col - sc) + (this.values.swirl_spin || 0) * 0.015;
						const radius = Math.hypot(col - sc, row - sr);
						col = positiveMod(sc + Math.cos(angle) * radius, this.w);
						row = Math.max(0, Math.min(groundRow - 2, sr + Math.sin(angle) * radius * 0.94));
					}
					const hue = ((this.cfg.hue + (this._hash(11000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + this._hash(12000 + idx) * 0.7));
					const alpha = clamp01(0.4 + layerRatio * 0.45);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if ((idx + this.tick) % 3 === 0) {
						const accent = hslToRGB((hue + 12) % 360, clamp01(this.cfg.sat * 0.85), clamp01(light * 1.08));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col) + (this._hash(13000 + idx) < 0.5 ? 1 : 0), Math.round(row), 1, size > 1 ? 1 : 0.8, `rgb(${accent.r},${accent.g},${accent.b})`, alpha * 0.8);
					}
				}
			}
		}

		_renderStarfield(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#050912');
				sky.addColorStop(0.6, '#090f20');
				sky.addColorStop(1, '#0b1128');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const density = this._densityLevelStarfield();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			const burst = this.timers['twinkle-burst'] > 0 ? this.cfg.twinkle_burst_mult : 1;

			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(10, Math.round(this.w * density * (0.4 + layerRatio * 1.2)));
				const speed = this.cfg.speed * (0.18 + layerRatio * 0.82);
				const drift = this.cfg.drift * (0.25 + layerRatio * 0.9);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1400 + i;
					const baseX = this._hash(15000 + idx) * this.w;
					const baseY = this._hash(16000 + idx) * this.h;
					const col = positiveMod(baseX + this.tick * drift * speed * 2, this.w);
					const row = baseY;
					const hue = ((this.cfg.hue + (this._hash(17000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const twinkle = 0.4 + 0.6 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * (0.02 + this._hash(18000 + idx) * 0.03) + idx), 2);
					const light = clamp01((this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + layerRatio * 0.7)) * twinkle * burst);
					const alpha = clamp01(0.35 + 0.25 * layerRatio + 0.25 * twinkle);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			if (this.timers['shooting-star'] > 0) {
				const total = Math.max(1, this.values['shooting-star_total'] || this.cfg.shooting_star_dur);
				const progress = 1 - (this.timers['shooting-star'] / total);
				const dir = this.values.shooting_dir || 1;
				const row = this.values.shooting_row || this.h * 0.25;
				const start = this.values.shooting_start || this.w * 0.25;
				const head = positiveMod(start + dir * progress * this.w * 0.6, this.w);
				for (let i = 0; i < 7; i++) {
					const fade = 1 - i / 7;
					const x = Math.round(head - dir * i * 1.5);
					const y = Math.round(row + i * 0.6);
					const light = clamp01(this.cfg.lmax * this.cfg.shooting_star_mult * fade * 0.55);
					const color = hslToRGB(this.cfg.hue - 8, clamp01(this.cfg.sat * 0.9), light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1 + (i === 0 ? 1 : 0), 1, `rgb(${color.r},${color.g},${color.b})`, fade);
				}
			}
		}

		_renderWheatField(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#16213f');
				sky.addColorStop(0.56, '#556785');
				sky.addColorStop(1, '#d2ad66');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const motion = this._motionLevelWheatField();
			const density = Math.max(0.08, this.cfg.density);
			const layers = Math.max(1, Math.round(this.cfg.layers));
			const gustPush = this.values.gust_push || 0;
			const fieldBase = Math.floor(this.h * this.cfg.field_top);

			const sunX = canvasW * (0.16 + this._hash(21000) * 0.18);
			const sunY = canvasH * (0.2 + this._hash(21001) * 0.06);
			const sunR = Math.max(18, Math.min(canvasW, canvasH) * 0.06);
			const sun = ctx.createRadialGradient(sunX, sunY, 0, sunX, sunY, sunR * 2.8);
			sun.addColorStop(0, 'rgba(255, 228, 153, 0.2)');
			sun.addColorStop(1, 'rgba(255, 228, 153, 0)');
			ctx.fillStyle = sun;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const hillColor = hslToRGB(38, 0.18, 0.3);
			const hillPoints = [];
			for (let i = 0; i <= 8; i++) {
				hillPoints.push(Math.floor(this.h * 0.56) - Math.floor(this._hash(21100 + i) * 5) - Math.floor((0.5 + 0.5 * Math.sin(i * 0.9 + this._hash(21200 + i) * 3)) * 5));
			}
			ctx.fillStyle = `rgb(${hillColor.r},${hillColor.g},${hillColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * 8;
				const idx = Math.min(7, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const y = hillPoints[idx] + (hillPoints[idx + 1] - hillPoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(y * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const groundGrad = ctx.createLinearGradient(0, Math.floor(fieldBase * sy), 0, canvasH);
			groundGrad.addColorStop(0, '#b28d45');
			groundGrad.addColorStop(0.45, '#be9c56');
			groundGrad.addColorStop(1, '#8e6e32');
			ctx.fillStyle = groundGrad;
			ctx.fillRect(0, Math.floor(fieldBase * sy), canvasW, canvasH - Math.floor(fieldBase * sy));

			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const amp = motion * (2.4 + layerRatio * 4.8);
				const speed = this.cfg.speed * (0.4 + layerRatio * 0.65);
				const drift = this.cfg.drift * (0.25 + layerRatio * 0.75) + gustPush * 0.04 * (0.5 + layerRatio * 0.5);
				const topBase = fieldBase - this.cfg.stalk_h * (0.28 + layerRatio * 0.46);
				const tipChance = clamp01(density * (0.4 + layerRatio * 0.22));
				for (let x = 0; x < this.w;) {
					const clumpSeed = layer * 4000 + x;
					const width = Math.max(1, Math.min(4, Math.round(1 + this._hash(21700 + clumpSeed) * (1.2 + layerRatio * 2.4))));
					const sampleX = Math.min(this.w - 1, x + width * 0.5);
					const idx = layer * 2000 + sampleX;
					const wave = Math.sin(sampleX * this.cfg.wave_freq * (0.85 + layerRatio * 0.25) + this.tick * speed + layer * 1.7);
					const subWave = Math.sin(sampleX * this.cfg.wave_freq * 0.42 - this.tick * speed * 0.62 + layer * 2.3);
					const lean = wave * amp + subWave * amp * 0.32 + Math.sin(this.tick * 0.012 + sampleX * 0.05) * drift * 3.2;
					const top = Math.max(0, Math.min(this.h - 2, topBase + lean + this._hash(21300 + idx) * 2));
					const depth = Math.max(6, Math.round(this.cfg.stalk_h * (0.48 + layerRatio * 0.42)));
					const hue = ((this.cfg.hue + (this._hash(21400 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.18 + layerRatio * 0.6));
					const alpha = clamp01(0.34 + layerRatio * 0.24);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, Math.round(top), width, depth, `rgb(${color.r},${color.g},${color.b})`, alpha);
					const shadow = hslToRGB(hue, clamp01(this.cfg.sat * 0.72), clamp01(light * 0.76));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, Math.round(top), width, Math.max(1, Math.round(depth * 0.28)), `rgb(${shadow.r},${shadow.g},${shadow.b})`, clamp01(alpha * 0.55));

					if (this._hash(21500 + idx) < tipChance) {
						const tipHeight = 1 + Math.floor(this._hash(21600 + idx) * (1 + layerRatio * 3));
						const tipX = Math.max(0, Math.min(this.w - 1, Math.round(x + width * 0.5 + lean * 0.18)));
						const accent = hslToRGB((hue + 4) % 360, clamp01(this.cfg.sat * 0.82), clamp01(light * 1.08));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, tipX, Math.round(top) - tipHeight + 1, 1, tipHeight, `rgb(${accent.r},${accent.g},${accent.b})`, clamp01(alpha * 0.9));
					}
					x += width;
				}
			}
		}

		_renderLighthouse(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue + 358) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.92));
				const skyMid = hslToRGB(this.cfg.hue, this.cfg.sat, clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const skyLow = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.6 + 360) % 360, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.46));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.min(this.h - 10, Math.floor(this.h * this.cfg.horizon)));
			const towerX = Math.floor(this.w * 0.18);
			const towerH = Math.max(10, Math.round(this.cfg.tower_height));
			const towerW = Math.max(3, Math.round(this.cfg.tower_width));
			const beamLevel = this._beamLevelLighthouse();
			const fogLevel = clamp01(this.cfg.haze * (this.timers['fog-thicken'] > 0 ? this.values.fog_gain || this.cfg.fog_thicken_mult : 1) * (this.timers.calm > 0 ? 0.82 : 1));
			const beamAngle = -0.26 + Math.sin(this.tick * this.cfg.sweep_speed * 0.06) * 0.78;
			const beamWidth = this.cfg.beam_width * (1 + fogLevel * 0.9);
			const beamSoftness = clamp01(this.cfg.beam_softness * (1 + fogLevel * 0.35));

			const seaTop = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.12));
			const seaLow = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin * 0.8));
			const sea = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			sea.addColorStop(0, `rgb(${seaTop.r},${seaTop.g},${seaTop.b})`);
			sea.addColorStop(1, `rgb(${seaLow.r},${seaLow.g},${seaLow.b})`);
			ctx.fillStyle = sea;
			ctx.fillRect(0, Math.floor(horizon * sy), canvasW, canvasH - Math.floor(horizon * sy));

			const coastRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const bluff = Math.exp(-Math.pow((x - towerX) / 18, 2)) * 7.5;
				const swell = Math.sin(x * 0.032 + 0.5) * 1.3 + Math.sin(x * 0.013 + 2.1) * 1.1;
				coastRows[x] = Math.round(horizon + swell - bluff);
			}
			const coastColor = hslToRGB((this.cfg.hue + 214) % 360, clamp01(this.cfg.sat * 0.12), 0.08);
			ctx.fillStyle = `rgb(${coastColor.r},${coastColor.g},${coastColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				ctx.lineTo(Math.floor(x * sx), Math.floor(coastRows[x] * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const oceanLine = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.35), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.3));
			for (let x = 0; x < this.w; x += 2) {
				if ((x + this.tick) % 5 !== 0) continue;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, horizon + 1 + Math.round(Math.sin(x * 0.07 + this.tick * 0.02)), 2, 1, `rgb(${oceanLine.r},${oceanLine.g},${oceanLine.b})`, 0.18);
			}

			const towerBase = coastRows[Math.max(0, Math.min(this.w - 1, towerX))];
			const lampY = towerBase - towerH + 2;
			const towerColor = hslToRGB((this.cfg.hue + 212) % 360, clamp01(this.cfg.sat * 0.08), 0.1);
			for (let y = lampY; y <= towerBase; y++) {
				const ratio = (y - lampY) / Math.max(1, towerBase - lampY);
				const half = Math.max(1, Math.round((towerW * (0.32 + ratio * 0.68)) * 0.5));
				for (let dx = -half; dx <= half; dx++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX + dx, y, 1, 1, `rgb(${towerColor.r},${towerColor.g},${towerColor.b})`, 1);
				}
			}
			for (let dx = -Math.max(2, Math.round(towerW * 0.6)); dx <= Math.max(2, Math.round(towerW * 0.6)); dx++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX + dx, lampY - 1 + Math.round(Math.abs(dx) * 0.15), 1, 1, `rgb(${towerColor.r},${towerColor.g},${towerColor.b})`, 1);
			}

			const lampGlow = hslToRGB(48, 0.68, clamp01(0.42 + this.cfg.glow * 0.34));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX, lampY, 1, 1, `rgb(${lampGlow.r},${lampGlow.g},${lampGlow.b})`, clamp01(0.3 + this.cfg.glow * 0.7));

			const glowX = towerX * sx;
			const glowY = lampY * sy;
			const glowR = Math.max(18, Math.min(canvasW, canvasH) * (0.04 + this.cfg.glow * 0.05));
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			glow.addColorStop(0, `rgba(255, 236, 184, ${0.16 + this.cfg.glow * 0.22})`);
			glow.addColorStop(1, 'rgba(255, 236, 184, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			const beamHue = (this.cfg.hue - 18 + 360) % 360;
			const beamBase = hslToRGB(beamHue, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 0.98));
			const beamCore = hslToRGB(44, 0.42, clamp01(this.cfg.lmax * 1.02));
			const angleDiff = (a, b) => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b)));
			for (let y = 0; y <= horizon + 5; y++) {
				for (let x = towerX + 1; x < this.w; x++) {
					const dx = x - towerX;
					const dy = y - lampY;
					if (dx <= 0) continue;
					const dist = Math.hypot(dx, dy);
					if (dist < 2) continue;
					const ang = Math.atan2(dy, dx);
					const diff = angleDiff(ang, beamAngle);
					if (diff > beamWidth) continue;
					const cone = 1 - diff / beamWidth;
					const edge = Math.pow(cone, Math.max(0.6, 1.8 - beamSoftness));
					const falloff = Math.pow(clamp01(1 - dist / (this.w * 0.92)), 0.72);
					const strength = edge * falloff * beamLevel;
					if (strength < 0.02) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${beamBase.r},${beamBase.g},${beamBase.b})`, clamp01(strength * (0.12 + fogLevel * 0.08)));
					if (diff < beamWidth * 0.34 && strength > 0.08) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${beamCore.r},${beamCore.g},${beamCore.b})`, clamp01(strength * 0.16));
					}
				}
			}

			const fog = ctx.createLinearGradient(0, Math.floor((horizon - 7) * sy), 0, Math.floor((horizon + 9) * sy));
			fog.addColorStop(0, `rgba(201, 214, 226, ${0.02 + fogLevel * 0.08})`);
			fog.addColorStop(1, 'rgba(201, 214, 226, 0)');
			ctx.fillStyle = fog;
			ctx.fillRect(0, Math.floor((horizon - 8) * sy), canvasW, Math.ceil(18 * sy));
		}

		_renderWindmill(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue + 210) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.95));
				const skyMid = hslToRGB((this.cfg.hue + 248) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.32));
				const skyLow = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.78));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.58, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.min(this.h - 8, Math.floor(this.h * this.cfg.horizon)));
			const centerX = Math.floor(this.w * 0.58);
			const rotationLevel = this._rotationLevelWindmill();
			const angle = this.tick * this.cfg.turn_speed * rotationLevel + Math.PI * 0.08;
			const towerH = Math.max(10, Math.round(this.cfg.tower_height));
			const towerW = Math.max(3, Math.round(this.cfg.tower_width));
			const bladeLen = Math.max(5, Math.round(this.cfg.blade_len));
			const bladeWidth = Math.max(1, this.cfg.blade_width);

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 3) * sy), 0, Math.floor((horizon + 7) * sy));
			horizonGlow.addColorStop(0, `rgba(255, 214, 163, ${0.04 + this.cfg.glow * 0.12})`);
			horizonGlow.addColorStop(1, 'rgba(255, 214, 163, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 3) * sy), canvasW, Math.ceil(12 * sy));

			const hillRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const broad = Math.sin(x * 0.045 + 0.4) * 1.2 + Math.sin(x * 0.012 + 1.3) * 2.1;
				const mound = Math.exp(-Math.pow((x - centerX) / 18, 2)) * 6.2 + Math.exp(-Math.pow((x - this.w * 0.18) / 24, 2)) * 2.1;
				hillRows[x] = Math.round(horizon + broad - mound);
			}

			const hillColor = hslToRGB((this.cfg.hue + 205) % 360, clamp01(this.cfg.sat * 0.16), 0.08);
			ctx.fillStyle = `rgb(${hillColor.r},${hillColor.g},${hillColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				ctx.lineTo(Math.floor(x * sx), Math.floor(hillRows[x] * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const baseY = hillRows[Math.max(0, Math.min(this.w - 1, centerX))];
			const hubY = baseY - towerH + 2;
			const millColor = hslToRGB((this.cfg.hue + 208) % 360, clamp01(this.cfg.sat * 0.08), 0.1);
			for (let y = hubY; y <= baseY; y++) {
				const ratio = (y - hubY) / Math.max(1, baseY - hubY);
				const half = Math.max(1, Math.round((towerW * (0.38 + ratio * 0.62)) * 0.5));
				for (let dx = -half; dx <= half; dx++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, y, 1, 1, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);
				}
			}

			for (let dx = -Math.max(2, Math.round(towerW * 0.42)); dx <= Math.max(2, Math.round(towerW * 0.42)); dx++) {
				const roofY = hubY - 2 + Math.abs(dx) * 0.4;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, Math.round(roofY), 1, 1, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);
			}

			const windowGlow = hslToRGB(42, 0.72, clamp01(0.38 + this.cfg.glow * 0.5));
			const windowY = Math.round(hubY + towerH * 0.46);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX, windowY, 1, 2, `rgb(${windowGlow.r},${windowGlow.g},${windowGlow.b})`, clamp01(0.18 + this.cfg.glow * 0.58));

			const bladeColor = hslToRGB((this.cfg.hue + 210) % 360, clamp01(this.cfg.sat * 0.08), 0.13);
			for (let blade = 0; blade < 4; blade++) {
				const theta = angle + blade * Math.PI * 0.5;
				const px = -Math.sin(theta);
				const py = Math.cos(theta) * 0.88;
				for (let r = 1; r <= bladeLen; r++) {
					const fade = 1 - r / Math.max(1, bladeLen);
					const bx = centerX + Math.cos(theta) * r;
					const by = hubY + Math.sin(theta) * r * 0.88;
					const half = Math.max(0, Math.round(bladeWidth * fade * 0.55));
					for (let spread = -half; spread <= half; spread++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(bx + px * spread * 0.7), Math.round(by + py * spread * 0.7), 1, 1, `rgb(${bladeColor.r},${bladeColor.g},${bladeColor.b})`, 1);
					}
				}
			}

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - 1, hubY - 1, 3, 3, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);

			const grassColor = hslToRGB((this.cfg.hue + 120) % 360, 0.16, 0.14);
			for (let x = 0; x < this.w; x += 2) {
				const top = hillRows[x];
				if ((x + this.tick) % 5 !== 0) continue;
				const sway = this.timers.gust > 0 ? (this.values.gust_gain || this.cfg.gust_mult) * 0.2 : this.timers.lull > 0 ? -this.cfg.lull_mult * 0.08 : 0.04;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + Math.round(Math.sin(this.tick * 0.05 + x * 0.1) * sway), top - 1, 1, 2, `rgb(${grassColor.r},${grassColor.g},${grassColor.b})`, 0.28);
			}
		}

		_renderCampfire(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#08111a');
				sky.addColorStop(0.62, '#0f1520');
				sky.addColorStop(1, '#16110c');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.84);
			const centerX = Math.floor(this.w * 0.5);
			const flameLevel = this._flameLevelCampfire();
			const crackleGain = this.values.crackle_gain || 1;
			const halfW = Math.max(2, Math.round(this.cfg.flame_width * 0.5));
			const flameH = Math.max(4, this.cfg.flame_height * (0.52 + flameLevel * 0.38));
			const speed = this.tick * this.cfg.flame_speed * 1.7;

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const ground = hslToRGB(18, 0.24, 0.08 + ratio * 0.12);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${ground.r},${ground.g},${ground.b})`, 1);
			}

			const glowStrength = clamp01(this.cfg.glow * (0.6 + flameLevel * 0.45));
			const glowX = centerX * sx;
			const glowY = groundRow * sy;
			const glowR = Math.max(28, Math.min(canvasW, canvasH) * (0.08 + glowStrength * 0.12));
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			glow.addColorStop(0, `rgba(255, 178, 82, ${0.24 + glowStrength * 0.22})`);
			glow.addColorStop(0.42, `rgba(255, 120, 44, ${0.12 + glowStrength * 0.12})`);
			glow.addColorStop(1, 'rgba(255, 120, 44, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			const vignette = ctx.createRadialGradient(glowX, glowY, glowR * 0.35, glowX, glowY, glowR * 2.4);
			vignette.addColorStop(0, 'rgba(0, 0, 0, 0)');
			vignette.addColorStop(1, 'rgba(0, 0, 0, 0.26)');
			ctx.fillStyle = vignette;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const logColor = hslToRGB(20, 0.46, 0.22);
			const logHighlight = hslToRGB(24, 0.44, 0.32);
			const logHalf = halfW + 2;
			for (let dx = -logHalf; dx <= logHalf; dx++) {
				const rowA = groundRow + 1 + Math.round(dx * 0.12);
				const rowB = groundRow + 1 - Math.round(dx * 0.1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowA, 1, 1, `rgb(${logColor.r},${logColor.g},${logColor.b})`, 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowB, 1, 1, `rgb(${logColor.r},${logColor.g},${logColor.b})`, 0.82);
				if ((dx + logHalf) % 3 === 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowA, 1, 1, `rgb(${logHighlight.r},${logHighlight.g},${logHighlight.b})`, 0.34);
				}
			}

			for (let dx = -halfW; dx <= halfW; dx++) {
				const coalHeat = 0.45 + 0.55 * Math.pow(0.5 + 0.5 * Math.sin(speed * 1.8 + dx * 0.8), 2);
				const coal = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.88), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.22 + coalHeat * 0.45)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, groundRow, 1, 1, `rgb(${coal.r},${coal.g},${coal.b})`, 0.28 + coalHeat * 0.42);
			}

			for (let x = -halfW; x <= halfW; x++) {
				const nx = Math.abs(x) / Math.max(1, halfW);
				const widthShape = Math.max(0, 1 - Math.pow(nx, 1.32));
				if (widthShape <= 0.04) continue;
				const pulse = 0.8 + 0.2 * Math.sin(speed * 1.3 + x * 0.7 + this._hash(26000 + x + halfW) * 5);
				const columnH = Math.max(2, Math.round(flameH * widthShape * pulse));
				for (let y = 0; y < columnH; y++) {
					const lift = y / Math.max(1, columnH);
					const taper = 1 - lift;
					const sway = Math.sin(speed * 2.1 + x * 0.35 + y * 0.24 + this._hash(26100 + y * 31 + x + 400) * 6) * this.cfg.flicker * taper * 0.72;
					const col = Math.round(centerX + x + sway);
					const row = Math.round(groundRow - 1 - y);
					const hue = ((this.cfg.hue - lift * this.cfg.hue_sp * 0.34 + (this._hash(26200 + x * 17 + y + 700) * 2 - 1) * this.cfg.hue_sp * 0.08) % 360 + 360) % 360;
					const sat = clamp01(this.cfg.sat * (0.88 + taper * 0.16));
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.18 + taper * 0.8));
					const alpha = clamp01((0.12 + taper * 0.56) * (0.36 + widthShape * 0.64) * (0.72 + flameLevel * 0.18));
					const color = hslToRGB(hue, sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if (widthShape > 0.35 && lift < 0.6 && (x + y) % 2 === 0) {
						const core = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.74), clamp01(this.cfg.lmax * (0.72 + taper * 0.24)));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${core.r},${core.g},${core.b})`, alpha * 0.52);
					}
				}
			}

			const emberCount = Math.max(4, Math.round(this.cfg.flame_width * (0.8 + this.cfg.ember_rate * 3.8) * (this.timers.crackle > 0 ? 0.92 + crackleGain * 0.3 : 1)));
			const maxRise = Math.max(10, Math.round(this.cfg.flame_height * 2.1 + this.cfg.ember_speed * 12));
			for (let i = 0; i < emberCount; i++) {
				const cycle = maxRise + 8 + Math.floor(this._hash(27000 + i) * 12);
				const progress = positiveMod(this.tick * this.cfg.ember_speed * (0.7 + this._hash(27100 + i) * 0.7) + this._hash(27200 + i) * cycle, cycle);
				if (progress > maxRise) continue;
				const rise = progress;
				const fade = 1 - rise / Math.max(1, maxRise);
				const drift = (this._hash(27300 + i) * 2 - 1) * (1.2 + rise * 0.08) + Math.sin(speed + i * 0.7) * 0.6;
				const col = Math.round(centerX + drift);
				const row = Math.round(groundRow - 2 - rise);
				if (row < 1) continue;
				const size = fade > 0.72 && this.timers.crackle > 0 && this._hash(27400 + i) > 0.5 ? 2 : 1;
				const hue = ((this.cfg.hue - 6 + this._hash(27500 + i) * 10) % 360 + 360) % 360;
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.42 + fade * 0.5));
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.8), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, size, 1, `rgb(${color.r},${color.g},${color.b})`, clamp01((0.16 + fade * 0.68) * (0.78 + Math.max(0, crackleGain - 1) * 0.16)));
			}
		}

		_renderVolcano(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#0a0612');
				sky.addColorStop(0.55, '#15101c');
				sky.addColorStop(1, '#1f1212');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const baseRow = Math.max(8, Math.min(this.h - 4, Math.floor(this.h * this.cfg.horizon)));
			const centerX = Math.floor(this.w * 0.5);
			const pressure = this._pressureLevelVolcano();
			const eruptionActive = this.timers.eruption > 0;
			const eruptionGain = this.values.eruption_gain || 1;
			const flareActive = this.timers.flare > 0;
			const flareGain = this.values.flare_gain || 1;
			const eruptionTotal = this.timers.eruption > 0 ? Math.max(1, Math.round(this.cfg.eruption_dur)) : 0;
			const eruptionPhase = eruptionActive
				? this._phaseProgress(Math.max(eruptionTotal, this.timers.eruption), this.timers.eruption)
				: 0;
			const eruptionEnvelope = eruptionActive ? Math.sin(Math.PI * Math.min(1, eruptionPhase * 1.0)) : 0;

			const halfW = Math.max(4, Math.round(this.cfg.cone_width * 0.5));
			const coneH = Math.max(6, Math.round(this.cfg.cone_height));
			const craterHalf = Math.max(2, Math.round(this.cfg.crater_width * 0.5));
			const peakRow = baseRow - coneH;

			// silhouette colors
			const coneColor = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.55));
			const coneEdge = hslToRGB((this.cfg.hue + 348) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin * 0.78));
			const ground = hslToRGB((this.cfg.hue + 352) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * 0.4));

			// distant horizon haze
			const hazeColor = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
			const hazeStrength = clamp01(0.18 + this.cfg.glow * 0.18 + pressure * 0.08);
			const hazeGrad = ctx.createLinearGradient(0, Math.floor((baseRow - 4) * sy), 0, Math.floor((baseRow + 6) * sy));
			hazeGrad.addColorStop(0, `rgba(${hazeColor.r},${hazeColor.g},${hazeColor.b},0)`);
			hazeGrad.addColorStop(1, `rgba(${hazeColor.r},${hazeColor.g},${hazeColor.b},${hazeStrength})`);
			ctx.fillStyle = hazeGrad;
			ctx.fillRect(0, Math.floor((baseRow - 4) * sy), canvasW, Math.ceil(14 * sy));

			// flat foreground ground past the cone
			for (let y = baseRow; y < this.h; y++) {
				const ratio = (y - baseRow) / Math.max(1, this.h - baseRow);
				const groundShade = hslToRGB((this.cfg.hue + 352) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * (0.4 + ratio * 0.45)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${groundShade.r},${groundShade.g},${groundShade.b})`, 1);
			}

			// cone silhouette: crater dips into the peak
			for (let dx = -halfW; dx <= halfW; dx++) {
				const nx = Math.abs(dx) / Math.max(1, halfW);
				// slight curvature on the slopes (rounded base, sharper near peak)
				const slope = Math.pow(1 - nx, 1.3);
				const jitter = (this._hash(31000 + dx) * 2 - 1) * this.cfg.slope_jitter;
				let topY = baseRow - Math.round(coneH * slope + jitter);
				// crater notch
				if (Math.abs(dx) < craterHalf) {
					const craterDepth = Math.max(1, Math.round(2 + craterHalf * 0.4 * (1 - Math.abs(dx) / Math.max(1, craterHalf))));
					topY = peakRow + craterDepth;
				}
				if (topY > baseRow) topY = baseRow;
				const col = centerX + dx;
				if (col < 0 || col >= this.w) continue;
				for (let y = topY; y <= baseRow; y++) {
					const isEdge = y === topY || y === topY + 1;
					const color = isEdge ? coneEdge : coneColor;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
				}
			}

			// idle crater glow + flare bloom
			const glowStrength = clamp01(this.cfg.glow * pressure * (flareActive ? flareGain : 1));
			const glowX = centerX * sx;
			const glowY = peakRow * sy;
			const glowR = Math.max(28, Math.min(canvasW, canvasH) * (0.07 + glowStrength * 0.18));
			const glowGrad = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			const glowHue = (this.cfg.hue + 4) % 360;
			const glowCore = hslToRGB(glowHue, clamp01(this.cfg.sat * 0.95), clamp01(this.cfg.lmax * (0.7 + glowStrength * 0.25)));
			const glowOuter = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.45));
			glowGrad.addColorStop(0, `rgba(${glowCore.r},${glowCore.g},${glowCore.b},${0.35 + glowStrength * 0.45})`);
			glowGrad.addColorStop(0.45, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},${0.18 + glowStrength * 0.22})`);
			glowGrad.addColorStop(1, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},0)`);
			ctx.fillStyle = glowGrad;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			// crater rim hot lining
			for (let dx = -craterHalf; dx <= craterHalf; dx++) {
				const t = 1 - Math.abs(dx) / Math.max(1, craterHalf);
				const lava = hslToRGB((this.cfg.hue + this._hash(31300 + dx) * this.cfg.hue_sp * 0.4) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.4 + t * 0.5 + glowStrength * 0.2)));
				const row = peakRow + Math.max(1, Math.round(2 + craterHalf * 0.4 * t));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, row, 1, 1, `rgb(${lava.r},${lava.g},${lava.b})`, clamp01(0.35 + t * 0.5 + glowStrength * 0.25));
			}

			// rising smoke plume (idle + thicker during eruption)
			const smokeBase = Math.max(0, this.cfg.smoke);
			const smokeBoost = eruptionActive ? eruptionEnvelope * 0.85 : (flareActive ? 0.18 : 0);
			const smokeStrength = clamp01(smokeBase * (this.timers.smolder > 0 ? this.cfg.smolder_mult : 1) + smokeBoost);
			if (smokeStrength > 0.02) {
				const smokeMaxRise = Math.max(8, Math.round(this.cfg.smoke_height * (1 + smokeBoost * 0.6)));
				const puffCount = Math.max(4, Math.round(8 + smokeStrength * 14));
				for (let i = 0; i < puffCount; i++) {
					const cycle = smokeMaxRise + 6 + Math.floor(this._hash(31600 + i) * 12);
					const progress = positiveMod(this.tick * 0.12 * (0.7 + this._hash(31700 + i) * 0.6) + this._hash(31800 + i) * cycle, cycle);
					if (progress > smokeMaxRise) continue;
					const fade = 1 - progress / Math.max(1, smokeMaxRise);
					const drift = Math.sin(this.tick * 0.04 + i * 0.7) * (1.4 + progress * 0.12) + (this._hash(31900 + i) * 2 - 1) * 1.6;
					const col = Math.round(centerX + drift);
					const row = Math.round(peakRow - 1 - progress);
					if (row < 1) continue;
					const tint = hslToRGB((this.cfg.hue + 12 + this._hash(32000 + i) * this.cfg.hue_sp * 0.4) % 360, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.36 + fade * 0.34)));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${tint.r},${tint.g},${tint.b})`, clamp01(0.18 + fade * smokeStrength * 0.7));
					if (smokeStrength > 0.4 && (i & 1) === 0) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col + Math.sign(drift || 1), row, 1, 1, `rgb(${tint.r},${tint.g},${tint.b})`, clamp01(0.1 + fade * smokeStrength * 0.4));
					}
				}
			}

			// eruption: ballistic lava sparks arcing out of the crater
			if (eruptionActive) {
				const archHeight = Math.max(6, this.cfg.eruption_height) * (0.65 + eruptionEnvelope * 0.5) * eruptionGain * 0.55;
				const sparkCount = Math.max(8, Math.round(this.cfg.crater_width * 1.6 + archHeight * 0.8));
				const seed = this.values.eruption_seed || 0;
				for (let i = 0; i < sparkCount; i++) {
					const cycle = Math.max(14, Math.round(archHeight * 1.2 + 14));
					const phase = positiveMod(this.tick * 0.4 * (0.7 + this._hash(32200 + i + seed) * 0.6) + this._hash(32300 + i + seed) * cycle, cycle);
					const t = phase / cycle;
					if (t >= 1) continue;
					const angle = (this._hash(32400 + i + seed) * 2 - 1) * Math.PI * 0.42;
					const v0 = archHeight * (0.7 + this._hash(32500 + i + seed) * 0.6);
					const dxStart = Math.sin(angle) * (1.2 + craterHalf * 0.6);
					const yArc = -v0 * Math.sin(Math.PI * t);
					const xArc = dxStart + (this._hash(32600 + i + seed) * 2 - 1) * v0 * 0.18 * t;
					const col = Math.round(centerX + xArc);
					const row = Math.round(peakRow + 1 + yArc);
					if (col < 0 || col >= this.w || row < 0 || row >= this.h) continue;
					const fade = 1 - Math.pow(t, 1.6);
					const hue = ((this.cfg.hue + (this._hash(32700 + i + seed) * 2 - 1) * this.cfg.hue_sp * 0.5) + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.55 + fade * 0.45));
					const lava = hslToRGB(hue, clamp01(this.cfg.sat), light);
					const size = fade > 0.7 ? 2 : 1;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, size, 1, `rgb(${lava.r},${lava.g},${lava.b})`, clamp01(0.45 + fade * 0.5));
				}

				// short lava streaks running down the cone surface during peak eruption
				const streakCount = Math.max(0, Math.round((eruptionGain - 1) * 4));
				for (let s = 0; s < streakCount; s++) {
					const side = s % 2 === 0 ? -1 : 1;
					const dxStart = side * (craterHalf + 1 + this._hash(32800 + s + seed) * 2);
					const length = Math.max(2, Math.round(coneH * 0.3 * eruptionEnvelope));
					for (let r = 0; r < length; r++) {
						const col = Math.round(centerX + dxStart + side * r * 0.4);
						const row = peakRow + r;
						if (row > baseRow || col < 0 || col >= this.w) break;
						const fade = 1 - r / Math.max(1, length);
						const lava = hslToRGB((this.cfg.hue + 4) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.45 + fade * 0.4)));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${lava.r},${lava.g},${lava.b})`, clamp01(0.32 + fade * 0.5));
					}
				}
			}
		}

		_renderBeach(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#f4b17a');
				sky.addColorStop(0.38, '#f8d6a9');
				sky.addColorStop(0.68, '#cfe3e6');
				sky.addColorStop(1, '#8bb4c4');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.floor(this.h * 0.34));
			const tideLevel = this._tideLevelBeach();
			const tideBias = this.values.tide_bias || 0;
			const foamGain = this.values.foam_gain || 1;
			const tidePhase = this.tick * this.cfg.speed * 0.08;
			const baseShore = this.h * this.cfg.shoreline + Math.sin(tidePhase) * this.cfg.tide_amp * tideLevel * 0.34 + tideBias * 1.6;

			const sunX = canvasW * (0.16 + this._hash(24000) * 0.18);
			const sunY = canvasH * (0.18 + this._hash(24001) * 0.08);
			const sunR = Math.max(22, Math.min(canvasW, canvasH) * 0.085);
			const sun = ctx.createRadialGradient(sunX, sunY, 0, sunX, sunY, sunR * 2.8);
			sun.addColorStop(0, 'rgba(255, 239, 187, 0.38)');
			sun.addColorStop(0.34, 'rgba(255, 224, 168, 0.2)');
			sun.addColorStop(1, 'rgba(255, 224, 168, 0)');
			ctx.fillStyle = sun;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const haze = ctx.createLinearGradient(0, canvasH * 0.18, 0, canvasH * 0.6);
			haze.addColorStop(0, 'rgba(255, 246, 224, 0)');
			haze.addColorStop(1, 'rgba(255, 246, 224, 0.14)');
			ctx.fillStyle = haze;
			ctx.fillRect(0, canvasH * 0.16, canvasW, canvasH * 0.44);

			const waterTop = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmax * 0.74));
			const waterMid = hslToRGB(this.cfg.hue, this.cfg.sat, clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.44));
			const waterDeep = hslToRGB((this.cfg.hue + 6) % 360, clamp01(this.cfg.sat * 1.06), clamp01(this.cfg.lmin * 0.8));
			const waterGrad = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			waterGrad.addColorStop(0, `rgb(${waterTop.r},${waterTop.g},${waterTop.b})`);
			waterGrad.addColorStop(0.46, `rgb(${waterMid.r},${waterMid.g},${waterMid.b})`);
			waterGrad.addColorStop(1, `rgb(${waterDeep.r},${waterDeep.g},${waterDeep.b})`);
			ctx.fillStyle = waterGrad;
			ctx.fillRect(0, Math.floor(horizon * sy), canvasW, canvasH - Math.floor(horizon * sy));

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 1) * sy), 0, Math.floor((horizon + 4) * sy));
			horizonGlow.addColorStop(0, 'rgba(255, 247, 221, 0.35)');
			horizonGlow.addColorStop(1, 'rgba(255, 247, 221, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 1) * sy), canvasW, Math.ceil(7 * sy));

			for (let band = 0; band < 3; band++) {
				const y = Math.floor((horizon + 2 + band * 3 + Math.sin(tidePhase * 1.8 + band) * 1.2) * sy);
				ctx.fillStyle = `rgba(${waterTop.r},${waterTop.g},${waterTop.b},${0.08 + band * 0.03})`;
				ctx.fillRect(0, y, canvasW, Math.max(1, Math.ceil(sy)));
			}

			const shoreRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const nx = x / Math.max(1, this.w - 1);
				const slopeOffset = (nx - 0.5) * this.cfg.slope * this.h * 0.34;
				const wave = Math.sin(x * this.cfg.wave_freq + tidePhase * 2.2);
				const backwash = Math.sin(x * this.cfg.wave_freq * 0.46 - tidePhase * 1.6 + 0.8);
				const chop = Math.sin(x * this.cfg.wave_freq * 1.85 + tidePhase * 3.1 + this._hash(24100 + x) * 3);
				const shore = baseShore + slopeOffset + wave * this.cfg.wave_amp * (0.14 + tideLevel * 0.1) + backwash * this.cfg.wave_amp * 0.08 + chop * this.cfg.wave_amp * 0.03;
				shoreRows[x] = Math.max(horizon + 3, Math.min(this.h - 4, shore));
			}
			for (let pass = 0; pass < 2; pass++) {
				for (let x = 1; x < this.w - 1; x++) {
					shoreRows[x] = (shoreRows[x - 1] + shoreRows[x] * 2 + shoreRows[x + 1]) / 4;
				}
			}

			const sandTop = hslToRGB(39, 0.54, 0.8);
			const sandMid = hslToRGB(36, 0.48, 0.68);
			const sandLow = hslToRGB(33, 0.42, 0.54);
			const drawSandPath = () => {
				ctx.beginPath();
				ctx.moveTo(0, canvasH);
				for (let x = 0; x < this.w; x++) {
					ctx.lineTo(Math.floor(x * sx), Math.floor(shoreRows[x] * sy));
				}
				ctx.lineTo(canvasW, canvasH);
				ctx.closePath();
			};
			drawSandPath();
			const sandGrad = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			sandGrad.addColorStop(0, `rgb(${sandTop.r},${sandTop.g},${sandTop.b})`);
			sandGrad.addColorStop(0.55, `rgb(${sandMid.r},${sandMid.g},${sandMid.b})`);
			sandGrad.addColorStop(1, `rgb(${sandLow.r},${sandLow.g},${sandLow.b})`);
			ctx.fillStyle = sandGrad;
			ctx.fill();

			const duneColor = hslToRGB(31, 0.32, 0.42);
			const dunePoints = [];
			for (let i = 0; i <= 6; i++) {
				dunePoints.push(Math.floor(this.h * 0.74) + Math.floor(this._hash(24200 + i) * 5) + Math.floor(Math.sin(i * 1.08 + this._hash(24300 + i) * 2) * 2));
			}
			ctx.fillStyle = `rgba(${duneColor.r},${duneColor.g},${duneColor.b},0.16)`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * 6;
				const idx = Math.min(5, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const y = dunePoints[idx] + (dunePoints[idx + 1] - dunePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(y * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const wetColor = hslToRGB(34, 0.34, 0.34 + clamp01(0.22 + tideLevel * 0.36) * 0.12);
			const foamColor = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 1.02));
			const shimmerColor = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmax * 0.96));
			const shimmerLevel = clamp01(this.cfg.shimmer * (0.6 + tideLevel * 0.5));

			for (let x = 0; x < this.w; x++) {
				const shore = shoreRows[x];
				const surfRow = Math.round(shore);
				const wetBand = Math.max(2, Math.round(2 + tideLevel * 3 + Math.max(0, foamGain - 1) * 1.4));
				const foamBand = Math.max(1, Math.round(1 + this.cfg.foam * 2.8 + Math.max(0, foamGain - 1) * 1.4));

				for (let row = surfRow; row < Math.min(this.h, surfRow + wetBand); row++) {
					const fade = 1 - (row - shore) / Math.max(1, wetBand);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${wetColor.r},${wetColor.g},${wetColor.b})`, clamp01(0.14 + fade * 0.32));
				}

				for (let i = 0; i < foamBand; i++) {
					const row = surfRow - i;
					const pulse = 0.55 + 0.45 * Math.sin(this.tick * 0.05 + x * 0.18 + i * 0.9);
					const alpha = clamp01((0.12 + this.cfg.foam * 0.42) * foamGain * (0.5 + 0.5 * pulse));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, alpha);
				}

				if ((x + this.tick) % 2 === 0) {
					const depth = 0.18 + this._hash(24400 + x) * 0.56;
					const row = Math.max(horizon + 1, Math.floor(horizon + (shore - horizon) * depth));
					const width = 1 + Math.floor(this._hash(24500 + x) * 3);
					const blink = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.03 + x * 0.12), 2);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, width, 1, `rgb(${shimmerColor.r},${shimmerColor.g},${shimmerColor.b})`, clamp01((0.08 + shimmerLevel * 0.34) * blink));
				}

				if ((x + Math.floor(this.tick / 3)) % 7 === 0) {
					const pebbleRow = Math.min(this.h - 2, surfRow + wetBand + 1 + Math.floor(this._hash(24600 + x) * 8));
					const pebble = hslToRGB(34 + this._hash(24700 + x) * 10, 0.2, 0.4 + this._hash(24800 + x) * 0.12);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, pebbleRow, 1, 1, `rgb(${pebble.r},${pebble.g},${pebble.b})`, 0.22);
				}
			}
		}

		_renderRowboat(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.45), clamp01(this.cfg.lmin + 0.02));
				const skyMid = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.22 + 360) % 360, clamp01(this.cfg.sat * 0.58), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.34));
				const skyLow = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.18) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.62));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.58, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const waterline = Math.max(12, Math.min(this.h - 10, Math.floor(this.h * this.cfg.waterline)));
			const motion = this._rippleLevelRowboat();
			const driftPush = this.values.drift_push || 0;
			const phase = this.tick * this.cfg.drift_speed * 0.08;

			const glowX = canvasW * 0.74;
			const glowY = canvasH * 0.24;
			const glowR = Math.max(20, Math.min(canvasW, canvasH) * 0.09);
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR * 2.8);
			glow.addColorStop(0, 'rgba(255, 223, 174, 0.22)');
			glow.addColorStop(1, 'rgba(255, 223, 174, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const ridgeBase = waterline - Math.max(5, Math.round(this.h * 0.12));
			const shorelineY = Math.max(ridgeBase + 2, waterline - Math.max(2, Math.round(this.h * 0.04)));
			const ridgePoints = [];
			const ridgeSegments = 7;
			for (let i = 0; i <= ridgeSegments; i++) {
				const ridgeWave = Math.sin(i * 0.9 + this._hash(25100 + i) * 2.4) * 2.8;
				ridgePoints.push(Math.round(ridgeBase - Math.abs(ridgeWave) - this._hash(25200 + i) * 3));
			}
			const ridgeColor = hslToRGB((this.cfg.hue + 54) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin * 0.7));
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, Math.floor(shorelineY * sy));
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const ridgeY = ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(ridgeY * sy));
			}
			ctx.lineTo(canvasW, Math.floor(shorelineY * sy));
			ctx.closePath();
			ctx.fill();

			const treelineColor = hslToRGB((this.cfg.hue + 72) % 360, clamp01(this.cfg.sat * 0.2), clamp01(this.cfg.lmin * 0.52));
			for (let i = 0; i < 11; i++) {
				const col = Math.floor((i + 0.4) * this.w / 11 + (this._hash(25300 + i) - 0.5) * 6);
				const top = Math.round(ridgeBase - 1 - this._hash(25400 + i) * 4);
				const height = 2 + Math.floor(this._hash(25500 + i) * 3);
				for (let row = 0; row < height; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, top + row, 1, 1, `rgb(${treelineColor.r},${treelineColor.g},${treelineColor.b})`, 0.92);
				}
			}

			for (let y = waterline; y < this.h; y++) {
				const depth = (y - waterline) / Math.max(1, this.h - waterline);
				const hue = ((this.cfg.hue + depth * this.cfg.hue_sp * 0.22) % 360 + 360) % 360;
				const sat = clamp01(this.cfg.sat * (0.8 - depth * 0.22));
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.36 - depth * 0.18));
				const color = hslToRGB(hue, sat, light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
			}

			const mist = ctx.createLinearGradient(0, Math.floor((shorelineY - 3) * sy), 0, Math.floor((waterline + 8) * sy));
			mist.addColorStop(0, 'rgba(255, 240, 220, 0.14)');
			mist.addColorStop(1, 'rgba(255, 240, 220, 0)');
			ctx.fillStyle = mist;
			ctx.fillRect(0, Math.floor((shorelineY - 3) * sy), canvasW, Math.ceil((waterline - shorelineY + 11) * sy));

			const surfaceColor = hslToRGB((this.cfg.hue - 6 + 360) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmax * 0.92));
			for (let x = 0; x < this.w; x++) {
				const wave = Math.sin(x * this.cfg.wave_freq + phase) * this.cfg.wave_amp;
				const subWave = Math.sin(x * this.cfg.wave_freq * 0.42 - phase * 1.7) * this.cfg.wave_amp * 0.36;
				const row = waterline + Math.round((wave + subWave) * motion * 0.22);
				const twinkle = 0.45 + 0.55 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.03 + x * 0.11), 2);
				const alpha = clamp01((0.05 + this.cfg.reflection * 0.16) * twinkle);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, alpha);
				if ((x + this.tick) % 6 === 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row + 2, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, alpha * 0.45);
				}
			}

			const boatLen = Math.max(7, Math.round(this.cfg.boat_len));
			const boatHeight = Math.max(2, Math.round(this.cfg.boat_height));
			const boatX = Math.max(Math.floor(boatLen * 0.6), Math.min(this.w - Math.ceil(boatLen * 0.6), Math.round(this.w * 0.34 + Math.sin(phase * 1.6 + 0.8) * (2.4 + motion * 1.6) + driftPush * 1.8)));
			const bob = Math.sin(phase * 2.4 + 0.6) * this.cfg.bob_amp * motion * 0.52 + Math.sin(phase * 0.95 + 1.3) * 0.35;
			const hullBaseY = waterline - Math.round(bob * 0.55);
			const tilt = Math.sin(phase * 1.9 + 0.4) * motion * 0.9 + driftPush * 0.2;
			const hullColor = hslToRGB(24, 0.34, 0.22);
			const railColor = hslToRGB(31, 0.26, 0.36);
			const seatColor = hslToRGB(28, 0.18, 0.16);
			const hullRows = [];

			for (let row = 0; row < boatHeight; row++) {
				const t = boatHeight === 1 ? 0.5 : row / (boatHeight - 1);
				const arch = 1 - Math.abs(t * 2 - 1);
				const width = Math.max(4, Math.round(boatLen * (0.54 + arch * 0.42)));
				const y = hullBaseY - (boatHeight - 1 - row);
				const offset = Math.round((t - 0.5) * tilt * 1.8);
				const startX = Math.round(boatX - width / 2 + offset);
				hullRows.push({ startX, width, y });
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, startX, y, width, 1, `rgb(${hullColor.r},${hullColor.g},${hullColor.b})`, clamp01(0.78 + t * 0.2));
			}

			const topHull = hullRows[0];
			if (topHull && topHull.width > 3) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, topHull.startX + 1, topHull.y, topHull.width - 2, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.72);
			}
			const seatWidth = Math.max(2, Math.round(boatLen * 0.22));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - seatWidth / 2), hullBaseY - Math.max(1, Math.floor(boatHeight / 2)), seatWidth, 1, `rgb(${seatColor.r},${seatColor.g},${seatColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - boatLen * 0.46), hullBaseY - 1, 1, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX + boatLen * 0.44), hullBaseY - 1, 1, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.82);

			const shadowColor = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.9));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - boatLen * 0.5), waterline, boatLen, 1, `rgb(${shadowColor.r},${shadowColor.g},${shadowColor.b})`, 0.26);

			const reflectionColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmax * 0.58));
			const reflectionLevel = clamp01(this.cfg.reflection * (0.3 + motion * 0.22));
			for (let i = 0; i < hullRows.length; i++) {
				const row = hullRows[i];
				const distance = hullBaseY - row.y + 1;
				const wobble = Math.round(Math.sin(this.tick * 0.08 + i * 0.8 + row.startX * 0.03) * (0.5 + motion * 0.45));
				const reflY = hullBaseY + distance + Math.round(Math.sin(row.startX * this.cfg.wave_freq + phase) * motion * 0.35);
				const reflWidth = Math.max(2, row.width - 1 - Math.floor(distance / 2));
				const alpha = clamp01(reflectionLevel * (0.4 - distance * 0.045));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, row.startX + wobble, reflY, reflWidth, 1, `rgb(${reflectionColor.r},${reflectionColor.g},${reflectionColor.b})`, alpha);
			}

			const rippleColor = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmax * 0.98));
			const wakeGain = this.timers.wake > 0 ? (this.values.wake_gain || this.cfg.wake_mult) : 1;
			const rippleBands = 4 + Math.round(this.cfg.ripple * 7);
			for (let band = 0; band < rippleBands; band++) {
				const centerX = Math.round(boatX + boatLen * 0.18 + band * (1.2 + wakeGain * 0.28));
				const half = Math.max(3, Math.round(boatLen * (0.24 + band * 0.14 + wakeGain * 0.03)));
				const centerY = waterline + Math.round(0.8 + band * 0.75 + Math.abs(Math.sin(phase * 4.2 + band * 0.9)) * 1.1);
				for (let dx = -half; dx <= half; dx++) {
					if ((dx + band + this.tick) % 2 !== 0) continue;
					const edge = 1 - Math.abs(dx) / Math.max(1, half);
					const waveY = centerY + Math.round(Math.sin(dx * 0.22 + this.tick * 0.08 + band * 0.7) * 0.7);
					const alpha = clamp01((0.08 + this.cfg.ripple * 0.24 * motion) * Math.pow(edge, 0.45));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, waveY, 1, 1, `rgb(${rippleColor.r},${rippleColor.g},${rippleColor.b})`, alpha);
				}
			}

			for (let band = 0; band < 3; band++) {
				const half = Math.max(2, Math.round(boatLen * (0.16 + band * 0.08)));
				const centerX = Math.round(boatX - boatLen * 0.2 - band * 1.2);
				const centerY = waterline + Math.round(band * 0.85 + Math.abs(Math.sin(phase * 3.6 + band)) * 0.9);
				for (let dx = -half; dx <= half; dx++) {
					if ((dx + band) % 2 !== 0) continue;
					const edge = 1 - Math.abs(dx) / Math.max(1, half);
					const alpha = clamp01((0.03 + this.cfg.ripple * 0.12 * motion) * Math.pow(edge, 0.65));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, centerY + Math.round(Math.sin(dx * 0.3 + this.tick * 0.06) * 0.5), 1, 1, `rgb(${rippleColor.r},${rippleColor.g},${rippleColor.b})`, alpha);
				}
			}
		}

		_renderUnderwater(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const top = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.58), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.54));
				const mid = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.28));
				const deep = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin * (0.72 - this.cfg.depth * 0.18)));
				const water = ctx.createLinearGradient(0, 0, 0, canvasH);
				water.addColorStop(0, `rgb(${top.r},${top.g},${top.b})`);
				water.addColorStop(0.46, `rgb(${mid.r},${mid.g},${mid.b})`);
				water.addColorStop(1, `rgb(${deep.r},${deep.g},${deep.b})`);
				ctx.fillStyle = water;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const level = this._sceneLevelUnderwater();
			const currentPush = this.timers['current-shift'] > 0 ? (this.values.current_push || this.cfg.current_shift_push) : 0;
			const floorBase = Math.max(Math.floor(this.h * 0.78), Math.min(this.h - 4, Math.floor(this.h * (0.84 + this.cfg.depth * 0.08))));
			const phase = this.tick * this.cfg.rise_speed * 0.04;

			const surfaceGlow = ctx.createRadialGradient(canvasW * 0.32, 0, 0, canvasW * 0.32, 0, Math.max(canvasW, canvasH) * 0.52);
			surfaceGlow.addColorStop(0, `rgba(210, 244, 238, ${clamp01(0.08 + this.cfg.caustics * 0.18 * level)})`);
			surfaceGlow.addColorStop(1, 'rgba(210, 244, 238, 0)');
			ctx.fillStyle = surfaceGlow;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const beamCount = 4;
			for (let i = 0; i < beamCount; i++) {
				const sourceX = canvasW * (0.08 + i * 0.24 + this._hash(30100 + i) * 0.08);
				const spread = canvasW * (0.08 + this._hash(30200 + i) * 0.08);
				const bend = (currentPush * 12 + Math.sin(this.tick * 0.02 + i) * 10) * this.cfg.caustics;
				ctx.fillStyle = `rgba(210, 248, 242, ${clamp01(0.04 + this.cfg.caustics * 0.12 * level)})`;
				ctx.beginPath();
				ctx.moveTo(sourceX - spread * 0.2, 0);
				ctx.lineTo(sourceX + spread * 0.22, 0);
				ctx.lineTo(sourceX + spread + bend, canvasH * 0.7);
				ctx.lineTo(sourceX - spread * 0.65 + bend * 0.45, canvasH * 0.7);
				ctx.closePath();
				ctx.fill();
			}

			const causticColor = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmax * 0.96));
			for (let band = 0; band < 5; band++) {
				const baseY = Math.floor(this.h * (0.16 + band * 0.09));
				for (let x = 0; x < this.w; x++) {
					if ((x + band) % 2 !== 0) continue;
					const wave = Math.sin(x * 0.16 + phase * (1.2 + band * 0.18) + band) + Math.sin(x * 0.07 - phase * 1.4 + band * 1.7);
					const row = baseY + Math.round(wave * this.cfg.caustics * level * 1.3);
					const alpha = clamp01((0.03 + this.cfg.caustics * 0.18 * level) * (0.82 - band * 0.12));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${causticColor.r},${causticColor.g},${causticColor.b})`, alpha);
				}
			}

			const particulateColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmax * 0.8));
			const particulateCount = Math.max(18, Math.round(this.w * 0.14));
			for (let i = 0; i < particulateCount; i++) {
				const col = Math.floor(this._hash(30300 + i) * this.w);
				const row = Math.floor(this._hash(30400 + i) * Math.max(1, floorBase - 6));
				const blink = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.018 + i * 0.6), 2);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${particulateColor.r},${particulateColor.g},${particulateColor.b})`, clamp01((0.04 + this.cfg.depth * 0.08) * blink));
			}

			const seabedPoints = [];
			const seabedSegments = 8;
			for (let i = 0; i <= seabedSegments; i++) {
				seabedPoints.push(Math.round(floorBase - Math.abs(Math.sin(i * 0.8 + this._hash(30500 + i) * 2.4)) * 2 - this._hash(30600 + i) * 2));
			}
			const seabedColor = hslToRGB((this.cfg.hue + 36) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 0.85));
			ctx.fillStyle = `rgb(${seabedColor.r},${seabedColor.g},${seabedColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * seabedSegments;
				const idx = Math.min(seabedSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const row = seabedPoints[idx] + (seabedPoints[idx + 1] - seabedPoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(row * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const weedColor = hslToRGB((this.cfg.hue - 36 + 360) % 360, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.28));
			const weedAccent = hslToRGB((this.cfg.hue - 18 + 360) % 360, clamp01(this.cfg.sat * 0.48), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.4));
			const weedCount = Math.max(4, Math.round(this.cfg.weed_count));
			for (let i = 0; i < weedCount; i++) {
				const baseX = Math.floor((i + 0.35) * this.w / weedCount + (this._hash(30700 + i) - 0.5) * 5);
				const rootY = floorBase - 1 - Math.floor(this._hash(30800 + i) * 3);
				const fronds = 2 + Math.floor(this._hash(30900 + i) * 2);
				for (let f = 0; f < fronds; f++) {
					const height = Math.max(7, Math.round(this.cfg.weed_height * (0.58 + this._hash(31000 + i * 5 + f) * 0.5)));
					const offset = (f - (fronds - 1) / 2) * 1.2;
					const localPhase = this.tick * 0.035 * (0.8 + this._hash(31100 + i * 5 + f) * 0.4) + i * 0.7 + f * 0.4;
					for (let seg = 0; seg < height; seg++) {
						const progress = seg / Math.max(1, height - 1);
						const sway = Math.sin(localPhase + progress * 2.6) * this.cfg.sway * level * (1.1 + Math.abs(currentPush) * 0.55) + currentPush * progress * 1.4;
						const x = Math.round(baseX + offset + sway * progress * 1.2);
						const y = rootY - seg;
						const color = seg < height * 0.28 ? weedColor : weedAccent;
						const alpha = clamp01(0.3 + progress * 0.42);
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, seg < height * 0.25 ? 2 : 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					}
				}
			}

			const burstGain = this.timers['bubble-burst'] > 0 ? (this.values.bubble_gain || this.cfg.bubble_burst_mult) : 1;
			const bubbleDensity = Math.max(0.04, this.cfg.density * level);
			const bubbleCount = Math.max(12, Math.round(this.w * bubbleDensity * (0.44 + Math.max(0, burstGain - 1) * 0.18)));
			const bubbleColor = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 0.98));
			for (let i = 0; i < bubbleCount; i++) {
				const baseX = this._hash(31200 + i) * this.w;
				const baseY = this._hash(31300 + i) * Math.max(6, floorBase - 6);
				const rise = baseY - this.tick * this.cfg.rise_speed * (0.55 + this._hash(31400 + i) * 0.7);
				const row = 1 + positiveMod(rise, Math.max(1, floorBase - 5));
				const drift = this.cfg.drift * (0.4 + this._hash(31500 + i) * 0.9) + currentPush * 0.05 * (0.45 + this._hash(31600 + i) * 0.55);
				const wobble = Math.sin(this.tick * 0.03 + i * 0.72) * this.cfg.sway * (0.6 + this._hash(31700 + i) * 0.5);
				const col = positiveMod(baseX + this.tick * drift + wobble, this.w);
				const size = this._hash(31800 + i) > 0.82 ? 2 : 1;
				const alpha = clamp01((0.22 + this._hash(31900 + i) * 0.28) * (0.8 + Math.max(0, burstGain - 1) * 0.14));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${bubbleColor.r},${bubbleColor.g},${bubbleColor.b})`, alpha);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), 1, 1, `rgb(235,248,246)`, clamp01(alpha * 0.62));
			}
		}

		_renderAurora(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#02060f');
				sky.addColorStop(0.52, '#07101c');
				sky.addColorStop(1, '#0a1220');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.82);
			const intensity = this._intensityLevelAurora();
			const bands = Math.max(1, Math.round(this.cfg.bands));
			const shiftPush = this.values.shift_push || 0;
			const shiftSeed = this.values.shift_seed || 0;

			const horizonGlow = ctx.createLinearGradient(0, canvasH * 0.5, 0, canvasH);
			horizonGlow.addColorStop(0, 'rgba(36, 84, 92, 0)');
			horizonGlow.addColorStop(1, `rgba(48, 168, 140, ${clamp01(0.18 + intensity * 0.16)})`);
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, canvasH * 0.48, canvasW, canvasH * 0.52);

			const baseGround = hslToRGB(212, 0.2, 0.04);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, groundRow - 1, this.w, this.h - groundRow + 1, `rgb(${baseGround.r},${baseGround.g},${baseGround.b})`, 1);

			const starCount = Math.max(12, Math.round(this.w * 0.18));
			for (let i = 0; i < starCount; i++) {
				const col = Math.floor(this._hash(19000 + i) * this.w);
				const row = Math.floor(this._hash(19100 + i) * Math.max(1, groundRow - 10));
				const twinkle = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * (0.018 + this._hash(19200 + i) * 0.02) + i), 2);
				const alpha = clamp01((0.14 + twinkle * 0.22) * (1 - Math.min(0.65, intensity * 0.55)));
				const color = hslToRGB(205 + this._hash(19300 + i) * 18, 0.18, 0.72 + this._hash(19400 + i) * 0.2);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
			}

			const ridgePoints = [];
			const ridgeSegments = 7;
			const ridgeColor = hslToRGB(210, 0.24, 0.055);
			for (let i = 0; i <= ridgeSegments; i++) {
				ridgePoints.push(groundRow - 4 - Math.floor(this._hash(19500 + i) * 6) - Math.floor((0.5 + 0.5 * Math.sin(i * 1.3 + this._hash(19600 + i) * 4)) * 4));
			}
			const ridgeCoords = [];
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const ridge = Math.round(ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased + Math.sin(x * 0.08 + shiftSeed) * 0.8);
				ridgeCoords.push({ x, ridge });
			}
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (const point of ridgeCoords) {
				ctx.lineTo(Math.floor(point.x * sx), Math.floor(point.ridge * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const treeCount = 12;
			for (let i = 0; i < treeCount; i++) {
				const center = Math.floor((i + 0.5) * this.w / treeCount + (this._hash(19900 + i) - 0.5) * 5);
				const trunkH = 1 + Math.floor(this._hash(20000 + i) * 2);
				const crownH = 5 + Math.floor(this._hash(20100 + i) * 5);
				const half = 1 + Math.floor(this._hash(20200 + i) * 2);
				const baseY = groundRow - 1 - Math.floor(this._hash(20300 + i) * 4);
				const treeColor = hslToRGB(210 + this._hash(20400 + i) * 10, 0.22, 0.045 + this._hash(20500 + i) * 0.02);
				for (let row = 0; row < crownH; row++) {
					const width = Math.max(1, half - Math.floor(row / 2));
					const y = baseY - crownH + row;
					for (let dx = -width; dx <= width; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center + dx, y, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
					}
				}
				for (let row = 0; row < trunkH; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center, baseY - row, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
				}
			}

			for (let band = 0; band < bands; band++) {
				const bandRatio = bands === 1 ? 0.5 : band / (bands - 1);
				const phase = this.tick * this.cfg.speed * (0.18 + bandRatio * 0.12) + band * 1.6 + shiftSeed;
				const amp = this.cfg.wave_amp * (0.8 + bandRatio * 0.35);
				const freq = this.cfg.wave_freq * (0.82 + bandRatio * 0.26);
				const thickness = this.cfg.thickness * (0.8 + bandRatio * 0.28);
				const curtain = this.cfg.curtain_len * (0.8 + bandRatio * 0.22);
				const baseY = this.h * (0.16 + bandRatio * 0.08) + Math.sin(this.tick * 0.01 + band * 0.8) * 1.1;
				const hueBase = ((this.cfg.hue + (bandRatio - 0.5) * this.cfg.hue_sp * 1.15 + shiftPush * 5) % 360 + 360) % 360;

				for (let x = 0; x < this.w; x++) {
					const nx = x / Math.max(1, this.w - 1);
					const arch = Math.sin(nx * Math.PI * (1.08 + bandRatio * 0.24) + band * 0.75);
					const wave = Math.sin(x * freq + phase + this.tick * this.cfg.drift * 0.04);
					const subWave = Math.sin(x * freq * 0.47 - phase * 0.62 + band * 2.1);
					const center = baseY + arch * amp * 0.72 + wave * amp * 0.52 + subWave * amp * 0.22 + shiftPush * Math.sin(x * 0.07 + phase) * 1.05;
					const startY = Math.max(0, Math.floor(center - thickness * 1.15));
					const endY = Math.min(groundRow - 2, Math.ceil(center + curtain));
					for (let y = startY; y <= endY; y++) {
						const dy = y - center;
						const core = Math.exp(-(dy * dy) / Math.max(1, thickness * thickness * 1.4));
						const tail = y >= center ? Math.exp(-(y - center) / Math.max(1, curtain)) : 0;
						const shimmer = 0.76 + 0.24 * Math.sin(this.tick * 0.03 + x * 0.1 + band * 1.7);
						const strength = (core * 0.9 + tail * 0.7) * intensity * shimmer * (0.58 + 0.42 * Math.max(0.2, arch));
						if (strength < 0.025) continue;
						const hue = ((hueBase + Math.sin(y * 0.18 + x * 0.06 + phase) * this.cfg.hue_sp * 0.32 + band * 6) % 360 + 360) % 360;
						const sat = clamp01(this.cfg.sat * (0.84 + core * 0.28));
						const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * Math.min(1, 0.24 + strength));
						const color = hslToRGB(hue, sat, light);
						const alpha = clamp01(strength * (0.34 + core * 0.46));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
						if (core > 0.62 && y < groundRow - 3) {
							const accent = hslToRGB((hue + 12) % 360, clamp01(sat * 0.9), clamp01(light * 1.08));
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${accent.r},${accent.g},${accent.b})`, alpha * 0.45);
						}
					}
				}
			}
		}
	}

	// Expose helpers and the ProceduralScene base on the namespace so per-
	// effect files can pull them out of api._helpers / api._ProceduralScene
	// at the top of their own IIFE. positiveMod is included because Burning-
	// Trees (and ProceduralScene itself) use it.
	api._helpers = { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod };
	api._ProceduralScene = ProceduralScene;
	api._applyProceduralDefaults = applyProceduralDefaults;
	api.subscribe = subscribe;
	api.EffectTransition = EffectTransition;

	// Back-compat: hslToRGB used to be a top-level field on the
	// AmbienceSim export object. Keep it reachable so any external caller
	// using the old name still works.
	api.hslToRGB = hslToRGB;
})(window.AmbienceSim);
