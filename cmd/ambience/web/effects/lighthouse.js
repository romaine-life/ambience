'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 50,
		intro_beam: 0.16,
		ending_dur: 65,
		ending_linger: 18,
		ending_beam: 0.08,
		sweep_speed: 0.08,
		beam_width: 0.22,
		beam_softness: 0.42,
		tower_height: 22,
		tower_width: 6.5,
		horizon: 0.74,
		haze: 0.14,
		glow: 0.22,
		hue: 214,
		hue_sp: 18,
		sat: 0.34,
		lmin: 0.12,
		lmax: 0.84,
		bright_pass_p: 0,
		fog_thicken_p: 0,
		calm_p: 0,
		bright_pass_dur: 42,
		bright_pass_mult: 1.75,
		fog_thicken_dur: 72,
		fog_thicken_mult: 1.85,
		calm_dur: 64,
		calm_mult: 0.55,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_beam = clamp01(c.intro_beam);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_beam = clamp01(c.ending_beam);
		if (c.sweep_speed <= 0) c.sweep_speed = DEFAULTS.sweep_speed;
		if (c.beam_width <= 0) c.beam_width = DEFAULTS.beam_width;
		if (c.beam_softness <= 0) c.beam_softness = DEFAULTS.beam_softness;
		if (c.tower_height <= 0) c.tower_height = DEFAULTS.tower_height;
		if (c.tower_width <= 0) c.tower_width = DEFAULTS.tower_width;
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.haze <= 0) c.haze = DEFAULTS.haze;
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.bright_pass_dur <= 0) c.bright_pass_dur = DEFAULTS.bright_pass_dur;
		if (c.bright_pass_mult <= 0) c.bright_pass_mult = DEFAULTS.bright_pass_mult;
		if (c.fog_thicken_dur <= 0) c.fog_thicken_dur = DEFAULTS.fog_thicken_dur;
		if (c.fog_thicken_mult <= 0) c.fog_thicken_mult = DEFAULTS.fog_thicken_mult;
		if (c.calm_dur <= 0) c.calm_dur = DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DEFAULTS.calm_mult;
		return c;
	}

	class Lighthouse {
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
			const rng = this._eventRng(name.length + 79);
			switch (name) {
				case 'bright-pass':
					this.timers['bright-pass'] = jitterInt(rng, this.cfg.bright_pass_dur, 0.3);
					this.values.bright_gain = this.cfg.bright_pass_mult * (0.8 + rng() * 0.4);
					return true;
				case 'fog-thicken':
					this.timers['fog-thicken'] = jitterInt(rng, this.cfg.fog_thicken_dur, 0.3);
					this.timers.calm = 0;
					this.values.fog_gain = this.cfg.fog_thicken_mult * (0.8 + rng() * 0.45);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.timers['fog-thicken'] = 0;
					this.values.fog_gain = 1;
					return true;
				case 'intro':
					this.timers['bright-pass'] = 0;
					this.timers['fog-thicken'] = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.bright_gain = 1;
					this.values.fog_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['bright-pass'] = 0;
					this.timers['fog-thicken'] = 0;
					this.timers.calm = 0;
					this.values.bright_gain = 1;
					this.values.fog_gain = 1;
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
			if (!this.timers['bright-pass'] || this.timers['bright-pass'] <= 0) this.values.bright_gain = 1;
			if (!this.timers['fog-thicken'] || this.timers['fog-thicken'] <= 0) this.values.fog_gain = 1;
		}

		_beamLevel() {
			let level = 1;
			if (this.timers['bright-pass'] > 0) level *= this.values.bright_gain || this.cfg.bright_pass_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_beam + (1 - this.cfg.intro_beam) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_beam) * progress;
			}
			return Math.max(0.05, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue + 358) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.92));
				const skyMid = hslToRGB(this.cfg.hue, this.cfg.sat, clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const skyLow = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.6 + 360) % 360, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.46));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.min(this.h - 10, Math.floor(this.h * this.cfg.horizon)));
			const towerX = Math.floor(this.w * 0.18);
			const towerH = Math.max(10, Math.round(this.cfg.tower_height));
			const towerW = Math.max(3, Math.round(this.cfg.tower_width));
			const beamLevel = this._beamLevel();
			const fogLevel = clamp01(this.cfg.haze * (this.timers['fog-thicken'] > 0 ? this.values.fog_gain || this.cfg.fog_thicken_mult : 1) * (this.timers.calm > 0 ? 0.82 : 1));
			const beamAngle = -0.26 + Math.sin(this.tick * this.cfg.sweep_speed * 0.06) * 0.78;
			const beamWidth = this.cfg.beam_width * (1 + fogLevel * 0.9);
			const beamSoftness = clamp01(this.cfg.beam_softness * (1 + fogLevel * 0.35));

			const seaTop = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.12));
			const seaLow = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin * 0.8));
			const sea = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			sea.addColorStop(0, `rgb(${seaTop.r},${seaTop.g},${seaTop.b})`);
			sea.addColorStop(1, `rgb(${seaLow.r},${seaLow.g},${seaLow.b})`);
			ctx.fillStyle = sea;
			ctx.fillRect(0, Math.floor(horizon * sy), canvasW, canvasH - Math.floor(horizon * sy));

			const coastRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const bluff = Math.exp(-Math.pow((x - towerX) / 18, 2)) * 7.5;
				const swell = Math.sin(x * 0.032 + 0.5) * 1.3 + Math.sin(x * 0.013 + 2.1) * 1.1;
				coastRows[x] = Math.round(horizon + swell - bluff);
			}
			const coastColor = hslToRGB((this.cfg.hue + 214) % 360, clamp01(this.cfg.sat * 0.12), 0.08);
			ctx.fillStyle = `rgb(${coastColor.r},${coastColor.g},${coastColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				ctx.lineTo(Math.floor(x * sx), Math.floor(coastRows[x] * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const oceanLine = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.35), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.3));
			for (let x = 0; x < this.w; x += 2) {
				if ((x + this.tick) % 5 !== 0) continue;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, horizon + 1 + Math.round(Math.sin(x * 0.07 + this.tick * 0.02)), 2, 1, `rgb(${oceanLine.r},${oceanLine.g},${oceanLine.b})`, 0.18);
			}

			const towerBase = coastRows[Math.max(0, Math.min(this.w - 1, towerX))];
			const lampY = towerBase - towerH + 2;
			const towerColor = hslToRGB((this.cfg.hue + 212) % 360, clamp01(this.cfg.sat * 0.08), 0.1);
			for (let y = lampY; y <= towerBase; y++) {
				const ratio = (y - lampY) / Math.max(1, towerBase - lampY);
				const half = Math.max(1, Math.round((towerW * (0.32 + ratio * 0.68)) * 0.5));
				for (let dx = -half; dx <= half; dx++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX + dx, y, 1, 1, `rgb(${towerColor.r},${towerColor.g},${towerColor.b})`, 1);
				}
			}
			for (let dx = -Math.max(2, Math.round(towerW * 0.6)); dx <= Math.max(2, Math.round(towerW * 0.6)); dx++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX + dx, lampY - 1 + Math.round(Math.abs(dx) * 0.15), 1, 1, `rgb(${towerColor.r},${towerColor.g},${towerColor.b})`, 1);
			}

			const lampGlow = hslToRGB(48, 0.68, clamp01(0.42 + this.cfg.glow * 0.34));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX, lampY, 1, 1, `rgb(${lampGlow.r},${lampGlow.g},${lampGlow.b})`, clamp01(0.3 + this.cfg.glow * 0.7));

			const glowX = towerX * sx;
			const glowY = lampY * sy;
			const glowR = Math.max(18, Math.min(canvasW, canvasH) * (0.04 + this.cfg.glow * 0.05));
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			glow.addColorStop(0, `rgba(255, 236, 184, ${0.16 + this.cfg.glow * 0.22})`);
			glow.addColorStop(1, 'rgba(255, 236, 184, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			const beamHue = (this.cfg.hue - 18 + 360) % 360;
			const beamBase = hslToRGB(beamHue, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 0.98));
			const beamCore = hslToRGB(44, 0.42, clamp01(this.cfg.lmax * 1.02));
			const angleDiff = (a, b) => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b)));
			for (let y = 0; y <= horizon + 5; y++) {
				for (let x = towerX + 1; x < this.w; x++) {
					const dx = x - towerX;
					const dy = y - lampY;
					if (dx <= 0) continue;
					const dist = Math.hypot(dx, dy);
					if (dist < 2) continue;
					const ang = Math.atan2(dy, dx);
					const diff = angleDiff(ang, beamAngle);
					if (diff > beamWidth) continue;
					const cone = 1 - diff / beamWidth;
					const edge = Math.pow(cone, Math.max(0.6, 1.8 - beamSoftness));
					const falloff = Math.pow(clamp01(1 - dist / (this.w * 0.92)), 0.72);
					const strength = edge * falloff * beamLevel;
					if (strength < 0.02) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${beamBase.r},${beamBase.g},${beamBase.b})`, clamp01(strength * (0.12 + fogLevel * 0.08)));
					if (diff < beamWidth * 0.34 && strength > 0.08) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${beamCore.r},${beamCore.g},${beamCore.b})`, clamp01(strength * 0.16));
					}
				}
			}

			const fog = ctx.createLinearGradient(0, Math.floor((horizon - 7) * sy), 0, Math.floor((horizon + 9) * sy));
			fog.addColorStop(0, `rgba(201, 214, 226, ${0.02 + fogLevel * 0.08})`);
			fog.addColorStop(1, 'rgba(201, 214, 226, 0)');
			ctx.fillStyle = fog;
			ctx.fillRect(0, Math.floor((horizon - 8) * sy), canvasW, Math.ceil(18 * sy));
		}
	}

	api.presets['lighthouse'] = [
		{
			key: 'clear-night',
			label: 'clear night',
			config: {
				sweep_speed: 0.07,
				beam_width: 0.18,
				beam_softness: 0.32,
				tower_height: 22,
				tower_width: 6,
				horizon: 0.74,
				haze: 0.08,
				glow: 0.2,
				hue: 216,
				hue_sp: 14,
				sat: 0.3,
				lmin: 0.1,
				lmax: 0.8,
			},
		},
		{
			key: 'steady-sweep',
			label: 'steady sweep',
			config: {
				sweep_speed: 0.08,
				beam_width: 0.22,
				beam_softness: 0.42,
				tower_height: 22,
				tower_width: 6.5,
				horizon: 0.74,
				haze: 0.14,
				glow: 0.22,
				hue: 214,
				hue_sp: 18,
				sat: 0.34,
				lmin: 0.12,
				lmax: 0.84,
				bright_pass_p: 0.0007,
			},
		},
		{
			key: 'foggy-coast',
			label: 'foggy coast',
			config: {
				sweep_speed: 0.06,
				beam_width: 0.28,
				beam_softness: 0.62,
				tower_height: 23,
				tower_width: 7,
				horizon: 0.76,
				haze: 0.24,
				glow: 0.18,
				hue: 210,
				hue_sp: 12,
				sat: 0.24,
				lmin: 0.1,
				lmax: 0.72,
				fog_thicken_p: 0.0012,
				fog_thicken_mult: 2.2,
			},
		},
		{
			key: 'bright-beacon',
			label: 'bright beacon',
			config: {
				sweep_speed: 0.1,
				beam_width: 0.24,
				beam_softness: 0.36,
				tower_height: 21,
				tower_width: 6,
				horizon: 0.72,
				haze: 0.12,
				glow: 0.3,
				hue: 218,
				hue_sp: 20,
				sat: 0.36,
				lmin: 0.12,
				lmax: 0.9,
				bright_pass_p: 0.0014,
				bright_pass_mult: 2.1,
				calm_p: 0.0009,
			},
		},
	];
	api.effects['lighthouse'] = Lighthouse;
})(window.AmbienceSim);
