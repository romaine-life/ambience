'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 55,
		intro_glow: 0.16,
		ending_dur: 70,
		ending_linger: 22,
		ending_glow: 0.10,
		horizon: 0.86,
		cone_height: 28,
		cone_width: 46,
		crater_width: 8,
		slope_jitter: 1.6,
		glow: 0.55,
		smoke: 0.32,
		smoke_height: 18,
		hue: 18,
		hue_sp: 16,
		sat: 0.78,
		lmin: 0.18,
		lmax: 0.92,
		eruption_p: 0,
		smolder_p: 0,
		flare_p: 0,
		eruption_dur: 80,
		eruption_height: 28,
		eruption_mult: 2.4,
		smolder_dur: 80,
		smolder_mult: 0.55,
		flare_dur: 24,
		flare_mult: 1.85,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.cone_height <= 0) c.cone_height = DEFAULTS.cone_height;
		if (c.cone_width <= 0) c.cone_width = DEFAULTS.cone_width;
		if (c.crater_width <= 0) c.crater_width = DEFAULTS.crater_width;
		if (c.slope_jitter < 0) c.slope_jitter = 0;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.smoke < 0) c.smoke = 0;
		if (c.smoke_height <= 0) c.smoke_height = DEFAULTS.smoke_height;
		if (c.hue < 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.eruption_dur <= 0) c.eruption_dur = DEFAULTS.eruption_dur;
		if (c.eruption_height <= 0) c.eruption_height = DEFAULTS.eruption_height;
		if (c.eruption_mult <= 0) c.eruption_mult = DEFAULTS.eruption_mult;
		if (c.smolder_dur <= 0) c.smolder_dur = DEFAULTS.smolder_dur;
		if (c.smolder_mult <= 0) c.smolder_mult = DEFAULTS.smolder_mult;
		if (c.flare_dur <= 0) c.flare_dur = DEFAULTS.flare_dur;
		if (c.flare_mult <= 0) c.flare_mult = DEFAULTS.flare_mult;
		return c;
	}

	class Volcano {
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

		_paintCraterGlow(ctx, sx, sy, ceilSx, ceilSy, centerX, peakRow, craterHalf, glowStrength) {
			if (glowStrength <= 0.02) return;
			const glowHue = (this.cfg.hue + 4) % 360;
			const core = hslToRGB(glowHue, clamp01(this.cfg.sat * 0.95), clamp01(this.cfg.lmax * (0.7 + glowStrength * 0.25)));
			const outer = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.45));
			const maxRise = Math.max(5, Math.round(4 + glowStrength * 11));
			const maxSide = Math.max(craterHalf + 3, Math.round(craterHalf + glowStrength * 16));
			const bandH = Math.max(1, Math.round(1 + glowStrength * 1.6));

			for (let band = maxRise; band >= 0; band--) {
				const lift = band / Math.max(1, maxRise);
				const width = Math.max(craterHalf + 1, Math.round(maxSide * (1 - lift * 0.62)));
				const row = peakRow - band;
				const colorMix = 1 - lift;
				const color = colorMix > 0.58 ? core : outer;
				const baseAlpha = clamp01((0.08 + glowStrength * 0.38) * (1 - lift * 0.78));
				for (let dx = -width; dx <= width; dx++) {
					const side = Math.abs(dx) / Math.max(1, width);
					const checker = (dx + band + this.tick) & 1;
					const edgeNoise = this._hash(33400 + band * 31 + dx) * 0.22;
					const alpha = clamp01(baseAlpha * (1 - side * 0.72 + edgeNoise) * (checker ? 0.72 : 1));
					if (alpha < 0.035) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, row, 1, bandH, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			const floorRows = Math.max(2, Math.round(2 + glowStrength * 3));
			for (let y = 0; y < floorRows; y++) {
				const width = Math.max(craterHalf + 2, Math.round(maxSide * (0.55 - y * 0.08)));
				const row = peakRow + y;
				const alpha = clamp01((0.12 + glowStrength * 0.3) * (1 - y / Math.max(1, floorRows)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - width, row, width * 2 + 1, 1, `rgb(${outer.r},${outer.g},${outer.b})`, alpha);
			}
		}

		triggerEvent(name) {
			const rng = this._eventRng(name.length + 97);
			switch (name) {
				case 'eruption':
					this.timers.eruption = jitterInt(rng, this.cfg.eruption_dur, 0.3);
					this.timers.smolder = 0;
					this.values.eruption_gain = this.cfg.eruption_mult * (0.8 + rng() * 0.45);
					this.values.eruption_seed = rng() * 1024;
					return true;
				case 'smolder':
					this.timers.smolder = jitterInt(rng, this.cfg.smolder_dur, 0.3);
					this.timers.eruption = 0;
					this.values.eruption_gain = 1;
					return true;
				case 'flare':
					this.timers.flare = jitterInt(rng, this.cfg.flare_dur, 0.3);
					this.values.flare_gain = this.cfg.flare_mult * (0.85 + rng() * 0.3);
					return true;
				case 'intro':
					this.timers.eruption = 0;
					this.timers.smolder = 0;
					this.timers.flare = 0;
					this.timers.ending = 0;
					this.values.eruption_gain = 1;
					this.values.flare_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.eruption = 0;
					this.timers.smolder = 0;
					this.timers.flare = 0;
					this.values.eruption_gain = 1;
					this.values.flare_gain = 1;
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
			if (!this.timers.eruption || this.timers.eruption <= 0) this.values.eruption_gain = 1;
			if (!this.timers.flare || this.timers.flare <= 0) this.values.flare_gain = 1;
			api._helpers.paintProceduralGrid(this);
		}

		_pressureLevel() {
			let level = 1;
			if (this.timers.eruption > 0) level *= this.values.eruption_gain || this.cfg.eruption_mult;
			if (this.timers.smolder > 0) level *= this.cfg.smolder_mult;
			if (this.timers.flare > 0) level *= 1 + ((this.values.flare_gain || this.cfg.flare_mult) - 1) * 0.5;
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

	api.presets['volcano'] = [
		{
			key: 'sleeping-cone',
			label: 'sleeping cone',
			config: {
				intro_glow: 0.1,
				ending_glow: 0.06,
				horizon: 0.86,
				cone_height: 26,
				cone_width: 48,
				crater_width: 7,
				slope_jitter: 1.4,
				glow: 0.3,
				smoke: 0.16,
				smoke_height: 14,
				hue: 16,
				hue_sp: 12,
				sat: 0.6,
				lmin: 0.16,
				lmax: 0.76,
				eruption_p: 0.0001,
				smolder_p: 0.0008,
				flare_p: 0.0006,
			},
		},
		{
			key: 'smoldering-crater',
			label: 'smoldering crater',
			config: {
				horizon: 0.86,
				cone_height: 28,
				cone_width: 46,
				crater_width: 9,
				slope_jitter: 1.6,
				glow: 0.55,
				smoke: 0.42,
				smoke_height: 22,
				hue: 18,
				hue_sp: 18,
				sat: 0.78,
				lmin: 0.18,
				lmax: 0.9,
				eruption_p: 0.0004,
				flare_p: 0.0014,
				smolder_p: 0.0006,
				eruption_height: 22,
				eruption_mult: 2.1,
			},
		},
		{
			key: 'active-vent',
			label: 'active vent',
			config: {
				intro_glow: 0.22,
				horizon: 0.84,
				cone_height: 30,
				cone_width: 48,
				crater_width: 10,
				slope_jitter: 1.8,
				glow: 0.78,
				smoke: 0.48,
				smoke_height: 28,
				hue: 14,
				hue_sp: 22,
				sat: 0.88,
				lmin: 0.22,
				lmax: 0.96,
				eruption_p: 0.0014,
				eruption_dur: 96,
				eruption_height: 32,
				eruption_mult: 2.7,
				flare_p: 0.0018,
				flare_mult: 2.0,
			},
		},
		{
			key: 'ember-burst',
			label: 'ember burst',
			config: {
				intro_glow: 0.18,
				ending_glow: 0.16,
				horizon: 0.88,
				cone_height: 24,
				cone_width: 42,
				crater_width: 12,
				slope_jitter: 1.2,
				glow: 0.7,
				smoke: 0.22,
				smoke_height: 16,
				hue: 22,
				hue_sp: 26,
				sat: 0.9,
				lmin: 0.2,
				lmax: 0.98,
				eruption_p: 0.0022,
				eruption_dur: 60,
				eruption_height: 36,
				eruption_mult: 3.0,
				flare_p: 0.0024,
				flare_mult: 2.2,
				smolder_p: 0.0004,
			},
		},
	];
	api.effects['volcano'] = Volcano;
})(window.AmbienceSim);
