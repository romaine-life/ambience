'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_tide = clamp01(c.intro_tide);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_wet = clamp01(c.ending_wet);
		if (c.shoreline <= 0) c.shoreline = DEFAULTS.shoreline;
		if (c.tide_amp <= 0) c.tide_amp = DEFAULTS.tide_amp;
		if (c.wave_amp <= 0) c.wave_amp = DEFAULTS.wave_amp;
		if (c.wave_freq <= 0) c.wave_freq = DEFAULTS.wave_freq;
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.foam <= 0) c.foam = DEFAULTS.foam;
		if (c.shimmer <= 0) c.shimmer = DEFAULTS.shimmer;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.high_tide_dur <= 0) c.high_tide_dur = DEFAULTS.high_tide_dur;
		if (c.high_tide_push <= 0) c.high_tide_push = DEFAULTS.high_tide_push;
		if (c.low_tide_dur <= 0) c.low_tide_dur = DEFAULTS.low_tide_dur;
		if (c.low_tide_pull <= 0) c.low_tide_pull = DEFAULTS.low_tide_pull;
		if (c.foam_burst_dur <= 0) c.foam_burst_dur = DEFAULTS.foam_burst_dur;
		if (c.foam_burst_mult <= 0) c.foam_burst_mult = DEFAULTS.foam_burst_mult;
		return c;
	}

	class Beach {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if ((!this.timers['high-tide'] || this.timers['high-tide'] <= 0) && (!this.timers['low-tide'] || this.timers['low-tide'] <= 0)) {
				this.values.tide_bias = 0;
			}
			if (!this.timers['foam-burst'] || this.timers['foam-burst'] <= 0) {
				this.values.foam_gain = 1;
			}
			api._helpers.paintProceduralGrid(this);
		}

		_tideLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['beach'] = [
		{
			key: 'still-shore',
			label: 'still shore',
			config: {
				shoreline: 0.56,
				tide_amp: 3.2,
				wave_amp: 1.3,
				wave_freq: 0.14,
				speed: 0.05,
				slope: 0.08,
				foam: 0.24,
				shimmer: 0.18,
				hue: 196,
				hue_sp: 10,
				sat: 0.42,
				lmin: 0.26,
				lmax: 0.78,
			},
		},
		{
			key: 'gentle-tide',
			label: 'gentle tide',
			config: {
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
				high_tide_p: 0.0008,
				low_tide_p: 0.0006,
			},
		},
		{
			key: 'foamy-edge',
			label: 'foamy edge',
			config: {
				shoreline: 0.6,
				tide_amp: 7.4,
				wave_amp: 3.1,
				wave_freq: 0.21,
				speed: 0.12,
				slope: 0.2,
				foam: 0.5,
				shimmer: 0.18,
				hue: 194,
				hue_sp: 18,
				sat: 0.54,
				lmin: 0.3,
				lmax: 0.84,
				high_tide_p: 0.0012,
				foam_burst_p: 0.0013,
				foam_burst_mult: 2.2,
			},
		},
		{
			key: 'wide-beach',
			label: 'wide beach',
			config: {
				shoreline: 0.52,
				tide_amp: 4.8,
				wave_amp: 1.8,
				wave_freq: 0.12,
				speed: 0.08,
				slope: -0.1,
				foam: 0.3,
				shimmer: 0.28,
				hue: 202,
				hue_sp: 14,
				sat: 0.44,
				lmin: 0.24,
				lmax: 0.78,
				low_tide_p: 0.0011,
			},
		},
	];
	api.effects['beach'] = Beach;
})(window.AmbienceSim);
