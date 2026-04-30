'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	const WATERFALL_DEFAULTS = {
		intro_dur: 60,
		intro_trickle: 0.18,
		intro_mist: 0.25,
		ending_dur: 60,
		ending_linger: 24,
		ending_mist: 0.2,
		width: 7,
		wobble: 1.8,
		speed: 1.0,
		pool_y: 0.72,
		pool_span: 0.34,
		mist_spawn: 2,
		mist_max: 48,
		ripple_every: 8,
		ripple_max: 10,
		hue: 204,
		hue_sp: 12,
		sat: 0.48,
		lmin: 0.45,
		lmax: 0.82,
		surge_p: 0,
		calm_p: 0,
		mist_burst_p: 0,
		surge_dur: 55,
		surge_mult: 1.6,
		calm_dur: 70,
		calm_mult: 0.55,
		mist_burst_dur: 40,
		mist_burst_mult: 2.5,
	};

	function applyWaterfallDefaults(cfg) {
		const c = Object.assign({}, WATERFALL_DEFAULTS, cfg || {});
		if (c.intro_dur === 0 && c.intro_trickle === 0 && c.intro_mist === 0) {
			c.intro_dur = WATERFALL_DEFAULTS.intro_dur;
			c.intro_trickle = WATERFALL_DEFAULTS.intro_trickle;
			c.intro_mist = WATERFALL_DEFAULTS.intro_mist;
		} else {
			if (c.intro_dur <= 0) c.intro_dur = WATERFALL_DEFAULTS.intro_dur;
			if (c.intro_trickle <= 0) c.intro_trickle = WATERFALL_DEFAULTS.intro_trickle;
			if (c.intro_mist < 0) c.intro_mist = 0;
		}
		c.intro_trickle = clamp01(c.intro_trickle);
		c.intro_mist = clamp01(c.intro_mist);
		if (c.ending_dur === 0 && c.ending_linger === 0 && c.ending_mist === 0) {
			c.ending_dur = WATERFALL_DEFAULTS.ending_dur;
			c.ending_linger = WATERFALL_DEFAULTS.ending_linger;
			c.ending_mist = WATERFALL_DEFAULTS.ending_mist;
		} else {
			if (c.ending_dur <= 0) c.ending_dur = WATERFALL_DEFAULTS.ending_dur;
			if (c.ending_linger < 0) c.ending_linger = 0;
			if (c.ending_mist < 0) c.ending_mist = 0;
		}
		c.ending_mist = clamp01(c.ending_mist);
		if (c.width <= 0) c.width = WATERFALL_DEFAULTS.width;
		if (c.wobble < 0) c.wobble = 0;
		if (c.wobble === 0) c.wobble = WATERFALL_DEFAULTS.wobble;
		if (c.speed <= 0) c.speed = WATERFALL_DEFAULTS.speed;
		if (c.pool_y <= 0) c.pool_y = WATERFALL_DEFAULTS.pool_y;
		c.pool_y = clamp01(c.pool_y);
		if (c.pool_span <= 0) c.pool_span = WATERFALL_DEFAULTS.pool_span;
		c.pool_span = clamp01(c.pool_span);
		if (c.mist_spawn <= 0) c.mist_spawn = WATERFALL_DEFAULTS.mist_spawn;
		if (c.mist_max <= 0) c.mist_max = WATERFALL_DEFAULTS.mist_max;
		if (c.ripple_every <= 0) c.ripple_every = WATERFALL_DEFAULTS.ripple_every;
		if (c.ripple_max <= 0) c.ripple_max = WATERFALL_DEFAULTS.ripple_max;
		if (c.hue_sp <= 0) c.hue_sp = WATERFALL_DEFAULTS.hue_sp;
		if (c.sat <= 0) c.sat = WATERFALL_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = WATERFALL_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = WATERFALL_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.surge_dur <= 0) c.surge_dur = WATERFALL_DEFAULTS.surge_dur;
		if (c.surge_mult <= 0) c.surge_mult = WATERFALL_DEFAULTS.surge_mult;
		if (c.calm_dur <= 0) c.calm_dur = WATERFALL_DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = WATERFALL_DEFAULTS.calm_mult;
		if (c.mist_burst_dur <= 0) c.mist_burst_dur = WATERFALL_DEFAULTS.mist_burst_dur;
		if (c.mist_burst_mult <= 0) c.mist_burst_mult = WATERFALL_DEFAULTS.mist_burst_mult;
		return c;
	}

	class Waterfall {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.cfg = applyWaterfallDefaults(cfg);
			this.rng = makeRNG(seed || Date.now());
			this.tick = 0;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.mists = [];
			this.ripples = [];
			this.surgeTicks = 0;
			this.calmTicks = 0;
			this.mistBurstTicks = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.rippleCooldown = 0;
		}

		setConfig(cfg) {
			const prev = this.cfg;
			const next = applyWaterfallDefaults(Object.assign({}, this.cfg, cfg));
			if (prev && prev.speed > 0 && next.speed !== prev.speed) {
				const ratio = next.speed / prev.speed;
				for (const mist of this.mists) {
					mist.vRow *= ratio;
					mist.vCol *= ratio;
				}
				for (const ripple of this.ripples) {
					ripple.speed *= 0.7 + 0.3 * ratio;
				}
			}
			this.cfg = next;
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.surgeTicks = state.surgeTicks || 0;
			this.calmTicks = state.calmTicks || 0;
			this.mistBurstTicks = state.mistBurstTicks || 0;
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			this.rippleCooldown = state.rippleCooldown || 0;
			if (typeof snap.seed === 'number') this.rng = makeRNG(snap.seed);
			if (snap.gridW > 0 && snap.gridH > 0 &&
				(snap.gridW !== this.w || snap.gridH !== this.h)) {
				this.w = snap.gridW;
				this.h = snap.gridH;
				this.grid = new Uint8ClampedArray(this.w * this.h * 3);
			}
			this.mists = Array.isArray(state.mists) ? state.mists.map(m => ({
				row: m.row,
				col: m.col,
				vRow: m.vRow,
				vCol: m.vCol,
				life: m.life,
				maxLife: m.maxLife,
				color: m.color,
			})) : [];
			this.ripples = Array.isArray(state.ripples) ? state.ripples.map(r => ({
				col: r.col,
				radius: r.radius,
				speed: r.speed,
				life: r.life,
				maxLife: r.maxLife,
				strength: r.strength,
			})) : [];
		}

		triggerEvent(name) {
			switch (name) {
				case 'surge':
					this.surgeTicks = jitterInt(this.rng, this.cfg.surge_dur, 0.3);
					this._spawnRipple(this._flowLevel());
					return true;
				case 'calm':
					this.calmTicks = jitterInt(this.rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'mist-burst':
					this.mistBurstTicks = jitterInt(this.rng, this.cfg.mist_burst_dur, 0.3);
					this._spawnRipple(this._flowLevel());
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
			if (this.surgeTicks > 0) this.surgeTicks--;
			if (this.calmTicks > 0) this.calmTicks--;
			if (this.mistBurstTicks > 0) this.mistBurstTicks--;
			if (this.introTicks > 0) this.introTicks--;
			if (this.endingTicks > 0) this.endingTicks--;
			if (this.rippleCooldown > 0) this.rippleCooldown--;

			if (this.surgeTicks === 0 && this.rng() < this.cfg.surge_p) {
				this.surgeTicks = jitterInt(this.rng, this.cfg.surge_dur, 0.3);
				this._spawnRipple(this._flowLevel());
			}
			if (this.calmTicks === 0 && this.rng() < this.cfg.calm_p) {
				this.calmTicks = jitterInt(this.rng, this.cfg.calm_dur, 0.3);
			}
			if (this.mistBurstTicks === 0 && this.rng() < this.cfg.mist_burst_p) {
				this.mistBurstTicks = jitterInt(this.rng, this.cfg.mist_burst_dur, 0.3);
				this._spawnRipple(this._flowLevel());
			}

			this._stepMists();
			this._stepRipples();
			this._stepRippleSpawner();
			this._stepMistSpawner();
			this.grid.fill(0);
			this._paintPool();
			this._paintSheet();
			this._paintImpact();
			this._paintRipples();
			this._paintMists();
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				ctx.fillStyle = opts.bg || '#0a0a0a';
				ctx.fillRect(0, 0, canvasW, canvasH);
			}
			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx), ceilSy = Math.ceil(sy);
			for (let y = 0; y < this.h; y++) {
				for (let x = 0; x < this.w; x++) {
					const i = (y * this.w + x) * 3;
					const r = this.grid[i], g = this.grid[i + 1], b = this.grid[i + 2];
					if (r === 0 && g === 0 && b === 0) continue;
					ctx.fillStyle = `rgb(${r},${g},${b})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}
		}

		_startIntro() {
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.introTotal = this.cfg.intro_dur > 0 ? this.cfg.intro_dur : WATERFALL_DEFAULTS.intro_dur;
			this.introTicks = this.introTotal;
			this.rippleCooldown = 1;
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = this.cfg.ending_dur > 0 ? this.cfg.ending_dur : WATERFALL_DEFAULTS.ending_dur;
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

		_flowLevel() {
			let flow = 1.0;
			if (this.surgeTicks > 0) flow *= this.cfg.surge_mult;
			if (this.calmTicks > 0) flow *= this.cfg.calm_mult;
			if (this.introTicks > 0) {
				const progress = this._phaseProgress(this.introTotal, this.introTicks);
				flow *= this.cfg.intro_trickle + (1 - this.cfg.intro_trickle) * progress;
			}
			if (this.endingTicks > 0) {
				const elapsed = this.endingTotal - this.endingTicks;
				if (elapsed < this.endingFade) {
					const fade = clamp01(elapsed / Math.max(1, this.endingFade - 1));
					flow *= 1 - 0.88 * fade;
				} else {
					flow *= 0.12;
				}
			}
			return Math.max(0.05, flow);
		}

		_mistLevel() {
			let level = 1.0;
			if (this.surgeTicks > 0) level *= 1.25;
			if (this.calmTicks > 0) level *= 0.65;
			if (this.mistBurstTicks > 0) level *= this.cfg.mist_burst_mult;
			if (this.introTicks > 0) {
				const progress = this._phaseProgress(this.introTotal, this.introTicks);
				level *= this.cfg.intro_mist + (1 - this.cfg.intro_mist) * progress;
			}
			if (this.endingTicks > 0) {
				const progress = this._phaseProgress(this.endingTotal, this.endingTicks);
				level *= 1 - (1 - this.cfg.ending_mist) * progress;
			}
			return Math.max(0.05, level);
		}

		_poolRow() {
			let row = Math.round(this.cfg.pool_y * (this.h - 1));
			if (row < 6) row = 6;
			if (row > this.h - 4) row = this.h - 4;
			return row;
		}

		_poolBounds() {
			const center = Math.round(this.w * 0.5);
			let half = Math.round(this.cfg.pool_span * this.w * 0.5);
			if (half < 4) half = 4;
			let left = center - half;
			let right = center + half;
			if (left < 0) left = 0;
			if (right >= this.w) right = this.w - 1;
			return [left, right];
		}

		_stepMists() {
			if (!this.mists.length) return;
			const speedScale = 0.75 + 0.4 * this.cfg.speed;
			const alive = [];
			for (const mist of this.mists) {
				mist.vCol += (this.rng() * 2 - 1) * 0.015;
				mist.vCol = Math.max(-0.4, Math.min(0.4, mist.vCol));
				mist.row += mist.vRow * speedScale;
				mist.col += mist.vCol;
				mist.vRow *= 0.99;
				mist.life--;
				if (mist.life > 0 && mist.row >= -2 && mist.row < this.h && mist.col >= -2 && mist.col < this.w + 2) {
					alive.push(mist);
				}
			}
			this.mists = alive;
		}

		_stepRipples() {
			if (!this.ripples.length) return;
			const alive = [];
			for (const ripple of this.ripples) {
				ripple.radius += ripple.speed;
				ripple.life--;
				if (ripple.life > 0 && ripple.radius < this.w) alive.push(ripple);
			}
			this.ripples = alive;
		}

		_stepRippleSpawner() {
			if (this.ripples.length >= this.cfg.ripple_max || this.rippleCooldown > 0) return;
			const flow = this._flowLevel();
			let cadence = this.cfg.ripple_every;
			if (flow > 0) cadence /= Math.max(0.25, flow);
			if (this.endingTicks > 0 && this.endingTotal - this.endingTicks >= this.endingFade) cadence *= 2;
			cadence = Math.max(1, cadence);
			this._spawnRipple(flow);
			this.rippleCooldown = jitterInt(this.rng, Math.round(cadence), 0.25);
		}

		_stepMistSpawner() {
			if (this.mists.length >= this.cfg.mist_max) return;
			const level = this._mistLevel();
			let spawnEvery = Math.round(this.cfg.mist_spawn / Math.max(0.2, level));
			if (spawnEvery < 1) spawnEvery = 1;
			let attempts = 1;
			if (level > 1) {
				attempts += Math.floor(level);
				if (this.rng() < (level - Math.floor(level))) attempts++;
			}
			if (this.endingTicks > 0 && this.endingTotal - this.endingTicks >= this.endingFade) {
				spawnEvery *= 3;
				attempts = 1;
			}
			for (let i = 0; i < attempts && this.mists.length < this.cfg.mist_max; i++) {
				if (this.rng.intn(spawnEvery) === 0) this._spawnMist(level);
			}
		}

		_spawnRipple(flow) {
			if (this.ripples.length >= this.cfg.ripple_max) return;
			const center = this.w * 0.5;
			const col = center + (this.rng() * 2 - 1) * this.cfg.width * Math.max(0.35, flow) * 0.35;
			const life = jitterInt(this.rng, 18, 0.25);
			const speed = (0.5 + this.rng() * 0.55) * (0.8 + 0.25 * Math.max(0.5, flow));
			const strength = clamp01(0.45 + this.rng() * 0.35 + (flow - 1) * 0.12);
			this.ripples.push({
				col,
				radius: 0,
				speed,
				life,
				maxLife: life,
				strength,
			});
		}

		_spawnMist(level) {
			if (this.mists.length >= this.cfg.mist_max) return;
			const center = this.w * 0.5;
			const flow = this._flowLevel();
			const surface = this._poolRow();
			const col = center + (this.rng() * 2 - 1) * this.cfg.width * Math.max(0.35, flow) * 0.6;
			const row = surface - 1 - this.rng() * 2;
			const vRow = -(0.12 + this.rng() * 0.22) * (0.8 + 0.35 * this.cfg.speed);
			const vCol = (this.rng() * 2 - 1) * (0.08 + 0.1 * Math.max(0.5, level) + this.cfg.wobble * 0.02);
			const life = jitterInt(this.rng, 22, 0.35);
			const hue = ((this.cfg.hue + (this.rng() * 2 - 1) * this.cfg.hue_sp * 0.35) % 360 + 360) % 360;
			const light = clamp01(this.cfg.lmax * (0.88 + this.rng() * 0.12));
			const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.45), light);
			this.mists.push({
				row,
				col,
				vRow,
				vCol,
				life,
				maxLife: life,
				color,
			});
		}

		_paintPool() {
			const surface = this._poolRow();
			const [left, right] = this._poolBounds();
			let depth = this.h - surface;
			if (depth > 10) depth = 10;
			if (depth < 3) depth = 3;
			const center = this.w * 0.5;
			const half = Math.max(1, (right - left) / 2);
			for (let y = surface; y < this.h && y < surface + depth; y++) {
				const rowDepth = 1 - (y - surface) / depth;
				for (let x = left; x <= right; x++) {
					const edge = 1 - Math.abs(x - center) / half;
					if (edge <= 0) continue;
					const shimmer = 0.72 + 0.28 * Math.sin(x * 0.13 + y * 0.27 + this.tick * 0.07 * this.cfg.speed);
					const light = clamp01(this.cfg.lmin * 0.22 + (this.cfg.lmax - this.cfg.lmin) * 0.28 * edge * rowDepth * shimmer);
					const color = hslToRGB(((this.cfg.hue - 8) % 360 + 360) % 360, clamp01(this.cfg.sat * 0.9), light);
					this._paintMax(y, x, color);
				}
			}
		}

		_paintSheet() {
			const surface = this._poolRow();
			if (surface <= 0) return;
			const center = this.w * 0.5;
			const flow = this._flowLevel();
			const width = Math.max(1, this.cfg.width * flow);
			for (let y = 0; y < surface; y++) {
				const progress = y / Math.max(1, surface - 1);
				// Let the sheet bend drift downward so it reads as falling water.
				const rowCenter = center + Math.sin(progress * 5.1 - this.tick * 0.05 * this.cfg.speed) * this.cfg.wobble * 0.55;
				const rowWidth = width * (0.86 + 0.32 * progress);
				const half = Math.max(0.6, rowWidth * 0.5);
				let start = Math.floor(rowCenter - half - 1);
				let end = Math.ceil(rowCenter + half + 1);
				if (start < 0) start = 0;
				if (end >= this.w) end = this.w - 1;
				for (let x = start; x <= end; x++) {
					const dist = Math.abs((x + 0.5) - rowCenter) / half;
					if (dist > 1.1) continue;
					const edge = clamp01(1 - dist * dist);
					const pulse = 0.72 + 0.28 * Math.sin(progress * 11 - this.tick * 0.22 * this.cfg.speed + x * 0.35);
					const intensity = edge * pulse;
					if (intensity < 0.08) continue;
					const hue = ((this.cfg.hue + Math.sin(progress * 3 + x * 0.1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + 0.7 * intensity));
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._paintMax(y, x, color);
				}
			}
		}

		_paintImpact() {
			const surface = this._poolRow();
			const center = Math.round(this.w * 0.5);
			const flow = this._flowLevel();
			const level = this._mistLevel();
			const radius = Math.round(Math.max(2, this.cfg.width * flow * 0.6));
			for (let dx = -radius; dx <= radius; dx++) {
				const x = center + dx;
				const dist = Math.abs(dx) / (radius + 1);
				if (dist > 1) continue;
				const foam = clamp01((1 - dist * dist) * (0.65 + 0.2 * Math.max(0.5, level)));
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.55 + 0.45 * foam));
				const color = hslToRGB(((this.cfg.hue - 16) % 360 + 360) % 360, clamp01(this.cfg.sat * 0.25), light);
				this._paintMax(surface, x, color);
				this._paintMax(surface - 1, x, color);
				if (surface + 1 < this.h && dx % 2 === 0) {
					this._paintMax(surface + 1, x, {
						r: Math.floor(color.r * 0.8),
						g: Math.floor(color.g * 0.8),
						b: Math.floor(color.b * 0.8),
					});
				}
			}
		}

		_paintRipples() {
			if (!this.ripples.length) return;
			const surface = this._poolRow();
			const [left, right] = this._poolBounds();
			for (const ripple of this.ripples) {
				const fade = clamp01(ripple.life / Math.max(1, ripple.maxLife));
				if (fade <= 0) continue;
				for (let x = left; x <= right; x++) {
					const wave = Math.abs(Math.abs(x - ripple.col) - ripple.radius);
					if (wave > 0.8) continue;
					const bright = ripple.strength * fade * (1 - wave / 0.8);
					const light = clamp01(this.cfg.lmin * 0.85 + (this.cfg.lmax - this.cfg.lmin) * (0.25 + 0.55 * bright));
					const color = hslToRGB(((this.cfg.hue - 10) % 360 + 360) % 360, clamp01(this.cfg.sat * 0.7), light);
					this._paintMax(surface, x, color);
					if (surface + 1 < this.h && bright > 0.45) {
						this._paintMax(surface + 1, x, {
							r: Math.floor(color.r * 0.75),
							g: Math.floor(color.g * 0.75),
							b: Math.floor(color.b * 0.75),
						});
					}
				}
			}
		}

		_paintMists() {
			for (const mist of this.mists) {
				const fade = clamp01(mist.life / Math.max(1, mist.maxLife));
				if (fade <= 0) continue;
				const row = Math.round(mist.row);
				const col = Math.round(mist.col);
				const scale = 0.25 + 0.75 * fade;
				const color = {
					r: Math.floor(mist.color.r * scale),
					g: Math.floor(mist.color.g * scale),
					b: Math.floor(mist.color.b * scale),
				};
				this._paintMax(row, col, color);
				if (fade > 0.7) {
					const side = mist.vCol >= 0 ? col + 1 : col - 1;
					this._paintMax(row, side, {
						r: Math.floor(color.r * 0.65),
						g: Math.floor(color.g * 0.65),
						b: Math.floor(color.b * 0.65),
					});
				}
			}
		}

		_paintMax(row, col, color) {
			if (row < 0 || row >= this.h || col < 0 || col >= this.w) return;
			if (color.r === 0 && color.g === 0 && color.b === 0) return;
			const i = (row * this.w + col) * 3;
			if (color.r > this.grid[i]) this.grid[i] = color.r;
			if (color.g > this.grid[i + 1]) this.grid[i + 1] = color.g;
			if (color.b > this.grid[i + 2]) this.grid[i + 2] = color.b;
		}
	}

	api.presets['waterfall'] = [
		{
			key: 'thin-falls',
			label: 'thin falls',
			config: {
				intro_trickle: 0.1,
				intro_mist: 0.12,
				ending_linger: 18,
				ending_mist: 0.08,
				width: 4.5,
				wobble: 1.2,
				speed: 0.85,
				pool_span: 0.26,
				mist_spawn: 3,
				mist_max: 24,
				ripple_every: 11,
				ripple_max: 6,
				hue: 200,
				hue_sp: 10,
				sat: 0.42,
				lmin: 0.4,
				lmax: 0.78,
				calm_p: 0.001,
				calm_mult: 0.35,
				mist_burst_mult: 1.8,
			},
		},
		{
			key: 'steady-cascade',
			label: 'steady cascade',
			config: {
				width: 7.5,
				wobble: 1.6,
				speed: 1,
				pool_span: 0.36,
				mist_spawn: 2,
				mist_max: 48,
				ripple_every: 8,
				ripple_max: 10,
				hue: 204,
				hue_sp: 12,
				sat: 0.48,
				lmin: 0.45,
				lmax: 0.82,
				surge_p: 0.0008,
				calm_p: 0.0004,
				mist_burst_p: 0.0006,
			},
		},
		{
			key: 'misty-drop',
			label: 'misty drop',
			config: {
				intro_mist: 0.45,
				ending_mist: 0.45,
				width: 6.5,
				wobble: 1.4,
				speed: 0.95,
				pool_span: 0.38,
				mist_spawn: 1,
				mist_max: 72,
				ripple_every: 12,
				ripple_max: 8,
				hue: 196,
				hue_sp: 16,
				sat: 0.36,
				lmin: 0.42,
				lmax: 0.88,
				mist_burst_p: 0.0012,
				mist_burst_mult: 3.4,
			},
		},
		{
			key: 'heavy-plunge',
			label: 'heavy plunge',
			config: {
				intro_trickle: 0.22,
				intro_mist: 0.3,
				ending_dur: 75,
				ending_linger: 32,
				width: 10.5,
				wobble: 2.4,
				speed: 1.25,
				pool_span: 0.42,
				mist_spawn: 1,
				mist_max: 60,
				ripple_every: 6,
				ripple_max: 14,
				hue: 208,
				hue_sp: 14,
				sat: 0.52,
				lmin: 0.47,
				lmax: 0.86,
				surge_p: 0.0015,
				surge_mult: 2.1,
				calm_mult: 0.65,
				mist_burst_p: 0.001,
			},
		},
	];
	api.effects['waterfall'] = Waterfall;
})(window.AmbienceSim);
