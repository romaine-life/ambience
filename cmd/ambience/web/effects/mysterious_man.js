'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 70,
		intro_glow: 0.10,
		ending_dur: 85,
		ending_linger: 24,
		ending_glow: 0.06,
		figure_x: 0.5,
		figure_height: 30,
		figure_width: 11,
		silhouette: 0.92,
		hat: 1,
		shoulder: 1,
		ember_x: 0.56,
		ember_y: 0.62,
		ember_brightness: 0.86,
		ember_pulse: 0.34,
		smoke_density: 0.42,
		smoke_rise: 0.46,
		smoke_drift: 0.18,
		smoke_softness: 0.62,
		hue: 22,
		hue_sp: 10,
		sat: 0.72,
		lmin: 0.06,
		lmax: 0.86,
		inhale_p: 0,
		exhale_p: 0,
		ash_fall_p: 0,
		lighter_flick_p: 0,
		inhale_dur: 32,
		inhale_mult: 1.85,
		exhale_dur: 60,
		exhale_plume: 1.4,
		ash_fall_dur: 28,
		ash_fall_mult: 1.3,
		lighter_flick_dur: 20,
		lighter_flick_mult: 2.4,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.figure_x <= 0) c.figure_x = DEFAULTS.figure_x;
		if (c.figure_height <= 0) c.figure_height = DEFAULTS.figure_height;
		if (c.figure_width <= 0) c.figure_width = DEFAULTS.figure_width;
		c.silhouette = clamp01(c.silhouette);
		if (c.silhouette <= 0) c.silhouette = DEFAULTS.silhouette;
		c.hat = clamp01(c.hat);
		c.shoulder = clamp01(c.shoulder);
		if (c.ember_x <= 0) c.ember_x = DEFAULTS.ember_x;
		if (c.ember_y <= 0) c.ember_y = DEFAULTS.ember_y;
		if (c.ember_brightness <= 0) c.ember_brightness = DEFAULTS.ember_brightness;
		if (c.ember_pulse < 0) c.ember_pulse = 0;
		if (c.smoke_density < 0) c.smoke_density = 0;
		if (c.smoke_rise <= 0) c.smoke_rise = DEFAULTS.smoke_rise;
		if (c.smoke_softness <= 0) c.smoke_softness = DEFAULTS.smoke_softness;
		if (c.hue < 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin < 0) c.lmin = 0;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.inhale_dur <= 0) c.inhale_dur = DEFAULTS.inhale_dur;
		if (c.inhale_mult <= 0) c.inhale_mult = DEFAULTS.inhale_mult;
		if (c.exhale_dur <= 0) c.exhale_dur = DEFAULTS.exhale_dur;
		if (c.exhale_plume <= 0) c.exhale_plume = DEFAULTS.exhale_plume;
		if (c.ash_fall_dur <= 0) c.ash_fall_dur = DEFAULTS.ash_fall_dur;
		if (c.ash_fall_mult <= 0) c.ash_fall_mult = DEFAULTS.ash_fall_mult;
		if (c.lighter_flick_dur <= 0) c.lighter_flick_dur = DEFAULTS.lighter_flick_dur;
		if (c.lighter_flick_mult <= 0) c.lighter_flick_mult = DEFAULTS.lighter_flick_mult;
		return c;
	}

	class MysteriousMan {
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
			const rng = this._eventRng(name.length + 109);
			switch (name) {
				case 'inhale':
					this.timers.inhale = jitterInt(rng, this.cfg.inhale_dur, 0.25);
					this.values.inhale_gain = this.cfg.inhale_mult * (0.8 + rng() * 0.4);
					return true;
				case 'exhale':
					this.timers.exhale = jitterInt(rng, this.cfg.exhale_dur, 0.25);
					this.values.exhale_gain = this.cfg.exhale_plume * (0.85 + rng() * 0.35);
					this.values.exhale_seed = rng() * 1024;
					return true;
				case 'ash-fall':
					this.timers['ash-fall'] = jitterInt(rng, this.cfg.ash_fall_dur, 0.3);
					this.values.ash_gain = this.cfg.ash_fall_mult * (0.85 + rng() * 0.3);
					this.values.ash_seed = rng() * 1024;
					return true;
				case 'lighter-flick':
					this.timers['lighter-flick'] = jitterInt(rng, this.cfg.lighter_flick_dur, 0.25);
					this.values.flick_gain = this.cfg.lighter_flick_mult * (0.85 + rng() * 0.3);
					return true;
				case 'intro':
					this.timers.inhale = 0;
					this.timers.exhale = 0;
					this.timers['ash-fall'] = 0;
					this.timers['lighter-flick'] = 0;
					this.timers.ending = 0;
					this.values.inhale_gain = 1;
					this.values.exhale_gain = 1;
					this.values.ash_gain = 1;
					this.values.flick_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.inhale = 0;
					this.timers.exhale = 0;
					this.timers['ash-fall'] = 0;
					this.timers['lighter-flick'] = 0;
					this.values.inhale_gain = 1;
					this.values.exhale_gain = 1;
					this.values.ash_gain = 1;
					this.values.flick_gain = 1;
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
			if (!this.timers.inhale || this.timers.inhale <= 0) this.values.inhale_gain = 1;
			if (!this.timers.exhale || this.timers.exhale <= 0) this.values.exhale_gain = 1;
			if (!this.timers['ash-fall'] || this.timers['ash-fall'] <= 0) this.values.ash_gain = 1;
			if (!this.timers['lighter-flick'] || this.timers['lighter-flick'] <= 0) this.values.flick_gain = 1;
		}

		_emberLevel() {
			let level = 1;
			if (this.timers.inhale > 0) level *= this.values.inhale_gain || this.cfg.inhale_mult;
			if (this.timers['lighter-flick'] > 0) level *= this.values.flick_gain || this.cfg.lighter_flick_mult;
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
			return Math.max(0.0, level);
		}

		_revealLevel() {
			let level = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				// silhouette stays dark until the ember is established, then resolves
				level *= Math.pow(progress, 1.6);
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - progress * 0.92;
			}
			return clamp01(level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				// near-darkness with a faint warmth toward the ember side
				const tintHue = this.cfg.hue;
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				const top = hslToRGB((tintHue + 220) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin + 0.02));
				const mid = hslToRGB((tintHue + 240) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin + 0.04));
				const low = hslToRGB((tintHue + 6) % 360, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmin + 0.06));
				sky.addColorStop(0, `rgb(${top.r},${top.g},${top.b})`);
				sky.addColorStop(0.62, `rgb(${mid.r},${mid.g},${mid.b})`);
				sky.addColorStop(1, `rgb(${low.r},${low.g},${low.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);

			const figureCenterX = Math.floor(this.w * this.cfg.figure_x);
			const figH = Math.max(12, Math.round(this.cfg.figure_height));
			const figW = Math.max(4, Math.round(this.cfg.figure_width));
			const halfBody = Math.max(2, Math.round(figW * 0.5));
			const groundRow = Math.min(this.h - 1, Math.floor(this.h * 0.94));
			const headRadius = Math.max(2, Math.round(figW * 0.32));
			const headTop = Math.max(2, groundRow - figH);
			const headCenterY = headTop + headRadius;
			const shoulderRow = headCenterY + headRadius + 1;
			const torsoTop = shoulderRow;
			const reveal = this._revealLevel();
			const ember = this._emberLevel();
			const emberX = Math.floor(this.w * this.cfg.ember_x);
			const emberY = Math.floor(this.h * this.cfg.ember_y);
			const breathPhase = Math.sin(this.tick * 0.05);
			const emberPulse = clamp01(this.cfg.ember_brightness * (1 + breathPhase * this.cfg.ember_pulse * 0.45) * ember);

			// soft ember halo cast around the cigarette
			const haloR = Math.max(20, Math.min(canvasW, canvasH) * (0.05 + emberPulse * 0.09));
			const haloX = emberX * sx;
			const haloY = emberY * sy;
			const haloHue = (this.cfg.hue + 4) % 360;
			const haloCore = hslToRGB(haloHue, clamp01(this.cfg.sat * 0.95), clamp01(this.cfg.lmax * 0.9));
			const haloMid = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat * 0.7), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.4));
			const haloGrad = ctx.createRadialGradient(haloX, haloY, 0, haloX, haloY, haloR);
			haloGrad.addColorStop(0, `rgba(${haloCore.r},${haloCore.g},${haloCore.b},${0.32 + emberPulse * 0.4})`);
			haloGrad.addColorStop(0.5, `rgba(${haloMid.r},${haloMid.g},${haloMid.b},${0.12 + emberPulse * 0.18})`);
			haloGrad.addColorStop(1, `rgba(${haloMid.r},${haloMid.g},${haloMid.b},0)`);
			ctx.fillStyle = haloGrad;
			ctx.fillRect(haloX - haloR, haloY - haloR, haloR * 2, haloR * 2);

			// silhouette body (only renders once the intro reveal has progressed)
			const silAlpha = clamp01(this.cfg.silhouette * reveal);
			if (silAlpha > 0.02) {
				const silColor = hslToRGB((this.cfg.hue + 220) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin * 0.4));
				const silStr = `rgb(${silColor.r},${silColor.g},${silColor.b})`;

				// torso: a slightly tapered column from shoulders to ground
				for (let y = torsoTop; y <= groundRow; y++) {
					const t = (y - torsoTop) / Math.max(1, groundRow - torsoTop);
					const half = Math.max(2, Math.round(halfBody * (0.78 + t * 0.34)));
					for (let dx = -half; dx <= half; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, figureCenterX + dx, y, 1, 1, silStr, silAlpha);
					}
				}

				// shoulder bulge so the figure reads as a coated person
				if (this.cfg.shoulder > 0.05) {
					const shoulderHalf = halfBody + Math.max(1, Math.round(this.cfg.shoulder * 2));
					for (let dx = -shoulderHalf; dx <= shoulderHalf; dx++) {
						const nx = Math.abs(dx) / Math.max(1, shoulderHalf);
						const fall = Math.pow(1 - nx, 1.6);
						const top = shoulderRow - Math.round(this.cfg.shoulder * 1.4 * fall);
						for (let y = top; y < shoulderRow + 2; y++) {
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, figureCenterX + dx, y, 1, 1, silStr, silAlpha);
						}
					}
				}

				// head: a roundish cap above the shoulders
				for (let dy = -headRadius; dy <= headRadius; dy++) {
					const span = Math.round(Math.sqrt(Math.max(0, headRadius * headRadius - dy * dy)));
					if (span <= 0) continue;
					for (let dx = -span; dx <= span; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, figureCenterX + dx, headCenterY + dy, 1, 1, silStr, silAlpha);
					}
				}

				// hat brim/crown (optional, helps the noir read)
				if (this.cfg.hat > 0.05) {
					const brimHalf = headRadius + Math.max(1, Math.round(this.cfg.hat * 2));
					const brimY = Math.max(0, headCenterY - headRadius);
					const crownH = Math.max(1, Math.round(this.cfg.hat * 2));
					// brim
					for (let dx = -brimHalf; dx <= brimHalf; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, figureCenterX + dx, brimY, 1, 1, silStr, silAlpha);
					}
					// crown
					for (let dy = 1; dy <= crownH; dy++) {
						const crownHalf = Math.max(1, headRadius - Math.round(dy * 0.4));
						for (let dx = -crownHalf; dx <= crownHalf; dx++) {
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, figureCenterX + dx, brimY - dy, 1, 1, silStr, silAlpha);
						}
					}
				}

				// faint warm rim-light on the side facing the ember
				const rimDir = emberX >= figureCenterX ? 1 : -1;
				const rimColor = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.32));
				for (let y = torsoTop; y <= groundRow; y++) {
					const dist = Math.hypot((figureCenterX + halfBody * rimDir) - emberX, y - emberY);
					const fall = Math.exp(-dist / Math.max(4, figH * 0.6));
					const a = clamp01(fall * (0.18 + emberPulse * 0.32));
					if (a < 0.02) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, figureCenterX + halfBody * rimDir, y, 1, 1, `rgb(${rimColor.r},${rimColor.g},${rimColor.b})`, a);
				}
			}

			// faint cigarette stem from the figure's mouth area to the ember
			const mouthX = figureCenterX + Math.round(headRadius * 0.6) * (emberX >= figureCenterX ? 1 : -1);
			const mouthY = headCenterY + Math.max(1, Math.round(headRadius * 0.5));
			const cigDir = emberX >= mouthX ? 1 : -1;
			if (silAlpha > 0.05 && Math.abs(emberX - mouthX) > 0) {
				const stemColor = hslToRGB(0, 0, 0.55);
				const steps = Math.max(1, Math.abs(emberX - mouthX));
				for (let i = 1; i < steps; i++) {
					const cx = mouthX + cigDir * i;
					const t = i / Math.max(1, steps);
					const cy = Math.round(mouthY + (emberY - mouthY) * t);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cx, cy, 1, 1, `rgb(${stemColor.r},${stemColor.g},${stemColor.b})`, clamp01(0.22 * silAlpha));
				}
			}

			// ember itself: square pixel-art tip with flat ends
			const emberHueShift = (this.cfg.hue + 6) % 360;
			const emberCore = hslToRGB(emberHueShift, clamp01(this.cfg.sat * 0.95), clamp01(this.cfg.lmax * (0.86 + emberPulse * 0.14)));
			const emberRim = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.7));
			const emberAlpha = clamp01(0.6 + emberPulse * 0.4);
			const rimAlpha = clamp01(0.3 + emberPulse * 0.34);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, emberX - cigDir, emberY, 1, 1, `rgb(${emberRim.r},${emberRim.g},${emberRim.b})`, rimAlpha);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, emberX, emberY, 1, 1, `rgb(${emberCore.r},${emberCore.g},${emberCore.b})`, emberAlpha);

			// drifting smoke puffs above the ember
			const smokeColor = hslToRGB((this.cfg.hue + 220) % 360, 0.06, clamp01(0.62 + this.cfg.lmin * 0.4));
			const exhaleActive = this.timers.exhale > 0;
			const exhaleGain = this.values.exhale_gain || this.cfg.exhale_plume;
			const inhaleActive = this.timers.inhale > 0;
			const baseDensity = this.cfg.smoke_density * reveal;
			const puffCount = Math.max(2, Math.round(baseDensity * 22 * (exhaleActive ? exhaleGain : 1)));
			const maxRise = Math.max(8, Math.round(this.h * 0.42 + this.cfg.smoke_rise * 14));
			for (let i = 0; i < puffCount; i++) {
				const cycle = maxRise + 12 + Math.floor(this._hash(28000 + i) * 16);
				const speed = this.cfg.smoke_rise * (0.5 + this._hash(28100 + i) * 0.9);
				let progress = positiveMod(this.tick * speed + this._hash(28200 + i) * cycle, cycle);
				if (progress > maxRise) continue;
				if (inhaleActive && progress < 4) continue; // inhale briefly compresses the rise
				const rise = progress;
				const fade = 1 - rise / Math.max(1, maxRise);
				const drift = (this._hash(28300 + i) * 2 - 1) * 0.6 + this.cfg.smoke_drift * (0.3 + rise * 0.06) + Math.sin(this.tick * 0.03 + i * 0.7) * 0.4;
				const col = Math.round(emberX + drift + (i % 3 - 1) * 0.5);
				const row = Math.round(emberY - 1 - rise);
				if (row < 1 || row >= this.h) continue;
				const size = fade > 0.6 ? 2 : 1;
				const softness = clamp01(this.cfg.smoke_softness);
				const alpha = clamp01((0.08 + fade * 0.42) * (0.6 + softness * 0.5) * (exhaleActive ? exhaleGain * 0.6 : 1) * reveal);
				if (alpha < 0.02) continue;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, size, size, `rgb(${smokeColor.r},${smokeColor.g},${smokeColor.b})`, alpha);
				if (size === 2) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col + 1, row, 1, 1, `rgb(${smokeColor.r},${smokeColor.g},${smokeColor.b})`, alpha * 0.7);
				}
			}

			// ash fleck breaking off
			if (this.timers['ash-fall'] > 0) {
				const ashSeed = this.values.ash_seed || 0;
				const totalDur = Math.max(1, Math.round(this.cfg.ash_fall_dur));
				const elapsed = totalDur - this.timers['ash-fall'];
				const t = clamp01(elapsed / totalDur);
				const ashCol = Math.round(emberX + Math.sin(ashSeed * 6.28 + t * 0.6) * 1.4);
				const ashRow = Math.round(emberY + 1 + t * (this.h - emberY - 4) * 0.6);
				if (ashRow < this.h - 1) {
					const ashColor = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.85), clamp01(this.cfg.lmax * (0.65 + (1 - t) * 0.3)));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, ashCol, ashRow, 1, 1, `rgb(${ashColor.r},${ashColor.g},${ashColor.b})`, clamp01((0.6 + (this.values.ash_gain || 1) * 0.2) * (1 - t * 0.7)));
				}
			}

			// vignette darkens the edges so the silhouette read stays
			const vignette = ctx.createRadialGradient(canvasW * 0.5, canvasH * 0.5, Math.min(canvasW, canvasH) * 0.4, canvasW * 0.5, canvasH * 0.5, Math.max(canvasW, canvasH) * 0.85);
			vignette.addColorStop(0, 'rgba(0,0,0,0)');
			vignette.addColorStop(1, 'rgba(0,0,0,0.55)');
			ctx.fillStyle = vignette;
			ctx.fillRect(0, 0, canvasW, canvasH);
		}
	}

	api.presets['mysterious-man'] = [
		{
			key: 'noir-stillness',
			label: 'noir stillness',
			config: {
				figure_x: 0.5,
				figure_height: 30,
				figure_width: 11,
				silhouette: 0.94,
				hat: 1,
				shoulder: 1,
				ember_x: 0.56,
				ember_y: 0.62,
				ember_brightness: 0.78,
				ember_pulse: 0.28,
				smoke_density: 0.38,
				smoke_rise: 0.42,
				smoke_drift: 0.14,
				smoke_softness: 0.66,
				hue: 22,
				hue_sp: 10,
				sat: 0.7,
				lmin: 0.06,
				lmax: 0.84,
				exhale_p: 0.0009,
				ash_fall_p: 0.0006,
			},
		},
		{
			key: 'deep-inhale',
			label: 'deep inhale',
			config: {
				figure_x: 0.48,
				figure_height: 32,
				figure_width: 12,
				silhouette: 0.92,
				hat: 0.7,
				shoulder: 1,
				ember_x: 0.55,
				ember_y: 0.6,
				ember_brightness: 1.0,
				ember_pulse: 0.5,
				smoke_density: 0.5,
				smoke_rise: 0.5,
				smoke_drift: 0.22,
				smoke_softness: 0.58,
				hue: 18,
				hue_sp: 14,
				sat: 0.82,
				lmin: 0.05,
				lmax: 0.92,
				inhale_p: 0.0026,
				inhale_dur: 36,
				inhale_mult: 2.1,
				exhale_p: 0.0018,
				exhale_dur: 64,
				exhale_plume: 1.6,
			},
		},
		{
			key: 'cold-alley',
			label: 'cold alley',
			config: {
				figure_x: 0.42,
				figure_height: 30,
				figure_width: 10,
				silhouette: 0.96,
				hat: 1,
				shoulder: 1,
				ember_x: 0.49,
				ember_y: 0.6,
				ember_brightness: 0.74,
				ember_pulse: 0.24,
				smoke_density: 0.6,
				smoke_rise: 0.34,
				smoke_drift: -0.12,
				smoke_softness: 0.78,
				hue: 14,
				hue_sp: 8,
				sat: 0.58,
				lmin: 0.08,
				lmax: 0.78,
				exhale_p: 0.0012,
				exhale_dur: 70,
				exhale_plume: 1.55,
				ash_fall_p: 0.0008,
			},
		},
		{
			key: 'ember-watch',
			label: 'ember watch',
			config: {
				intro_glow: 0.04,
				ending_glow: 0.04,
				figure_x: 0.5,
				figure_height: 28,
				figure_width: 11,
				silhouette: 0.97,
				hat: 1,
				shoulder: 0.9,
				ember_x: 0.57,
				ember_y: 0.6,
				ember_brightness: 0.92,
				ember_pulse: 0.46,
				smoke_density: 0.32,
				smoke_rise: 0.4,
				smoke_drift: 0.08,
				smoke_softness: 0.7,
				hue: 26,
				hue_sp: 16,
				sat: 0.8,
				lmin: 0.04,
				lmax: 0.9,
				inhale_p: 0.0014,
				exhale_p: 0.0009,
				ash_fall_p: 0.0008,
				lighter_flick_p: 0.0004,
				lighter_flick_mult: 2.6,
			},
		},
	];
	api.effects['mysterious-man'] = MysteriousMan;
})(window.AmbienceSim);
