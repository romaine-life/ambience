'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_turn = clamp01(c.intro_turn);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_turn = clamp01(c.ending_turn);
		if (c.turn_speed <= 0) c.turn_speed = DEFAULTS.turn_speed;
		if (c.blade_len <= 0) c.blade_len = DEFAULTS.blade_len;
		if (c.blade_width <= 0) c.blade_width = DEFAULTS.blade_width;
		if (c.tower_height <= 0) c.tower_height = DEFAULTS.tower_height;
		if (c.tower_width <= 0) c.tower_width = DEFAULTS.tower_width;
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.gust_dur <= 0) c.gust_dur = DEFAULTS.gust_dur;
		if (c.gust_mult <= 0) c.gust_mult = DEFAULTS.gust_mult;
		if (c.lull_dur <= 0) c.lull_dur = DEFAULTS.lull_dur;
		if (c.lull_mult <= 0) c.lull_mult = DEFAULTS.lull_mult;
		return c;
	}

	class Windmill {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_gain = 1;
			api._helpers.paintProceduralGrid(this);
		}

		_rotationLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['windmill'] = [
		{
			key: 'still-dusk',
			label: 'still dusk',
			config: {
				intro_turn: 0.08,
				ending_turn: 0.04,
				turn_speed: 0.04,
				blade_len: 12,
				blade_width: 1.6,
				tower_height: 19,
				tower_width: 5.5,
				horizon: 0.74,
				glow: 0.22,
				hue: 26,
				hue_sp: 14,
				sat: 0.38,
				lmin: 0.16,
				lmax: 0.78,
			},
		},
		{
			key: 'steady-turning',
			label: 'steady turning',
			config: {
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
				gust_p: 0.0006,
			},
		},
		{
			key: 'windy-hill',
			label: 'windy hill',
			config: {
				turn_speed: 0.12,
				blade_len: 15,
				blade_width: 2.1,
				tower_height: 21,
				tower_width: 6.5,
				horizon: 0.7,
				glow: 0.14,
				hue: 24,
				hue_sp: 20,
				sat: 0.4,
				lmin: 0.16,
				lmax: 0.8,
				gust_p: 0.0014,
				gust_mult: 2.2,
				gust_dur: 62,
			},
		},
		{
			key: 'silhouette-mill',
			label: 'silhouette mill',
			config: {
				turn_speed: 0.06,
				blade_len: 16,
				blade_width: 1.5,
				tower_height: 23,
				tower_width: 5,
				horizon: 0.76,
				glow: 0.1,
				hue: 222,
				hue_sp: 12,
				sat: 0.22,
				lmin: 0.12,
				lmax: 0.68,
				lull_p: 0.0012,
				lull_mult: 0.38,
			},
		},
	];
	api.effects['windmill'] = Windmill;
})(window.AmbienceSim);
