'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_density = clamp01(c.intro_density);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_density = clamp01(c.ending_density);
		if (c.density <= 0) c.density = DEFAULTS.density;
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.layers < 1) c.layers = DEFAULTS.layers;
		if (c.size <= 0) c.size = DEFAULTS.size;
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
		if (c.swirl_dur <= 0) c.swirl_dur = DEFAULTS.swirl_dur;
		if (c.swirl_pull <= 0) c.swirl_pull = DEFAULTS.swirl_pull;
		return c;
	}

	class AutumnLeaves {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_push = 0;
			if (!this.timers.swirl || this.timers.swirl <= 0) this.values.swirl_spin = 0;
			api._helpers.paintProceduralGrid(this);
		}

		_densityLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['autumn-leaves'] = [
		{
			key: 'few-leaves',
			label: 'few leaves',
			config: {
				density: 0.14,
				speed: 0.36,
				drift: 0.12,
				sway: 0.7,
				layers: 1,
				size: 1,
				hue: 24,
				hue_sp: 18,
				sat: 0.58,
				lmin: 0.36,
				lmax: 0.7,
				lull_p: 0.0014,
			},
		},
		{
			key: 'gentle-fall',
			label: 'gentle fall',
			config: {
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
				gust_p: 0.0008,
			},
		},
		{
			key: 'windy-autumn',
			label: 'windy autumn',
			config: {
				density: 0.3,
				speed: 0.5,
				drift: 0.26,
				sway: 1.05,
				layers: 2,
				size: 1.4,
				hue: 22,
				hue_sp: 28,
				sat: 0.68,
				lmin: 0.36,
				lmax: 0.8,
				gust_p: 0.0016,
				gust_mult: 2.35,
			},
		},
		{
			key: 'swirl-study',
			label: 'swirl study',
			config: {
				density: 0.28,
				speed: 0.42,
				drift: 0.12,
				sway: 1.15,
				layers: 2,
				size: 1.4,
				hue: 30,
				hue_sp: 34,
				sat: 0.7,
				lmin: 0.4,
				lmax: 0.84,
				swirl_p: 0.0015,
				swirl_dur: 68,
				swirl_pull: 1.55,
			},
		},
	];
	api.effects['autumn-leaves'] = AutumnLeaves;
})(window.AmbienceSim);
