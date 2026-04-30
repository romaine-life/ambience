'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 50,
		intro_drift: 0.18,
		ending_dur: 65,
		ending_linger: 18,
		ending_ripple: 0.08,
		waterline: 0.58,
		drift_speed: 0.08,
		bob_amp: 1.2,
		wave_amp: 1.6,
		wave_freq: 0.16,
		ripple: 0.24,
		reflection: 0.22,
		boat_len: 14,
		boat_height: 3.5,
		hue: 206,
		hue_sp: 16,
		sat: 0.36,
		lmin: 0.16,
		lmax: 0.82,
		wake_p: 0,
		drift_p: 0,
		calm_p: 0,
		wake_dur: 40,
		wake_mult: 1.85,
		drift_dur: 58,
		drift_push: 1.3,
		calm_dur: 72,
		calm_mult: 0.5,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_drift = clamp01(c.intro_drift);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_ripple = clamp01(c.ending_ripple);
		if (c.waterline <= 0) c.waterline = DEFAULTS.waterline;
		if (c.drift_speed <= 0) c.drift_speed = DEFAULTS.drift_speed;
		if (c.bob_amp <= 0) c.bob_amp = DEFAULTS.bob_amp;
		if (c.wave_amp <= 0) c.wave_amp = DEFAULTS.wave_amp;
		if (c.wave_freq <= 0) c.wave_freq = DEFAULTS.wave_freq;
		if (c.ripple <= 0) c.ripple = DEFAULTS.ripple;
		if (c.reflection <= 0) c.reflection = DEFAULTS.reflection;
		if (c.boat_len <= 0) c.boat_len = DEFAULTS.boat_len;
		if (c.boat_height <= 0) c.boat_height = DEFAULTS.boat_height;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.wake_dur <= 0) c.wake_dur = DEFAULTS.wake_dur;
		if (c.wake_mult <= 0) c.wake_mult = DEFAULTS.wake_mult;
		if (c.drift_dur <= 0) c.drift_dur = DEFAULTS.drift_dur;
		if (c.drift_push <= 0) c.drift_push = DEFAULTS.drift_push;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class Rowboat {
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
			const rng = this._eventRng(name.length + 83);
			switch (name) {
				case 'wake':
					this.timers.wake = jitterInt(rng, this.cfg.wake_dur, 0.3);
					this.values.wake_gain = this.cfg.wake_mult * (0.8 + rng() * 0.45);
					return true;
				case 'drift':
					this.timers.drift = jitterInt(rng, this.cfg.drift_dur, 0.3);
					this.values.drift_push = (rng() < 0.5 ? -1 : 1) * this.cfg.drift_push * (0.65 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.wake = 0;
					this.timers.drift = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.wake_gain = 1;
					this.values.drift_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.wake = 0;
					this.timers.drift = 0;
					this.timers.calm = 0;
					this.values.wake_gain = 1;
					this.values.drift_push = 0;
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
			if (!this.timers.wake || this.timers.wake <= 0) this.values.wake_gain = 1;
			if (!this.timers.drift || this.timers.drift <= 0) this.values.drift_push = 0;
		}

		_rippleLevel() {
			let level = 1;
			if (this.timers.wake > 0) level *= this.values.wake_gain || this.cfg.wake_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_drift + (1 - this.cfg.intro_drift) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_ripple) * progress;
			}
			if (this.timers.drift > 0) {
				level *= 1 + Math.abs(this.values.drift_push || this.cfg.drift_push) * 0.22;
			}
			return Math.max(0.04, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.45), clamp01(this.cfg.lmin + 0.02));
				const skyMid = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.22 + 360) % 360, clamp01(this.cfg.sat * 0.58), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.34));
				const skyLow = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.18) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.62));
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
			const waterline = Math.max(12, Math.min(this.h - 10, Math.floor(this.h * this.cfg.waterline)));
			const motion = this._rippleLevel();
			const driftPush = this.values.drift_push || 0;
			const phase = this.tick * this.cfg.drift_speed * 0.08;

			const glowX = canvasW * 0.74;
			const glowY = canvasH * 0.24;
			const glowR = Math.max(20, Math.min(canvasW, canvasH) * 0.09);
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR * 2.8);
			glow.addColorStop(0, 'rgba(255, 223, 174, 0.22)');
			glow.addColorStop(1, 'rgba(255, 223, 174, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const ridgeBase = waterline - Math.max(5, Math.round(this.h * 0.12));
			const shorelineY = Math.max(ridgeBase + 2, waterline - Math.max(2, Math.round(this.h * 0.04)));
			const ridgePoints = [];
			const ridgeSegments = 7;
			for (let i = 0; i <= ridgeSegments; i++) {
				const ridgeWave = Math.sin(i * 0.9 + this._hash(25100 + i) * 2.4) * 2.8;
				ridgePoints.push(Math.round(ridgeBase - Math.abs(ridgeWave) - this._hash(25200 + i) * 3));
			}
			const ridgeColor = hslToRGB((this.cfg.hue + 54) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin * 0.7));
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, Math.floor(shorelineY * sy));
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const ridgeY = ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(ridgeY * sy));
			}
			ctx.lineTo(canvasW, Math.floor(shorelineY * sy));
			ctx.closePath();
			ctx.fill();

			const treelineColor = hslToRGB((this.cfg.hue + 72) % 360, clamp01(this.cfg.sat * 0.2), clamp01(this.cfg.lmin * 0.52));
			for (let i = 0; i < 11; i++) {
				const col = Math.floor((i + 0.4) * this.w / 11 + (this._hash(25300 + i) - 0.5) * 6);
				const top = Math.round(ridgeBase - 1 - this._hash(25400 + i) * 4);
				const height = 2 + Math.floor(this._hash(25500 + i) * 3);
				for (let row = 0; row < height; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, top + row, 1, 1, `rgb(${treelineColor.r},${treelineColor.g},${treelineColor.b})`, 0.92);
				}
			}

			for (let y = waterline; y < this.h; y++) {
				const depth = (y - waterline) / Math.max(1, this.h - waterline);
				const hue = ((this.cfg.hue + depth * this.cfg.hue_sp * 0.22) % 360 + 360) % 360;
				const sat = clamp01(this.cfg.sat * (0.8 - depth * 0.22));
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.36 - depth * 0.18));
				const color = hslToRGB(hue, sat, light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
			}

			const mist = ctx.createLinearGradient(0, Math.floor((shorelineY - 3) * sy), 0, Math.floor((waterline + 8) * sy));
			mist.addColorStop(0, 'rgba(255, 240, 220, 0.14)');
			mist.addColorStop(1, 'rgba(255, 240, 220, 0)');
			ctx.fillStyle = mist;
			ctx.fillRect(0, Math.floor((shorelineY - 3) * sy), canvasW, Math.ceil((waterline - shorelineY + 11) * sy));

			const surfaceColor = hslToRGB((this.cfg.hue - 6 + 360) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmax * 0.92));
			for (let x = 0; x < this.w; x++) {
				const wave = Math.sin(x * this.cfg.wave_freq + phase) * this.cfg.wave_amp;
				const subWave = Math.sin(x * this.cfg.wave_freq * 0.42 - phase * 1.7) * this.cfg.wave_amp * 0.36;
				const row = waterline + Math.round((wave + subWave) * motion * 0.22);
				const twinkle = 0.45 + 0.55 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.03 + x * 0.11), 2);
				const alpha = clamp01((0.05 + this.cfg.reflection * 0.16) * twinkle);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, alpha);
				if ((x + this.tick) % 6 === 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row + 2, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, alpha * 0.45);
				}
			}

			const boatLen = Math.max(7, Math.round(this.cfg.boat_len));
			const boatHeight = Math.max(2, Math.round(this.cfg.boat_height));
			const boatX = Math.max(Math.floor(boatLen * 0.6), Math.min(this.w - Math.ceil(boatLen * 0.6), Math.round(this.w * 0.34 + Math.sin(phase * 1.6 + 0.8) * (2.4 + motion * 1.6) + driftPush * 1.8)));
			const bob = Math.sin(phase * 2.4 + 0.6) * this.cfg.bob_amp * motion * 0.52 + Math.sin(phase * 0.95 + 1.3) * 0.35;
			const hullBaseY = waterline - Math.round(bob * 0.55);
			const tilt = Math.sin(phase * 1.9 + 0.4) * motion * 0.9 + driftPush * 0.2;
			const hullColor = hslToRGB(24, 0.34, 0.22);
			const railColor = hslToRGB(31, 0.26, 0.36);
			const seatColor = hslToRGB(28, 0.18, 0.16);
			const hullRows = [];

			for (let row = 0; row < boatHeight; row++) {
				const t = boatHeight === 1 ? 0.5 : row / (boatHeight - 1);
				const arch = 1 - Math.abs(t * 2 - 1);
				const width = Math.max(4, Math.round(boatLen * (0.54 + arch * 0.42)));
				const y = hullBaseY - (boatHeight - 1 - row);
				const offset = Math.round((t - 0.5) * tilt * 1.8);
				const startX = Math.round(boatX - width / 2 + offset);
				hullRows.push({ startX, width, y });
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, startX, y, width, 1, `rgb(${hullColor.r},${hullColor.g},${hullColor.b})`, clamp01(0.78 + t * 0.2));
			}

			const topHull = hullRows[0];
			if (topHull && topHull.width > 3) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, topHull.startX + 1, topHull.y, topHull.width - 2, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.72);
			}
			const seatWidth = Math.max(2, Math.round(boatLen * 0.22));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - seatWidth / 2), hullBaseY - Math.max(1, Math.floor(boatHeight / 2)), seatWidth, 1, `rgb(${seatColor.r},${seatColor.g},${seatColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - boatLen * 0.46), hullBaseY - 1, 1, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX + boatLen * 0.44), hullBaseY - 1, 1, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.82);

			const shadowColor = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.9));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - boatLen * 0.5), waterline, boatLen, 1, `rgb(${shadowColor.r},${shadowColor.g},${shadowColor.b})`, 0.26);

			const reflectionColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmax * 0.58));
			const reflectionLevel = clamp01(this.cfg.reflection * (0.3 + motion * 0.22));
			for (let i = 0; i < hullRows.length; i++) {
				const row = hullRows[i];
				const distance = hullBaseY - row.y + 1;
				const wobble = Math.round(Math.sin(this.tick * 0.08 + i * 0.8 + row.startX * 0.03) * (0.5 + motion * 0.45));
				const reflY = hullBaseY + distance + Math.round(Math.sin(row.startX * this.cfg.wave_freq + phase) * motion * 0.35);
				const reflWidth = Math.max(2, row.width - 1 - Math.floor(distance / 2));
				const alpha = clamp01(reflectionLevel * (0.4 - distance * 0.045));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, row.startX + wobble, reflY, reflWidth, 1, `rgb(${reflectionColor.r},${reflectionColor.g},${reflectionColor.b})`, alpha);
			}

			const rippleColor = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmax * 0.98));
			const wakeGain = this.timers.wake > 0 ? (this.values.wake_gain || this.cfg.wake_mult) : 1;
			const rippleBands = 4 + Math.round(this.cfg.ripple * 7);
			for (let band = 0; band < rippleBands; band++) {
				const centerX = Math.round(boatX + boatLen * 0.18 + band * (1.2 + wakeGain * 0.28));
				const half = Math.max(3, Math.round(boatLen * (0.24 + band * 0.14 + wakeGain * 0.03)));
				const centerY = waterline + Math.round(0.8 + band * 0.75 + Math.abs(Math.sin(phase * 4.2 + band * 0.9)) * 1.1);
				for (let dx = -half; dx <= half; dx++) {
					if ((dx + band + this.tick) % 2 !== 0) continue;
					const edge = 1 - Math.abs(dx) / Math.max(1, half);
					const waveY = centerY + Math.round(Math.sin(dx * 0.22 + this.tick * 0.08 + band * 0.7) * 0.7);
					const alpha = clamp01((0.08 + this.cfg.ripple * 0.24 * motion) * Math.pow(edge, 0.45));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, waveY, 1, 1, `rgb(${rippleColor.r},${rippleColor.g},${rippleColor.b})`, alpha);
				}
			}

			for (let band = 0; band < 3; band++) {
				const half = Math.max(2, Math.round(boatLen * (0.16 + band * 0.08)));
				const centerX = Math.round(boatX - boatLen * 0.2 - band * 1.2);
				const centerY = waterline + Math.round(band * 0.85 + Math.abs(Math.sin(phase * 3.6 + band)) * 0.9);
				for (let dx = -half; dx <= half; dx++) {
					if ((dx + band) % 2 !== 0) continue;
					const edge = 1 - Math.abs(dx) / Math.max(1, half);
					const alpha = clamp01((0.03 + this.cfg.ripple * 0.12 * motion) * Math.pow(edge, 0.65));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, centerY + Math.round(Math.sin(dx * 0.3 + this.tick * 0.06) * 0.5), 1, 1, `rgb(${rippleColor.r},${rippleColor.g},${rippleColor.b})`, alpha);
				}
			}
		}
	}

	api.presets['rowboat'] = [
		{
			key: 'still-lake',
			label: 'still lake',
			config: {
				intro_drift: 0.12,
				ending_ripple: 0.12,
				waterline: 0.57,
				drift_speed: 0.05,
				bob_amp: 0.7,
				wave_amp: 0.9,
				wave_freq: 0.12,
				ripple: 0.12,
				reflection: 0.28,
				boat_len: 13,
				boat_height: 3.5,
				hue: 202,
				hue_sp: 10,
				sat: 0.26,
				lmin: 0.16,
				lmax: 0.74,
				calm_p: 0.0011,
			},
		},
		{
			key: 'gentle-drift',
			label: 'gentle drift',
			config: {
				waterline: 0.58,
				drift_speed: 0.08,
				bob_amp: 1.2,
				wave_amp: 1.6,
				wave_freq: 0.16,
				ripple: 0.24,
				reflection: 0.22,
				boat_len: 14,
				boat_height: 3.5,
				hue: 206,
				hue_sp: 16,
				sat: 0.36,
				lmin: 0.16,
				lmax: 0.82,
				drift_p: 0.0009,
			},
		},
		{
			key: 'evening-ripples',
			label: 'evening ripples',
			config: {
				waterline: 0.6,
				drift_speed: 0.1,
				bob_amp: 1.4,
				wave_amp: 1.9,
				wave_freq: 0.18,
				ripple: 0.34,
				reflection: 0.24,
				boat_len: 14.5,
				boat_height: 4,
				hue: 212,
				hue_sp: 18,
				sat: 0.4,
				lmin: 0.18,
				lmax: 0.86,
				wake_p: 0.001,
			},
		},
		{
			key: 'wind-touched-water',
			label: 'wind-touched water',
			config: {
				waterline: 0.56,
				drift_speed: 0.12,
				bob_amp: 1.8,
				wave_amp: 2.5,
				wave_freq: 0.2,
				ripple: 0.42,
				reflection: 0.18,
				boat_len: 15,
				boat_height: 4,
				hue: 198,
				hue_sp: 20,
				sat: 0.46,
				lmin: 0.18,
				lmax: 0.8,
				wake_p: 0.0012,
				wake_mult: 2.1,
				drift_p: 0.0014,
				drift_push: 1.55,
			},
		},
	];
	api.effects['rowboat'] = Rowboat;
})(window.AmbienceSim);
