'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
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
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_beam = clamp01(c.intro_beam);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_beam = clamp01(c.ending_beam);
		if (c.sweep_speed <= 0) c.sweep_speed = DEFAULTS.sweep_speed;
		if (c.beam_width <= 0) c.beam_width = DEFAULTS.beam_width;
		if (c.beam_softness <= 0) c.beam_softness = DEFAULTS.beam_softness;
		if (c.tower_height <= 0) c.tower_height = DEFAULTS.tower_height;
		if (c.tower_width <= 0) c.tower_width = DEFAULTS.tower_width;
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.haze <= 0) c.haze = DEFAULTS.haze;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.bright_pass_dur <= 0) c.bright_pass_dur = DEFAULTS.bright_pass_dur;
		if (c.bright_pass_mult <= 0) c.bright_pass_mult = DEFAULTS.bright_pass_mult;
		if (c.fog_thicken_dur <= 0) c.fog_thicken_dur = DEFAULTS.fog_thicken_dur;
		if (c.fog_thicken_mult <= 0) c.fog_thicken_mult = DEFAULTS.fog_thicken_mult;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class Lighthouse {
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

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers['bright-pass'] || this.timers['bright-pass'] <= 0) this.values.bright_gain = 1;
			if (!this.timers['fog-thicken'] || this.timers['fog-thicken'] <= 0) this.values.fog_gain = 1;
			api._helpers.paintProceduralGrid(this);
		}

		_beamLevel() {
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

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['lighthouse'] = [
		{
			key: 'clear-night',
			label: 'clear night',
			config: {
				sweep_speed: 0.07,
				beam_width: 0.18,
				beam_softness: 0.32,
				tower_height: 22,
				tower_width: 6,
				horizon: 0.74,
				haze: 0.08,
				glow: 0.2,
				hue: 216,
				hue_sp: 14,
				sat: 0.3,
				lmin: 0.1,
				lmax: 0.8,
			},
		},
		{
			key: 'steady-sweep',
			label: 'steady sweep',
			config: {
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
				bright_pass_p: 0.0007,
			},
		},
		{
			key: 'foggy-coast',
			label: 'foggy coast',
			config: {
				sweep_speed: 0.06,
				beam_width: 0.28,
				beam_softness: 0.62,
				tower_height: 23,
				tower_width: 7,
				horizon: 0.76,
				haze: 0.24,
				glow: 0.18,
				hue: 210,
				hue_sp: 12,
				sat: 0.24,
				lmin: 0.1,
				lmax: 0.72,
				fog_thicken_p: 0.0012,
				fog_thicken_mult: 2.2,
			},
		},
		{
			key: 'bright-beacon',
			label: 'bright beacon',
			config: {
				sweep_speed: 0.1,
				beam_width: 0.24,
				beam_softness: 0.36,
				tower_height: 21,
				tower_width: 6,
				horizon: 0.72,
				haze: 0.12,
				glow: 0.3,
				hue: 218,
				hue_sp: 20,
				sat: 0.36,
				lmin: 0.12,
				lmax: 0.9,
				bright_pass_p: 0.0014,
				bright_pass_mult: 2.1,
				calm_p: 0.0009,
			},
		},
	];
	api.effects['lighthouse'] = Lighthouse;
})(window.AmbienceSim);
