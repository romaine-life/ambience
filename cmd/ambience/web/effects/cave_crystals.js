'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	// CaveCrystals — long-arc effect: angular crystals nucleate on a dim
	// cave floor and grow incrementally over the lifetime of the scene.
	// Mirrors sim/cave_crystals.go: per-crystal state (position, growth
	// step, target height, hue/tilt jitter) and the intro/steady/ending
	// lifecycle that wraps it. The authority decides which crystals exist
	// and which step they're on; the browser replays the same per-tick
	// growth so step transitions stay synced even though sparkle particles
	// drift per client.
	const CAVE_CRYSTALS_DEFAULTS = {
		intro_dur: 80, intro_seed: 4,
		ending_dur: 90, ending_linger: 40, ending_resid: 0.18,
		max_count: 32, baseline: 0.82, floor_bump: 1.5,
		nucleate_p: 0.012, growth_dur: 110, max_steps: 5,
		min_size: 4, max_size: 12, width_ratio: 0.35, sparkle_dur: 12,
		hue: 280, hue_sp: 24, sat: 0.55, lmin: 0.22, lmax: 0.78, glow: 0.4,
		pop_p: 0, burst_p: 0, quiet_p: 0,
		pop_mult: 1.4, burst_dur: 30, burst_count: 4,
		quiet_dur: 200, quiet_mult: 0.3,
	};

	function applyCaveCrystalsDefaults(cfg) {
		const c = Object.assign({}, CAVE_CRYSTALS_DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = CAVE_CRYSTALS_DEFAULTS.intro_dur;
		if (c.intro_seed <= 0) c.intro_seed = CAVE_CRYSTALS_DEFAULTS.intro_seed;
		if (c.ending_dur <= 0) c.ending_dur = CAVE_CRYSTALS_DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_resid = clamp01(c.ending_resid);
		if (c.ending_resid === 0) c.ending_resid = CAVE_CRYSTALS_DEFAULTS.ending_resid;
		if (c.max_count <= 0) c.max_count = CAVE_CRYSTALS_DEFAULTS.max_count;
		if (c.max_count > 96) c.max_count = 96;
		if (c.baseline <= 0) c.baseline = CAVE_CRYSTALS_DEFAULTS.baseline;
		if (c.floor_bump < 0) c.floor_bump = 0;
		if (c.floor_bump === 0) c.floor_bump = CAVE_CRYSTALS_DEFAULTS.floor_bump;
		if (c.nucleate_p < 0) c.nucleate_p = 0;
		if (c.nucleate_p === 0) c.nucleate_p = CAVE_CRYSTALS_DEFAULTS.nucleate_p;
		if (c.growth_dur <= 0) c.growth_dur = CAVE_CRYSTALS_DEFAULTS.growth_dur;
		if (c.max_steps <= 0) c.max_steps = CAVE_CRYSTALS_DEFAULTS.max_steps;
		if (c.min_size <= 0) c.min_size = CAVE_CRYSTALS_DEFAULTS.min_size;
		if (c.max_size <= 0) c.max_size = CAVE_CRYSTALS_DEFAULTS.max_size;
		if (c.max_size < c.min_size) [c.min_size, c.max_size] = [c.max_size, c.min_size];
		if (c.width_ratio <= 0) c.width_ratio = CAVE_CRYSTALS_DEFAULTS.width_ratio;
		if (c.sparkle_dur <= 0) c.sparkle_dur = CAVE_CRYSTALS_DEFAULTS.sparkle_dur;
		if (c.hue === 0) c.hue = CAVE_CRYSTALS_DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = CAVE_CRYSTALS_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = CAVE_CRYSTALS_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = CAVE_CRYSTALS_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.glow < 0) c.glow = 0;
		if (c.glow === 0) c.glow = CAVE_CRYSTALS_DEFAULTS.glow;
		if (c.pop_p < 0) c.pop_p = 0;
		if (c.burst_p < 0) c.burst_p = 0;
		if (c.quiet_p < 0) c.quiet_p = 0;
		if (c.pop_mult <= 0) c.pop_mult = CAVE_CRYSTALS_DEFAULTS.pop_mult;
		if (c.burst_dur <= 0) c.burst_dur = CAVE_CRYSTALS_DEFAULTS.burst_dur;
		if (c.burst_count <= 0) c.burst_count = CAVE_CRYSTALS_DEFAULTS.burst_count;
		if (c.quiet_dur <= 0) c.quiet_dur = CAVE_CRYSTALS_DEFAULTS.quiet_dur;
		if (c.quiet_mult <= 0) c.quiet_mult = CAVE_CRYSTALS_DEFAULTS.quiet_mult;
		if (c.quiet_mult > 1) c.quiet_mult = 1;
		return c;
	}

	class CaveCrystals {
		constructor(w, h, cfg, seed) {
			this.kind = 'cave-crystals';
			this.w = w;
			this.h = h;
			this.cfg = applyCaveCrystalsDefaults(cfg);
			this.seed = Number(seed || Date.now());
			this.rng = makeRNG(this.seed);
			this.tick = 0;
			this.crystals = [];
			this.floor = this._buildFloor();
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.quietTicks = 0;
			this.burstTicks = 0;
		}

		setConfig(cfg) {
			this.cfg = applyCaveCrystalsDefaults(Object.assign({}, this.cfg, cfg));
			if (this.crystals.length > this.cfg.max_count) {
				this.crystals.length = this.cfg.max_count;
			}
			this.floor = this._buildFloor();
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || 0;
			const list = state.crystals || state.Crystals;
			this.crystals = [];
			if (Array.isArray(list)) {
				for (const cr of list) {
					this.crystals.push({
						x: Number(cr.x) || 0,
						baseRow: Number(cr.row) || 0,
						step: Number(cr.s) || 0,
						maxStep: Math.max(1, Number(cr.m) || this.cfg.max_steps),
						height: Number(cr.h) || this.cfg.min_size,
						width: Number(cr.w) || 1,
						tilt: Number(cr.t) || 0,
						hueOff: Number(cr.hu) || 0,
						timer: Number(cr.tm) || 0,
						sparkle: Number(cr.sp) || 0,
						popped: Boolean(cr.pop),
					});
				}
			}
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			this.quietTicks = state.quietTicks || 0;
			this.burstTicks = state.burstTicks || 0;
			const floor = state.floor || state.FloorProfile;
			if (Array.isArray(floor) && floor.length > 0) {
				this.floor = floor.map((v) => Number(v) || 0);
			} else {
				this.floor = this._buildFloor();
			}
			if (typeof snap.seed === 'number') {
				this.seed = snap.seed;
				this.rng = makeRNG(snap.seed);
			}
			if (snap.gridW > 0 && snap.gridH > 0) {
				this.w = snap.gridW;
				this.h = snap.gridH;
			}
		}

		triggerEvent(name) {
			switch (name) {
				case 'nucleus-spawn':
					this._spawnCrystal(false);
					return true;
				case 'crystal-pop':
					this._spawnCrystal(true);
					return true;
				case 'growth-pulse': {
					const idx = this._pickGrowing();
					if (idx >= 0) this._advanceCrystal(idx);
					return true;
				}
				case 'sparkle-burst':
					this._startSparkleBurst();
					return true;
				case 'quiet-cave':
					this.quietTicks = jitterInt(this.rng, this.cfg.quiet_dur, 0.3);
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
			if (this.endingTicks > 0) {
				this.endingTicks--;
				if (this.endingTicks === 0) this._startIntro();
			} else if (this.introTicks > 0) {
				this.introTicks--;
			}
			if (this.quietTicks > 0) this.quietTicks--;
			if (this.burstTicks > 0) this.burstTicks--;

			// Decay sparkles + advance growth timers locally so each
			// browser stays in lockstep with the authority on which step
			// each crystal occupies, even between config broadcasts.
			for (const cr of this.crystals) {
				if (cr.sparkle > 0) cr.sparkle--;
				if (cr.step >= cr.maxStep) continue;
				if (cr.timer > 0) cr.timer--;
				if (cr.timer > 0) continue;
				cr.step++;
				cr.timer = this._growthInterval();
				cr.sparkle = this.cfg.sparkle_dur;
			}
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#050409');
				sky.addColorStop(0.5, '#0a0712');
				sky.addColorStop(1, '#0e0a16');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const dim = this._dimLevel();

			// Cave floor — soil band that follows the floor profile.
			const baselineRow = Math.max(4, Math.min(this.h - 2, Math.floor(this.h * this.cfg.baseline)));
			for (let x = 0; x < this.w; x++) {
				const row = this._floorRowAt(x);
				for (let y = row; y < this.h; y++) {
					const depth = (y - row) / Math.max(1, this.h - row);
					const dirtL = clamp01(this.cfg.lmin * (0.45 + depth * 0.5) * (0.7 + dim * 0.3));
					const dirt = hslToRGB((this.cfg.hue + 200) % 360, clamp01(this.cfg.sat * 0.18), dirtL);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${dirt.r},${dirt.g},${dirt.b})`, 1);
				}
				// Soft cave ceiling tint above the upper third — a hint of the
				// cavern, not a literal ceiling.
				const ceilingLimit = Math.max(0, Math.floor(this.h * 0.18));
				for (let y = 0; y < ceilingLimit; y++) {
					const fade = (ceilingLimit - y) / ceilingLimit;
					const ceil = hslToRGB((this.cfg.hue + 30) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin * 0.35 * fade));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${ceil.r},${ceil.g},${ceil.b})`, fade * 0.6);
				}
			}

			// Per-crystal paint. Pre-sort by base row + height so crystals
			// behind taller ones are painted first and the foreground reads
			// in front.
			const order = this.crystals.map((cr, i) => i).sort((a, b) => {
				const ca = this.crystals[a], cb = this.crystals[b];
				return (ca.baseRow - cb.baseRow) || (ca.height - cb.height);
			});
			for (const idx of order) {
				this._paintCrystal(ctx, sx, sy, ceilSx, ceilSy, this.crystals[idx], idx, dim);
			}

			// Glow wash anchored on the brightest crystals.
			if (this.cfg.glow > 0 && dim > 0.05) {
				this._paintGlow(ctx, canvasW, canvasH, dim);
			}

			// During the ending, drop a darkening overlay so the cave
			// reads as going dark rather than just statically dim.
			if (this.endingTicks > 0 && this.endingFade > 0) {
				const fadeProgress = clamp01((this.endingTotal - this.endingTicks) / this.endingFade);
				ctx.fillStyle = `rgba(2,2,6,${0.55 * fadeProgress})`;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}
		}

		_paintCrystal(ctx, sx, sy, ceilSx, ceilSy, cr, idx, dim) {
			if (cr.step <= 0) {
				// Pure nucleus — a single glint on the floor so the seeded
				// crystal is visible before any growth step lands.
				const seedHue = ((this.cfg.hue + cr.hueOff) % 360 + 360) % 360;
				const seed = hslToRGB(seedHue, clamp01(this.cfg.sat * 0.7), clamp01(this.cfg.lmax * 0.55 * dim));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(cr.x), cr.baseRow - 1, 1, 1, `rgb(${seed.r},${seed.g},${seed.b})`, clamp01(0.5 + dim * 0.4));
				return;
			}
			const stepFrac = clamp01(cr.step / Math.max(1, cr.maxStep));
			const fullH = cr.height;
			const visH = Math.max(1, Math.round(fullH * stepFrac));
			const halfW = Math.max(1, cr.width);
			const cx = Math.round(cr.x);
			const baseRow = cr.baseRow;
			const hue = ((this.cfg.hue + cr.hueOff) % 360 + 360) % 360;
			const coreLight = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.55 + stepFrac * 0.35));
			const edgeLight = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18);
			const core = hslToRGB(hue, this.cfg.sat, coreLight * dim);
			const edge = hslToRGB((hue + 12) % 360, clamp01(this.cfg.sat * 0.6), edgeLight * dim);
			const ash = hslToRGB((hue + 200) % 360, clamp01(this.cfg.sat * 0.15), clamp01(this.cfg.lmin * 0.6 * dim));

			// Angular shard: starts narrow at the floor for a faceted look,
			// expands to half-width near the middle, tapers back at the tip.
			for (let y = 0; y < visH; y++) {
				const yFrac = y / Math.max(1, visH - 1);
				const widthShape = this._shardWidth(yFrac);
				const tilt = Math.round(cr.tilt * y);
				const halfCells = Math.max(0, Math.round(halfW * widthShape));
				const row = baseRow - 1 - y;
				if (row < 0) break;
				const isTip = yFrac > 0.85;
				for (let dx = -halfCells; dx <= halfCells; dx++) {
					const col = cx + dx + tilt;
					const radial = Math.abs(dx) / Math.max(1, halfCells);
					let color;
					if (isTip && radial < 0.55) {
						color = core;
					} else if (radial < 0.35) {
						color = core;
					} else if (radial < 0.8) {
						color = edge;
					} else {
						color = ash;
					}
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
				}
				// Center facet stripe — single column highlight that picks
				// up extra brightness near the tip.
				if (halfCells >= 1) {
					const facetLight = clamp01(coreLight + (1 - radialFalloff(yFrac)) * 0.15);
					const facet = hslToRGB(hue, this.cfg.sat, facetLight * dim);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cx + tilt, row, 1, 1, `rgb(${facet.r},${facet.g},${facet.b})`, 1);
				}
			}

			// Sparkle decoration: a couple of pixels above the tip while
			// sparkle ticks remain on this crystal.
			if (cr.sparkle > 0) {
				const sparkleStrength = clamp01(cr.sparkle / Math.max(1, this.cfg.sparkle_dur));
				const tipRow = Math.max(0, baseRow - visH - 1);
				const sparkleHue = (hue + 30) % 360;
				const sparkle = hslToRGB(sparkleHue, clamp01(this.cfg.sat * 0.7), clamp01(this.cfg.lmax * dim));
				const wig = Math.sin(this.tick * 0.6 + idx) * 1.2;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(cx + wig), tipRow, 1, 1, `rgb(${sparkle.r},${sparkle.g},${sparkle.b})`, clamp01(0.4 + sparkleStrength * 0.5));
				if (sparkleStrength > 0.5) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(cx - wig * 0.7), tipRow - 1, 1, 1, `rgb(${sparkle.r},${sparkle.g},${sparkle.b})`, clamp01(sparkleStrength * 0.4));
				}
			}
		}

		_paintGlow(ctx, canvasW, canvasH, dim) {
			const strength = clamp01(this.cfg.glow * dim);
			if (strength < 0.05) return;
			let pickIdx = -1;
			for (let i = 0; i < this.crystals.length; i++) {
				const cr = this.crystals[i];
				if (cr.step >= cr.maxStep) {
					if (pickIdx < 0 || cr.height > this.crystals[pickIdx].height) pickIdx = i;
				}
			}
			if (pickIdx < 0) return;
			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const cr = this.crystals[pickIdx];
			const hue = ((this.cfg.hue + cr.hueOff) % 360 + 360) % 360;
			const cx = cr.x * sx;
			const cy = (cr.baseRow - cr.height * 0.5) * sy;
			const radius = Math.max(40, cr.height * sy * (3 + strength * 4));
			const grad = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius);
			const core = hslToRGB(hue, this.cfg.sat, clamp01(this.cfg.lmax * 0.85));
			const outer = hslToRGB((hue + 16) % 360, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + 0.05));
			grad.addColorStop(0, `rgba(${core.r},${core.g},${core.b},${0.18 + strength * 0.18})`);
			grad.addColorStop(0.5, `rgba(${outer.r},${outer.g},${outer.b},${0.06 + strength * 0.1})`);
			grad.addColorStop(1, `rgba(${outer.r},${outer.g},${outer.b},0)`);
			ctx.fillStyle = grad;
			ctx.fillRect(cx - radius, cy - radius, radius * 2, radius * 2);
		}

		// Lifecycle helpers.
		_startIntro() {
			this.crystals = [];
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.quietTicks = 0;
			this.burstTicks = 0;
			this.introTotal = Math.max(1, this.cfg.intro_dur);
			this.introTicks = this.introTotal;
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.quietTicks = 0;
			this.burstTicks = 0;
			this.endingFade = Math.max(1, this.cfg.ending_dur);
			this.endingTotal = Math.max(1, this.endingFade + Math.max(0, this.cfg.ending_linger));
			this.endingTicks = this.endingTotal;
		}

		_startSparkleBurst() {
			const dur = jitterInt(this.rng, this.cfg.burst_dur, 0.25);
			this.burstTicks = dur;
			let count = Math.min(this.cfg.burst_count, this.crystals.length);
			while (count-- > 0 && this.crystals.length > 0) {
				const idx = Math.floor(this.rng() * this.crystals.length);
				this.crystals[idx].sparkle = dur;
			}
		}

		_spawnCrystal(popped) {
			if (this.crystals.length >= this.cfg.max_count) return;
			const x = this.rng() * this.w;
			const hueOff = (this.rng() * 2 - 1) * this.cfg.hue_sp;
			const tilt = (this.rng() * 2 - 1) * 0.4;
			const heightFrac = this.rng();
			let target = this.cfg.min_size + (this.cfg.max_size - this.cfg.min_size) * heightFrac;
			if (popped) target *= this.cfg.pop_mult;
			const width = Math.max(1, target * this.cfg.width_ratio * (0.7 + this.rng() * 0.6));
			const cr = {
				x,
				baseRow: this._floorRowAt(x),
				step: 0,
				maxStep: Math.max(1, this.cfg.max_steps),
				height: target,
				width,
				tilt,
				hueOff,
				timer: this._growthInterval(),
				sparkle: popped ? this.cfg.sparkle_dur * 2 : 0,
				popped: !!popped,
			};
			if (popped) cr.step = cr.maxStep;
			this.crystals.push(cr);
		}

		_pickGrowing() {
			const candidates = [];
			for (let i = 0; i < this.crystals.length; i++) {
				if (this.crystals[i].step < this.crystals[i].maxStep) candidates.push(i);
			}
			if (!candidates.length) return -1;
			return candidates[Math.floor(this.rng() * candidates.length)];
		}

		_advanceCrystal(idx) {
			const cr = this.crystals[idx];
			if (!cr || cr.step >= cr.maxStep) return;
			cr.step++;
			cr.timer = this._growthInterval();
			cr.sparkle = this.cfg.sparkle_dur;
		}

		_growthInterval() {
			let dur = this.cfg.growth_dur;
			if (this.quietTicks > 0 && this.cfg.quiet_mult > 0) {
				dur = Math.round(dur / this.cfg.quiet_mult);
			}
			return jitterInt(this.rng, Math.max(1, dur), 0.2);
		}

		_dimLevel() {
			let level = 1;
			if (this.endingTicks > 0 && this.endingFade > 0) {
				const fadeProgress = clamp01((this.endingTotal - this.endingTicks) / this.endingFade);
				level = clamp01(this.cfg.ending_resid + (1 - this.cfg.ending_resid) * (1 - fadeProgress));
			}
			if (this.introTicks > 0 && this.introTotal > 0) {
				const introProgress = clamp01((this.introTotal - this.introTicks) / this.introTotal);
				level *= 0.5 + 0.5 * introProgress;
			}
			if (this.burstTicks > 0) {
				const burstStrength = clamp01(this.burstTicks / Math.max(1, this.cfg.burst_dur));
				level = Math.min(1.4, level + burstStrength * 0.25);
			}
			return Math.max(0.05, level);
		}

		_buildFloor() {
			const out = new Array(this.w);
			const bump = this.cfg.floor_bump;
			for (let x = 0; x < this.w; x++) {
				const t = x / Math.max(1, this.w);
				const w = Math.sin(t * Math.PI * 1.6 + 0.7) * 0.55 + Math.sin(t * Math.PI * 4.1 + 1.9) * 0.3;
				out[x] = w * bump;
			}
			return out;
		}

		_floorRowAt(x) {
			let col = Math.round(x);
			if (col < 0) col = 0;
			if (col >= this.w) col = this.w - 1;
			const bump = this.floor && col < this.floor.length ? this.floor[col] : 0;
			const base = Math.floor(this.h * this.cfg.baseline);
			let row = base + Math.round(bump);
			if (row < 1) row = 1;
			if (row >= this.h) row = this.h - 1;
			return row;
		}

		_shardWidth(yFrac) {
			// 0 at the floor, peaks ~0.4, tapers toward 0 at the tip. The
			// peak isn't dead-center so the shard reads as growing upward.
			const peak = 0.32;
			if (yFrac < peak) {
				return 0.6 + (yFrac / peak) * 0.4;
			}
			return clamp01(1 - (yFrac - peak) / (1 - peak)) * 0.95 + 0.05;
		}

		_fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, w, h, color, alpha) {
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), Math.max(1, Math.ceil(w * sx || ceilSx)), Math.max(1, Math.ceil(h * sy || ceilSy)));
			ctx.globalAlpha = 1;
		}
	}

	function radialFalloff(yFrac) {
		// Used for the centre-facet highlight — brighter near the tip.
		return clamp01(yFrac);
	}

	api.presets['cave-crystals'] = [
		{
			key: 'amethyst-cluster',
			label: 'amethyst cluster',
			config: {
				max_count: 28, baseline: 0.82, floor_bump: 1.5,
				nucleate_p: 0.011, growth_dur: 100, max_steps: 5,
				min_size: 5, max_size: 13, width_ratio: 0.32, sparkle_dur: 14,
				hue: 282, hue_sp: 22, sat: 0.62, lmin: 0.22, lmax: 0.82, glow: 0.45,
				pop_p: 0.0006, burst_p: 0.0008, quiet_p: 0.0002,
				pop_mult: 1.4, burst_dur: 28, burst_count: 4, quiet_dur: 220, quiet_mult: 0.3,
			},
		},
		{
			key: 'quartz-shelf',
			label: 'quartz shelf',
			config: {
				max_count: 36, baseline: 0.78, floor_bump: 0.5,
				nucleate_p: 0.018, growth_dur: 80, max_steps: 4,
				min_size: 4, max_size: 10, width_ratio: 0.28, sparkle_dur: 10,
				hue: 200, hue_sp: 14, sat: 0.32, lmin: 0.3, lmax: 0.92, glow: 0.55,
				pop_p: 0.0004, burst_p: 0.0014, quiet_p: 0.0001,
				pop_mult: 1.3, burst_dur: 32, burst_count: 6, quiet_dur: 160, quiet_mult: 0.4,
			},
		},
		{
			key: 'glowstone',
			label: 'glowstone',
			config: {
				max_count: 22, baseline: 0.84, floor_bump: 2,
				nucleate_p: 0.008, growth_dur: 130, max_steps: 6,
				min_size: 6, max_size: 14, width_ratio: 0.42, sparkle_dur: 18,
				hue: 50, hue_sp: 16, sat: 0.7, lmin: 0.26, lmax: 0.88, glow: 0.7,
				pop_p: 0.0008, burst_p: 0.001, quiet_p: 0.0003,
				pop_mult: 1.5, burst_dur: 36, burst_count: 5, quiet_dur: 240, quiet_mult: 0.25,
			},
		},
		{
			key: 'obsidian-shard',
			label: 'obsidian shard',
			config: {
				max_count: 18, baseline: 0.86, floor_bump: 2.5,
				nucleate_p: 0.006, growth_dur: 160, max_steps: 5,
				min_size: 6, max_size: 16, width_ratio: 0.28, sparkle_dur: 8,
				hue: 260, hue_sp: 36, sat: 0.45, lmin: 0.16, lmax: 0.6, glow: 0.25,
				pop_p: 0.0005, burst_p: 0.0006, quiet_p: 0.0006,
				pop_mult: 1.6, burst_dur: 24, burst_count: 3, quiet_dur: 320, quiet_mult: 0.2,
			},
		},
	];
	api.effects['cave-crystals'] = CaveCrystals;
})(window.AmbienceSim);
