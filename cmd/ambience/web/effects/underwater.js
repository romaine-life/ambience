'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_reveal = clamp01(c.intro_reveal);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_murk = clamp01(c.ending_murk);
		if (c.density <= 0) c.density = DEFAULTS.density;
		if (c.rise_speed <= 0) c.rise_speed = DEFAULTS.rise_speed;
		if (c.sway <= 0) c.sway = DEFAULTS.sway;
		if (c.weed_height <= 0) c.weed_height = DEFAULTS.weed_height;
		if (c.weed_count < 1) c.weed_count = DEFAULTS.weed_count;
		if (c.caustics <= 0) c.caustics = DEFAULTS.caustics;
		if (c.depth <= 0) c.depth = DEFAULTS.depth;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.bubble_burst_dur <= 0) c.bubble_burst_dur = DEFAULTS.bubble_burst_dur;
		if (c.bubble_burst_mult <= 0) c.bubble_burst_mult = DEFAULTS.bubble_burst_mult;
		if (c.current_shift_dur <= 0) c.current_shift_dur = DEFAULTS.current_shift_dur;
		if (c.current_shift_push <= 0) c.current_shift_push = DEFAULTS.current_shift_push;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class Underwater {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.seed = Number(seed || Date.now());
			this.tick = 0;
			this.timers = {};
			this.values = {};
			this.cfg = applyDefaults(cfg);
		}

		setConfig(cfg) {
			this.cfg = applyDefaults(Object.assign({}, this.cfg, cfg));
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

		triggerEvent(name) {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers['bubble-burst'] || this.timers['bubble-burst'] <= 0) this.values.bubble_gain = 1;
			if (!this.timers['current-shift'] || this.timers['current-shift'] <= 0) this.values.current_push = 0;
			api._helpers.paintProceduralGrid(this);
		}

		_sceneLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['underwater'] = [
		{
			key: 'quiet-shallows',
			label: 'quiet shallows',
			config: {
				intro_reveal: 0.18,
				ending_murk: 0.12,
				density: 0.18,
				rise_speed: 0.32,
				drift: 0.04,
				sway: 0.34,
				weed_height: 16,
				weed_count: 9,
				caustics: 0.44,
				depth: 0.28,
				hue: 184,
				hue_sp: 12,
				sat: 0.38,
				lmin: 0.14,
				lmax: 0.86,
				calm_p: 0.0011,
			},
		},
		{
			key: 'bubble-field',
			label: 'bubble field',
			config: {
				density: 0.42,
				rise_speed: 0.54,
				drift: 0.08,
				sway: 0.46,
				weed_height: 18,
				weed_count: 8,
				caustics: 0.26,
				depth: 0.42,
				hue: 190,
				hue_sp: 16,
				sat: 0.42,
				lmin: 0.12,
				lmax: 0.82,
				bubble_burst_p: 0.0012,
			},
		},
		{
			key: 'slow-current',
			label: 'slow current',
			config: {
				density: 0.28,
				rise_speed: 0.4,
				drift: 0.12,
				sway: 0.78,
				weed_height: 22,
				weed_count: 11,
				caustics: 0.3,
				depth: 0.56,
				hue: 192,
				hue_sp: 18,
				sat: 0.42,
				lmin: 0.12,
				lmax: 0.82,
				current_shift_p: 0.0011,
			},
		},
		{
			key: 'deep-water',
			label: 'deep water',
			config: {
				intro_reveal: 0.1,
				ending_murk: 0.16,
				density: 0.16,
				rise_speed: 0.28,
				drift: 0.05,
				sway: 0.26,
				weed_height: 13,
				weed_count: 6,
				caustics: 0.14,
				depth: 0.82,
				hue: 204,
				hue_sp: 10,
				sat: 0.3,
				lmin: 0.08,
				lmax: 0.62,
				calm_p: 0.0014,
				calm_mult: 0.42,
			},
		},
	];
	api.effects['underwater'] = Underwater;
})(window.AmbienceSim);
