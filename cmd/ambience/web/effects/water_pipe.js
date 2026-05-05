'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	const WATER_PIPE_DEFAULTS = {
		intro_dur: 70,
		intro_drip: 0.12,
		intro_fill: 0.05,
		ending_dur: 70,
		ending_linger: 30,
		ending_residue: 0.18,
		pipe_x: 0.32,
		pipe_y: 0.18,
		pipe_width: 6,
		stream_width: 3,
		basin_y: 0.66,
		basin_span: 0.34,
		basin_depth: 8,
		wall_thick: 1,
		inflow: 1.0,
		drain: 0,
		overflow_speed: 0.55,
		overflow_fade: 0.045,
		splatter_p: 0.42,
		droplet_max: 36,
		ripple_every: 7,
		ripple_max: 8,
		hue: 198,
		hue_sp: 14,
		sat: 0.55,
		lmin: 0.42,
		lmax: 0.84,
		pipe_hue: 28,
		pipe_light: 0.34,
		surge_p: 0,
		dry_p: 0,
		surge_dur: 60,
		surge_mult: 1.8,
		dry_dur: 70,
		dry_mult: 0.25,
	};

	function applyWaterPipeDefaults(cfg) {
		const c = Object.assign({}, WATER_PIPE_DEFAULTS, cfg || {});
		if (c.intro_dur === 0 && c.intro_drip === 0 && c.intro_fill === 0) {
			c.intro_dur = WATER_PIPE_DEFAULTS.intro_dur;
			c.intro_drip = WATER_PIPE_DEFAULTS.intro_drip;
			c.intro_fill = WATER_PIPE_DEFAULTS.intro_fill;
		} else {
			if (c.intro_dur <= 0) c.intro_dur = WATER_PIPE_DEFAULTS.intro_dur;
			if (c.intro_drip <= 0) c.intro_drip = WATER_PIPE_DEFAULTS.intro_drip;
			if (c.intro_fill < 0) c.intro_fill = 0;
		}
		c.intro_drip = clamp01(c.intro_drip);
		c.intro_fill = clamp01(c.intro_fill);
		if (c.ending_dur === 0 && c.ending_linger === 0 && c.ending_residue === 0) {
			c.ending_dur = WATER_PIPE_DEFAULTS.ending_dur;
			c.ending_linger = WATER_PIPE_DEFAULTS.ending_linger;
			c.ending_residue = WATER_PIPE_DEFAULTS.ending_residue;
		} else {
			if (c.ending_dur <= 0) c.ending_dur = WATER_PIPE_DEFAULTS.ending_dur;
			if (c.ending_linger < 0) c.ending_linger = 0;
			if (c.ending_residue < 0) c.ending_residue = 0;
		}
		c.ending_residue = clamp01(c.ending_residue);
		if (c.pipe_x <= 0) c.pipe_x = WATER_PIPE_DEFAULTS.pipe_x;
		c.pipe_x = clamp01(c.pipe_x);
		if (c.pipe_y <= 0) c.pipe_y = WATER_PIPE_DEFAULTS.pipe_y;
		c.pipe_y = clamp01(c.pipe_y);
		if (c.pipe_width <= 0) c.pipe_width = WATER_PIPE_DEFAULTS.pipe_width;
		if (c.stream_width <= 0) c.stream_width = WATER_PIPE_DEFAULTS.stream_width;
		if (c.basin_y <= 0) c.basin_y = WATER_PIPE_DEFAULTS.basin_y;
		c.basin_y = clamp01(c.basin_y);
		if (c.basin_span <= 0) c.basin_span = WATER_PIPE_DEFAULTS.basin_span;
		c.basin_span = clamp01(c.basin_span);
		if (c.basin_depth <= 0) c.basin_depth = WATER_PIPE_DEFAULTS.basin_depth;
		if (c.wall_thick <= 0) c.wall_thick = WATER_PIPE_DEFAULTS.wall_thick;
		if (c.inflow <= 0) c.inflow = WATER_PIPE_DEFAULTS.inflow;
		if (c.drain < 0) c.drain = 0;
		if (c.overflow_speed <= 0) c.overflow_speed = WATER_PIPE_DEFAULTS.overflow_speed;
		if (c.overflow_fade <= 0) c.overflow_fade = WATER_PIPE_DEFAULTS.overflow_fade;
		if (c.splatter_p < 0) c.splatter_p = 0;
		if (c.droplet_max <= 0) c.droplet_max = WATER_PIPE_DEFAULTS.droplet_max;
		if (c.ripple_every <= 0) c.ripple_every = WATER_PIPE_DEFAULTS.ripple_every;
		if (c.ripple_max <= 0) c.ripple_max = WATER_PIPE_DEFAULTS.ripple_max;
		if (c.hue_sp <= 0) c.hue_sp = WATER_PIPE_DEFAULTS.hue_sp;
		if (c.sat <= 0) c.sat = WATER_PIPE_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = WATER_PIPE_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = WATER_PIPE_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.pipe_light <= 0) c.pipe_light = WATER_PIPE_DEFAULTS.pipe_light;
		c.pipe_light = clamp01(c.pipe_light);
		if (c.surge_dur <= 0) c.surge_dur = WATER_PIPE_DEFAULTS.surge_dur;
		if (c.surge_mult <= 0) c.surge_mult = WATER_PIPE_DEFAULTS.surge_mult;
		if (c.dry_dur <= 0) c.dry_dur = WATER_PIPE_DEFAULTS.dry_dur;
		if (c.dry_mult < 0) c.dry_mult = 0;
		return c;
	}

	class WaterPipe {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.cfg = applyWaterPipeDefaults(cfg);
			this.rng = makeRNG(seed || Date.now());
			this.tick = 0;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.droplets = [];
			this.ripples = [];
			this.runoff = [];
			this.surgeTicks = 0;
			this.dryTicks = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.rippleCooldown = 0;
			this.fill = 0;
		}

		setConfig(cfg) {
			this.cfg = applyWaterPipeDefaults(Object.assign({}, this.cfg, cfg));
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.surgeTicks = state.surgeTicks || 0;
			this.dryTicks = state.dryTicks || 0;
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			this.rippleCooldown = state.rippleCooldown || 0;
			this.fill = state.fill || 0;
			if (typeof snap.seed === 'number') this.rng = makeRNG(snap.seed);
			if (snap.gridW > 0 && snap.gridH > 0 &&
				(snap.gridW !== this.w || snap.gridH !== this.h)) {
				this.w = snap.gridW;
				this.h = snap.gridH;
				this.grid = new Uint8ClampedArray(this.w * this.h * 3);
			}
			this.droplets = Array.isArray(state.droplets) ? state.droplets.map(d => ({
				row: d.row, col: d.col, vRow: d.vRow, vCol: d.vCol,
				life: d.life, maxLife: d.maxLife, color: d.color,
			})) : [];
			this.ripples = Array.isArray(state.ripples) ? state.ripples.map(r => ({
				col: r.col, radius: r.radius, speed: r.speed,
				life: r.life, maxLife: r.maxLife, strength: r.strength,
			})) : [];
			this.runoff = Array.isArray(state.runoff) ? state.runoff.map(r => ({
				col: r.col, vel: r.vel, life: r.life, maxLife: r.maxLife,
				strength: r.strength, side: r.side,
			})) : [];
		}

		triggerEvent(name) {
			switch (name) {
				case 'surge':
					this.surgeTicks = jitterInt(this.rng, this.cfg.surge_dur, 0.3);
					return true;
				case 'dry-up':
					this.dryTicks = jitterInt(this.rng, this.cfg.dry_dur, 0.3);
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
			if (this.dryTicks > 0) this.dryTicks--;
			if (this.introTicks > 0) this.introTicks--;
			if (this.endingTicks > 0) this.endingTicks--;
			if (this.rippleCooldown > 0) this.rippleCooldown--;

			if (this.surgeTicks === 0 && this.rng() < this.cfg.surge_p) {
				this.surgeTicks = jitterInt(this.rng, this.cfg.surge_dur, 0.3);
			}
			if (this.dryTicks === 0 && this.rng() < this.cfg.dry_p) {
				this.dryTicks = jitterInt(this.rng, this.cfg.dry_dur, 0.3);
			}

			this._updateFill();
			this._stepDroplets();
			this._stepRipples();
			this._stepRunoff();
			this._spawnRipple();
			this._spawnRunoff();
			this._spawnSplatter();

			this.grid.fill(0);
			this._paintBasin();
			this._paintPool();
			this._paintStream();
			this._paintImpact();
			this._paintRipples();
			this._paintRunoff();
			this._paintPipe();
			this._paintDroplets();
		}

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}

		_startIntro() {
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.introTotal = this.cfg.intro_dur > 0 ? this.cfg.intro_dur : WATER_PIPE_DEFAULTS.intro_dur;
			this.introTicks = this.introTotal;
			this.fill = clamp01(this.cfg.intro_fill);
			this.rippleCooldown = 1;
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = this.cfg.ending_dur > 0 ? this.cfg.ending_dur : WATER_PIPE_DEFAULTS.ending_dur;
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
			if (this.dryTicks > 0) flow *= this.cfg.dry_mult;
			if (this.introTicks > 0) {
				const progress = this._phaseProgress(this.introTotal, this.introTicks);
				flow *= this.cfg.intro_drip + (1 - this.cfg.intro_drip) * progress;
			}
			if (this.endingTicks > 0) {
				const elapsed = this.endingTotal - this.endingTicks;
				if (elapsed < this.endingFade) {
					const fade = clamp01(elapsed / Math.max(1, this.endingFade - 1));
					flow *= 1 - 0.94 * fade;
				} else {
					flow *= 0.06;
				}
			}
			if (flow < 0) flow = 0;
			return flow;
		}

		_updateFill() {
			const flow = this._flowLevel();
			this.fill += this.cfg.inflow * flow * 0.012;
			this.fill -= this.cfg.drain * 0.012;
			if (this.endingTicks > 0) {
				const progress = this._phaseProgress(this.endingTotal, this.endingTicks);
				const target = this.cfg.ending_residue;
				this.fill = this.fill * (1 - 0.06 * progress) + target * 0.06 * progress;
			}
			if (this.fill < 0) this.fill = 0;
			if (this.fill > 1.6) this.fill = 1.6;
		}

		_pipeGeometry() {
			const width = Math.max(3, this.cfg.pipe_width);
			let half = Math.round(width * 0.5);
			if (half < 2) half = 2;
			const center = Math.round(this.cfg.pipe_x * (this.w - 1));
			let left = center - half;
			let right = center + half;
			if (left < 1) left = 1;
			if (right >= this.w - 1) right = this.w - 2;
			if (right < left) right = left;
			let lip = Math.round(this.cfg.pipe_y * (this.h - 1));
			if (lip < 2) lip = 2;
			if (lip > this.h - 6) lip = this.h - 6;
			return { lip, left, right, center };
		}

		_basinGeometry() {
			let brim = Math.round(this.cfg.basin_y * (this.h - 1));
			if (brim < 6) brim = 6;
			if (brim > this.h - 3) brim = this.h - 3;
			let depth = Math.round(this.cfg.basin_depth);
			if (depth < 3) depth = 3;
			if (depth > this.h - brim - 1) depth = this.h - brim - 1;
			if (depth < 2) depth = 2;
			const bottom = brim + depth;
			let half = Math.round(this.cfg.basin_span * this.w * 0.5);
			if (half < 4) half = 4;
			const pipe = this._pipeGeometry();
			const center = pipe.center > 0 ? pipe.center : Math.round(this.w / 2);
			let left = center - half;
			let right = center + half;
			if (left < 1) left = 1;
			if (right >= this.w - 1) right = this.w - 2;
			return { brim, bottom, left, right };
		}

		_wallThick() {
			let w = Math.round(this.cfg.wall_thick);
			if (w < 1) w = 1;
			if (w > 4) w = 4;
			return w;
		}

		_stepDroplets() {
			if (!this.droplets.length) return;
			const alive = [];
			const gravity = 0.085;
			for (const d of this.droplets) {
				d.vRow += gravity;
				d.row += d.vRow;
				d.col += d.vCol;
				d.life--;
				if (d.life > 0 && d.row < this.h + 1 && d.col >= -2 && d.col < this.w + 2) {
					alive.push(d);
				}
			}
			this.droplets = alive;
		}

		_stepRipples() {
			if (!this.ripples.length) return;
			const alive = [];
			for (const r of this.ripples) {
				r.radius += r.speed;
				r.life--;
				if (r.life > 0 && r.radius < this.w) alive.push(r);
			}
			this.ripples = alive;
		}

		_stepRunoff() {
			if (!this.runoff.length) return;
			const alive = [];
			for (const r of this.runoff) {
				r.col += r.vel * r.side;
				r.strength *= 1 - this.cfg.overflow_fade;
				r.life--;
				if (r.life > 0 && r.strength > 0.04 && r.col >= -1 && r.col <= this.w + 1) {
					alive.push(r);
				}
			}
			this.runoff = alive;
		}

		_spawnRipple() {
			if (this.ripples.length >= this.cfg.ripple_max || this.rippleCooldown > 0) return;
			const flow = this._flowLevel();
			if (flow < 0.1) return;
			let cadence = this.cfg.ripple_every / Math.max(0.25, flow);
			if (cadence < 1) cadence = 1;
			const pipe = this._pipeGeometry();
			const col = pipe.center + (this.rng() * 2 - 1) * 1.2;
			const life = jitterInt(this.rng, 22, 0.25);
			const speed = (0.45 + this.rng() * 0.45) * (0.85 + 0.2 * flow);
			const strength = clamp01(0.45 + 0.3 * flow + this.rng() * 0.2);
			this.ripples.push({ col, radius: 0, speed, life, maxLife: life, strength });
			this.rippleCooldown = jitterInt(this.rng, Math.round(cadence), 0.25);
		}

		_spawnRunoff() {
			const overflow = this.fill - 1.0;
			if (overflow <= 0) return;
			if (this.endingTicks > 0 && this.endingTotal - this.endingTicks >= this.endingFade) {
				if (this.rng() > 0.25) return;
			}
			const flow = this._flowLevel();
			const intensity = Math.min(1, overflow / 0.6);
			if (this.rng() > 0.55 + 0.4 * intensity) return;
			const basin = this._basinGeometry();
			for (const side of [-1, 1]) {
				const col = side > 0 ? basin.right : basin.left;
				const vel = this.cfg.overflow_speed * (0.85 + 0.4 * this.rng()) * (0.7 + 0.3 * flow);
				const life = jitterInt(this.rng, 60, 0.3);
				const strength = clamp01(0.55 + 0.45 * intensity + this.rng() * 0.15);
				this.runoff.push({ col, vel, life, maxLife: life, strength, side });
			}
		}

		_spawnSplatter() {
			const flow = this._flowLevel();
			if (flow <= 0.05) return;
			const chance = this.cfg.splatter_p * (0.5 + 0.6 * flow);
			if (this.rng() > chance) return;
			if (this.droplets.length >= this.cfg.droplet_max) return;
			const basin = this._basinGeometry();
			const pipe = this._pipeGeometry();
			const col = pipe.center + (this.rng() * 2 - 1) * this.cfg.stream_width * 0.7;
			const row = basin.brim - 1 + this.rng() * 1.5;
			const vCol = (this.rng() * 2 - 1) * (0.5 + 0.4 * flow);
			const vRow = -(0.6 + this.rng() * 0.5) * (0.7 + 0.3 * flow);
			const life = jitterInt(this.rng, 18, 0.4);
			const hue = ((this.cfg.hue + (this.rng() * 2 - 1) * this.cfg.hue_sp * 0.5) % 360 + 360) % 360;
			const light = clamp01(this.cfg.lmax * (0.85 + this.rng() * 0.15));
			const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.85), light);
			this.droplets.push({ row, col, vRow, vCol, life, maxLife: life, color });
		}

		_paintBasin() {
			const { brim, bottom, left, right } = this._basinGeometry();
			const wall = this._wallThick();
			const wallHue = ((this.cfg.pipe_hue % 360) + 360) % 360;
			const wallC = hslToRGB(wallHue, 0.45, this.cfg.pipe_light);
			const wallC2 = hslToRGB(wallHue, 0.35, clamp01(this.cfg.pipe_light * 0.7));
			for (let y = bottom; y < bottom + wall && y < this.h; y++) {
				for (let x = left - wall; x <= right + wall && x < this.w; x++) {
					if (x < 0) continue;
					this._paintMax(y, x, y > bottom ? wallC2 : wallC);
				}
			}
			for (let y = brim; y <= bottom; y++) {
				for (let w = 0; w < wall; w++) {
					this._paintMax(y, left - 1 - w, wallC);
					this._paintMax(y, right + 1 + w, wallC);
				}
			}
			if (brim - 1 >= 0) {
				for (let w = 0; w < wall; w++) {
					const highlight = hslToRGB(wallHue, 0.3, clamp01(this.cfg.pipe_light * 1.4));
					this._paintMax(brim - 1, left - 1 - w, highlight);
					this._paintMax(brim - 1, right + 1 + w, highlight);
				}
			}
		}

		_paintPool() {
			const { brim, bottom, left, right } = this._basinGeometry();
			if (right <= left || bottom <= brim) return;
			const depth = bottom - brim;
			const level = clamp01(this.fill);
			let surface = bottom - Math.round(level * depth);
			if (surface > bottom) surface = bottom;
			if (surface < brim) surface = brim;
			const hue = ((this.cfg.hue % 360) + 360) % 360;
			for (let y = surface; y < bottom; y++) {
				const dist = (y - surface) / Math.max(1, bottom - surface - 1);
				const shimmer = 0.7 + 0.3 * Math.sin(y * 0.31 + this.tick * 0.08);
				const light = clamp01(this.cfg.lmin * (0.55 + 0.4 * dist) +
					(this.cfg.lmax - this.cfg.lmin) * 0.15 * shimmer);
				const color = hslToRGB(((hue - 6) % 360 + 360) % 360,
					clamp01(this.cfg.sat * 0.85), light);
				for (let x = left; x <= right; x++) {
					this._paintMax(y, x, color);
				}
			}
			if (surface >= brim && surface <= bottom) {
				const light = clamp01(this.cfg.lmax * 0.85);
				const color = hslToRGB(((hue + 2) % 360 + 360) % 360,
					clamp01(this.cfg.sat * 0.7), light);
				for (let x = left; x <= right; x++) {
					const wave = Math.sin(x * 0.42 + this.tick * 0.12) * 0.3;
					let row = surface + Math.round(wave);
					if (row < brim) row = brim;
					if (row > bottom) row = bottom;
					this._paintMax(row, x, color);
				}
			}
		}

		_paintStream() {
			const pipe = this._pipeGeometry();
			const basin = this._basinGeometry();
			const flow = this._flowLevel();
			if (flow <= 0.02) return;
			const depth = basin.bottom - basin.brim;
			const level = clamp01(this.fill);
			let surface = basin.bottom - Math.round(level * depth);
			if (surface < basin.brim + 1) surface = basin.brim + 1;
			const streamTop = pipe.lip + 1;
			const streamBottom = surface;
			if (streamBottom <= streamTop) return;
			const width = Math.max(1, this.cfg.stream_width * flow);
			const hue = ((this.cfg.hue % 360) + 360) % 360;
			for (let y = streamTop; y < streamBottom; y++) {
				const progress = (y - streamTop) / Math.max(1, streamBottom - streamTop - 1);
				const sway = Math.sin(y * 0.55 - this.tick * 0.18) * 0.6 * width * 0.18;
				const rowCenter = pipe.center + sway;
				const half = Math.max(0.6, width * 0.5);
				let start = Math.floor(rowCenter - half);
				let end = Math.ceil(rowCenter + half);
				if (start < 0) start = 0;
				if (end >= this.w) end = this.w - 1;
				for (let x = start; x <= end; x++) {
					const dist = Math.abs((x + 0.5) - rowCenter) / half;
					if (dist > 1.05) continue;
					const edge = clamp01(1 - dist * dist);
					const pulse = 0.7 + 0.3 * Math.sin(progress * 9 - this.tick * 0.36 + x * 0.4);
					const intensity = edge * pulse;
					if (intensity < 0.1) continue;
					const h = ((hue + Math.sin(progress * 2 + x * 0.1) * this.cfg.hue_sp * 0.5) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) *
						(0.35 + 0.6 * intensity));
					const color = hslToRGB(h, this.cfg.sat, light);
					this._paintMax(y, x, color);
				}
			}
			for (let x = pipe.left; x <= pipe.right; x++) {
				if (Math.abs(x - pipe.center) > this.cfg.stream_width * 0.55) continue;
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.6),
					clamp01(this.cfg.lmax * 0.95));
				this._paintMax(pipe.lip + 1, x, color);
			}
		}

		_paintImpact() {
			const flow = this._flowLevel();
			if (flow <= 0.05) return;
			const basin = this._basinGeometry();
			const depth = basin.bottom - basin.brim;
			const level = clamp01(this.fill);
			let surface = basin.bottom - Math.round(level * depth);
			if (surface < basin.brim) surface = basin.brim;
			if (surface > basin.bottom) surface = basin.bottom;
			const pipe = this._pipeGeometry();
			const radius = Math.round(Math.max(2, this.cfg.stream_width * flow * 0.7));
			const hue = ((this.cfg.hue % 360) + 360) % 360;
			for (let dx = -radius; dx <= radius; dx++) {
				const x = pipe.center + dx;
				if (x < 0 || x >= this.w) continue;
				const dist = Math.abs(dx) / (radius + 1);
				if (dist > 1) continue;
				const foam = clamp01((1 - dist * dist) * (0.65 + 0.25 * flow));
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) *
					(0.6 + 0.4 * foam));
				const color = hslToRGB(((hue + 10) % 360 + 360) % 360,
					clamp01(this.cfg.sat * 0.4), light);
				this._paintMax(surface, x, color);
				if (surface - 1 >= 0) this._paintMax(surface - 1, x, color);
			}
		}

		_paintRipples() {
			if (!this.ripples.length) return;
			const basin = this._basinGeometry();
			const depth = basin.bottom - basin.brim;
			const level = clamp01(this.fill);
			let surface = basin.bottom - Math.round(level * depth);
			if (surface < basin.brim) surface = basin.brim;
			if (surface > basin.bottom) surface = basin.bottom;
			const hue = ((this.cfg.hue % 360) + 360) % 360;
			for (const r of this.ripples) {
				const fade = clamp01(r.life / Math.max(1, r.maxLife));
				if (fade <= 0) continue;
				for (let x = basin.left; x <= basin.right; x++) {
					const wave = Math.abs(Math.abs(x - r.col) - r.radius);
					if (wave > 0.85) continue;
					const bright = r.strength * fade * (1 - wave / 0.85);
					const light = clamp01(this.cfg.lmin * 0.85 + (this.cfg.lmax - this.cfg.lmin) *
						(0.25 + 0.6 * bright));
					const color = hslToRGB(((hue - 6) % 360 + 360) % 360,
						clamp01(this.cfg.sat * 0.7), light);
					this._paintMax(surface, x, color);
				}
			}
		}

		_paintRunoff() {
			if (!this.runoff.length) return;
			const basin = this._basinGeometry();
			const wall = this._wallThick();
			let floor = basin.bottom + wall;
			if (floor >= this.h) floor = this.h - 1;
			const hue = ((this.cfg.hue % 360) + 360) % 360;
			for (const r of this.runoff) {
				const fade = clamp01(r.life / Math.max(1, r.maxLife));
				const intensity = r.strength * fade;
				if (intensity <= 0.02) continue;
				let col = Math.round(r.col);
				if (r.side > 0 && col < basin.right) col = basin.right + 1;
				if (r.side < 0 && col > basin.left) col = basin.left - 1;
				if (col < 0 || col >= this.w) continue;
				const light = clamp01(this.cfg.lmin * 0.85 + (this.cfg.lmax - this.cfg.lmin) *
					(0.3 + 0.6 * intensity));
				const color = hslToRGB(((hue - 4) % 360 + 360) % 360,
					clamp01(this.cfg.sat * 0.75), light);
				this._paintMax(floor, col, color);
				if (floor + 1 < this.h && intensity > 0.3) {
					this._paintMax(floor + 1, col, {
						r: Math.floor(color.r * 0.75),
						g: Math.floor(color.g * 0.75),
						b: Math.floor(color.b * 0.75),
					});
				}
				const trail = Math.round(2 + 3 * intensity);
				for (let t = 1; t <= trail; t++) {
					const tcol = col - r.side * t;
					if (tcol < 0 || tcol >= this.w) continue;
					const tfade = intensity * (1 - t / (trail + 1));
					if (tfade <= 0.05) continue;
					const tlight = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) *
						(0.2 + 0.5 * tfade));
					const tc = hslToRGB(((hue - 8) % 360 + 360) % 360,
						clamp01(this.cfg.sat * 0.65), tlight);
					this._paintMax(floor, tcol, tc);
				}
			}
		}

		_paintPipe() {
			const pipe = this._pipeGeometry();
			const wallHue = ((this.cfg.pipe_hue % 360) + 360) % 360;
			const body = hslToRGB(wallHue, 0.55, this.cfg.pipe_light);
			const rim = hslToRGB(wallHue, 0.45, clamp01(this.cfg.pipe_light * 1.5));
			const shade = hslToRGB(wallHue, 0.45, clamp01(this.cfg.pipe_light * 0.65));
			for (let y = 0; y <= pipe.lip; y++) {
				for (let x = pipe.left; x <= pipe.right; x++) {
					this._paintMax(y, x, (x === pipe.left || x === pipe.right) ? shade : body);
				}
			}
			for (let x = pipe.left; x <= pipe.right; x++) {
				this._paintMax(pipe.lip, x, rim);
			}
			if (pipe.lip + 1 < this.h) {
				for (let x = pipe.left - 1; x <= pipe.right + 1; x++) {
					if (x < 0 || x >= this.w) continue;
					this._paintMax(pipe.lip, x, rim);
				}
			}
		}

		_paintDroplets() {
			for (const d of this.droplets) {
				const fade = clamp01(d.life / Math.max(1, d.maxLife));
				if (fade <= 0) continue;
				const row = Math.round(d.row);
				const col = Math.round(d.col);
				const scale = 0.3 + 0.7 * fade;
				this._paintMax(row, col, {
					r: Math.floor(d.color.r * scale),
					g: Math.floor(d.color.g * scale),
					b: Math.floor(d.color.b * scale),
				});
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

	api.presets['water-pipe'] = [
		{
			key: 'small-trickle',
			label: 'small trickle',
			config: {
				intro_drip: 0.1,
				intro_fill: 0.02,
				ending_residue: 0.1,
				pipe_width: 5,
				stream_width: 1.5,
				basin_y: 0.7,
				basin_span: 0.28,
				basin_depth: 7,
				inflow: 0.45,
				drain: 0.04,
				overflow_speed: 0.4,
				overflow_fade: 0.08,
				splatter_p: 0.18,
				droplet_max: 18,
				ripple_every: 11,
				ripple_max: 5,
				hue: 198,
				hue_sp: 10,
				sat: 0.5,
				lmin: 0.4,
				lmax: 0.82,
				pipe_hue: 24,
				pipe_light: 0.32,
				dry_p: 0.0008,
				dry_mult: 0.15,
			},
		},
		{
			key: 'steady-pool',
			label: 'steady pool',
			config: {
				intro_drip: 0.18,
				intro_fill: 0.1,
				ending_residue: 0.32,
				pipe_width: 6,
				stream_width: 3,
				basin_y: 0.66,
				basin_span: 0.36,
				basin_depth: 8,
				inflow: 1.0,
				drain: 0.6,
				overflow_speed: 0.5,
				overflow_fade: 0.06,
				splatter_p: 0.4,
				droplet_max: 36,
				ripple_every: 7,
				ripple_max: 8,
				hue: 200,
				hue_sp: 14,
				sat: 0.55,
				lmin: 0.42,
				lmax: 0.84,
				pipe_hue: 28,
				pipe_light: 0.34,
				surge_p: 0.0006,
				surge_mult: 1.6,
			},
		},
		{
			key: 'heavy-spill',
			label: 'heavy spill',
			config: {
				intro_drip: 0.22,
				intro_fill: 0.4,
				ending_residue: 0.45,
				pipe_width: 8,
				stream_width: 4.5,
				basin_y: 0.62,
				basin_span: 0.4,
				basin_depth: 6,
				wall_thick: 1,
				inflow: 1.8,
				drain: 0.05,
				overflow_speed: 0.85,
				overflow_fade: 0.035,
				splatter_p: 0.7,
				droplet_max: 64,
				ripple_every: 5,
				ripple_max: 12,
				hue: 196,
				hue_sp: 16,
				sat: 0.6,
				lmin: 0.45,
				lmax: 0.88,
				pipe_hue: 22,
				pipe_light: 0.36,
				surge_p: 0.0014,
				surge_mult: 2.0,
				surge_dur: 80,
			},
		},
		{
			key: 'edge-runoff',
			label: 'edge runoff',
			config: {
				intro_drip: 0.16,
				intro_fill: 0.6,
				ending_residue: 0.5,
				pipe_x: 0.22,
				pipe_width: 6,
				stream_width: 3.5,
				basin_y: 0.68,
				basin_span: 0.3,
				basin_depth: 5,
				wall_thick: 1,
				inflow: 1.4,
				drain: 0.02,
				overflow_speed: 1.1,
				overflow_fade: 0.025,
				splatter_p: 0.55,
				droplet_max: 48,
				ripple_every: 6,
				ripple_max: 10,
				hue: 204,
				hue_sp: 18,
				sat: 0.58,
				lmin: 0.44,
				lmax: 0.86,
				pipe_hue: 32,
				pipe_light: 0.34,
				surge_p: 0.0009,
				surge_mult: 1.7,
				dry_p: 0.0005,
				dry_mult: 0.2,
			},
		},
	];
	api.effects['water-pipe'] = WaterPipe;
})(window.AmbienceSim);
