'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.intensity <= 0) c.intensity = DEFAULTS.intensity;
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.bands < 1) c.bands = DEFAULTS.bands;
		if (c.thickness <= 0) c.thickness = DEFAULTS.thickness;
		if (c.wave_amp <= 0) c.wave_amp = DEFAULTS.wave_amp;
		if (c.wave_freq <= 0) c.wave_freq = DEFAULTS.wave_freq;
		if (c.curtain_len <= 0) c.curtain_len = DEFAULTS.curtain_len;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.brighten_dur <= 0) c.brighten_dur = DEFAULTS.brighten_dur;
		if (c.brighten_mult <= 0) c.brighten_mult = DEFAULTS.brighten_mult;
		if (c.shift_dur <= 0) c.shift_dur = DEFAULTS.shift_dur;
		if (c.shift_amt <= 0) c.shift_amt = DEFAULTS.shift_amt;
		if (c.fade_dur <= 0) c.fade_dur = DEFAULTS.fade_dur;
		if (c.fade_mult <= 0) c.fade_mult = DEFAULTS.fade_mult;
		return c;
	}

	class Aurora {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.brighten || this.timers.brighten <= 0) this.values.brighten_gain = 0;
			if (!this.timers.shift || this.timers.shift <= 0) {
				this.values.shift_push = 0;
				this.values.shift_seed = 0;
			}
			api._helpers.paintProceduralGrid(this);
		}

		_intensityLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['aurora'] = [
		{
			key: 'green-veil',
			label: 'green veil',
			config: {
				intensity: 0.54,
				speed: 0.1,
				drift: 0.06,
				bands: 3,
				thickness: 9,
				wave_amp: 5.5,
				wave_freq: 0.15,
				curtain_len: 14,
				hue: 134,
				hue_sp: 18,
				sat: 0.7,
				lmin: 0.2,
				lmax: 0.72,
				shift_p: 0.0007,
			},
		},
		{
			key: 'cold-ribbons',
			label: 'cold ribbons',
			config: {
				intensity: 0.48,
				speed: 0.12,
				drift: 0.1,
				bands: 4,
				thickness: 7.5,
				wave_amp: 6.5,
				wave_freq: 0.18,
				curtain_len: 13,
				hue: 164,
				hue_sp: 34,
				sat: 0.66,
				lmin: 0.18,
				lmax: 0.76,
				shift_p: 0.0011,
				fade_p: 0.0005,
			},
		},
		{
			key: 'quiet-sky',
			label: 'quiet sky',
			config: {
				intensity: 0.34,
				speed: 0.07,
				drift: 0.03,
				bands: 2,
				thickness: 8.5,
				wave_amp: 4.5,
				wave_freq: 0.12,
				curtain_len: 11,
				hue: 142,
				hue_sp: 14,
				sat: 0.58,
				lmin: 0.16,
				lmax: 0.64,
				fade_p: 0.0008,
			},
		},
		{
			key: 'bright-aurora',
			label: 'bright aurora',
			config: {
				intensity: 0.72,
				speed: 0.14,
				drift: 0.12,
				bands: 4,
				thickness: 10,
				wave_amp: 7.2,
				wave_freq: 0.19,
				curtain_len: 18,
				hue: 136,
				hue_sp: 30,
				sat: 0.78,
				lmin: 0.22,
				lmax: 0.82,
				brighten_p: 0.0012,
				brighten_mult: 1.7,
				shift_p: 0.001,
			},
		},
	];
	api.effects['aurora'] = Aurora;
})(window.AmbienceSim);
