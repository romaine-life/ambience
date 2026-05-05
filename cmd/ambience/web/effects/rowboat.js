'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_drift = clamp01(c.intro_drift);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_ripple = clamp01(c.ending_ripple);
		if (c.waterline <= 0) c.waterline = DEFAULTS.waterline;
		if (c.drift_speed <= 0) c.drift_speed = DEFAULTS.drift_speed;
		if (c.bob_amp <= 0) c.bob_amp = DEFAULTS.bob_amp;
		if (c.wave_amp <= 0) c.wave_amp = DEFAULTS.wave_amp;
		if (c.wave_freq <= 0) c.wave_freq = DEFAULTS.wave_freq;
		if (c.ripple <= 0) c.ripple = DEFAULTS.ripple;
		if (c.reflection <= 0) c.reflection = DEFAULTS.reflection;
		if (c.boat_len <= 0) c.boat_len = DEFAULTS.boat_len;
		if (c.boat_height <= 0) c.boat_height = DEFAULTS.boat_height;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.wake_dur <= 0) c.wake_dur = DEFAULTS.wake_dur;
		if (c.wake_mult <= 0) c.wake_mult = DEFAULTS.wake_mult;
		if (c.drift_dur <= 0) c.drift_dur = DEFAULTS.drift_dur;
		if (c.drift_push <= 0) c.drift_push = DEFAULTS.drift_push;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class Rowboat {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.wake || this.timers.wake <= 0) this.values.wake_gain = 1;
			if (!this.timers.drift || this.timers.drift <= 0) this.values.drift_push = 0;
			api._helpers.paintProceduralGrid(this);
		}

		_rippleLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['rowboat'] = [
		{
			key: 'still-lake',
			label: 'still lake',
			config: {
				intro_drift: 0.12,
				ending_ripple: 0.12,
				waterline: 0.57,
				drift_speed: 0.05,
				bob_amp: 0.7,
				wave_amp: 0.9,
				wave_freq: 0.12,
				ripple: 0.12,
				reflection: 0.28,
				boat_len: 13,
				boat_height: 3.5,
				hue: 202,
				hue_sp: 10,
				sat: 0.26,
				lmin: 0.16,
				lmax: 0.74,
				calm_p: 0.0011,
			},
		},
		{
			key: 'gentle-drift',
			label: 'gentle drift',
			config: {
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
				drift_p: 0.0009,
			},
		},
		{
			key: 'evening-ripples',
			label: 'evening ripples',
			config: {
				waterline: 0.6,
				drift_speed: 0.1,
				bob_amp: 1.4,
				wave_amp: 1.9,
				wave_freq: 0.18,
				ripple: 0.34,
				reflection: 0.24,
				boat_len: 14.5,
				boat_height: 4,
				hue: 212,
				hue_sp: 18,
				sat: 0.4,
				lmin: 0.18,
				lmax: 0.86,
				wake_p: 0.001,
			},
		},
		{
			key: 'wind-touched-water',
			label: 'wind-touched water',
			config: {
				waterline: 0.56,
				drift_speed: 0.12,
				bob_amp: 1.8,
				wave_amp: 2.5,
				wave_freq: 0.2,
				ripple: 0.42,
				reflection: 0.18,
				boat_len: 15,
				boat_height: 4,
				hue: 198,
				hue_sp: 20,
				sat: 0.46,
				lmin: 0.18,
				lmax: 0.8,
				wake_p: 0.0012,
				wake_mult: 2.1,
				drift_p: 0.0014,
				drift_push: 1.55,
			},
		},
	];
	api.effects['rowboat'] = Rowboat;
})(window.AmbienceSim);
