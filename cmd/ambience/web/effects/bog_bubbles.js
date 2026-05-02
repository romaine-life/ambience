'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	// BogBubbles — sister to underwater (#32), but the action is at the
	// surface. Server owns lifecycle (intro/ending) and the methane-burst /
	// quiet-bog event timers. Per-bubble motion is rendered deterministically
	// from tick + seed; the timer values just bias the visible spawn rate.
	const DEFAULTS = {
		intro_dur: 80,
		intro_first: 30,
		ending_dur: 90,
		ending_linger: 60,
		spawn_rate: 1.6,
		rise_speed: 0.18,
		bubble_size: 3.5,
		viscosity: 0.6,
		water_level: 0.55,
		mist: 0.18,
		ripple_life: 26,
		ripple_size: 8,
		hue: 110,
		hue_sp: 14,
		sat: 0.42,
		lmin: 0.10,
		lmax: 0.78,
		burst_p: 0,
		quiet_p: 0,
		burst_dur: 40,
		burst_mult: 2.6,
		quiet_dur: 100,
		quiet_mult: 0.25,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		if (c.intro_first < 0) c.intro_first = 0;
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		if (c.spawn_rate <= 0) c.spawn_rate = DEFAULTS.spawn_rate;
		if (c.rise_speed <= 0) c.rise_speed = DEFAULTS.rise_speed;
		if (c.bubble_size <= 0) c.bubble_size = DEFAULTS.bubble_size;
		if (c.viscosity < 0) c.viscosity = 0;
		if (c.water_level <= 0) c.water_level = DEFAULTS.water_level;
		if (c.mist < 0) c.mist = 0;
		if (c.ripple_life <= 0) c.ripple_life = DEFAULTS.ripple_life;
		if (c.ripple_size <= 0) c.ripple_size = DEFAULTS.ripple_size;
		if (c.hue === 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.burst_p < 0) c.burst_p = 0;
		if (c.quiet_p < 0) c.quiet_p = 0;
		if (c.burst_dur <= 0) c.burst_dur = DEFAULTS.burst_dur;
		if (c.burst_mult <= 0) c.burst_mult = DEFAULTS.burst_mult;
		if (c.quiet_dur <= 0) c.quiet_dur = DEFAULTS.quiet_dur;
		if (c.quiet_mult < 0) c.quiet_mult = 0;
		return c;
	}

	class BogBubbles {
		constructor(w, h, cfg, seed) {
			this.kind = 'bog-bubbles';
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
			const rng = this._eventRng(name.length + 31);
			switch (name) {
				case 'methane-burst':
					this.timers['methane-burst'] = jitterInt(rng, this.cfg.burst_dur, 0.3);
					this.timers['quiet-bog'] = 0;
					this.values.spawn_gain = this.cfg.burst_mult * (0.85 + rng() * 0.3);
					return true;
				case 'quiet-bog':
					this.timers['quiet-bog'] = jitterInt(rng, this.cfg.quiet_dur, 0.3);
					this.timers['methane-burst'] = 0;
					this.values.spawn_gain = 1;
					return true;
				case 'intro':
					this.timers['methane-burst'] = 0;
					this.timers['quiet-bog'] = 0;
					this.timers.ending = 0;
					this.values.spawn_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['methane-burst'] = 0;
					this.timers['quiet-bog'] = 0;
					this.values.spawn_gain = 1;
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
			if (!this.timers['methane-burst'] || this.timers['methane-burst'] <= 0) this.values.spawn_gain = 1;
		}

		_spawnLevel() {
			let level = 1;
			if (this.timers['methane-burst'] > 0) level *= this.values.spawn_gain || this.cfg.burst_mult;
			if (this.timers['quiet-bog'] > 0) level *= this.cfg.quiet_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				const introFirst = Math.max(0, this.cfg.intro_first);
				const elapsed = total - this.timers.intro;
				if (elapsed < introFirst) {
					level *= 0;
				} else {
					level *= clamp01(progress);
				}
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				// Ramp toward 0 across the fade, then stay at 0 through linger.
				const fadeRatio = clamp01(this.cfg.ending_dur / Math.max(1, total));
				if (progress >= fadeRatio) {
					level *= 0;
				} else {
					level *= 1 - progress / Math.max(0.01, fadeRatio);
				}
			}
			return Math.max(0, level);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const surfaceRow = Math.max(4, Math.min(this.h - 4, Math.floor(this.h * this.cfg.water_level)));
			const bottomRow = this.h - 1;
			const riseRows = Math.max(4, bottomRow - surfaceRow);

			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				// Sky / mist gradient above the water line.
				const skyTop = hslToRGB((this.cfg.hue + 18) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const skyBot = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.34));
				const sky = ctx.createLinearGradient(0, 0, 0, surfaceRow * sy);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(1, `rgb(${skyBot.r},${skyBot.g},${skyBot.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, Math.ceil(surfaceRow * sy));

				// Water column.
				const waterTop = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.78), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.34));
				const waterBot = hslToRGB((this.cfg.hue + 6) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.85));
				const water = ctx.createLinearGradient(0, surfaceRow * sy, 0, canvasH);
				water.addColorStop(0, `rgb(${waterTop.r},${waterTop.g},${waterTop.b})`);
				water.addColorStop(1, `rgb(${waterBot.r},${waterBot.g},${waterBot.b})`);
				ctx.fillStyle = water;
				ctx.fillRect(0, Math.floor(surfaceRow * sy), canvasW, canvasH - Math.floor(surfaceRow * sy));
			}

			const level = this._spawnLevel();

			// Mist layer drifting just above the surface.
			if (this.cfg.mist > 0) {
				const mistColor = hslToRGB((this.cfg.hue + 20) % 360, clamp01(this.cfg.sat * 0.3), clamp01(this.cfg.lmax * 0.85));
				const mistRows = Math.max(2, Math.round(this.h * 0.06));
				for (let dy = 0; dy < mistRows; dy++) {
					const row = surfaceRow - 1 - dy;
					if (row < 0) break;
					const fade = 1 - dy / Math.max(1, mistRows - 1);
					for (let x = 0; x < this.w; x++) {
						const drift = Math.sin(this.tick * 0.012 + x * 0.18 + dy * 0.4) * 0.5 + 0.5;
						const alpha = clamp01(this.cfg.mist * 0.45 * fade * (0.4 + drift * 0.6));
						if (alpha < 0.04) continue;
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${mistColor.r},${mistColor.g},${mistColor.b})`, alpha);
					}
				}
			}

			// Surface line — slightly brighter band picking up the mist colour.
			const surfaceColor = hslToRGB((this.cfg.hue + 4) % 360, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.55));
			for (let x = 0; x < this.w; x++) {
				const wobble = Math.sin(this.tick * 0.04 + x * 0.32) * 0.5;
				const row = surfaceRow + Math.round(wobble * 0.0); // surface stays flat by default
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, 0.85);
			}

			// Subsurface murk shading: darker silt near the bottom.
			const muckColor = hslToRGB((this.cfg.hue + 12) % 360, clamp01(this.cfg.sat * 0.35), clamp01(this.cfg.lmin * 0.55));
			for (let row = bottomRow; row > bottomRow - Math.max(2, Math.round(this.h * 0.08)); row--) {
				const fade = (bottomRow - row) / Math.max(1, Math.round(this.h * 0.08));
				for (let x = 0; x < this.w; x += 2) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + (this.tick % 2), row, 1, 1, `rgb(${muckColor.r},${muckColor.g},${muckColor.b})`, clamp01(0.5 - fade * 0.4));
				}
			}

			// Bubble + ripple slots. Each slot is a deterministic emitter cycling
			// through rise → pop → cooldown. The current cycle index picks the
			// horizontal column so bubbles do not appear to come from the same
			// fixed line of "vents" each loop.
			const slotCount = Math.max(3, Math.round(this.cfg.spawn_rate * 5));
			const baseRise = Math.max(0.05, this.cfg.rise_speed);
			const riseTicks = Math.max(8, Math.round(riseRows / baseRise));
			const popTicks = Math.max(4, Math.round(this.cfg.ripple_life));
			const cycleLen = Math.max(40, riseTicks + popTicks + 30);
			const baseSize = Math.max(1, this.cfg.bubble_size);

			for (let i = 0; i < slotCount; i++) {
				const phaseOffset = Math.floor(this._hash(4000 + i) * cycleLen);
				const totalPhase = this.tick + phaseOffset;
				const cycleIdx = Math.floor(totalPhase / cycleLen);
				const phase = positiveMod(totalPhase, cycleLen);
				// Probabilistically skip slots based on level so spawn rate
				// reads as denser/sparser without changing slot count. Hash is
				// stable per-cycle so a bubble that "starts" in a cycle will
				// continue to render through that cycle.
				const cycleHash = this._hash(4100 + i * 17 + cycleIdx);
				if (cycleHash > clamp01(level)) continue;

				const colHash = this._hash(4200 + i * 13 + cycleIdx);
				const sizeHash = this._hash(4300 + i * 23 + cycleIdx);
				const wobbleHash = this._hash(4400 + i * 29 + cycleIdx);
				const baseCol = colHash * this.w;
				const bubbleSize = Math.max(1, Math.round(baseSize * (0.55 + sizeHash * 0.7)));

				if (phase < riseTicks) {
					const climb = phase / riseTicks; // 0 at bottom, 1 at surface
					const cy = bottomRow - Math.round(climb * riseRows);
					const wobble = Math.sin(phase * 0.18 + wobbleHash * 6.28) * this.cfg.viscosity * (0.5 + climb * 0.6);
					const cx = Math.round(baseCol + wobble);
					this._paintBubble(ctx, sx, sy, ceilSx, ceilSy, cx, cy, bubbleSize, climb);
					// Slight upward distortion of the surface as the bubble nears it.
					if (climb > 0.7) {
						const bulge = Math.round((climb - 0.7) * 4 + bubbleSize * 0.3);
						const bulgeStrength = (climb - 0.7) / 0.3;
						for (let dx = -bulge; dx <= bulge; dx++) {
							const t = 1 - Math.abs(dx) / Math.max(1, bulge);
							const lift = Math.round(t * t * (1.2 + this.cfg.viscosity) * bulgeStrength);
							if (lift <= 0) continue;
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cx + dx, surfaceRow - lift, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, 0.7);
						}
					}
				} else if (phase < riseTicks + popTicks) {
					// Surface-pop ripple: concentric ring expanding from the
					// bubble's surfacing column.
					const ringPhase = (phase - riseTicks) / popTicks;
					const radius = Math.max(1, Math.round(this.cfg.ripple_size * ringPhase));
					const cx = Math.round(baseCol);
					this._paintRipple(ctx, sx, sy, ceilSx, ceilSy, cx, surfaceRow, radius, 1 - ringPhase, bubbleSize, surfaceColor);
				}
			}
		}

		_paintBubble(ctx, sx, sy, ceilSx, ceilSy, cx, cy, size, climb) {
			const hue = this.cfg.hue + (this._hash(cx * 9 + cy * 7) - 0.5) * this.cfg.hue_sp * 0.5;
			const body = hslToRGB((hue + 360) % 360, clamp01(this.cfg.sat * 0.45), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.42 + climb * 0.2)));
			const rim = hslToRGB((hue + 4) % 360, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.68));
			const radius = Math.max(0.8, size * 0.5);
			const reach = Math.ceil(radius + 0.3);
			// Render as a filled circle of body cells with rim cells at the
			// boundary. Skip stray highlight pixels — at low pixel sizes a
			// bright top dot reads as a separate hat instead of a sphere.
			for (let dy = -reach; dy <= reach; dy++) {
				for (let dx = -reach; dx <= reach; dx++) {
					const dist = Math.sqrt(dx * dx + dy * dy);
					if (dist > radius + 0.2) continue;
					const onRim = dist >= radius - 0.55;
					const color = onRim ? rim : body;
					const alpha = onRim ? 0.85 : 0.55;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cx + dx, cy + dy, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}
		}

		_paintRipple(ctx, sx, sy, ceilSx, ceilSy, cx, cy, radius, fade, bubbleSize, surfaceColor) {
			if (radius <= 0 || fade <= 0) return;
			const ringColor = hslToRGB((this.cfg.hue + 6) % 360, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmax * (0.7 + fade * 0.25)));
			const splashAlpha = clamp01(0.65 * fade);
			// Inner splash at the pop site (only for the first portion of the ring's life).
			if (fade > 0.6) {
				const splashHalf = Math.max(1, Math.round(bubbleSize * 0.5));
				for (let dy = -splashHalf; dy <= splashHalf; dy++) {
					for (let dx = -splashHalf; dx <= splashHalf; dx++) {
						if (Math.abs(dx) + Math.abs(dy) > splashHalf + 1) continue;
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cx + dx, cy - 1 + dy, 1, 1, `rgb(${ringColor.r},${ringColor.g},${ringColor.b})`, splashAlpha);
					}
				}
			}
			// Concentric ring at the current radius: top half on the surface row,
			// shallower extensions one row deeper as it widens.
			for (let theta = 0; theta < Math.PI * 2; theta += Math.PI / Math.max(8, radius * 4)) {
				const dx = Math.round(Math.cos(theta) * radius);
				const dy = Math.round(Math.sin(theta) * radius * 0.35);
				const col = cx + dx;
				const row = cy + dy;
				const alpha = clamp01(0.5 * fade * (1 - Math.abs(dy) * 0.18));
				if (alpha < 0.04) continue;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${ringColor.r},${ringColor.g},${ringColor.b})`, alpha);
			}
		}
	}

	api.presets['bog-bubbles'] = [
		{
			key: 'swampy-green',
			label: 'swampy green',
			config: {
				hue: 110,
				hue_sp: 14,
				sat: 0.42,
				lmin: 0.10,
				lmax: 0.78,
				spawn_rate: 1.6,
				rise_speed: 0.18,
				bubble_size: 3.5,
				viscosity: 0.6,
				water_level: 0.55,
				mist: 0.22,
				ripple_life: 26,
				ripple_size: 8,
				burst_p: 0.0006,
				quiet_p: 0.0008,
			},
		},
		{
			key: 'tar-pit',
			label: 'tar pit',
			config: {
				hue: 36,
				hue_sp: 10,
				sat: 0.32,
				lmin: 0.06,
				lmax: 0.5,
				spawn_rate: 1.0,
				rise_speed: 0.12,
				bubble_size: 4.5,
				viscosity: 1.0,
				water_level: 0.6,
				mist: 0.06,
				ripple_life: 36,
				ripple_size: 9,
				burst_p: 0.0008,
				quiet_p: 0.0011,
			},
		},
		{
			key: 'firefly-bog',
			label: 'firefly bog',
			config: {
				hue: 78,
				hue_sp: 24,
				sat: 0.6,
				lmin: 0.08,
				lmax: 0.88,
				spawn_rate: 2.2,
				rise_speed: 0.22,
				bubble_size: 3,
				viscosity: 0.7,
				water_level: 0.52,
				mist: 0.32,
				ripple_life: 22,
				ripple_size: 7,
				burst_p: 0.0014,
				quiet_p: 0.0006,
			},
		},
		{
			key: 'winter-freeze',
			label: 'winter freeze',
			config: {
				hue: 198,
				hue_sp: 10,
				sat: 0.22,
				lmin: 0.18,
				lmax: 0.86,
				spawn_rate: 0.9,
				rise_speed: 0.14,
				bubble_size: 2.5,
				viscosity: 0.3,
				water_level: 0.58,
				mist: 0.42,
				ripple_life: 30,
				ripple_size: 6,
				burst_p: 0.0004,
				quiet_p: 0.0014,
				quiet_mult: 0.15,
			},
		},
	];
	api.effects['bog-bubbles'] = BogBubbles;
})(window.AmbienceSim);
