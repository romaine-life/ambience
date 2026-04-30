'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 55,
		intro_reveal: 0.14,
		ending_dur: 70,
		ending_linger: 22,
		ending_murk: 0.08,
		density: 0.28,
		rise_speed: 0.42,
		drift: 0.1,
		sway: 0.54,
		weed_height: 20,
		weed_count: 11,
		caustics: 0.3,
		depth: 0.56,
		hue: 192,
		hue_sp: 18,
		sat: 0.42,
		lmin: 0.12,
		lmax: 0.82,
		bubble_burst_p: 0,
		current_shift_p: 0,
		calm_p: 0,
		bubble_burst_dur: 38,
		bubble_burst_mult: 1.9,
		current_shift_dur: 62,
		current_shift_push: 1.2,
		calm_dur: 74,
		calm_mult: 0.55,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_reveal = clamp01(c.intro_reveal);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_murk = clamp01(c.ending_murk);
		if (c.density <= 0) c.density = DEFAULTS.density;
		if (c.rise_speed <= 0) c.rise_speed = DEFAULTS.rise_speed;
		if (c.sway <= 0) c.sway = DEFAULTS.sway;
		if (c.weed_height <= 0) c.weed_height = DEFAULTS.weed_height;
		if (c.weed_count < 1) c.weed_count = DEFAULTS.weed_count;
		if (c.caustics <= 0) c.caustics = DEFAULTS.caustics;
		if (c.depth <= 0) c.depth = DEFAULTS.depth;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.bubble_burst_dur <= 0) c.bubble_burst_dur = DEFAULTS.bubble_burst_dur;
		if (c.bubble_burst_mult <= 0) c.bubble_burst_mult = DEFAULTS.bubble_burst_mult;
		if (c.current_shift_dur <= 0) c.current_shift_dur = DEFAULTS.current_shift_dur;
		if (c.current_shift_push <= 0) c.current_shift_push = DEFAULTS.current_shift_push;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class Underwater {
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
			const rng = this._eventRng(name.length + 89);
			switch (name) {
				case 'bubble-burst':
					this.timers['bubble-burst'] = jitterInt(rng, this.cfg.bubble_burst_dur, 0.3);
					this.values.bubble_gain = this.cfg.bubble_burst_mult * (0.8 + rng() * 0.45);
					return true;
				case 'current-shift':
					this.timers['current-shift'] = jitterInt(rng, this.cfg.current_shift_dur, 0.3);
					this.timers.calm = 0;
					this.values.current_push = (rng() < 0.5 ? -1 : 1) * this.cfg.current_shift_push * (0.55 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.timers['current-shift'] = 0;
					this.values.current_push = 0;
					return true;
				case 'intro':
					this.timers['bubble-burst'] = 0;
					this.timers['current-shift'] = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.bubble_gain = 1;
					this.values.current_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['bubble-burst'] = 0;
					this.timers['current-shift'] = 0;
					this.timers.calm = 0;
					this.values.bubble_gain = 1;
					this.values.current_push = 0;
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
			if (!this.timers['bubble-burst'] || this.timers['bubble-burst'] <= 0) this.values.bubble_gain = 1;
			if (!this.timers['current-shift'] || this.timers['current-shift'] <= 0) this.values.current_push = 0;
		}

		_sceneLevel() {
			let level = 1;
			if (this.timers['bubble-burst'] > 0) level *= this.values.bubble_gain || this.cfg.bubble_burst_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_reveal + (1 - this.cfg.intro_reveal) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_murk) * progress;
			}
			if (this.timers['current-shift'] > 0) {
				level *= 1 + Math.abs(this.values.current_push || this.cfg.current_shift_push) * 0.18;
			}
			return Math.max(0.04, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const top = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.58), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.54));
				const mid = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.28));
				const deep = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin * (0.72 - this.cfg.depth * 0.18)));
				const water = ctx.createLinearGradient(0, 0, 0, canvasH);
				water.addColorStop(0, `rgb(${top.r},${top.g},${top.b})`);
				water.addColorStop(0.46, `rgb(${mid.r},${mid.g},${mid.b})`);
				water.addColorStop(1, `rgb(${deep.r},${deep.g},${deep.b})`);
				ctx.fillStyle = water;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const level = this._sceneLevel();
			const currentPush = this.timers['current-shift'] > 0 ? (this.values.current_push || this.cfg.current_shift_push) : 0;
			const floorBase = Math.max(Math.floor(this.h * 0.78), Math.min(this.h - 4, Math.floor(this.h * (0.84 + this.cfg.depth * 0.08))));
			const phase = this.tick * this.cfg.rise_speed * 0.04;

			const surfaceGlow = ctx.createRadialGradient(canvasW * 0.32, 0, 0, canvasW * 0.32, 0, Math.max(canvasW, canvasH) * 0.52);
			surfaceGlow.addColorStop(0, `rgba(210, 244, 238, ${clamp01(0.08 + this.cfg.caustics * 0.18 * level)})`);
			surfaceGlow.addColorStop(1, 'rgba(210, 244, 238, 0)');
			ctx.fillStyle = surfaceGlow;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const beamCount = 4;
			for (let i = 0; i < beamCount; i++) {
				const sourceX = canvasW * (0.08 + i * 0.24 + this._hash(30100 + i) * 0.08);
				const spread = canvasW * (0.08 + this._hash(30200 + i) * 0.08);
				const bend = (currentPush * 12 + Math.sin(this.tick * 0.02 + i) * 10) * this.cfg.caustics;
				ctx.fillStyle = `rgba(210, 248, 242, ${clamp01(0.04 + this.cfg.caustics * 0.12 * level)})`;
				ctx.beginPath();
				ctx.moveTo(sourceX - spread * 0.2, 0);
				ctx.lineTo(sourceX + spread * 0.22, 0);
				ctx.lineTo(sourceX + spread + bend, canvasH * 0.7);
				ctx.lineTo(sourceX - spread * 0.65 + bend * 0.45, canvasH * 0.7);
				ctx.closePath();
				ctx.fill();
			}

			const causticColor = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmax * 0.96));
			for (let band = 0; band < 5; band++) {
				const baseY = Math.floor(this.h * (0.16 + band * 0.09));
				for (let x = 0; x < this.w; x++) {
					if ((x + band) % 2 !== 0) continue;
					const wave = Math.sin(x * 0.16 + phase * (1.2 + band * 0.18) + band) + Math.sin(x * 0.07 - phase * 1.4 + band * 1.7);
					const row = baseY + Math.round(wave * this.cfg.caustics * level * 1.3);
					const alpha = clamp01((0.03 + this.cfg.caustics * 0.18 * level) * (0.82 - band * 0.12));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${causticColor.r},${causticColor.g},${causticColor.b})`, alpha);
				}
			}

			const particulateColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmax * 0.8));
			const particulateCount = Math.max(18, Math.round(this.w * 0.14));
			for (let i = 0; i < particulateCount; i++) {
				const col = Math.floor(this._hash(30300 + i) * this.w);
				const row = Math.floor(this._hash(30400 + i) * Math.max(1, floorBase - 6));
				const blink = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.018 + i * 0.6), 2);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${particulateColor.r},${particulateColor.g},${particulateColor.b})`, clamp01((0.04 + this.cfg.depth * 0.08) * blink));
			}

			const seabedPoints = [];
			const seabedSegments = 8;
			for (let i = 0; i <= seabedSegments; i++) {
				seabedPoints.push(Math.round(floorBase - Math.abs(Math.sin(i * 0.8 + this._hash(30500 + i) * 2.4)) * 2 - this._hash(30600 + i) * 2));
			}
			const seabedColor = hslToRGB((this.cfg.hue + 36) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 0.85));
			ctx.fillStyle = `rgb(${seabedColor.r},${seabedColor.g},${seabedColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * seabedSegments;
				const idx = Math.min(seabedSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const row = seabedPoints[idx] + (seabedPoints[idx + 1] - seabedPoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(row * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const weedColor = hslToRGB((this.cfg.hue - 36 + 360) % 360, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.28));
			const weedAccent = hslToRGB((this.cfg.hue - 18 + 360) % 360, clamp01(this.cfg.sat * 0.48), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.4));
			const weedCount = Math.max(4, Math.round(this.cfg.weed_count));
			for (let i = 0; i < weedCount; i++) {
				const baseX = Math.floor((i + 0.35) * this.w / weedCount + (this._hash(30700 + i) - 0.5) * 5);
				const rootY = floorBase - 1 - Math.floor(this._hash(30800 + i) * 3);
				const fronds = 2 + Math.floor(this._hash(30900 + i) * 2);
				for (let f = 0; f < fronds; f++) {
					const height = Math.max(7, Math.round(this.cfg.weed_height * (0.58 + this._hash(31000 + i * 5 + f) * 0.5)));
					const offset = (f - (fronds - 1) / 2) * 1.2;
					const localPhase = this.tick * 0.035 * (0.8 + this._hash(31100 + i * 5 + f) * 0.4) + i * 0.7 + f * 0.4;
					for (let seg = 0; seg < height; seg++) {
						const progress = seg / Math.max(1, height - 1);
						const sway = Math.sin(localPhase + progress * 2.6) * this.cfg.sway * level * (1.1 + Math.abs(currentPush) * 0.55) + currentPush * progress * 1.4;
						const x = Math.round(baseX + offset + sway * progress * 1.2);
						const y = rootY - seg;
						const color = seg < height * 0.28 ? weedColor : weedAccent;
						const alpha = clamp01(0.3 + progress * 0.42);
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, seg < height * 0.25 ? 2 : 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					}
				}
			}

			const burstGain = this.timers['bubble-burst'] > 0 ? (this.values.bubble_gain || this.cfg.bubble_burst_mult) : 1;
			const bubbleDensity = Math.max(0.04, this.cfg.density * level);
			const bubbleCount = Math.max(12, Math.round(this.w * bubbleDensity * (0.44 + Math.max(0, burstGain - 1) * 0.18)));
			const bubbleColor = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 0.98));
			for (let i = 0; i < bubbleCount; i++) {
				const baseX = this._hash(31200 + i) * this.w;
				const baseY = this._hash(31300 + i) * Math.max(6, floorBase - 6);
				const rise = baseY - this.tick * this.cfg.rise_speed * (0.55 + this._hash(31400 + i) * 0.7);
				const row = 1 + positiveMod(rise, Math.max(1, floorBase - 5));
				const drift = this.cfg.drift * (0.4 + this._hash(31500 + i) * 0.9) + currentPush * 0.05 * (0.45 + this._hash(31600 + i) * 0.55);
				const wobble = Math.sin(this.tick * 0.03 + i * 0.72) * this.cfg.sway * (0.6 + this._hash(31700 + i) * 0.5);
				const col = positiveMod(baseX + this.tick * drift + wobble, this.w);
				const size = this._hash(31800 + i) > 0.82 ? 2 : 1;
				const alpha = clamp01((0.22 + this._hash(31900 + i) * 0.28) * (0.8 + Math.max(0, burstGain - 1) * 0.14));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${bubbleColor.r},${bubbleColor.g},${bubbleColor.b})`, alpha);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), 1, 1, `rgb(235,248,246)`, clamp01(alpha * 0.62));
			}
		}
	}

	api.presets['underwater'] = [
		{
			key: 'quiet-shallows',
			label: 'quiet shallows',
			config: {
				intro_reveal: 0.18,
				ending_murk: 0.12,
				density: 0.18,
				rise_speed: 0.32,
				drift: 0.04,
				sway: 0.34,
				weed_height: 16,
				weed_count: 9,
				caustics: 0.44,
				depth: 0.28,
				hue: 184,
				hue_sp: 12,
				sat: 0.38,
				lmin: 0.14,
				lmax: 0.86,
				calm_p: 0.0011,
			},
		},
		{
			key: 'bubble-field',
			label: 'bubble field',
			config: {
				density: 0.42,
				rise_speed: 0.54,
				drift: 0.08,
				sway: 0.46,
				weed_height: 18,
				weed_count: 8,
				caustics: 0.26,
				depth: 0.42,
				hue: 190,
				hue_sp: 16,
				sat: 0.42,
				lmin: 0.12,
				lmax: 0.82,
				bubble_burst_p: 0.0012,
			},
		},
		{
			key: 'slow-current',
			label: 'slow current',
			config: {
				density: 0.28,
				rise_speed: 0.4,
				drift: 0.12,
				sway: 0.78,
				weed_height: 22,
				weed_count: 11,
				caustics: 0.3,
				depth: 0.56,
				hue: 192,
				hue_sp: 18,
				sat: 0.42,
				lmin: 0.12,
				lmax: 0.82,
				current_shift_p: 0.0011,
			},
		},
		{
			key: 'deep-water',
			label: 'deep water',
			config: {
				intro_reveal: 0.1,
				ending_murk: 0.16,
				density: 0.16,
				rise_speed: 0.28,
				drift: 0.05,
				sway: 0.26,
				weed_height: 13,
				weed_count: 6,
				caustics: 0.14,
				depth: 0.82,
				hue: 204,
				hue_sp: 10,
				sat: 0.3,
				lmin: 0.08,
				lmax: 0.62,
				calm_p: 0.0014,
				calm_mult: 0.42,
			},
		},
	];
	api.effects['underwater'] = Underwater;
})(window.AmbienceSim);
