'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 60,
		intro_density: 0.16,
		ending_dur: 70,
		ending_linger: 22,
		ending_density: 0.08,
		density: 0.32,
		speed: 0.48,
		drift: 0.08,
		sway: 0.42,
		layers: 3,
		size: 1,
		hue: 210,
		hue_sp: 12,
		sat: 0.16,
		lmin: 0.74,
		lmax: 0.98,
		gust_p: 0,
		calm_p: 0,
		gust_dur: 55,
		gust_mult: 1.85,
		calm_dur: 80,
		calm_mult: 0.42,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_density = clamp01(c.intro_density);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_density = clamp01(c.ending_density);
		if (c.density <= 0) c.density = DEFAULTS.density;
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.layers < 1) c.layers = DEFAULTS.layers;
		if (c.size <= 0) c.size = DEFAULTS.size;
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

	class Snow {
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
			const rng = this._eventRng(name.length + 17);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.5 ? -1 : 1) * this.cfg.gust_mult * (0.45 + rng() * 0.55);
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
			if (!this.timers.gust || this.timers.gust <= 0) {
				this.values.gust_push = 0;
			}
		}

		_densityLevel() {
			let level = this.cfg.density;
			if (this.timers.gust > 0) level *= 1.28;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.02, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#09111d');
				sky.addColorStop(0.58, '#102033');
				sky.addColorStop(1, '#17263a');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.8);

			const moonX = canvasW * (0.16 + this._hash(401) * 0.18);
			const moonY = canvasH * (0.14 + this._hash(402) * 0.08);
			const moonR = Math.max(12, Math.min(canvasW, canvasH) * 0.035);
			const moon = ctx.createRadialGradient(moonX, moonY, 0, moonX, moonY, moonR * 2.6);
			moon.addColorStop(0, 'rgba(225, 234, 255, 0.18)');
			moon.addColorStop(1, 'rgba(225, 234, 255, 0)');
			ctx.fillStyle = moon;
			ctx.fillRect(0, 0, canvasW, canvasH);

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const hue = ((this.cfg.hue - 8) % 360 + 360) % 360;
				const light = clamp01(0.06 + 0.2 * ratio);
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.55), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
			}

			const treeCount = 13;
			for (let i = 0; i < treeCount; i++) {
				const center = Math.floor((i + 0.5) * this.w / treeCount + (this._hash(500 + i) - 0.5) * 6);
				const trunkH = 1 + Math.floor(this._hash(530 + i) * 2);
				const crownH = 8 + Math.floor(this._hash(560 + i) * 9);
				const maxHalf = 2 + Math.floor(this._hash(590 + i) * 4);
				const hue = ((this.cfg.hue - 26) % 360 + 360) % 360;
				const treeColor = hslToRGB(hue, clamp01(this.cfg.sat * 0.6), 0.18 + this._hash(620 + i) * 0.08);
				for (let row = 0; row < crownH; row++) {
					const width = Math.max(1, maxHalf - Math.floor(row / 2));
					const y = groundRow - crownH + row;
					for (let dx = -width; dx <= width; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center + dx, y, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
					}
				}
				for (let row = 0; row < trunkH; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center, groundRow + row - trunkH, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
				}
			}

			const density = this._densityLevel();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(8, Math.round(this.w * density * (0.35 + layerRatio * 0.8)));
				const baseSpeed = this.cfg.speed * (0.4 + layerRatio * 0.85);
				const drift = this.cfg.drift * (0.35 + layerRatio * 0.65) + (this.values.gust_push || 0) * 0.035 * (0.5 + layerRatio * 0.65);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1000 + i;
					const baseX = this._hash(1000 + idx) * this.w;
					const baseY = this._hash(2000 + idx) * Math.max(1, groundRow - 2);
					const sway = (this._hash(3000 + idx) * 2 - 1) * this.cfg.sway * (1.4 + layerRatio * 2.4);
					const fall = baseY + this.tick * baseSpeed * (0.75 + this._hash(4000 + idx) * 0.5);
					const row = positiveMod(fall, Math.max(1, groundRow - 2));
					const col = positiveMod(baseX + this.tick * drift + Math.sin(this.tick * 0.035 + idx * 0.19) * sway, this.w);
					const hue = ((this.cfg.hue + (this._hash(5000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.35 + 0.55 * (0.3 + layerRatio * 0.7)));
					const alpha = clamp01(0.35 + 0.55 * (0.25 + layerRatio * 0.75));
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}
		}
	}

	api.presets['snow'] = [
		{
			key: 'quiet-flurries',
			label: 'quiet flurries',
			config: {
				density: 0.2,
				speed: 0.38,
				drift: 0.04,
				sway: 0.35,
				layers: 2,
				size: 1,
				hue: 208,
				hue_sp: 8,
				sat: 0.12,
				lmin: 0.76,
				lmax: 0.96,
				calm_p: 0.0012,
			},
		},
		{
			key: 'pine-evening',
			label: 'pine evening',
			config: {
				density: 0.3,
				speed: 0.5,
				drift: 0.08,
				sway: 0.4,
				layers: 3,
				size: 1,
				hue: 214,
				hue_sp: 12,
				sat: 0.16,
				lmin: 0.74,
				lmax: 0.98,
				gust_p: 0.0008,
			},
		},
		{
			key: 'crosswind',
			label: 'crosswind',
			config: {
				density: 0.34,
				speed: 0.56,
				drift: 0.16,
				sway: 0.58,
				layers: 3,
				size: 1.2,
				hue: 206,
				hue_sp: 10,
				sat: 0.14,
				lmin: 0.72,
				lmax: 0.98,
				gust_p: 0.0015,
				gust_mult: 2.25,
				gust_dur: 68,
			},
		},
		{
			key: 'whiteout-edge',
			label: 'whiteout edge',
			config: {
				intro_density: 0.22,
				ending_density: 0.14,
				density: 0.52,
				speed: 0.7,
				drift: 0.12,
				sway: 0.74,
				layers: 4,
				size: 1.5,
				hue: 212,
				hue_sp: 16,
				sat: 0.18,
				lmin: 0.76,
				lmax: 1,
				gust_p: 0.0018,
				gust_mult: 2.8,
				gust_dur: 76,
				calm_p: 0.0003,
			},
		},
	];
	api.effects['snow'] = Snow;
})(window.AmbienceSim);
