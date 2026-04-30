'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 55,
		intro_density: 0.12,
		ending_dur: 60,
		ending_linger: 18,
		ending_density: 0.04,
		density: 0.24,
		speed: 0.44,
		drift: 0.18,
		sway: 0.86,
		layers: 2,
		size: 1.2,
		hue: 28,
		hue_sp: 24,
		sat: 0.62,
		lmin: 0.38,
		lmax: 0.78,
		gust_p: 0,
		lull_p: 0,
		swirl_p: 0,
		gust_dur: 48,
		gust_mult: 1.9,
		lull_dur: 72,
		lull_mult: 0.35,
		swirl_dur: 52,
		swirl_pull: 1.15,
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
		if (c.lull_dur <= 0) c.lull_dur = DEFAULTS.lull_dur;
		if (c.lull_mult <= 0) c.lull_mult = DEFAULTS.lull_mult;
		if (c.swirl_dur <= 0) c.swirl_dur = DEFAULTS.swirl_dur;
		if (c.swirl_pull <= 0) c.swirl_pull = DEFAULTS.swirl_pull;
		return c;
	}

	class AutumnLeaves {
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
			const rng = this._eventRng(name.length + 29);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.5 ? -1 : 1) * this.cfg.gust_mult * (0.5 + rng() * 0.7);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'swirl':
					this.timers.swirl = jitterInt(rng, this.cfg.swirl_dur, 0.3);
					this.values.swirl_spin = (rng() < 0.5 ? -1 : 1) * this.cfg.swirl_pull * (0.65 + rng() * 0.45);
					this.values.swirl_row = Math.max(8, this.h / 3) + rng() * Math.max(1, this.h / 2);
					this.values.swirl_col = rng() * this.w;
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.swirl = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.values.swirl_spin = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.swirl = 0;
					this.values.gust_push = 0;
					this.values.swirl_spin = 0;
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
			if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_push = 0;
			if (!this.timers.swirl || this.timers.swirl <= 0) this.values.swirl_spin = 0;
		}

		_densityLevel() {
			let level = this.cfg.density;
			if (this.timers.gust > 0) level *= 1.22;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
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
			return Math.max(0.015, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#20170f');
				sky.addColorStop(0.52, '#58422b');
				sky.addColorStop(1, '#7c6042');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.82);

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const ground = hslToRGB(38, 0.35, 0.1 + ratio * 0.16);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${ground.r},${ground.g},${ground.b})`, 1);
			}

			for (let x = 0; x < this.w; x++) {
				const canopyDepth = 4 + Math.floor(this._hash(6100 + x) * 6);
				const shade = hslToRGB(24 + this._hash(6200 + x) * 18, 0.45, 0.14 + this._hash(6300 + x) * 0.08);
				for (let y = 0; y < canopyDepth; y++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${shade.r},${shade.g},${shade.b})`, 0.75);
				}
			}

			const density = this._densityLevel();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(6, Math.round(this.w * density * (0.28 + layerRatio * 0.62)));
				const baseSpeed = this.cfg.speed * (0.34 + layerRatio * 0.72);
				const drift = this.cfg.drift * (0.45 + layerRatio * 0.65) + (this.values.gust_push || 0) * 0.04 * (0.5 + layerRatio * 0.7);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio * 0.8));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1000 + i;
					const baseX = this._hash(7000 + idx) * this.w;
					const baseY = this._hash(8000 + idx) * Math.max(1, groundRow - 2);
					const flutter = (this._hash(9000 + idx) * 2 - 1) * this.cfg.sway * (2.4 + layerRatio * 2.8);
					let row = positiveMod(baseY + this.tick * baseSpeed * (0.7 + this._hash(10000 + idx) * 0.55), Math.max(1, groundRow - 2));
					let col = positiveMod(baseX + this.tick * drift + Math.sin(this.tick * 0.04 + idx * 0.23) * flutter, this.w);
					if (this.timers.swirl > 0) {
						const sr = this.values.swirl_row || this.h * 0.55;
						const sc = this.values.swirl_col || this.w * 0.5;
						const angle = Math.atan2(row - sr, col - sc) + (this.values.swirl_spin || 0) * 0.015;
						const radius = Math.hypot(col - sc, row - sr);
						col = positiveMod(sc + Math.cos(angle) * radius, this.w);
						row = Math.max(0, Math.min(groundRow - 2, sr + Math.sin(angle) * radius * 0.94));
					}
					const hue = ((this.cfg.hue + (this._hash(11000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + this._hash(12000 + idx) * 0.7));
					const alpha = clamp01(0.4 + layerRatio * 0.45);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if ((idx + this.tick) % 3 === 0) {
						const accent = hslToRGB((hue + 12) % 360, clamp01(this.cfg.sat * 0.85), clamp01(light * 1.08));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col) + (this._hash(13000 + idx) < 0.5 ? 1 : 0), Math.round(row), 1, size > 1 ? 1 : 0.8, `rgb(${accent.r},${accent.g},${accent.b})`, alpha * 0.8);
					}
				}
			}
		}
	}

	api.presets['autumn-leaves'] = [
		{
			key: 'few-leaves',
			label: 'few leaves',
			config: {
				density: 0.14,
				speed: 0.36,
				drift: 0.12,
				sway: 0.7,
				layers: 1,
				size: 1,
				hue: 24,
				hue_sp: 18,
				sat: 0.58,
				lmin: 0.36,
				lmax: 0.7,
				lull_p: 0.0014,
			},
		},
		{
			key: 'gentle-fall',
			label: 'gentle fall',
			config: {
				density: 0.24,
				speed: 0.44,
				drift: 0.18,
				sway: 0.86,
				layers: 2,
				size: 1.2,
				hue: 28,
				hue_sp: 24,
				sat: 0.62,
				lmin: 0.38,
				lmax: 0.78,
				gust_p: 0.0008,
			},
		},
		{
			key: 'windy-autumn',
			label: 'windy autumn',
			config: {
				density: 0.3,
				speed: 0.5,
				drift: 0.26,
				sway: 1.05,
				layers: 2,
				size: 1.4,
				hue: 22,
				hue_sp: 28,
				sat: 0.68,
				lmin: 0.36,
				lmax: 0.8,
				gust_p: 0.0016,
				gust_mult: 2.35,
			},
		},
		{
			key: 'swirl-study',
			label: 'swirl study',
			config: {
				density: 0.28,
				speed: 0.42,
				drift: 0.12,
				sway: 1.15,
				layers: 2,
				size: 1.4,
				hue: 30,
				hue_sp: 34,
				sat: 0.7,
				lmin: 0.4,
				lmax: 0.84,
				swirl_p: 0.0015,
				swirl_dur: 68,
				swirl_pull: 1.55,
			},
		},
	];
	api.effects['autumn-leaves'] = AutumnLeaves;
})(window.AmbienceSim);
