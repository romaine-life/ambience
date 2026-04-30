'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	// RainOnWindow — droplets nucleate on a windowpane, grow, then track
	// downward once they exceed a critical mass. A diffuse warm/cool glow
	// behind the pane carries the scene palette. Mirrors sim/rain_on_window.go:
	// authority owns lifecycle phases and discrete events (wind-gust,
	// quiet-pane); both server and clients run physics and snapshot the
	// droplet list so join-in-progress sessions land on roughly the same
	// pane state.
	const RAIN_ON_WINDOW_DEFAULTS = {
		intro_dur: 60,
		intro_density: 1.8,
		intro_bg: 0.35,
		ending_dur: 80,
		ending_linger: 40,
		ending_residue: 0.25,
		frame_thick: 2,
		pane_pad: 0.04,
		spawn_p: 0.5,
		drop_max: 80,
		grow_rate: 0.012,
		merge_r: 1.6,
		merge_p: 0.5,
		fall_thresh: 1.7,
		fall_speed: 0.18,
		gravity: 0.012,
		trail_life: 60,
		bg_hue: 32,
		bg_sat: 0.42,
		bg_light: 0.24,
		glow_hue: 38,
		glow: 0.6,
		frame_light: 0.08,
		drop_sat: 0.18,
		drop_light: 0.7,
		hi_light: 0.92,
		gust_p: 0,
		quiet_p: 0,
		gust_dur: 40,
		gust_str: 0.6,
		quiet_dur: 80,
		quiet_mult: 0.2,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, RAIN_ON_WINDOW_DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = RAIN_ON_WINDOW_DEFAULTS.intro_dur;
		if (c.intro_density <= 0) c.intro_density = RAIN_ON_WINDOW_DEFAULTS.intro_density;
		if (c.intro_bg < 0) c.intro_bg = 0;
		c.intro_bg = clamp01(c.intro_bg);
		if (c.ending_dur <= 0) c.ending_dur = RAIN_ON_WINDOW_DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_residue = clamp01(c.ending_residue);
		if (c.frame_thick < 0) c.frame_thick = 0;
		if (c.pane_pad < 0) c.pane_pad = 0;
		if (c.spawn_p < 0) c.spawn_p = 0;
		if (c.drop_max <= 0) c.drop_max = RAIN_ON_WINDOW_DEFAULTS.drop_max;
		if (c.grow_rate <= 0) c.grow_rate = RAIN_ON_WINDOW_DEFAULTS.grow_rate;
		if (c.merge_r <= 0) c.merge_r = RAIN_ON_WINDOW_DEFAULTS.merge_r;
		if (c.merge_p < 0) c.merge_p = 0;
		if (c.fall_thresh <= 0) c.fall_thresh = RAIN_ON_WINDOW_DEFAULTS.fall_thresh;
		if (c.fall_speed <= 0) c.fall_speed = RAIN_ON_WINDOW_DEFAULTS.fall_speed;
		if (c.gravity < 0) c.gravity = 0;
		if (c.trail_life <= 0) c.trail_life = RAIN_ON_WINDOW_DEFAULTS.trail_life;
		if (c.bg_sat <= 0) c.bg_sat = RAIN_ON_WINDOW_DEFAULTS.bg_sat;
		if (c.bg_light <= 0) c.bg_light = RAIN_ON_WINDOW_DEFAULTS.bg_light;
		if (c.glow < 0) c.glow = 0;
		if (c.frame_light < 0) c.frame_light = 0;
		if (c.drop_sat < 0) c.drop_sat = 0;
		if (c.drop_light <= 0) c.drop_light = RAIN_ON_WINDOW_DEFAULTS.drop_light;
		if (c.hi_light <= 0) c.hi_light = RAIN_ON_WINDOW_DEFAULTS.hi_light;
		if (c.gust_p < 0) c.gust_p = 0;
		if (c.quiet_p < 0) c.quiet_p = 0;
		if (c.gust_dur <= 0) c.gust_dur = RAIN_ON_WINDOW_DEFAULTS.gust_dur;
		if (c.gust_str <= 0) c.gust_str = RAIN_ON_WINDOW_DEFAULTS.gust_str;
		if (c.quiet_dur <= 0) c.quiet_dur = RAIN_ON_WINDOW_DEFAULTS.quiet_dur;
		if (c.quiet_mult < 0) c.quiet_mult = 0;
		return c;
	}

	function positiveMod(value, mod) {
		if (mod === 0) return 0;
		return ((value % mod) + mod) % mod;
	}

	class RainOnWindow {
		constructor(w, h, cfg, seed) {
			this.kind = 'rain-on-window';
			this.w = w;
			this.h = h;
			this.cfg = applyDefaults(cfg);
			this.seed = Number(seed || Date.now());
			this.rng = makeRNG(this.seed);
			this.tick = 0;
			this.droplets = [];
			this.tracks = [];
			this.gustTicks = 0;
			this.gustSide = 0;
			this.gustWind = 0;
			this.quietTicks = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
		}

		setConfig(cfg) {
			this.cfg = applyDefaults(Object.assign({}, this.cfg, cfg));
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || 0;
			this.gustTicks = state.gustTicks || 0;
			this.gustSide = state.gustSide || 0;
			this.gustWind = state.gustWind || 0;
			this.quietTicks = state.quietTicks || 0;
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			if (typeof snap.seed === 'number') {
				this.seed = snap.seed;
				this.rng = makeRNG(snap.seed);
			}
			if (snap.gridW > 0 && snap.gridH > 0) {
				this.w = snap.gridW;
				this.h = snap.gridH;
			}
			this.droplets = Array.isArray(state.droplets) ? state.droplets.map((d) => ({
				row: d.row, col: d.col, radius: d.radius,
				vRow: d.vRow || 0, vCol: d.vCol || 0,
				hue: d.hue || this.cfg.bg_hue,
				pinRow: d.pinRow || 0,
				falling: !!d.falling,
			})) : [];
			this.tracks = Array.isArray(state.tracks) ? state.tracks.map((t) => ({
				row: t.row, col: t.col, strength: t.strength || 1,
				life: t.life | 0, maxLife: (t.maxLife || t.life) | 0,
			})) : [];
		}

		triggerEvent(name) {
			switch (name) {
				case 'wind-gust':
					this._startGust();
					return true;
				case 'quiet-pane':
					this._startQuiet();
					return true;
				case 'intro':
					this._startIntro();
					return true;
				case 'ending':
					this._startEnding();
					return true;
			}
			return false;
		}

		step() {
			this.tick++;
			if (this.gustTicks > 0) {
				this.gustTicks--;
				if (this.gustTicks === 0) {
					this.gustWind = 0;
					this.gustSide = 0;
				}
			}
			if (this.quietTicks > 0) this.quietTicks--;
			if (this.introTicks > 0) this.introTicks--;
			if (this.endingTicks > 0) this.endingTicks--;
			this._spawn();
			this._advanceDroplets();
			this._advanceTracks();
		}

		_startGust() {
			this.gustTicks = jitterInt(this.rng, this.cfg.gust_dur, 0.3);
			this.gustSide = this.rng() < 0.5 ? -1 : 1;
			this.gustWind = this.gustSide * this.cfg.gust_str;
		}

		_startQuiet() {
			this.quietTicks = jitterInt(this.rng, this.cfg.quiet_dur, 0.3);
		}

		_startIntro() {
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.introTotal = this.cfg.intro_dur > 0 ? this.cfg.intro_dur : RAIN_ON_WINDOW_DEFAULTS.intro_dur;
			this.introTicks = this.introTotal;
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = this.cfg.ending_dur > 0 ? this.cfg.ending_dur : RAIN_ON_WINDOW_DEFAULTS.ending_dur;
			const linger = Math.max(0, this.cfg.ending_linger);
			this.endingTotal = Math.max(1, this.endingFade + linger);
			this.endingTicks = this.endingTotal;
		}

		_phaseProgress(total, left) {
			if (left <= 1 || total <= 1) return 1;
			const elapsed = total - left;
			if (elapsed <= 0) return 0;
			return clamp01(elapsed / (total - 1));
		}

		_activityLevel() {
			let level = 1;
			if (this.quietTicks > 0) level *= this.cfg.quiet_mult;
			if (this.introTicks > 0) {
				const progress = this._phaseProgress(this.introTotal, this.introTicks);
				level *= this.cfg.intro_density * (1 - progress) + progress;
			}
			if (this.endingTicks > 0) {
				const elapsed = this.endingTotal - this.endingTicks;
				if (elapsed < this.endingFade) {
					const fade = clamp01(elapsed / Math.max(1, this.endingFade - 1));
					level *= 1 - fade;
				} else {
					level = 0;
				}
			}
			return Math.max(0, level);
		}

		_paneRect() {
			const pad = Math.round(this.cfg.pane_pad * Math.min(this.w, this.h));
			const frame = Math.round(this.cfg.frame_thick);
			const inset = Math.max(0, pad + frame);
			let rowMin = inset, rowMax = this.h - 1 - inset;
			let colMin = inset, colMax = this.w - 1 - inset;
			if (rowMax <= rowMin) { rowMin = 0; rowMax = this.h - 1; }
			if (colMax <= colMin) { colMin = 0; colMax = this.w - 1; }
			return { rowMin, rowMax, colMin, colMax };
		}

		_spawn() {
			if (this.droplets.length >= this.cfg.drop_max) return;
			const level = this._activityLevel();
			if (level <= 0) return;
			const rate = this.cfg.spawn_p * level;
			if (rate <= 0) return;
			const { rowMin, rowMax, colMin, colMax } = this._paneRect();
			if (rowMax - rowMin < 4 || colMax - colMin < 4) return;
			const whole = Math.floor(rate);
			const frac = rate - whole;
			let expected = whole;
			if (this.rng() < frac) expected++;
			for (let i = 0; i < expected && this.droplets.length < this.cfg.drop_max; i++) {
				let col = colMin + this.rng() * (colMax - colMin);
				if (this.gustTicks > 0 && this.cfg.gust_str > 0) {
					const edge = this.gustSide > 0 ? colMax : colMin;
					const bias = 0.4 + 0.4 * this.rng();
					col = col * (1 - bias) + edge * bias;
				}
				const row = rowMin + this.rng() * (rowMax - rowMin);
				const hueJitter = (this.rng() * 2 - 1) * 8;
				this.droplets.push({
					row, col,
					radius: 0.4 + this.rng() * 0.4,
					vRow: 0, vCol: 0,
					hue: positiveMod(this.cfg.bg_hue + hueJitter, 360),
					pinRow: 0,
					falling: false,
				});
			}
		}

		_advanceDroplets() {
			if (!this.droplets.length) return;
			const { rowMax, colMin, colMax } = this._paneRect();
			let growBoost = 1;
			if (this.endingTicks > 0) {
				const elapsed = this.endingTotal - this.endingTicks;
				const fade = clamp01(elapsed / Math.max(1, this.endingFade - 1));
				growBoost = 1 - fade;
			}
			const alive = [];
			for (const d of this.droplets) {
				if (!d.falling) {
					d.radius += this.cfg.grow_rate * growBoost * (0.6 + 0.8 * this.rng());
					if (this.gustTicks > 0 && this.cfg.gust_str > 0) {
						d.col += this.gustSide * 0.04 * this.cfg.gust_str;
					}
					if (d.radius >= this.cfg.fall_thresh) {
						d.falling = true;
						d.pinRow = d.row;
						d.vRow = this.cfg.fall_speed * (0.85 + 0.4 * this.rng());
						d.vCol = this.gustWind * 0.6;
					}
				} else {
					d.vRow += this.cfg.gravity;
					d.row += d.vRow;
					d.col += d.vCol;
					d.vCol *= 0.94;
					d.radius -= this.cfg.grow_rate * 0.6;
					if ((Math.round(d.row) % 2) === 0 && this.rng() < 0.65) {
						const life = this.cfg.trail_life;
						const strength = clamp01(d.radius / Math.max(0.2, this.cfg.fall_thresh));
						this.tracks.push({ row: d.row, col: d.col, strength, life, maxLife: life });
					}
				}
				if (d.radius <= 0.2) continue;
				if (d.row > rowMax + 1) continue;
				if (d.col < colMin - 1 || d.col > colMax + 1) continue;
				alive.push(d);
			}
			this.droplets = alive;

			if (this.cfg.merge_r > 0) {
				const merged = [];
				const used = new Array(this.droplets.length).fill(false);
				for (let i = 0; i < this.droplets.length; i++) {
					if (used[i]) continue;
					const a = this.droplets[i];
					if (!a.falling) {
						for (let j = i + 1; j < this.droplets.length; j++) {
							if (used[j]) continue;
							const b = this.droplets[j];
							if (b.falling) continue;
							const dr = a.row - b.row;
							const dc = a.col - b.col;
							if (Math.hypot(dr, dc) > a.radius + b.radius + this.cfg.merge_r * 0.4) continue;
							if (this.rng() > this.cfg.merge_p) continue;
							const area = a.radius * a.radius + b.radius * b.radius;
							a.radius = Math.sqrt(area);
							a.row = (a.row * a.radius + b.row * b.radius) / (a.radius + b.radius);
							a.col = (a.col * a.radius + b.col * b.radius) / (a.radius + b.radius);
							a.hue = (a.hue + b.hue) * 0.5;
							used[j] = true;
						}
					}
					used[i] = true;
					merged.push(a);
				}
				this.droplets = merged;
			}
		}

		_advanceTracks() {
			if (!this.tracks.length) return;
			const alive = [];
			for (const t of this.tracks) {
				t.life--;
				if (t.life > 0) alive.push(t);
			}
			this.tracks = alive;
		}

		// Background-glow strength accounting for intro/ending so the pane
		// fades up at intro start and dims out at ending.
		_bgIntensity() {
			let intensity = 1;
			if (this.introTicks > 0) {
				const progress = this._phaseProgress(this.introTotal, this.introTicks);
				intensity = this.cfg.intro_bg + (1 - this.cfg.intro_bg) * progress;
			}
			if (this.endingTicks > 0) {
				const elapsed = this.endingTotal - this.endingTicks;
				if (elapsed < this.endingFade) {
					const fade = clamp01(elapsed / Math.max(1, this.endingFade - 1));
					intensity *= (1 - fade) + this.cfg.ending_residue * fade;
				} else {
					intensity *= this.cfg.ending_residue;
				}
			}
			return clamp01(intensity);
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				ctx.fillStyle = opts.bg || '#050608';
				ctx.fillRect(0, 0, canvasW, canvasH);
			}
			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const { rowMin, rowMax, colMin, colMax } = this._paneRect();
			const bgInt = this._bgIntensity();

			// Background — diffuse glow inside the pane, brighter toward the
			// center so it reads as light source behind the glass. Painted as
			// a radial-ish field by row distance from the visual center.
			const cx = (colMin + colMax) * 0.5;
			const cy = (rowMin + rowMax) * 0.5;
			const radiusMax = Math.hypot(colMax - cx, rowMax - cy) || 1;
			for (let y = rowMin; y <= rowMax; y++) {
				for (let x = colMin; x <= colMax; x++) {
					const r = Math.hypot(x - cx, y - cy) / radiusMax;
					const t = clamp01(1 - r * r);
					// Glow layer — central highlight in glow_hue.
					const glowAmt = clamp01(this.cfg.glow * t * bgInt);
					// Base ambient layer — fills the rest with bg_hue.
					const baseLight = this.cfg.bg_light * bgInt;
					const peakLight = (this.cfg.bg_light + (1 - this.cfg.bg_light) * 0.55) * bgInt;
					const baseColor = hslToRGB(positiveMod(this.cfg.bg_hue, 360),
						clamp01(this.cfg.bg_sat),
						clamp01(baseLight + (peakLight - baseLight) * t));
					const glowColor = hslToRGB(positiveMod(this.cfg.glow_hue, 360),
						clamp01(this.cfg.bg_sat * 0.85),
						clamp01(0.5 + 0.4 * t));
					const r8 = Math.round(baseColor.r * (1 - glowAmt) + glowColor.r * glowAmt);
					const g8 = Math.round(baseColor.g * (1 - glowAmt) + glowColor.g * glowAmt);
					const b8 = Math.round(baseColor.b * (1 - glowAmt) + glowColor.b * glowAmt);
					ctx.fillStyle = `rgb(${r8},${g8},${b8})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// Frame — silhouette around the pane.
			if (this.cfg.frame_thick > 0) {
				const frame = Math.round(this.cfg.frame_thick);
				const pad = Math.round(this.cfg.pane_pad * Math.min(this.w, this.h));
				const outer = Math.max(0, pad);
				const fc = hslToRGB(20, 0.18, clamp01(this.cfg.frame_light));
				ctx.fillStyle = `rgb(${fc.r},${fc.g},${fc.b})`;
				const top = outer;
				const bottom = this.h - outer - frame;
				const left = outer;
				const right = this.w - outer - frame;
				if (bottom >= top + frame && right >= left + frame) {
					ctx.fillRect(Math.floor(left * sx), Math.floor(top * sy),
						Math.ceil((this.w - 2 * outer) * sx), Math.ceil(frame * sy));
					ctx.fillRect(Math.floor(left * sx), Math.floor((bottom + 0) * sy),
						Math.ceil((this.w - 2 * outer) * sx), Math.ceil(frame * sy));
					ctx.fillRect(Math.floor(left * sx), Math.floor(top * sy),
						Math.ceil(frame * sx), Math.ceil((this.h - 2 * outer) * sy));
					ctx.fillRect(Math.floor((right + 0) * sx), Math.floor(top * sy),
						Math.ceil(frame * sx), Math.ceil((this.h - 2 * outer) * sy));
				}
			}

			// Falling-droplet trails first so droplets paint on top.
			for (const t of this.tracks) {
				const fade = clamp01(t.life / Math.max(1, t.maxLife));
				const intensity = t.strength * fade;
				if (intensity < 0.04) continue;
				const col = Math.round(t.col);
				const row = Math.round(t.row);
				if (col < colMin || col > colMax || row < rowMin || row > rowMax) continue;
				const trailLight = clamp01(this.cfg.drop_light * (0.35 + 0.55 * intensity));
				const trailC = hslToRGB(positiveMod(this.cfg.bg_hue + 6, 360),
					clamp01(this.cfg.drop_sat * 0.6),
					trailLight);
				ctx.fillStyle = `rgb(${trailC.r},${trailC.g},${trailC.b})`;
				ctx.fillRect(Math.floor(col * sx), Math.floor(row * sy), ceilSx, ceilSy);
			}

			// Droplets — body + upper-left specular highlight.
			for (const d of this.droplets) {
				if (d.radius <= 0.25) continue;
				const radius = d.radius;
				const cy = d.row;
				const cxd = d.col;
				const rInt = Math.max(1, Math.ceil(radius));
				const bodyLight = clamp01(this.cfg.drop_light);
				const hiLight = clamp01(this.cfg.hi_light);
				const body = hslToRGB(positiveMod(d.hue, 360),
					clamp01(this.cfg.drop_sat), bodyLight);
				const highlight = hslToRGB(positiveMod(d.hue, 360),
					clamp01(this.cfg.drop_sat * 0.5), hiLight);
				const rim = hslToRGB(positiveMod(d.hue, 360),
					clamp01(this.cfg.drop_sat),
					clamp01(this.cfg.drop_light * 0.5));
				for (let yy = -rInt; yy <= rInt; yy++) {
					for (let xx = -rInt; xx <= rInt; xx++) {
						const dist = Math.hypot(xx, yy);
						if (dist > radius + 0.4) continue;
						const px = Math.round(cxd + xx);
						const py = Math.round(cy + yy);
						if (px < colMin || px > colMax || py < rowMin || py > rowMax) continue;
						let color = body;
						const norm = dist / Math.max(0.4, radius);
						// Upper-left highlight position — matches the convention
						// for water droplets catching ambient light.
						const hiOff = Math.hypot(xx + radius * 0.45, yy + radius * 0.45);
						if (hiOff < radius * 0.5 && radius >= 0.8) {
							color = highlight;
						} else if (norm > 0.78) {
							color = rim;
						}
						ctx.fillStyle = `rgb(${color.r},${color.g},${color.b})`;
						ctx.fillRect(Math.floor(px * sx), Math.floor(py * sy), ceilSx, ceilSy);
					}
				}
			}
		}
	}

	api.presets['rain-on-window'] = [
		{
			key: 'quiet-city',
			label: 'quiet city',
			config: {
				spawn_p: 0.35,
				drop_max: 60,
				grow_rate: 0.01,
				fall_thresh: 1.6,
				fall_speed: 0.16,
				bg_hue: 215,
				bg_sat: 0.32,
				bg_light: 0.18,
				glow_hue: 205,
				glow: 0.45,
				frame_light: 0.06,
				drop_sat: 0.14,
				drop_light: 0.62,
				hi_light: 0.88,
				quiet_p: 0.0008,
				quiet_mult: 0.25,
			},
		},
		{
			key: 'evening-downpour',
			label: 'evening downpour',
			config: {
				spawn_p: 0.85,
				drop_max: 110,
				grow_rate: 0.018,
				fall_thresh: 1.5,
				fall_speed: 0.24,
				gravity: 0.018,
				trail_life: 80,
				bg_hue: 28,
				bg_sat: 0.4,
				bg_light: 0.22,
				glow_hue: 36,
				glow: 0.7,
				frame_light: 0.1,
				drop_sat: 0.2,
				drop_light: 0.74,
				hi_light: 0.94,
				gust_p: 0.0014,
				gust_dur: 50,
				gust_str: 0.8,
			},
		},
		{
			key: 'neon-street',
			label: 'neon street',
			config: {
				spawn_p: 0.55,
				drop_max: 80,
				grow_rate: 0.012,
				fall_thresh: 1.7,
				fall_speed: 0.2,
				bg_hue: 285,
				bg_sat: 0.55,
				bg_light: 0.22,
				glow_hue: 310,
				glow: 0.75,
				frame_light: 0.06,
				drop_sat: 0.28,
				drop_light: 0.72,
				hi_light: 0.96,
				gust_p: 0.0008,
				gust_str: 0.5,
			},
		},
		{
			key: 'gentle-drizzle',
			label: 'gentle drizzle',
			config: {
				spawn_p: 0.25,
				drop_max: 50,
				grow_rate: 0.008,
				fall_thresh: 1.8,
				fall_speed: 0.14,
				gravity: 0.008,
				trail_life: 50,
				bg_hue: 42,
				bg_sat: 0.38,
				bg_light: 0.26,
				glow_hue: 46,
				glow: 0.55,
				frame_light: 0.08,
				drop_sat: 0.16,
				drop_light: 0.7,
				hi_light: 0.92,
				quiet_p: 0.0006,
				quiet_mult: 0.3,
			},
		},
	];
	api.effects['rain-on-window'] = RainOnWindow;
})(window.AmbienceSim);
