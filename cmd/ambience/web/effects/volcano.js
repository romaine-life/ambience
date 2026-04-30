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
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#0a0612');
				sky.addColorStop(0.55, '#15101c');
				sky.addColorStop(1, '#1f1212');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const baseRow = Math.max(8, Math.min(this.h - 4, Math.floor(this.h * this.cfg.horizon)));
			const centerX = Math.floor(this.w * 0.5);
			const pressure = this._pressureLevel();
			const eruptionActive = this.timers.eruption > 0;
			const eruptionGain = this.values.eruption_gain || 1;
			const flareActive = this.timers.flare > 0;
			const flareGain = this.values.flare_gain || 1;
			const eruptionTotal = this.timers.eruption > 0 ? Math.max(1, Math.round(this.cfg.eruption_dur)) : 0;
			const eruptionPhase = eruptionActive
				? this._phaseProgress(Math.max(eruptionTotal, this.timers.eruption), this.timers.eruption)
				: 0;
			const eruptionEnvelope = eruptionActive ? Math.sin(Math.PI * Math.min(1, eruptionPhase * 1.0)) : 0;

			const halfW = Math.max(4, Math.round(this.cfg.cone_width * 0.5));
			const coneH = Math.max(6, Math.round(this.cfg.cone_height));
			const craterHalf = Math.max(2, Math.round(this.cfg.crater_width * 0.5));
			const peakRow = baseRow - coneH;

			// silhouette colors
			const coneColor = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.55));
			const coneEdge = hslToRGB((this.cfg.hue + 348) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin * 0.78));
			const ground = hslToRGB((this.cfg.hue + 352) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * 0.4));

			// distant horizon haze
			const hazeColor = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
			const hazeStrength = clamp01(0.18 + this.cfg.glow * 0.18 + pressure * 0.08);
			const hazeGrad = ctx.createLinearGradient(0, Math.floor((baseRow - 4) * sy), 0, Math.floor((baseRow + 6) * sy));
			hazeGrad.addColorStop(0, `rgba(${hazeColor.r},${hazeColor.g},${hazeColor.b},0)`);
			hazeGrad.addColorStop(1, `rgba(${hazeColor.r},${hazeColor.g},${hazeColor.b},${hazeStrength})`);
			ctx.fillStyle = hazeGrad;
			ctx.fillRect(0, Math.floor((baseRow - 4) * sy), canvasW, Math.ceil(14 * sy));

			// flat foreground ground past the cone
			for (let y = baseRow; y < this.h; y++) {
				const ratio = (y - baseRow) / Math.max(1, this.h - baseRow);
				const groundShade = hslToRGB((this.cfg.hue + 352) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * (0.4 + ratio * 0.45)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${groundShade.r},${groundShade.g},${groundShade.b})`, 1);
			}

			// cone silhouette: crater dips into the peak
			for (let dx = -halfW; dx <= halfW; dx++) {
				const nx = Math.abs(dx) / Math.max(1, halfW);
				// slight curvature on the slopes (rounded base, sharper near peak)
				const slope = Math.pow(1 - nx, 1.3);
				const jitter = (this._hash(31000 + dx) * 2 - 1) * this.cfg.slope_jitter;
				let topY = baseRow - Math.round(coneH * slope + jitter);
				// crater notch
				if (Math.abs(dx) < craterHalf) {
					const craterDepth = Math.max(1, Math.round(2 + craterHalf * 0.4 * (1 - Math.abs(dx) / Math.max(1, craterHalf))));
					topY = peakRow + craterDepth;
				}
				if (topY > baseRow) topY = baseRow;
				const col = centerX + dx;
				if (col < 0 || col >= this.w) continue;
				for (let y = topY; y <= baseRow; y++) {
					const isEdge = y === topY || y === topY + 1;
					const color = isEdge ? coneEdge : coneColor;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
				}
			}

			// idle crater glow + flare bloom
			const glowStrength = clamp01(this.cfg.glow * pressure * (flareActive ? flareGain : 1));
			const glowX = centerX * sx;
			const glowY = peakRow * sy;
			const glowR = Math.max(28, Math.min(canvasW, canvasH) * (0.07 + glowStrength * 0.18));
			const glowGrad = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			const glowHue = (this.cfg.hue + 4) % 360;
			const glowCore = hslToRGB(glowHue, clamp01(this.cfg.sat * 0.95), clamp01(this.cfg.lmax * (0.7 + glowStrength * 0.25)));
			const glowOuter = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.45));
			glowGrad.addColorStop(0, `rgba(${glowCore.r},${glowCore.g},${glowCore.b},${0.35 + glowStrength * 0.45})`);
			glowGrad.addColorStop(0.45, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},${0.18 + glowStrength * 0.22})`);
			glowGrad.addColorStop(1, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},0)`);
			ctx.fillStyle = glowGrad;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			// crater rim hot lining
			for (let dx = -craterHalf; dx <= craterHalf; dx++) {
				const t = 1 - Math.abs(dx) / Math.max(1, craterHalf);
				const lava = hslToRGB((this.cfg.hue + this._hash(31300 + dx) * this.cfg.hue_sp * 0.4) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.4 + t * 0.5 + glowStrength * 0.2)));
				const row = peakRow + Math.max(1, Math.round(2 + craterHalf * 0.4 * t));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, row, 1, 1, `rgb(${lava.r},${lava.g},${lava.b})`, clamp01(0.35 + t * 0.5 + glowStrength * 0.25));
			}

			// rising smoke plume (idle + thicker during eruption)
			const smokeBase = Math.max(0, this.cfg.smoke);
			const smokeBoost = eruptionActive ? eruptionEnvelope * 0.85 : (flareActive ? 0.18 : 0);
			const smokeStrength = clamp01(smokeBase * (this.timers.smolder > 0 ? this.cfg.smolder_mult : 1) + smokeBoost);
			if (smokeStrength > 0.02) {
				const smokeMaxRise = Math.max(8, Math.round(this.cfg.smoke_height * (1 + smokeBoost * 0.6)));
				const puffCount = Math.max(4, Math.round(8 + smokeStrength * 14));
				for (let i = 0; i < puffCount; i++) {
					const cycle = smokeMaxRise + 6 + Math.floor(this._hash(31600 + i) * 12);
					const progress = positiveMod(this.tick * 0.12 * (0.7 + this._hash(31700 + i) * 0.6) + this._hash(31800 + i) * cycle, cycle);
					if (progress > smokeMaxRise) continue;
					const fade = 1 - progress / Math.max(1, smokeMaxRise);
					const drift = Math.sin(this.tick * 0.04 + i * 0.7) * (1.4 + progress * 0.12) + (this._hash(31900 + i) * 2 - 1) * 1.6;
					const col = Math.round(centerX + drift);
					const row = Math.round(peakRow - 1 - progress);
					if (row < 1) continue;
					const tint = hslToRGB((this.cfg.hue + 12 + this._hash(32000 + i) * this.cfg.hue_sp * 0.4) % 360, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.36 + fade * 0.34)));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${tint.r},${tint.g},${tint.b})`, clamp01(0.18 + fade * smokeStrength * 0.7));
					if (smokeStrength > 0.4 && (i & 1) === 0) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col + Math.sign(drift || 1), row, 1, 1, `rgb(${tint.r},${tint.g},${tint.b})`, clamp01(0.1 + fade * smokeStrength * 0.4));
					}
				}
			}

			// eruption: ballistic lava sparks arcing out of the crater
			if (eruptionActive) {
				const archHeight = Math.max(6, this.cfg.eruption_height) * (0.65 + eruptionEnvelope * 0.5) * eruptionGain * 0.55;
				const sparkCount = Math.max(8, Math.round(this.cfg.crater_width * 1.6 + archHeight * 0.8));
				const seed = this.values.eruption_seed || 0;
				for (let i = 0; i < sparkCount; i++) {
					const cycle = Math.max(14, Math.round(archHeight * 1.2 + 14));
					const phase = positiveMod(this.tick * 0.4 * (0.7 + this._hash(32200 + i + seed) * 0.6) + this._hash(32300 + i + seed) * cycle, cycle);
					const t = phase / cycle;
					if (t >= 1) continue;
					const angle = (this._hash(32400 + i + seed) * 2 - 1) * Math.PI * 0.42;
					const v0 = archHeight * (0.7 + this._hash(32500 + i + seed) * 0.6);
					const dxStart = Math.sin(angle) * (1.2 + craterHalf * 0.6);
					const yArc = -v0 * Math.sin(Math.PI * t);
					const xArc = dxStart + (this._hash(32600 + i + seed) * 2 - 1) * v0 * 0.18 * t;
					const col = Math.round(centerX + xArc);
					const row = Math.round(peakRow + 1 + yArc);
					if (col < 0 || col >= this.w || row < 0 || row >= this.h) continue;
					const fade = 1 - Math.pow(t, 1.6);
					const hue = ((this.cfg.hue + (this._hash(32700 + i + seed) * 2 - 1) * this.cfg.hue_sp * 0.5) + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.55 + fade * 0.45));
					const lava = hslToRGB(hue, clamp01(this.cfg.sat), light);
					const size = fade > 0.7 ? 2 : 1;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, size, 1, `rgb(${lava.r},${lava.g},${lava.b})`, clamp01(0.45 + fade * 0.5));
				}

				// short lava streaks running down the cone surface during peak eruption
				const streakCount = Math.max(0, Math.round((eruptionGain - 1) * 4));
				for (let s = 0; s < streakCount; s++) {
					const side = s % 2 === 0 ? -1 : 1;
					const dxStart = side * (craterHalf + 1 + this._hash(32800 + s + seed) * 2);
					const length = Math.max(2, Math.round(coneH * 0.3 * eruptionEnvelope));
					for (let r = 0; r < length; r++) {
						const col = Math.round(centerX + dxStart + side * r * 0.4);
						const row = peakRow + r;
						if (row > baseRow || col < 0 || col >= this.w) break;
						const fade = 1 - r / Math.max(1, length);
						const lava = hslToRGB((this.cfg.hue + 4) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.45 + fade * 0.4)));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${lava.r},${lava.g},${lava.b})`, clamp01(0.32 + fade * 0.5));
					}
				}
			}
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
