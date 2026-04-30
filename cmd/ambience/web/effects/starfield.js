'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 50,
		intro_density: 0.08,
		ending_dur: 60,
		ending_linger: 16,
		ending_density: 0.03,
		density: 0.22,
		speed: 0.12,
		drift: 0.04,
		layers: 3,
		size: 1,
		hue: 218,
		hue_sp: 18,
		sat: 0.18,
		lmin: 0.55,
		lmax: 0.95,
		shooting_star_p: 0,
		twinkle_burst_p: 0,
		shooting_star_dur: 26,
		shooting_star_mult: 1.8,
		twinkle_burst_dur: 42,
		twinkle_burst_mult: 1.7,
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
		if (c.shooting_star_dur <= 0) c.shooting_star_dur = DEFAULTS.shooting_star_dur;
		if (c.shooting_star_mult <= 0) c.shooting_star_mult = DEFAULTS.shooting_star_mult;
		if (c.twinkle_burst_dur <= 0) c.twinkle_burst_dur = DEFAULTS.twinkle_burst_dur;
		if (c.twinkle_burst_mult <= 0) c.twinkle_burst_mult = DEFAULTS.twinkle_burst_mult;
		return c;
	}

	class Starfield {
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
			const rng = this._eventRng(name.length + 41);
			switch (name) {
				case 'shooting-star':
					this.timers['shooting-star'] = jitterInt(rng, this.cfg.shooting_star_dur, 0.3);
					this.values['shooting-star_total'] = this.timers['shooting-star'];
					this.values.shooting_dir = rng() < 0.5 ? -1 : 1;
					this.values.shooting_row = 6 + rng() * Math.max(4, this.h / 3);
					this.values.shooting_start = rng() * this.w;
					return true;
				case 'twinkle-burst':
					this.timers['twinkle-burst'] = jitterInt(rng, this.cfg.twinkle_burst_dur, 0.3);
					return true;
				case 'intro':
					this.timers.ending = 0;
					this.timers['shooting-star'] = 0;
					this.timers['twinkle-burst'] = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['shooting-star'] = 0;
					this.timers['twinkle-burst'] = 0;
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
		}

		_densityLevel() {
			let level = this.cfg.density;
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
				sky.addColorStop(0, '#050912');
				sky.addColorStop(0.6, '#090f20');
				sky.addColorStop(1, '#0b1128');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const density = this._densityLevel();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			const burst = this.timers['twinkle-burst'] > 0 ? this.cfg.twinkle_burst_mult : 1;

			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(10, Math.round(this.w * density * (0.4 + layerRatio * 1.2)));
				const speed = this.cfg.speed * (0.18 + layerRatio * 0.82);
				const drift = this.cfg.drift * (0.25 + layerRatio * 0.9);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1400 + i;
					const baseX = this._hash(15000 + idx) * this.w;
					const baseY = this._hash(16000 + idx) * this.h;
					const col = positiveMod(baseX + this.tick * drift * speed * 2, this.w);
					const row = baseY;
					const hue = ((this.cfg.hue + (this._hash(17000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const twinkle = 0.4 + 0.6 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * (0.02 + this._hash(18000 + idx) * 0.03) + idx), 2);
					const light = clamp01((this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + layerRatio * 0.7)) * twinkle * burst);
					const alpha = clamp01(0.35 + 0.25 * layerRatio + 0.25 * twinkle);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			if (this.timers['shooting-star'] > 0) {
				const total = Math.max(1, this.values['shooting-star_total'] || this.cfg.shooting_star_dur);
				const progress = 1 - (this.timers['shooting-star'] / total);
				const dir = this.values.shooting_dir || 1;
				const row = this.values.shooting_row || this.h * 0.25;
				const start = this.values.shooting_start || this.w * 0.25;
				const head = positiveMod(start + dir * progress * this.w * 0.6, this.w);
				for (let i = 0; i < 7; i++) {
					const fade = 1 - i / 7;
					const x = Math.round(head - dir * i * 1.5);
					const y = Math.round(row + i * 0.6);
					const light = clamp01(this.cfg.lmax * this.cfg.shooting_star_mult * fade * 0.55);
					const color = hslToRGB(this.cfg.hue - 8, clamp01(this.cfg.sat * 0.9), light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1 + (i === 0 ? 1 : 0), 1, `rgb(${color.r},${color.g},${color.b})`, fade);
				}
			}
		}
	}

	api.presets['starfield'] = [
		{
			key: 'still-night',
			label: 'still night',
			config: {
				density: 0.16,
				speed: 0.08,
				drift: 0.02,
				layers: 2,
				size: 1,
				hue: 214,
				hue_sp: 12,
				sat: 0.16,
				lmin: 0.5,
				lmax: 0.9,
			},
		},
		{
			key: 'soft-parallax',
			label: 'soft parallax',
			config: {
				density: 0.22,
				speed: 0.12,
				drift: 0.04,
				layers: 3,
				size: 1,
				hue: 218,
				hue_sp: 18,
				sat: 0.18,
				lmin: 0.55,
				lmax: 0.95,
				twinkle_burst_p: 0.0006,
			},
		},
		{
			key: 'meteor-watch',
			label: 'meteor watch',
			config: {
				density: 0.24,
				speed: 0.14,
				drift: 0.06,
				layers: 3,
				size: 1.2,
				hue: 214,
				hue_sp: 22,
				sat: 0.2,
				lmin: 0.56,
				lmax: 0.96,
				shooting_star_p: 0.0012,
				shooting_star_mult: 2.4,
			},
		},
		{
			key: 'cold-deep-space',
			label: 'cold deep space',
			config: {
				density: 0.2,
				speed: 0.09,
				drift: 0.03,
				layers: 4,
				size: 1,
				hue: 226,
				hue_sp: 26,
				sat: 0.22,
				lmin: 0.52,
				lmax: 0.94,
				twinkle_burst_p: 0.0009,
				twinkle_burst_mult: 1.9,
			},
		},
	];
	api.effects['starfield'] = Starfield;
})(window.AmbienceSim);
