'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 70,
		intro_glow: 0.10,
		ending_dur: 85,
		ending_linger: 24,
		ending_glow: 0.06,
		figure_x: 0.5,
		figure_height: 30,
		figure_width: 11,
		silhouette: 0.92,
		hat: 1,
		shoulder: 1,
		ember_x: 0.56,
		ember_y: 0.62,
		ember_brightness: 0.86,
		ember_pulse: 0.34,
		smoke_density: 0.42,
		smoke_rise: 0.46,
		smoke_drift: 0.18,
		smoke_softness: 0.62,
		hue: 22,
		hue_sp: 10,
		sat: 0.72,
		lmin: 0.06,
		lmax: 0.86,
		inhale_p: 0,
		exhale_p: 0,
		ash_fall_p: 0,
		lighter_flick_p: 0,
		inhale_dur: 32,
		inhale_mult: 1.85,
		exhale_dur: 60,
		exhale_plume: 1.4,
		ash_fall_dur: 28,
		ash_fall_mult: 1.3,
		lighter_flick_dur: 20,
		lighter_flick_mult: 2.4,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.figure_x <= 0) c.figure_x = DEFAULTS.figure_x;
		if (c.figure_height <= 0) c.figure_height = DEFAULTS.figure_height;
		if (c.figure_width <= 0) c.figure_width = DEFAULTS.figure_width;
		c.silhouette = clamp01(c.silhouette);
		if (c.silhouette <= 0) c.silhouette = DEFAULTS.silhouette;
		c.hat = clamp01(c.hat);
		c.shoulder = clamp01(c.shoulder);
		if (c.ember_x <= 0) c.ember_x = DEFAULTS.ember_x;
		if (c.ember_y <= 0) c.ember_y = DEFAULTS.ember_y;
		if (c.ember_brightness <= 0) c.ember_brightness = DEFAULTS.ember_brightness;
		if (c.ember_pulse < 0) c.ember_pulse = 0;
		if (c.smoke_density < 0) c.smoke_density = 0;
		if (c.smoke_rise <= 0) c.smoke_rise = DEFAULTS.smoke_rise;
		if (c.smoke_softness <= 0) c.smoke_softness = DEFAULTS.smoke_softness;
		if (c.hue < 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin < 0) c.lmin = 0;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.inhale_dur <= 0) c.inhale_dur = DEFAULTS.inhale_dur;
		if (c.inhale_mult <= 0) c.inhale_mult = DEFAULTS.inhale_mult;
		if (c.exhale_dur <= 0) c.exhale_dur = DEFAULTS.exhale_dur;
		if (c.exhale_plume <= 0) c.exhale_plume = DEFAULTS.exhale_plume;
		if (c.ash_fall_dur <= 0) c.ash_fall_dur = DEFAULTS.ash_fall_dur;
		if (c.ash_fall_mult <= 0) c.ash_fall_mult = DEFAULTS.ash_fall_mult;
		if (c.lighter_flick_dur <= 0) c.lighter_flick_dur = DEFAULTS.lighter_flick_dur;
		if (c.lighter_flick_mult <= 0) c.lighter_flick_mult = DEFAULTS.lighter_flick_mult;
		return c;
	}

	class MysteriousMan {
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
			const rng = this._eventRng(name.length + 109);
			switch (name) {
				case 'inhale':
					this.timers.inhale = jitterInt(rng, this.cfg.inhale_dur, 0.25);
					this.values.inhale_gain = this.cfg.inhale_mult * (0.8 + rng() * 0.4);
					return true;
				case 'exhale':
					this.timers.exhale = jitterInt(rng, this.cfg.exhale_dur, 0.25);
					this.values.exhale_gain = this.cfg.exhale_plume * (0.85 + rng() * 0.35);
					this.values.exhale_seed = rng() * 1024;
					return true;
				case 'ash-fall':
					this.timers['ash-fall'] = jitterInt(rng, this.cfg.ash_fall_dur, 0.3);
					this.values.ash_gain = this.cfg.ash_fall_mult * (0.85 + rng() * 0.3);
					this.values.ash_seed = rng() * 1024;
					return true;
				case 'lighter-flick':
					this.timers['lighter-flick'] = jitterInt(rng, this.cfg.lighter_flick_dur, 0.25);
					this.values.flick_gain = this.cfg.lighter_flick_mult * (0.85 + rng() * 0.3);
					return true;
				case 'intro':
					this.timers.inhale = 0;
					this.timers.exhale = 0;
					this.timers['ash-fall'] = 0;
					this.timers['lighter-flick'] = 0;
					this.timers.ending = 0;
					this.values.inhale_gain = 1;
					this.values.exhale_gain = 1;
					this.values.ash_gain = 1;
					this.values.flick_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.inhale = 0;
					this.timers.exhale = 0;
					this.timers['ash-fall'] = 0;
					this.timers['lighter-flick'] = 0;
					this.values.inhale_gain = 1;
					this.values.exhale_gain = 1;
					this.values.ash_gain = 1;
					this.values.flick_gain = 1;
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
			if (!this.timers.inhale || this.timers.inhale <= 0) this.values.inhale_gain = 1;
			if (!this.timers.exhale || this.timers.exhale <= 0) this.values.exhale_gain = 1;
			if (!this.timers['ash-fall'] || this.timers['ash-fall'] <= 0) this.values.ash_gain = 1;
			if (!this.timers['lighter-flick'] || this.timers['lighter-flick'] <= 0) this.values.flick_gain = 1;
			api._helpers.paintProceduralGrid(this);
		}

		_emberLevel() {
			let level = 1;
			if (this.timers.inhale > 0) level *= this.values.inhale_gain || this.cfg.inhale_mult;
			if (this.timers['lighter-flick'] > 0) level *= this.values.flick_gain || this.cfg.lighter_flick_mult;
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
			return Math.max(0.0, level);
		}

		_revealLevel() {
			let level = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				// silhouette stays dark until the ember is established, then resolves
				level *= Math.pow(progress, 1.6);
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - progress * 0.92;
			}
			return clamp01(level);
		}

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}
	}

	api.presets['mysterious-man'] = [
		{
			key: 'noir-stillness',
			label: 'noir stillness',
			config: {
				figure_x: 0.5,
				figure_height: 30,
				figure_width: 11,
				silhouette: 0.94,
				hat: 1,
				shoulder: 1,
				ember_x: 0.56,
				ember_y: 0.62,
				ember_brightness: 0.78,
				ember_pulse: 0.28,
				smoke_density: 0.38,
				smoke_rise: 0.42,
				smoke_drift: 0.14,
				smoke_softness: 0.66,
				hue: 22,
				hue_sp: 10,
				sat: 0.7,
				lmin: 0.06,
				lmax: 0.84,
				exhale_p: 0.0009,
				ash_fall_p: 0.0006,
			},
		},
		{
			key: 'deep-inhale',
			label: 'deep inhale',
			config: {
				figure_x: 0.48,
				figure_height: 32,
				figure_width: 12,
				silhouette: 0.92,
				hat: 0.7,
				shoulder: 1,
				ember_x: 0.55,
				ember_y: 0.6,
				ember_brightness: 1.0,
				ember_pulse: 0.5,
				smoke_density: 0.5,
				smoke_rise: 0.5,
				smoke_drift: 0.22,
				smoke_softness: 0.58,
				hue: 18,
				hue_sp: 14,
				sat: 0.82,
				lmin: 0.05,
				lmax: 0.92,
				inhale_p: 0.0026,
				inhale_dur: 36,
				inhale_mult: 2.1,
				exhale_p: 0.0018,
				exhale_dur: 64,
				exhale_plume: 1.6,
			},
		},
		{
			key: 'cold-alley',
			label: 'cold alley',
			config: {
				figure_x: 0.42,
				figure_height: 30,
				figure_width: 10,
				silhouette: 0.96,
				hat: 1,
				shoulder: 1,
				ember_x: 0.49,
				ember_y: 0.6,
				ember_brightness: 0.74,
				ember_pulse: 0.24,
				smoke_density: 0.6,
				smoke_rise: 0.34,
				smoke_drift: -0.12,
				smoke_softness: 0.78,
				hue: 14,
				hue_sp: 8,
				sat: 0.58,
				lmin: 0.08,
				lmax: 0.78,
				exhale_p: 0.0012,
				exhale_dur: 70,
				exhale_plume: 1.55,
				ash_fall_p: 0.0008,
			},
		},
		{
			key: 'ember-watch',
			label: 'ember watch',
			config: {
				intro_glow: 0.04,
				ending_glow: 0.04,
				figure_x: 0.5,
				figure_height: 28,
				figure_width: 11,
				silhouette: 0.97,
				hat: 1,
				shoulder: 0.9,
				ember_x: 0.57,
				ember_y: 0.6,
				ember_brightness: 0.92,
				ember_pulse: 0.46,
				smoke_density: 0.32,
				smoke_rise: 0.4,
				smoke_drift: 0.08,
				smoke_softness: 0.7,
				hue: 26,
				hue_sp: 16,
				sat: 0.8,
				lmin: 0.04,
				lmax: 0.9,
				inhale_p: 0.0014,
				exhale_p: 0.0009,
				ash_fall_p: 0.0008,
				lighter_flick_p: 0.0004,
				lighter_flick_mult: 2.6,
			},
		},
	];
	api.effects['mysterious-man'] = MysteriousMan;
})(window.AmbienceSim);
