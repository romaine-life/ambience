'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 45,
		intro_glow: 0.14,
		ending_dur: 60,
		ending_linger: 24,
		ending_glow: 0.08,
		flame_height: 14,
		flame_width: 10,
		flame_speed: 0.12,
		flicker: 0.72,
		ember_rate: 0.26,
		ember_speed: 0.62,
		glow: 0.54,
		hue: 24,
		hue_sp: 18,
		sat: 0.82,
		lmin: 0.32,
		lmax: 0.94,
		crackle_p: 0,
		lull_p: 0,
		crackle_dur: 36,
		crackle_mult: 1.85,
		lull_dur: 68,
		lull_mult: 0.55,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.flame_height <= 0) c.flame_height = DEFAULTS.flame_height;
		if (c.flame_width <= 0) c.flame_width = DEFAULTS.flame_width;
		if (c.flame_speed <= 0) c.flame_speed = DEFAULTS.flame_speed;
		if (c.flicker <= 0) c.flicker = DEFAULTS.flicker;
		if (c.ember_rate <= 0) c.ember_rate = DEFAULTS.ember_rate;
		if (c.ember_speed <= 0) c.ember_speed = DEFAULTS.ember_speed;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.crackle_dur <= 0) c.crackle_dur = DEFAULTS.crackle_dur;
		if (c.crackle_mult <= 0) c.crackle_mult = DEFAULTS.crackle_mult;
		if (c.lull_dur <= 0) c.lull_dur = DEFAULTS.lull_dur;
		if (c.lull_mult <= 0) c.lull_mult = DEFAULTS.lull_mult;
		return c;
	}

	class Campfire {
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
			const rng = this._eventRng(name.length + 67);
			switch (name) {
				case 'crackle':
					this.timers.crackle = jitterInt(rng, this.cfg.crackle_dur, 0.3);
					this.values.crackle_gain = this.cfg.crackle_mult * (0.75 + rng() * 0.5);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'intro':
					this.timers.crackle = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.crackle_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.crackle = 0;
					this.timers.lull = 0;
					this.values.crackle_gain = 1;
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
			if (!this.timers.crackle || this.timers.crackle <= 0) this.values.crackle_gain = 1;
		}

		_flameLevel() {
			let level = 1;
			if (this.timers.crackle > 0) level *= this.values.crackle_gain || this.cfg.crackle_mult;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
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
				sky.addColorStop(0, '#08111a');
				sky.addColorStop(0.62, '#0f1520');
				sky.addColorStop(1, '#16110c');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.84);
			const centerX = Math.floor(this.w * 0.5);
			const flameLevel = this._flameLevel();
			const crackleGain = this.values.crackle_gain || 1;
			const halfW = Math.max(2, Math.round(this.cfg.flame_width * 0.5));
			const flameH = Math.max(4, this.cfg.flame_height * (0.52 + flameLevel * 0.38));
			const speed = this.tick * this.cfg.flame_speed * 1.7;

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const ground = hslToRGB(18, 0.24, 0.08 + ratio * 0.12);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${ground.r},${ground.g},${ground.b})`, 1);
			}

			const glowStrength = clamp01(this.cfg.glow * (0.6 + flameLevel * 0.45));
			const glowX = centerX * sx;
			const glowY = groundRow * sy;
			const glowR = Math.max(28, Math.min(canvasW, canvasH) * (0.08 + glowStrength * 0.12));
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			glow.addColorStop(0, `rgba(255, 178, 82, ${0.24 + glowStrength * 0.22})`);
			glow.addColorStop(0.42, `rgba(255, 120, 44, ${0.12 + glowStrength * 0.12})`);
			glow.addColorStop(1, 'rgba(255, 120, 44, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			const vignette = ctx.createRadialGradient(glowX, glowY, glowR * 0.35, glowX, glowY, glowR * 2.4);
			vignette.addColorStop(0, 'rgba(0, 0, 0, 0)');
			vignette.addColorStop(1, 'rgba(0, 0, 0, 0.26)');
			ctx.fillStyle = vignette;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const logColor = hslToRGB(20, 0.46, 0.22);
			const logHighlight = hslToRGB(24, 0.44, 0.32);
			const logHalf = halfW + 2;
			for (let dx = -logHalf; dx <= logHalf; dx++) {
				const rowA = groundRow + 1 + Math.round(dx * 0.12);
				const rowB = groundRow + 1 - Math.round(dx * 0.1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowA, 1, 1, `rgb(${logColor.r},${logColor.g},${logColor.b})`, 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowB, 1, 1, `rgb(${logColor.r},${logColor.g},${logColor.b})`, 0.82);
				if ((dx + logHalf) % 3 === 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowA, 1, 1, `rgb(${logHighlight.r},${logHighlight.g},${logHighlight.b})`, 0.34);
				}
			}

			for (let dx = -halfW; dx <= halfW; dx++) {
				const coalHeat = 0.45 + 0.55 * Math.pow(0.5 + 0.5 * Math.sin(speed * 1.8 + dx * 0.8), 2);
				const coal = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.88), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.22 + coalHeat * 0.45)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, groundRow, 1, 1, `rgb(${coal.r},${coal.g},${coal.b})`, 0.28 + coalHeat * 0.42);
			}

			for (let x = -halfW; x <= halfW; x++) {
				const nx = Math.abs(x) / Math.max(1, halfW);
				const widthShape = Math.max(0, 1 - Math.pow(nx, 1.32));
				if (widthShape <= 0.04) continue;
				const pulse = 0.8 + 0.2 * Math.sin(speed * 1.3 + x * 0.7 + this._hash(26000 + x + halfW) * 5);
				const columnH = Math.max(2, Math.round(flameH * widthShape * pulse));
				for (let y = 0; y < columnH; y++) {
					const lift = y / Math.max(1, columnH);
					const taper = 1 - lift;
					const sway = Math.sin(speed * 2.1 + x * 0.35 + y * 0.24 + this._hash(26100 + y * 31 + x + 400) * 6) * this.cfg.flicker * taper * 0.72;
					const col = Math.round(centerX + x + sway);
					const row = Math.round(groundRow - 1 - y);
					const hue = ((this.cfg.hue - lift * this.cfg.hue_sp * 0.34 + (this._hash(26200 + x * 17 + y + 700) * 2 - 1) * this.cfg.hue_sp * 0.08) % 360 + 360) % 360;
					const sat = clamp01(this.cfg.sat * (0.88 + taper * 0.16));
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.18 + taper * 0.8));
					const alpha = clamp01((0.12 + taper * 0.56) * (0.36 + widthShape * 0.64) * (0.72 + flameLevel * 0.18));
					const color = hslToRGB(hue, sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if (widthShape > 0.35 && lift < 0.6 && (x + y) % 2 === 0) {
						const core = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.74), clamp01(this.cfg.lmax * (0.72 + taper * 0.24)));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${core.r},${core.g},${core.b})`, alpha * 0.52);
					}
				}
			}

			const emberCount = Math.max(4, Math.round(this.cfg.flame_width * (0.8 + this.cfg.ember_rate * 3.8) * (this.timers.crackle > 0 ? 0.92 + crackleGain * 0.3 : 1)));
			const maxRise = Math.max(10, Math.round(this.cfg.flame_height * 2.1 + this.cfg.ember_speed * 12));
			for (let i = 0; i < emberCount; i++) {
				const cycle = maxRise + 8 + Math.floor(this._hash(27000 + i) * 12);
				const progress = positiveMod(this.tick * this.cfg.ember_speed * (0.7 + this._hash(27100 + i) * 0.7) + this._hash(27200 + i) * cycle, cycle);
				if (progress > maxRise) continue;
				const rise = progress;
				const fade = 1 - rise / Math.max(1, maxRise);
				const drift = (this._hash(27300 + i) * 2 - 1) * (1.2 + rise * 0.08) + Math.sin(speed + i * 0.7) * 0.6;
				const col = Math.round(centerX + drift);
				const row = Math.round(groundRow - 2 - rise);
				if (row < 1) continue;
				const size = fade > 0.72 && this.timers.crackle > 0 && this._hash(27400 + i) > 0.5 ? 2 : 1;
				const hue = ((this.cfg.hue - 6 + this._hash(27500 + i) * 10) % 360 + 360) % 360;
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.42 + fade * 0.5));
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.8), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, size, 1, `rgb(${color.r},${color.g},${color.b})`, clamp01((0.16 + fade * 0.68) * (0.78 + Math.max(0, crackleGain - 1) * 0.16)));
			}
		}
	}

	api.presets['campfire'] = [
		{
			key: 'small-fire',
			label: 'small fire',
			config: {
				flame_height: 9,
				flame_width: 7,
				flame_speed: 0.1,
				flicker: 0.56,
				ember_rate: 0.18,
				ember_speed: 0.52,
				glow: 0.4,
				hue: 22,
				hue_sp: 12,
				sat: 0.76,
				lmin: 0.28,
				lmax: 0.88,
			},
		},
		{
			key: 'steady-campfire',
			label: 'steady campfire',
			config: {
				flame_height: 14,
				flame_width: 10,
				flame_speed: 0.12,
				flicker: 0.72,
				ember_rate: 0.26,
				ember_speed: 0.62,
				glow: 0.54,
				hue: 24,
				hue_sp: 18,
				sat: 0.82,
				lmin: 0.32,
				lmax: 0.94,
				crackle_p: 0.0008,
			},
		},
		{
			key: 'crackling-fire',
			label: 'crackling fire',
			config: {
				flame_height: 16,
				flame_width: 11,
				flame_speed: 0.15,
				flicker: 0.92,
				ember_rate: 0.34,
				ember_speed: 0.78,
				glow: 0.62,
				hue: 21,
				hue_sp: 22,
				sat: 0.88,
				lmin: 0.34,
				lmax: 0.96,
				crackle_p: 0.0015,
				crackle_mult: 2.15,
				crackle_dur: 48,
			},
		},
		{
			key: 'late-embers',
			label: 'late embers',
			config: {
				intro_glow: 0.1,
				ending_glow: 0.14,
				flame_height: 8,
				flame_width: 8,
				flame_speed: 0.08,
				flicker: 0.42,
				ember_rate: 0.3,
				ember_speed: 0.48,
				glow: 0.34,
				hue: 18,
				hue_sp: 14,
				sat: 0.68,
				lmin: 0.24,
				lmax: 0.8,
				lull_p: 0.0014,
				lull_mult: 0.42,
			},
		},
	];
	api.effects['campfire'] = Campfire;
})(window.AmbienceSim);
