'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 60,
		intro_glow: 0.05,
		intro_first_puff: 30,
		ending_dur: 80,
		ending_linger: 30,
		ending_glow: 0.05,
		ending_puffs: 3,
		puff_rate: 0.18,
		plume_rise: 0.55,
		plume_drift: 0.12,
		plume_width: 3.6,
		plume_softness: 0.45,
		plume_top: 0.06,
		cottage_width: 34,
		cottage_height: 18,
		roof_pitch: 0.55,
		chimney_height: 7,
		horizon: 0.78,
		window_glow: 0.78,
		window_hue: 46,
		hue: 222,
		hue_sp: 18,
		sat: 0.36,
		lmin: 0.06,
		lmax: 0.32,
		gust_p: 0,
		flicker_p: 0,
		ember_p: 0,
		quiet_p: 0,
		gust_dur: 54,
		gust_drift_mult: 2.4,
		flicker_dur: 18,
		flicker_mult: 0.45,
		ember_dur: 42,
		quiet_dur: 220,
		quiet_rate_mult: 0.35,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.intro_first_puff < 0) c.intro_first_puff = 0;
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.ending_puffs < 0) c.ending_puffs = 0;
		if (c.puff_rate <= 0) c.puff_rate = DEFAULTS.puff_rate;
		if (c.plume_rise <= 0) c.plume_rise = DEFAULTS.plume_rise;
		if (c.plume_width <= 0) c.plume_width = DEFAULTS.plume_width;
		if (c.plume_softness <= 0) c.plume_softness = DEFAULTS.plume_softness;
		if (c.plume_top < 0) c.plume_top = 0;
		if (c.cottage_width <= 0) c.cottage_width = DEFAULTS.cottage_width;
		if (c.cottage_height <= 0) c.cottage_height = DEFAULTS.cottage_height;
		if (c.roof_pitch <= 0) c.roof_pitch = DEFAULTS.roof_pitch;
		if (c.chimney_height <= 0) c.chimney_height = DEFAULTS.chimney_height;
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.window_glow <= 0) c.window_glow = DEFAULTS.window_glow;
		if (c.window_hue < 0) c.window_hue = DEFAULTS.window_hue;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.gust_dur <= 0) c.gust_dur = DEFAULTS.gust_dur;
		if (c.gust_drift_mult <= 0) c.gust_drift_mult = DEFAULTS.gust_drift_mult;
		if (c.flicker_dur <= 0) c.flicker_dur = DEFAULTS.flicker_dur;
		if (c.flicker_mult <= 0) c.flicker_mult = DEFAULTS.flicker_mult;
		if (c.ember_dur <= 0) c.ember_dur = DEFAULTS.ember_dur;
		if (c.quiet_dur <= 0) c.quiet_dur = DEFAULTS.quiet_dur;
		if (c.quiet_rate_mult <= 0) c.quiet_rate_mult = DEFAULTS.quiet_rate_mult;
		return c;
	}

	class CottageChimney {
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
				case 'puff':
				case 'puff-emit':
					this.values.last_puff_tick = this.tick;
					this.values.puff_count = (this.values.puff_count || 0) + 1;
					return true;
				case 'gust':
				case 'wind-gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_drift = this.cfg.gust_drift_mult * (0.85 + rng() * 0.3);
					return true;
				case 'flicker':
				case 'lamp-flicker':
					this.timers.flicker = jitterInt(rng, this.cfg.flicker_dur, 0.4);
					this.values.flicker_gain = this.cfg.flicker_mult * (0.7 + rng() * 0.4);
					return true;
				case 'ember':
				case 'embers':
					this.timers.ember = jitterInt(rng, this.cfg.ember_dur, 0.3);
					this.values.ember_seed = Math.floor(rng() * (1 << 30));
					return true;
				case 'quiet':
				case 'quiet-night':
					this.timers.quiet = jitterInt(rng, this.cfg.quiet_dur, 0.25);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.flicker = 0;
					this.timers.ember = 0;
					this.timers.quiet = 0;
					this.timers.ending = 0;
					this.values.gust_drift = 0;
					this.values.flicker_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.flicker = 0;
					this.timers.ember = 0;
					this.timers.quiet = 0;
					this.values.gust_drift = 0;
					this.values.flicker_gain = 1;
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
			if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_drift = 0;
			if (!this.timers.flicker || this.timers.flicker <= 0) this.values.flicker_gain = 1;
		}

		_windowLevel() {
			let level = this.cfg.window_glow;
			if (this.timers.flicker > 0) level *= this.values.flicker_gain || this.cfg.flicker_mult;
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
			return clamp01(level);
		}

		_plumeStrength() {
			let level = 1;
			if (this.timers.quiet > 0) level *= this.cfg.quiet_rate_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				// First puff doesn't leave until intro_first_puff has elapsed.
				const elapsed = total - this.timers.intro;
				if (elapsed < this.cfg.intro_first_puff) return 0;
				level *= 0.2 + 0.8 * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= Math.max(0, 1 - progress * 1.1);
			}
			return Math.max(0, level);
		}

		_currentDrift() {
			let drift = this.cfg.plume_drift;
			if (this.timers.gust > 0) drift *= this.values.gust_drift || this.cfg.gust_drift_mult;
			return drift;
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			const cfg = this.cfg;
			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);

			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTopHSL = { h: cfg.hue, s: clamp01(cfg.sat * 0.55), l: clamp01(cfg.lmin) };
				const skyMidHSL = { h: cfg.hue, s: cfg.sat, l: clamp01(cfg.lmin + (cfg.lmax - cfg.lmin) * 0.35) };
				const skyLowHSL = { h: (cfg.hue - cfg.hue_sp * 0.6 + 360) % 360, s: clamp01(cfg.sat * 0.78), l: clamp01(cfg.lmin + (cfg.lmax - cfg.lmin) * 0.7) };
				const skyTop = hslToRGB(skyTopHSL.h, skyTopHSL.s, skyTopHSL.l);
				const skyMid = hslToRGB(skyMidHSL.h, skyMidHSL.s, skyMidHSL.l);
				const skyLow = hslToRGB(skyLowHSL.h, skyLowHSL.s, skyLowHSL.l);
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.6, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			// Sparse stars in the upper sky.
			const starColor = hslToRGB(cfg.hue, 0.18, clamp01(cfg.lmax + 0.2));
			const starTwinkle = (this.tick * 0.04) % (Math.PI * 2);
			for (let i = 0; i < 28; i++) {
				const sxn = this._hash(31100 + i);
				const syn = this._hash(31200 + i);
				const sxCell = Math.floor(sxn * this.w);
				const syCell = Math.floor(syn * this.h * cfg.horizon * 0.85);
				const tw = 0.28 + 0.42 * (0.5 + 0.5 * Math.sin(starTwinkle + i * 1.7));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, sxCell, syCell, 1, 1, `rgb(${starColor.r},${starColor.g},${starColor.b})`, tw);
			}

			// Ground / foreground.
			const horizonRow = Math.max(8, Math.min(this.h - 6, Math.floor(this.h * cfg.horizon)));
			const groundColor = hslToRGB((cfg.hue + 200) % 360, clamp01(cfg.sat * 0.18), clamp01(cfg.lmin * 0.7));
			ctx.fillStyle = `rgb(${groundColor.r},${groundColor.g},${groundColor.b})`;
			ctx.fillRect(0, Math.floor(horizonRow * sy), canvasW, canvasH - Math.floor(horizonRow * sy));

			// Cottage geometry, anchored slightly off-center.
			const cottageW = Math.max(8, Math.round(cfg.cottage_width));
			const cottageH = Math.max(6, Math.round(cfg.cottage_height));
			const cottageCenter = Math.floor(this.w * 0.46);
			const cottageLeft = cottageCenter - Math.floor(cottageW * 0.5);
			const cottageRight = cottageLeft + cottageW - 1;
			const cottageBase = horizonRow;
			const wallTop = Math.max(2, cottageBase - Math.round(cottageH * 0.55));
			const roofPeak = Math.max(1, wallTop - Math.max(2, Math.round(cottageW * cfg.roof_pitch * 0.5)));

			// Walls.
			const wallColor = hslToRGB((cfg.hue + 195) % 360, clamp01(cfg.sat * 0.22), clamp01(cfg.lmin * 0.55));
			ctx.fillStyle = `rgb(${wallColor.r},${wallColor.g},${wallColor.b})`;
			ctx.fillRect(Math.floor(cottageLeft * sx), Math.floor(wallTop * sy),
				Math.ceil(cottageW * sx), Math.ceil((cottageBase - wallTop + 1) * sy));

			// Gable roof — a triangle from peak down to each wall corner.
			const roofColor = hslToRGB((cfg.hue + 200) % 360, clamp01(cfg.sat * 0.16), clamp01(cfg.lmin * 0.4));
			ctx.fillStyle = `rgb(${roofColor.r},${roofColor.g},${roofColor.b})`;
			ctx.beginPath();
			ctx.moveTo(cottageLeft * sx, wallTop * sy);
			ctx.lineTo(cottageCenter * sx, roofPeak * sy);
			ctx.lineTo((cottageRight + 1) * sx, wallTop * sy);
			ctx.closePath();
			ctx.fill();

			// Door — a tall narrow rectangle near the right side.
			const doorW = Math.max(2, Math.round(cottageW * 0.13));
			const doorH = Math.max(3, Math.round((cottageBase - wallTop) * 0.66));
			const doorLeft = cottageRight - Math.max(3, Math.round(cottageW * 0.28));
			const doorTop = cottageBase - doorH;
			const doorColor = hslToRGB((cfg.hue + 180) % 360, clamp01(cfg.sat * 0.24), clamp01(cfg.lmin * 0.34));
			ctx.fillStyle = `rgb(${doorColor.r},${doorColor.g},${doorColor.b})`;
			ctx.fillRect(Math.floor(doorLeft * sx), Math.floor(doorTop * sy),
				Math.ceil(doorW * sx), Math.ceil(doorH * sy));

			// Window — square, slightly left of center, glowing warm.
			const windowSize = Math.max(2, Math.round(cottageW * 0.16));
			const windowLeft = cottageLeft + Math.max(2, Math.round(cottageW * 0.22));
			const windowTop = wallTop + Math.max(1, Math.round((cottageBase - wallTop) * 0.22));
			const windowLevel = this._windowLevel();
			const windowFrame = hslToRGB((cfg.hue + 200) % 360, 0.1, 0.06);
			ctx.fillStyle = `rgb(${windowFrame.r},${windowFrame.g},${windowFrame.b})`;
			ctx.fillRect(Math.floor((windowLeft - 1) * sx), Math.floor((windowTop - 1) * sy),
				Math.ceil((windowSize + 2) * sx), Math.ceil((windowSize + 2) * sy));
			const windowColor = hslToRGB(cfg.window_hue, clamp01(0.62 + windowLevel * 0.3),
				clamp01(0.42 + windowLevel * 0.5));
			ctx.fillStyle = `rgb(${windowColor.r},${windowColor.g},${windowColor.b})`;
			ctx.globalAlpha = clamp01(0.7 + windowLevel * 0.3);
			ctx.fillRect(Math.floor(windowLeft * sx), Math.floor(windowTop * sy),
				Math.ceil(windowSize * sx), Math.ceil(windowSize * sy));
			ctx.globalAlpha = 1;
			// Window cross bars.
			const barColor = hslToRGB((cfg.window_hue + 12) % 360, 0.22, 0.18);
			ctx.fillStyle = `rgb(${barColor.r},${barColor.g},${barColor.b})`;
			ctx.fillRect(Math.floor((windowLeft + Math.floor(windowSize * 0.5)) * sx), Math.floor(windowTop * sy),
				Math.max(1, Math.floor(sx * 0.6)), Math.ceil(windowSize * sy));
			ctx.fillRect(Math.floor(windowLeft * sx), Math.floor((windowTop + Math.floor(windowSize * 0.5)) * sy),
				Math.ceil(windowSize * sx), Math.max(1, Math.floor(sy * 0.6)));

			// Window glow halo.
			const glowR = Math.max(14, Math.min(canvasW, canvasH) * (0.06 + windowLevel * 0.05));
			const glowCx = (windowLeft + windowSize * 0.5) * sx;
			const glowCy = (windowTop + windowSize * 0.5) * sy;
			const glow = ctx.createRadialGradient(glowCx, glowCy, 0, glowCx, glowCy, glowR);
			const glowAlpha = 0.18 + windowLevel * 0.32;
			glow.addColorStop(0, `rgba(255, 220, 150, ${glowAlpha})`);
			glow.addColorStop(1, 'rgba(255, 220, 150, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowCx - glowR, glowCy - glowR, glowR * 2, glowR * 2);

			// Chimney — short rectangle on the left side of the roof.
			const chimneyH = Math.max(2, Math.round(cfg.chimney_height));
			const chimneyW = Math.max(2, Math.round(cottageW * 0.09));
			const chimneyX = cottageLeft + Math.max(2, Math.round(cottageW * 0.18));
			// Roof line at chimneyX (linear interp between left edge wallTop and peak)
			const roofRatio = (chimneyX - cottageLeft) / Math.max(1, cottageCenter - cottageLeft);
			const roofRow = Math.round(wallTop + (roofPeak - wallTop) * Math.min(1, Math.max(0, roofRatio)));
			const chimneyTop = roofRow - chimneyH;
			const chimneyColor = hslToRGB((cfg.hue + 195) % 360, clamp01(cfg.sat * 0.2), clamp01(cfg.lmin * 0.45));
			ctx.fillStyle = `rgb(${chimneyColor.r},${chimneyColor.g},${chimneyColor.b})`;
			ctx.fillRect(Math.floor(chimneyX * sx), Math.floor(chimneyTop * sy),
				Math.ceil(chimneyW * sx), Math.ceil(chimneyH * sy));
			// Chimney cap.
			const capColor = hslToRGB((cfg.hue + 195) % 360, clamp01(cfg.sat * 0.22), clamp01(cfg.lmin * 0.6));
			ctx.fillStyle = `rgb(${capColor.r},${capColor.g},${capColor.b})`;
			ctx.fillRect(Math.floor((chimneyX - 0.5) * sx), Math.floor(chimneyTop * sy),
				Math.ceil((chimneyW + 1) * sx), Math.max(1, Math.floor(sy)));

			// Smoke plume — a stream of puffs continuously cycling above the chimney.
			// Rather than maintain a particle list, derive each puff's position from
			// its phase in [0, lifeTicks). This keeps things deterministic for any
			// tick value and survives snapshot/restore without extra state.
			const plumeStrength = this._plumeStrength();
			if (plumeStrength > 0) {
				const drift = this._currentDrift();
				const rate = Math.max(0.02, cfg.puff_rate);
				// puff_rate = puffs per second; sim runs at 10 ticks/s.
				const ticksPerPuff = Math.max(2, Math.round(10 / rate));
				const lifeTicks = Math.max(20, Math.round(((1 - cfg.plume_top) * this.h) / Math.max(0.1, cfg.plume_rise)));
				const puffCount = Math.max(2, Math.ceil(lifeTicks / ticksPerPuff) + 1);
				const chimneyTopXf = chimneyX + chimneyW * 0.5;
				for (let i = 0; i < puffCount; i++) {
					// Phase within the puff cycle. Tick advances continuously, so each
					// puff appears to creep upward smoothly between renders.
					const phase = positiveMod(this.tick + i * ticksPerPuff + this._hash(32000 + i) * ticksPerPuff * 0.6, lifeTicks);
					const age = phase;
					if (age < 0 || age >= lifeTicks) continue;
					const lift = age / lifeTicks;
					const rise = age * cfg.plume_rise;
					const swayPhase = (this.tick * 0.04 + i * 0.83 + age * 0.07);
					const sway = Math.sin(swayPhase) * (1.2 + lift * 1.8);
					const cx = chimneyTopXf + drift * age + sway;
					const cy = chimneyTop - rise;
					if (cy < 1 || cy > horizonRow) continue;
					// Puffs widen and fade as they rise.
					const widen = cfg.plume_width * (0.6 + lift * 1.3);
					const topFadeStart = 1 - cfg.plume_top;
					const topFade = lift > topFadeStart ? clamp01(1 - (lift - topFadeStart) / Math.max(0.05, 1 - topFadeStart)) : 1;
					const baseAlpha = clamp01(plumeStrength * (0.7 - lift * 0.55) * topFade);
					if (baseAlpha < 0.02) continue;
					const puffSeed = 32500 + i * 17;
					const halfW = Math.max(1, Math.ceil(widen));
					const halfH = Math.max(1, Math.round(widen * 0.7));
					for (let dy = -halfH; dy <= halfH; dy++) {
						for (let dx = -halfW; dx <= halfW; dx++) {
							const nx = dx / Math.max(1, halfW);
							const ny = dy / Math.max(1, halfH);
							const r = Math.sqrt(nx * nx + ny * ny);
							if (r > 1.05) continue;
							const noise = this._hash(puffSeed + (dx + halfW) * 7 + (dy + halfH) * 31);
							// Soft falloff modulated by per-cell noise so puffs aren't
							// solid disks.
							const shape = Math.pow(1 - r, Math.max(0.4, 1.4 - cfg.plume_softness));
							const alpha = baseAlpha * shape * (0.7 + noise * 0.5);
							if (alpha < 0.02) continue;
							const tone = clamp01(0.55 + lift * 0.25 + noise * 0.05);
							const puffColor = hslToRGB((cfg.hue + 30) % 360, 0.05, tone);
							const px = Math.round(cx + dx);
							const py = Math.round(cy + dy);
							if (px < 0 || px >= this.w || py < 0 || py >= this.h) continue;
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, px, py, 1, 1, `rgb(${puffColor.r},${puffColor.g},${puffColor.b})`, clamp01(alpha));
						}
					}
				}
			}

			// Ember sparks rising alongside the plume during the ember event.
			if (this.timers.ember > 0) {
				const drift = this._currentDrift();
				const emberCount = 4;
				const lifeTicks = Math.max(18, Math.round(((1 - cfg.plume_top) * this.h) / Math.max(0.2, cfg.plume_rise * 1.1)));
				for (let i = 0; i < emberCount; i++) {
					const phase = positiveMod(this.tick * 0.85 + this._hash(33000 + i) * lifeTicks, lifeTicks);
					const age = phase;
					const lift = age / lifeTicks;
					const rise = age * cfg.plume_rise * 1.1;
					const sway = Math.sin(this.tick * 0.05 + i * 1.2) * 1.5;
					const cx = Math.round(chimneyX + chimneyW * 0.5 + drift * age + sway);
					const cy = Math.round(chimneyTop - rise);
					if (cy < 2 || cy >= horizonRow) continue;
					const ember = hslToRGB((cfg.window_hue + this._hash(33100 + i) * 14) % 360, 0.78, clamp01(0.6 + (1 - lift) * 0.3));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cx, cy, 1, 1, `rgb(${ember.r},${ember.g},${ember.b})`, clamp01(0.7 * (1 - lift)));
				}
			}

			// Vignette to keep the cottage feeling intimate.
			const vignette = ctx.createRadialGradient(canvasW * 0.5, canvasH * 0.55, Math.min(canvasW, canvasH) * 0.32, canvasW * 0.5, canvasH * 0.55, Math.max(canvasW, canvasH) * 0.85);
			vignette.addColorStop(0, 'rgba(0,0,0,0)');
			vignette.addColorStop(1, 'rgba(0,0,0,0.32)');
			ctx.fillStyle = vignette;
			ctx.fillRect(0, 0, canvasW, canvasH);
		}
	}

	api.presets['cottage-chimney'] = [
		{
			key: 'still-night',
			label: 'still night',
			config: {
				puff_rate: 0.16,
				plume_rise: 0.55,
				plume_drift: 0.05,
				plume_width: 3.4,
				plume_softness: 0.5,
				horizon: 0.78,
				window_glow: 0.78,
				window_hue: 46,
				hue: 222,
				hue_sp: 16,
				sat: 0.34,
				lmin: 0.06,
				lmax: 0.3,
			},
		},
		{
			key: 'windy-peak',
			label: 'windy peak',
			config: {
				puff_rate: 0.22,
				plume_rise: 0.62,
				plume_drift: 0.32,
				plume_width: 3.8,
				plume_softness: 0.5,
				horizon: 0.76,
				window_glow: 0.82,
				window_hue: 42,
				hue: 218,
				hue_sp: 22,
				sat: 0.4,
				lmin: 0.08,
				lmax: 0.34,
				gust_p: 0.0014,
				gust_drift_mult: 2.6,
			},
		},
		{
			key: 'lamplit-cabin',
			label: 'lamplit cabin',
			config: {
				puff_rate: 0.18,
				plume_rise: 0.5,
				plume_drift: 0.1,
				plume_width: 3.4,
				plume_softness: 0.55,
				horizon: 0.78,
				window_glow: 0.92,
				window_hue: 36,
				hue: 226,
				hue_sp: 18,
				sat: 0.36,
				lmin: 0.06,
				lmax: 0.3,
				flicker_p: 0.0018,
				flicker_mult: 0.5,
				ember_p: 0.0006,
			},
		},
		{
			key: 'quiet-hearth',
			label: 'quiet hearth',
			config: {
				puff_rate: 0.12,
				plume_rise: 0.45,
				plume_drift: 0.08,
				plume_width: 3.0,
				plume_softness: 0.6,
				horizon: 0.8,
				window_glow: 0.66,
				window_hue: 34,
				hue: 228,
				hue_sp: 14,
				sat: 0.3,
				lmin: 0.05,
				lmax: 0.26,
				quiet_p: 0.0008,
				quiet_rate_mult: 0.4,
			},
		},
	];
	api.effects['cottage-chimney'] = CottageChimney;
})(window.AmbienceSim);
