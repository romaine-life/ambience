'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 55,
		intro_tide: 0.18,
		ending_dur: 65,
		ending_linger: 18,
		ending_wet: 0.1,
		shoreline: 0.58,
		tide_amp: 6,
		wave_amp: 2.4,
		wave_freq: 0.18,
		speed: 0.1,
		slope: 0.16,
		foam: 0.36,
		shimmer: 0.22,
		hue: 198,
		hue_sp: 16,
		sat: 0.5,
		lmin: 0.28,
		lmax: 0.82,
		high_tide_p: 0,
		low_tide_p: 0,
		foam_burst_p: 0,
		high_tide_dur: 60,
		high_tide_push: 1.4,
		low_tide_dur: 58,
		low_tide_pull: 1.2,
		foam_burst_dur: 34,
		foam_burst_mult: 1.9,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_tide = clamp01(c.intro_tide);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_wet = clamp01(c.ending_wet);
		if (c.shoreline <= 0) c.shoreline = DEFAULTS.shoreline;
		if (c.tide_amp <= 0) c.tide_amp = DEFAULTS.tide_amp;
		if (c.wave_amp <= 0) c.wave_amp = DEFAULTS.wave_amp;
		if (c.wave_freq <= 0) c.wave_freq = DEFAULTS.wave_freq;
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.foam <= 0) c.foam = DEFAULTS.foam;
		if (c.shimmer <= 0) c.shimmer = DEFAULTS.shimmer;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.high_tide_dur <= 0) c.high_tide_dur = DEFAULTS.high_tide_dur;
		if (c.high_tide_push <= 0) c.high_tide_push = DEFAULTS.high_tide_push;
		if (c.low_tide_dur <= 0) c.low_tide_dur = DEFAULTS.low_tide_dur;
		if (c.low_tide_pull <= 0) c.low_tide_pull = DEFAULTS.low_tide_pull;
		if (c.foam_burst_dur <= 0) c.foam_burst_dur = DEFAULTS.foam_burst_dur;
		if (c.foam_burst_mult <= 0) c.foam_burst_mult = DEFAULTS.foam_burst_mult;
		return c;
	}

	class Beach {
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
			const rng = this._eventRng(name.length + 61);
			switch (name) {
				case 'high-tide':
					this.timers['high-tide'] = jitterInt(rng, this.cfg.high_tide_dur, 0.3);
					this.timers['low-tide'] = 0;
					this.values.tide_bias = this.cfg.high_tide_push * (0.65 + rng() * 0.55);
					return true;
				case 'low-tide':
					this.timers['low-tide'] = jitterInt(rng, this.cfg.low_tide_dur, 0.3);
					this.timers['high-tide'] = 0;
					this.values.tide_bias = -this.cfg.low_tide_pull * (0.65 + rng() * 0.55);
					return true;
				case 'foam-burst':
					this.timers['foam-burst'] = jitterInt(rng, this.cfg.foam_burst_dur, 0.3);
					this.values.foam_gain = this.cfg.foam_burst_mult * (0.85 + rng() * 0.35);
					return true;
				case 'intro':
					this.timers['high-tide'] = 0;
					this.timers['low-tide'] = 0;
					this.timers['foam-burst'] = 0;
					this.timers.ending = 0;
					this.values.tide_bias = 0;
					this.values.foam_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['high-tide'] = 0;
					this.timers['low-tide'] = 0;
					this.timers['foam-burst'] = 0;
					this.values.tide_bias = 0;
					this.values.foam_gain = 1;
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
			if ((!this.timers['high-tide'] || this.timers['high-tide'] <= 0) && (!this.timers['low-tide'] || this.timers['low-tide'] <= 0)) {
				this.values.tide_bias = 0;
			}
			if (!this.timers['foam-burst'] || this.timers['foam-burst'] <= 0) {
				this.values.foam_gain = 1;
			}
		}

		_tideLevel() {
			let level = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_tide + (1 - this.cfg.intro_tide) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_wet) * progress;
			}
			return Math.max(0.05, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#f4b17a');
				sky.addColorStop(0.38, '#f8d6a9');
				sky.addColorStop(0.68, '#cfe3e6');
				sky.addColorStop(1, '#8bb4c4');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.floor(this.h * 0.34));
			const tideLevel = this._tideLevel();
			const tideBias = this.values.tide_bias || 0;
			const foamGain = this.values.foam_gain || 1;
			const tidePhase = this.tick * this.cfg.speed * 0.08;
			const baseShore = this.h * this.cfg.shoreline + Math.sin(tidePhase) * this.cfg.tide_amp * tideLevel * 0.34 + tideBias * 1.6;

			const sunX = canvasW * (0.16 + this._hash(24000) * 0.18);
			const sunY = canvasH * (0.18 + this._hash(24001) * 0.08);
			const sunR = Math.max(22, Math.min(canvasW, canvasH) * 0.085);
			const sun = ctx.createRadialGradient(sunX, sunY, 0, sunX, sunY, sunR * 2.8);
			sun.addColorStop(0, 'rgba(255, 239, 187, 0.38)');
			sun.addColorStop(0.34, 'rgba(255, 224, 168, 0.2)');
			sun.addColorStop(1, 'rgba(255, 224, 168, 0)');
			ctx.fillStyle = sun;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const haze = ctx.createLinearGradient(0, canvasH * 0.18, 0, canvasH * 0.6);
			haze.addColorStop(0, 'rgba(255, 246, 224, 0)');
			haze.addColorStop(1, 'rgba(255, 246, 224, 0.14)');
			ctx.fillStyle = haze;
			ctx.fillRect(0, canvasH * 0.16, canvasW, canvasH * 0.44);

			const waterTop = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmax * 0.74));
			const waterMid = hslToRGB(this.cfg.hue, this.cfg.sat, clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.44));
			const waterDeep = hslToRGB((this.cfg.hue + 6) % 360, clamp01(this.cfg.sat * 1.06), clamp01(this.cfg.lmin * 0.8));
			const waterGrad = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			waterGrad.addColorStop(0, `rgb(${waterTop.r},${waterTop.g},${waterTop.b})`);
			waterGrad.addColorStop(0.46, `rgb(${waterMid.r},${waterMid.g},${waterMid.b})`);
			waterGrad.addColorStop(1, `rgb(${waterDeep.r},${waterDeep.g},${waterDeep.b})`);
			ctx.fillStyle = waterGrad;
			ctx.fillRect(0, Math.floor(horizon * sy), canvasW, canvasH - Math.floor(horizon * sy));

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 1) * sy), 0, Math.floor((horizon + 4) * sy));
			horizonGlow.addColorStop(0, 'rgba(255, 247, 221, 0.35)');
			horizonGlow.addColorStop(1, 'rgba(255, 247, 221, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 1) * sy), canvasW, Math.ceil(7 * sy));

			for (let band = 0; band < 3; band++) {
				const y = Math.floor((horizon + 2 + band * 3 + Math.sin(tidePhase * 1.8 + band) * 1.2) * sy);
				ctx.fillStyle = `rgba(${waterTop.r},${waterTop.g},${waterTop.b},${0.08 + band * 0.03})`;
				ctx.fillRect(0, y, canvasW, Math.max(1, Math.ceil(sy)));
			}

			const shoreRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const nx = x / Math.max(1, this.w - 1);
				const slopeOffset = (nx - 0.5) * this.cfg.slope * this.h * 0.34;
				const wave = Math.sin(x * this.cfg.wave_freq + tidePhase * 2.2);
				const backwash = Math.sin(x * this.cfg.wave_freq * 0.46 - tidePhase * 1.6 + 0.8);
				const chop = Math.sin(x * this.cfg.wave_freq * 1.85 + tidePhase * 3.1 + this._hash(24100 + x) * 3);
				const shore = baseShore + slopeOffset + wave * this.cfg.wave_amp * (0.14 + tideLevel * 0.1) + backwash * this.cfg.wave_amp * 0.08 + chop * this.cfg.wave_amp * 0.03;
				shoreRows[x] = Math.max(horizon + 3, Math.min(this.h - 4, shore));
			}
			for (let pass = 0; pass < 2; pass++) {
				for (let x = 1; x < this.w - 1; x++) {
					shoreRows[x] = (shoreRows[x - 1] + shoreRows[x] * 2 + shoreRows[x + 1]) / 4;
				}
			}

			const sandTop = hslToRGB(39, 0.54, 0.8);
			const sandMid = hslToRGB(36, 0.48, 0.68);
			const sandLow = hslToRGB(33, 0.42, 0.54);
			const drawSandPath = () => {
				ctx.beginPath();
				ctx.moveTo(0, canvasH);
				for (let x = 0; x < this.w; x++) {
					ctx.lineTo(Math.floor(x * sx), Math.floor(shoreRows[x] * sy));
				}
				ctx.lineTo(canvasW, canvasH);
				ctx.closePath();
			};
			drawSandPath();
			const sandGrad = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			sandGrad.addColorStop(0, `rgb(${sandTop.r},${sandTop.g},${sandTop.b})`);
			sandGrad.addColorStop(0.55, `rgb(${sandMid.r},${sandMid.g},${sandMid.b})`);
			sandGrad.addColorStop(1, `rgb(${sandLow.r},${sandLow.g},${sandLow.b})`);
			ctx.fillStyle = sandGrad;
			ctx.fill();

			const duneColor = hslToRGB(31, 0.32, 0.42);
			const dunePoints = [];
			for (let i = 0; i <= 6; i++) {
				dunePoints.push(Math.floor(this.h * 0.74) + Math.floor(this._hash(24200 + i) * 5) + Math.floor(Math.sin(i * 1.08 + this._hash(24300 + i) * 2) * 2));
			}
			ctx.fillStyle = `rgba(${duneColor.r},${duneColor.g},${duneColor.b},0.16)`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * 6;
				const idx = Math.min(5, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const y = dunePoints[idx] + (dunePoints[idx + 1] - dunePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(y * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const wetColor = hslToRGB(34, 0.34, 0.34 + clamp01(0.22 + tideLevel * 0.36) * 0.12);
			const foamColor = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 1.02));
			const shimmerColor = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmax * 0.96));
			const shimmerLevel = clamp01(this.cfg.shimmer * (0.6 + tideLevel * 0.5));

			for (let x = 0; x < this.w; x++) {
				const shore = shoreRows[x];
				const surfRow = Math.round(shore);
				const wetBand = Math.max(2, Math.round(2 + tideLevel * 3 + Math.max(0, foamGain - 1) * 1.4));
				const foamBand = Math.max(1, Math.round(1 + this.cfg.foam * 2.8 + Math.max(0, foamGain - 1) * 1.4));

				for (let row = surfRow; row < Math.min(this.h, surfRow + wetBand); row++) {
					const fade = 1 - (row - shore) / Math.max(1, wetBand);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${wetColor.r},${wetColor.g},${wetColor.b})`, clamp01(0.14 + fade * 0.32));
				}

				for (let i = 0; i < foamBand; i++) {
					const row = surfRow - i;
					const pulse = 0.55 + 0.45 * Math.sin(this.tick * 0.05 + x * 0.18 + i * 0.9);
					const alpha = clamp01((0.12 + this.cfg.foam * 0.42) * foamGain * (0.5 + 0.5 * pulse));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, alpha);
				}

				if ((x + this.tick) % 2 === 0) {
					const depth = 0.18 + this._hash(24400 + x) * 0.56;
					const row = Math.max(horizon + 1, Math.floor(horizon + (shore - horizon) * depth));
					const width = 1 + Math.floor(this._hash(24500 + x) * 3);
					const blink = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.03 + x * 0.12), 2);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, width, 1, `rgb(${shimmerColor.r},${shimmerColor.g},${shimmerColor.b})`, clamp01((0.08 + shimmerLevel * 0.34) * blink));
				}

				if ((x + Math.floor(this.tick / 3)) % 7 === 0) {
					const pebbleRow = Math.min(this.h - 2, surfRow + wetBand + 1 + Math.floor(this._hash(24600 + x) * 8));
					const pebble = hslToRGB(34 + this._hash(24700 + x) * 10, 0.2, 0.4 + this._hash(24800 + x) * 0.12);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, pebbleRow, 1, 1, `rgb(${pebble.r},${pebble.g},${pebble.b})`, 0.22);
				}
			}
		}
	}

	api.presets['beach'] = [
		{
			key: 'still-shore',
			label: 'still shore',
			config: {
				shoreline: 0.56,
				tide_amp: 3.2,
				wave_amp: 1.3,
				wave_freq: 0.14,
				speed: 0.05,
				slope: 0.08,
				foam: 0.24,
				shimmer: 0.18,
				hue: 196,
				hue_sp: 10,
				sat: 0.42,
				lmin: 0.26,
				lmax: 0.78,
			},
		},
		{
			key: 'gentle-tide',
			label: 'gentle tide',
			config: {
				shoreline: 0.58,
				tide_amp: 6,
				wave_amp: 2.4,
				wave_freq: 0.18,
				speed: 0.1,
				slope: 0.16,
				foam: 0.36,
				shimmer: 0.22,
				hue: 198,
				hue_sp: 16,
				sat: 0.5,
				lmin: 0.28,
				lmax: 0.82,
				high_tide_p: 0.0008,
				low_tide_p: 0.0006,
			},
		},
		{
			key: 'foamy-edge',
			label: 'foamy edge',
			config: {
				shoreline: 0.6,
				tide_amp: 7.4,
				wave_amp: 3.1,
				wave_freq: 0.21,
				speed: 0.12,
				slope: 0.2,
				foam: 0.5,
				shimmer: 0.18,
				hue: 194,
				hue_sp: 18,
				sat: 0.54,
				lmin: 0.3,
				lmax: 0.84,
				high_tide_p: 0.0012,
				foam_burst_p: 0.0013,
				foam_burst_mult: 2.2,
			},
		},
		{
			key: 'wide-beach',
			label: 'wide beach',
			config: {
				shoreline: 0.52,
				tide_amp: 4.8,
				wave_amp: 1.8,
				wave_freq: 0.12,
				speed: 0.08,
				slope: -0.1,
				foam: 0.3,
				shimmer: 0.28,
				hue: 202,
				hue_sp: 14,
				sat: 0.44,
				lmin: 0.24,
				lmax: 0.78,
				low_tide_p: 0.0011,
			},
		},
	];
	api.effects['beach'] = Beach;
})(window.AmbienceSim);
