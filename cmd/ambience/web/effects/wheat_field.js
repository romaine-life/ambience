'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_breeze = clamp01(c.intro_breeze);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_sway = clamp01(c.ending_sway);
		if (c.density <= 0) c.density = DEFAULTS.density;
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.sway <= 0) c.sway = DEFAULTS.sway;
		if (c.wave_freq <= 0) c.wave_freq = DEFAULTS.wave_freq;
		if (c.field_top <= 0) c.field_top = DEFAULTS.field_top;
		if (c.stalk_h <= 0) c.stalk_h = DEFAULTS.stalk_h;
		if (c.layers < 1) c.layers = DEFAULTS.layers;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.gust_dur <= 0) c.gust_dur = DEFAULTS.gust_dur;
		if (c.gust_mult <= 0) c.gust_mult = DEFAULTS.gust_mult;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class WheatField {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_push = 0;
			api._helpers.paintProceduralGrid(this);
		}

		_motionLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['wheat-field'] = [
		{
			key: 'still-evening',
			label: 'still evening',
			config: {
				density: 0.4,
				speed: 0.07,
				drift: 0.05,
				sway: 0.34,
				wave_freq: 0.14,
				field_top: 0.64,
				stalk_h: 17,
				layers: 2,
				hue: 42,
				hue_sp: 12,
				sat: 0.56,
				lmin: 0.28,
				lmax: 0.7,
				calm_p: 0.001,
			},
		},
		{
			key: 'gentle-breeze',
			label: 'gentle breeze',
			config: {
				density: 0.48,
				speed: 0.12,
				drift: 0.14,
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
				gust_p: 0.0008,
			},
		},
		{
			key: 'rolling-field',
			label: 'rolling field',
			config: {
				density: 0.56,
				speed: 0.16,
				drift: 0.2,
				sway: 0.88,
				wave_freq: 0.16,
				field_top: 0.6,
				stalk_h: 20,
				layers: 3,
				hue: 48,
				hue_sp: 20,
				sat: 0.68,
				lmin: 0.3,
				lmax: 0.8,
				gust_p: 0.0012,
				gust_mult: 2.15,
			},
		},
		{
			key: 'windy-harvest',
			label: 'windy harvest',
			config: {
				density: 0.62,
				speed: 0.2,
				drift: 0.28,
				sway: 1.02,
				wave_freq: 0.21,
				field_top: 0.59,
				stalk_h: 22,
				layers: 4,
				hue: 44,
				hue_sp: 24,
				sat: 0.72,
				lmin: 0.32,
				lmax: 0.84,
				gust_p: 0.0016,
				gust_mult: 2.45,
				gust_dur: 66,
			},
		},
	];
	api.effects['wheat-field'] = WheatField;
})(window.AmbienceSim);
