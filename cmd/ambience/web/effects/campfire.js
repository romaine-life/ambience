'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.flame_height <= 0) c.flame_height = DEFAULTS.flame_height;
		if (c.flame_width <= 0) c.flame_width = DEFAULTS.flame_width;
		if (c.flame_speed <= 0) c.flame_speed = DEFAULTS.flame_speed;
		if (c.flicker <= 0) c.flicker = DEFAULTS.flicker;
		if (c.ember_rate <= 0) c.ember_rate = DEFAULTS.ember_rate;
		if (c.ember_speed <= 0) c.ember_speed = DEFAULTS.ember_speed;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.crackle_dur <= 0) c.crackle_dur = DEFAULTS.crackle_dur;
		if (c.crackle_mult <= 0) c.crackle_mult = DEFAULTS.crackle_mult;
		if (c.lull_dur <= 0) c.lull_dur = DEFAULTS.lull_dur;
		if (c.lull_mult <= 0) c.lull_mult = DEFAULTS.lull_mult;
		return c;
	}

	class Campfire {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.crackle || this.timers.crackle <= 0) this.values.crackle_gain = 1;
			api._helpers.paintProceduralGrid(this);
		}

		_flameLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['campfire'] = [
		{
			key: 'small-fire',
			label: 'small fire',
			config: {
				flame_height: 9,
				flame_width: 7,
				flame_speed: 0.1,
				flicker: 0.56,
				ember_rate: 0.18,
				ember_speed: 0.52,
				glow: 0.4,
				hue: 22,
				hue_sp: 12,
				sat: 0.76,
				lmin: 0.28,
				lmax: 0.88,
			},
		},
		{
			key: 'steady-campfire',
			label: 'steady campfire',
			config: {
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
				crackle_p: 0.0008,
			},
		},
		{
			key: 'crackling-fire',
			label: 'crackling fire',
			config: {
				flame_height: 16,
				flame_width: 11,
				flame_speed: 0.15,
				flicker: 0.92,
				ember_rate: 0.34,
				ember_speed: 0.78,
				glow: 0.62,
				hue: 21,
				hue_sp: 22,
				sat: 0.88,
				lmin: 0.34,
				lmax: 0.96,
				crackle_p: 0.0015,
				crackle_mult: 2.15,
				crackle_dur: 48,
			},
		},
		{
			key: 'late-embers',
			label: 'late embers',
			config: {
				intro_glow: 0.1,
				ending_glow: 0.14,
				flame_height: 8,
				flame_width: 8,
				flame_speed: 0.08,
				flicker: 0.42,
				ember_rate: 0.3,
				ember_speed: 0.48,
				glow: 0.34,
				hue: 18,
				hue_sp: 14,
				sat: 0.68,
				lmin: 0.24,
				lmax: 0.8,
				lull_p: 0.0014,
				lull_mult: 0.42,
			},
		},
	];
	api.effects['campfire'] = Campfire;
})(window.AmbienceSim);
