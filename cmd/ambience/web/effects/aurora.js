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
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#02060f');
				sky.addColorStop(0.52, '#07101c');
				sky.addColorStop(1, '#0a1220');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.82);
			const intensity = this._intensityLevel();
			const bands = Math.max(1, Math.round(this.cfg.bands));
			const shiftPush = this.values.shift_push || 0;
			const shiftSeed = this.values.shift_seed || 0;

			const horizonGlow = ctx.createLinearGradient(0, canvasH * 0.5, 0, canvasH);
			horizonGlow.addColorStop(0, 'rgba(36, 84, 92, 0)');
			horizonGlow.addColorStop(1, `rgba(48, 168, 140, ${clamp01(0.18 + intensity * 0.16)})`);
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, canvasH * 0.48, canvasW, canvasH * 0.52);

			const baseGround = hslToRGB(212, 0.2, 0.04);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, groundRow - 1, this.w, this.h - groundRow + 1, `rgb(${baseGround.r},${baseGround.g},${baseGround.b})`, 1);

			const starCount = Math.max(12, Math.round(this.w * 0.18));
			for (let i = 0; i < starCount; i++) {
				const col = Math.floor(this._hash(19000 + i) * this.w);
				const row = Math.floor(this._hash(19100 + i) * Math.max(1, groundRow - 10));
				const twinkle = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * (0.018 + this._hash(19200 + i) * 0.02) + i), 2);
				const alpha = clamp01((0.14 + twinkle * 0.22) * (1 - Math.min(0.65, intensity * 0.55)));
				const color = hslToRGB(205 + this._hash(19300 + i) * 18, 0.18, 0.72 + this._hash(19400 + i) * 0.2);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
			}

			const ridgePoints = [];
			const ridgeSegments = 7;
			const ridgeColor = hslToRGB(210, 0.24, 0.055);
			for (let i = 0; i <= ridgeSegments; i++) {
				ridgePoints.push(groundRow - 4 - Math.floor(this._hash(19500 + i) * 6) - Math.floor((0.5 + 0.5 * Math.sin(i * 1.3 + this._hash(19600 + i) * 4)) * 4));
			}
			const ridgeCoords = [];
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const ridge = Math.round(ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased + Math.sin(x * 0.08 + shiftSeed) * 0.8);
				ridgeCoords.push({ x, ridge });
			}
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (const point of ridgeCoords) {
				ctx.lineTo(Math.floor(point.x * sx), Math.floor(point.ridge * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const treeCount = 12;
			for (let i = 0; i < treeCount; i++) {
				const center = Math.floor((i + 0.5) * this.w / treeCount + (this._hash(19900 + i) - 0.5) * 5);
				const trunkH = 1 + Math.floor(this._hash(20000 + i) * 2);
				const crownH = 5 + Math.floor(this._hash(20100 + i) * 5);
				const half = 1 + Math.floor(this._hash(20200 + i) * 2);
				const baseY = groundRow - 1 - Math.floor(this._hash(20300 + i) * 4);
				const treeColor = hslToRGB(210 + this._hash(20400 + i) * 10, 0.22, 0.045 + this._hash(20500 + i) * 0.02);
				for (let row = 0; row < crownH; row++) {
					const width = Math.max(1, half - Math.floor(row / 2));
					const y = baseY - crownH + row;
					for (let dx = -width; dx <= width; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center + dx, y, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
					}
				}
				for (let row = 0; row < trunkH; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center, baseY - row, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
				}
			}

			for (let band = 0; band < bands; band++) {
				const bandRatio = bands === 1 ? 0.5 : band / (bands - 1);
				const phase = this.tick * this.cfg.speed * (0.18 + bandRatio * 0.12) + band * 1.6 + shiftSeed;
				const amp = this.cfg.wave_amp * (0.8 + bandRatio * 0.35);
				const freq = this.cfg.wave_freq * (0.82 + bandRatio * 0.26);
				const thickness = this.cfg.thickness * (0.8 + bandRatio * 0.28);
				const curtain = this.cfg.curtain_len * (0.8 + bandRatio * 0.22);
				const baseY = this.h * (0.16 + bandRatio * 0.08) + Math.sin(this.tick * 0.01 + band * 0.8) * 1.1;
				const hueBase = ((this.cfg.hue + (bandRatio - 0.5) * this.cfg.hue_sp * 1.15 + shiftPush * 5) % 360 + 360) % 360;

				for (let x = 0; x < this.w; x++) {
					const nx = x / Math.max(1, this.w - 1);
					const arch = Math.sin(nx * Math.PI * (1.08 + bandRatio * 0.24) + band * 0.75);
					const wave = Math.sin(x * freq + phase + this.tick * this.cfg.drift * 0.04);
					const subWave = Math.sin(x * freq * 0.47 - phase * 0.62 + band * 2.1);
					const center = baseY + arch * amp * 0.72 + wave * amp * 0.52 + subWave * amp * 0.22 + shiftPush * Math.sin(x * 0.07 + phase) * 1.05;
					const startY = Math.max(0, Math.floor(center - thickness * 1.15));
					const endY = Math.min(groundRow - 2, Math.ceil(center + curtain));
					for (let y = startY; y <= endY; y++) {
						const dy = y - center;
						const core = Math.exp(-(dy * dy) / Math.max(1, thickness * thickness * 1.4));
						const tail = y >= center ? Math.exp(-(y - center) / Math.max(1, curtain)) : 0;
						const shimmer = 0.76 + 0.24 * Math.sin(this.tick * 0.03 + x * 0.1 + band * 1.7);
						const strength = (core * 0.9 + tail * 0.7) * intensity * shimmer * (0.58 + 0.42 * Math.max(0.2, arch));
						if (strength < 0.025) continue;
						const hue = ((hueBase + Math.sin(y * 0.18 + x * 0.06 + phase) * this.cfg.hue_sp * 0.32 + band * 6) % 360 + 360) % 360;
						const sat = clamp01(this.cfg.sat * (0.84 + core * 0.28));
						const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * Math.min(1, 0.24 + strength));
						const color = hslToRGB(hue, sat, light);
						const alpha = clamp01(strength * (0.34 + core * 0.46));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
						if (core > 0.62 && y < groundRow - 3) {
							const accent = hslToRGB((hue + 12) % 360, clamp01(sat * 0.9), clamp01(light * 1.08));
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${accent.r},${accent.g},${accent.b})`, alpha * 0.45);
						}
					}
				}
			}
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
