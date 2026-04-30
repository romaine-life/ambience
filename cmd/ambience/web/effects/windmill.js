'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 45,
		intro_turn: 0.12,
		ending_dur: 60,
		ending_linger: 20,
		ending_turn: 0.05,
		turn_speed: 0.08,
		blade_len: 14,
		blade_width: 1.8,
		tower_height: 20,
		tower_width: 6,
		horizon: 0.72,
		glow: 0.18,
		hue: 28,
		hue_sp: 18,
		sat: 0.42,
		lmin: 0.18,
		lmax: 0.82,
		gust_p: 0,
		lull_p: 0,
		gust_dur: 50,
		gust_mult: 1.9,
		lull_dur: 72,
		lull_mult: 0.45,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_turn = clamp01(c.intro_turn);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_turn = clamp01(c.ending_turn);
		if (c.turn_speed <= 0) c.turn_speed = DEFAULTS.turn_speed;
		if (c.blade_len <= 0) c.blade_len = DEFAULTS.blade_len;
		if (c.blade_width <= 0) c.blade_width = DEFAULTS.blade_width;
		if (c.tower_height <= 0) c.tower_height = DEFAULTS.tower_height;
		if (c.tower_width <= 0) c.tower_width = DEFAULTS.tower_width;
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
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
		return c;
	}

	class Windmill {
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
			const rng = this._eventRng(name.length + 71);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_gain = this.cfg.gust_mult * (0.75 + rng() * 0.45);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.gust_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.values.gust_gain = 1;
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
			if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_gain = 1;
		}

		_rotationLevel() {
			let level = 1;
			if (this.timers.gust > 0) level *= this.values.gust_gain || this.cfg.gust_mult;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_turn + (1 - this.cfg.intro_turn) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_turn) * progress;
			}
			return Math.max(0.03, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue + 210) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.95));
				const skyMid = hslToRGB((this.cfg.hue + 248) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.32));
				const skyLow = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.78));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.58, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.min(this.h - 8, Math.floor(this.h * this.cfg.horizon)));
			const centerX = Math.floor(this.w * 0.58);
			const rotationLevel = this._rotationLevel();
			const angle = this.tick * this.cfg.turn_speed * rotationLevel + Math.PI * 0.08;
			const towerH = Math.max(10, Math.round(this.cfg.tower_height));
			const towerW = Math.max(3, Math.round(this.cfg.tower_width));
			const bladeLen = Math.max(5, Math.round(this.cfg.blade_len));
			const bladeWidth = Math.max(1, this.cfg.blade_width);

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 3) * sy), 0, Math.floor((horizon + 7) * sy));
			horizonGlow.addColorStop(0, `rgba(255, 214, 163, ${0.04 + this.cfg.glow * 0.12})`);
			horizonGlow.addColorStop(1, 'rgba(255, 214, 163, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 3) * sy), canvasW, Math.ceil(12 * sy));

			const hillRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const broad = Math.sin(x * 0.045 + 0.4) * 1.2 + Math.sin(x * 0.012 + 1.3) * 2.1;
				const mound = Math.exp(-Math.pow((x - centerX) / 18, 2)) * 6.2 + Math.exp(-Math.pow((x - this.w * 0.18) / 24, 2)) * 2.1;
				hillRows[x] = Math.round(horizon + broad - mound);
			}

			const hillColor = hslToRGB((this.cfg.hue + 205) % 360, clamp01(this.cfg.sat * 0.16), 0.08);
			ctx.fillStyle = `rgb(${hillColor.r},${hillColor.g},${hillColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				ctx.lineTo(Math.floor(x * sx), Math.floor(hillRows[x] * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const baseY = hillRows[Math.max(0, Math.min(this.w - 1, centerX))];
			const hubY = baseY - towerH + 2;
			const millColor = hslToRGB((this.cfg.hue + 208) % 360, clamp01(this.cfg.sat * 0.08), 0.1);
			for (let y = hubY; y <= baseY; y++) {
				const ratio = (y - hubY) / Math.max(1, baseY - hubY);
				const half = Math.max(1, Math.round((towerW * (0.38 + ratio * 0.62)) * 0.5));
				for (let dx = -half; dx <= half; dx++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, y, 1, 1, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);
				}
			}

			for (let dx = -Math.max(2, Math.round(towerW * 0.42)); dx <= Math.max(2, Math.round(towerW * 0.42)); dx++) {
				const roofY = hubY - 2 + Math.abs(dx) * 0.4;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, Math.round(roofY), 1, 1, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);
			}

			const windowGlow = hslToRGB(42, 0.72, clamp01(0.38 + this.cfg.glow * 0.5));
			const windowY = Math.round(hubY + towerH * 0.46);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX, windowY, 1, 2, `rgb(${windowGlow.r},${windowGlow.g},${windowGlow.b})`, clamp01(0.18 + this.cfg.glow * 0.58));

			const bladeColor = hslToRGB((this.cfg.hue + 210) % 360, clamp01(this.cfg.sat * 0.08), 0.13);
			for (let blade = 0; blade < 4; blade++) {
				const theta = angle + blade * Math.PI * 0.5;
				const px = -Math.sin(theta);
				const py = Math.cos(theta) * 0.88;
				for (let r = 1; r <= bladeLen; r++) {
					const fade = 1 - r / Math.max(1, bladeLen);
					const bx = centerX + Math.cos(theta) * r;
					const by = hubY + Math.sin(theta) * r * 0.88;
					const half = Math.max(0, Math.round(bladeWidth * fade * 0.55));
					for (let spread = -half; spread <= half; spread++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(bx + px * spread * 0.7), Math.round(by + py * spread * 0.7), 1, 1, `rgb(${bladeColor.r},${bladeColor.g},${bladeColor.b})`, 1);
					}
				}
			}

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - 1, hubY - 1, 3, 3, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);

			const grassColor = hslToRGB((this.cfg.hue + 120) % 360, 0.16, 0.14);
			for (let x = 0; x < this.w; x += 2) {
				const top = hillRows[x];
				if ((x + this.tick) % 5 !== 0) continue;
				const sway = this.timers.gust > 0 ? (this.values.gust_gain || this.cfg.gust_mult) * 0.2 : this.timers.lull > 0 ? -this.cfg.lull_mult * 0.08 : 0.04;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + Math.round(Math.sin(this.tick * 0.05 + x * 0.1) * sway), top - 1, 1, 2, `rgb(${grassColor.r},${grassColor.g},${grassColor.b})`, 0.28);
			}
		}
	}

	api.presets['windmill'] = [
		{
			key: 'still-dusk',
			label: 'still dusk',
			config: {
				intro_turn: 0.08,
				ending_turn: 0.04,
				turn_speed: 0.04,
				blade_len: 12,
				blade_width: 1.6,
				tower_height: 19,
				tower_width: 5.5,
				horizon: 0.74,
				glow: 0.22,
				hue: 26,
				hue_sp: 14,
				sat: 0.38,
				lmin: 0.16,
				lmax: 0.78,
			},
		},
		{
			key: 'steady-turning',
			label: 'steady turning',
			config: {
				turn_speed: 0.08,
				blade_len: 14,
				blade_width: 1.8,
				tower_height: 20,
				tower_width: 6,
				horizon: 0.72,
				glow: 0.18,
				hue: 28,
				hue_sp: 18,
				sat: 0.42,
				lmin: 0.18,
				lmax: 0.82,
				gust_p: 0.0006,
			},
		},
		{
			key: 'windy-hill',
			label: 'windy hill',
			config: {
				turn_speed: 0.12,
				blade_len: 15,
				blade_width: 2.1,
				tower_height: 21,
				tower_width: 6.5,
				horizon: 0.7,
				glow: 0.14,
				hue: 24,
				hue_sp: 20,
				sat: 0.4,
				lmin: 0.16,
				lmax: 0.8,
				gust_p: 0.0014,
				gust_mult: 2.2,
				gust_dur: 62,
			},
		},
		{
			key: 'silhouette-mill',
			label: 'silhouette mill',
			config: {
				turn_speed: 0.06,
				blade_len: 16,
				blade_width: 1.5,
				tower_height: 23,
				tower_width: 5,
				horizon: 0.76,
				glow: 0.1,
				hue: 222,
				hue_sp: 12,
				sat: 0.22,
				lmin: 0.12,
				lmax: 0.68,
				lull_p: 0.0012,
				lull_mult: 0.38,
			},
		},
	];
	api.effects['windmill'] = Windmill;
})(window.AmbienceSim);
