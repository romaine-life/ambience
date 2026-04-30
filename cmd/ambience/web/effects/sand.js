'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	const SAND_DEFAULTS = {
		intro_dur: 70,
		intro_trickle: 0.18,
		intro_pile: 0.05,
		ending_dur: 70,
		ending_linger: 40,
		ending_residue: 0.4,
		pipe_x: 0.5,
		pipe_y: 0.16,
		pipe_width: 6,
		stream_spread: 1.4,
		container_y: 0.62,
		container_span: 0.42,
		container_depth: 16,
		wall_thick: 1,
		emit_rate: 1.6,
		gravity: 0.085,
		drag: 0.04,
		spread: 0.06,
		splatter_p: 0.18,
		grain_max: 96,
		repose: 1.6,
		settle: 6,
		hue: 38,
		hue_sp: 12,
		sat: 0.6,
		lmin: 0.36,
		lmax: 0.78,
		pipe_hue: 22,
		pipe_light: 0.32,
		surge_p: 0,
		calm_p: 0,
		surge_dur: 60,
		surge_mult: 1.9,
		calm_dur: 70,
		calm_mult: 0.35,
	};

	function applySandDefaults(cfg) {
		const c = Object.assign({}, SAND_DEFAULTS, cfg || {});
		if (c.intro_dur === 0 && c.intro_trickle === 0 && c.intro_pile === 0) {
			c.intro_dur = SAND_DEFAULTS.intro_dur;
			c.intro_trickle = SAND_DEFAULTS.intro_trickle;
			c.intro_pile = SAND_DEFAULTS.intro_pile;
		} else {
			if (c.intro_dur <= 0) c.intro_dur = SAND_DEFAULTS.intro_dur;
			if (c.intro_trickle <= 0) c.intro_trickle = SAND_DEFAULTS.intro_trickle;
			if (c.intro_pile < 0) c.intro_pile = 0;
		}
		c.intro_trickle = clamp01(c.intro_trickle);
		c.intro_pile = clamp01(c.intro_pile);
		if (c.ending_dur === 0 && c.ending_linger === 0 && c.ending_residue === 0) {
			c.ending_dur = SAND_DEFAULTS.ending_dur;
			c.ending_linger = SAND_DEFAULTS.ending_linger;
			c.ending_residue = SAND_DEFAULTS.ending_residue;
		} else {
			if (c.ending_dur <= 0) c.ending_dur = SAND_DEFAULTS.ending_dur;
			if (c.ending_linger < 0) c.ending_linger = 0;
			if (c.ending_residue < 0) c.ending_residue = 0;
		}
		c.ending_residue = clamp01(c.ending_residue);
		if (c.pipe_x <= 0) c.pipe_x = SAND_DEFAULTS.pipe_x;
		c.pipe_x = clamp01(c.pipe_x);
		if (c.pipe_y <= 0) c.pipe_y = SAND_DEFAULTS.pipe_y;
		c.pipe_y = clamp01(c.pipe_y);
		if (c.pipe_width <= 0) c.pipe_width = SAND_DEFAULTS.pipe_width;
		if (c.stream_spread <= 0) c.stream_spread = SAND_DEFAULTS.stream_spread;
		if (c.container_y <= 0) c.container_y = SAND_DEFAULTS.container_y;
		c.container_y = clamp01(c.container_y);
		if (c.container_span <= 0) c.container_span = SAND_DEFAULTS.container_span;
		c.container_span = clamp01(c.container_span);
		if (c.container_depth <= 0) c.container_depth = SAND_DEFAULTS.container_depth;
		if (c.wall_thick <= 0) c.wall_thick = SAND_DEFAULTS.wall_thick;
		if (c.emit_rate <= 0) c.emit_rate = SAND_DEFAULTS.emit_rate;
		if (c.gravity <= 0) c.gravity = SAND_DEFAULTS.gravity;
		if (c.drag < 0) c.drag = 0;
		if (c.spread < 0) c.spread = 0;
		if (c.splatter_p < 0) c.splatter_p = 0;
		if (c.grain_max <= 0) c.grain_max = SAND_DEFAULTS.grain_max;
		if (c.repose <= 0) c.repose = SAND_DEFAULTS.repose;
		if (c.settle <= 0) c.settle = SAND_DEFAULTS.settle;
		if (c.hue_sp <= 0) c.hue_sp = SAND_DEFAULTS.hue_sp;
		if (c.sat <= 0) c.sat = SAND_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = SAND_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = SAND_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.pipe_light <= 0) c.pipe_light = SAND_DEFAULTS.pipe_light;
		c.pipe_light = clamp01(c.pipe_light);
		if (c.surge_dur <= 0) c.surge_dur = SAND_DEFAULTS.surge_dur;
		if (c.surge_mult <= 0) c.surge_mult = SAND_DEFAULTS.surge_mult;
		if (c.calm_dur <= 0) c.calm_dur = SAND_DEFAULTS.calm_dur;
		if (c.calm_mult < 0) c.calm_mult = 0;
		return c;
	}

	class Sand {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.cfg = applySandDefaults(cfg);
			this.rng = makeRNG(seed || Date.now());
			this.tick = 0;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.grains = [];
			this.surgeTicks = 0;
			this.calmTicks = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.pile = [];
			this.pileLeft = 0;
			this._resetPile();
		}

		setConfig(cfg) {
			const prev = this.cfg;
			this.cfg = applySandDefaults(Object.assign({}, this.cfg, cfg));
			if (prev.container_y !== this.cfg.container_y ||
				prev.container_span !== this.cfg.container_span ||
				prev.container_depth !== this.cfg.container_depth ||
				prev.pipe_x !== this.cfg.pipe_x ||
				prev.pipe_width !== this.cfg.pipe_width) {
				this._resetPile();
			}
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.surgeTicks = state.surgeTicks || 0;
			this.calmTicks = state.calmTicks || 0;
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			if (typeof snap.seed === 'number') this.rng = makeRNG(snap.seed);
			if (snap.gridW > 0 && snap.gridH > 0 &&
				(snap.gridW !== this.w || snap.gridH !== this.h)) {
				this.w = snap.gridW;
				this.h = snap.gridH;
				this.grid = new Uint8ClampedArray(this.w * this.h * 3);
				this._resetPile();
			}
			this.grains = Array.isArray(state.grains) ? state.grains.map(g => ({
				row: g.row, col: g.col, vRow: g.vRow, vCol: g.vCol,
				life: g.life, maxLife: g.maxLife, color: g.color, bright: g.bright,
			})) : [];
			if (Array.isArray(state.pile) && state.pile.length > 0) {
				this.pile = state.pile.slice();
				this.pileLeft = state.pileLeft || 0;
			} else {
				this._resetPile();
			}
		}

		triggerEvent(name) {
			switch (name) {
				case 'surge':
					this.surgeTicks = jitterInt(this.rng, this.cfg.surge_dur, 0.3);
					return true;
				case 'calm':
					this.calmTicks = jitterInt(this.rng, this.cfg.calm_dur, 0.3);
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
			if (this.introTicks > 0) this.introTicks--;
			if (this.endingTicks > 0) this.endingTicks--;

			if (this.surgeTicks === 0 && this.rng() < this.cfg.surge_p) {
				this.surgeTicks = jitterInt(this.rng, this.cfg.surge_dur, 0.3);
			}
			if (this.calmTicks === 0 && this.rng() < this.cfg.calm_p) {
				this.calmTicks = jitterInt(this.rng, this.cfg.calm_dur, 0.3);
			}

			this._spawnGrains();
			this._stepGrains();
			this._settlePile();
			this._applyEndingDrain();

			this.grid.fill(0);
			this._paintContainer();
			this._paintPile();
			this._paintPipe();
			this._paintGrains();
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
					flow *= 1 - 0.94 * fade;
				} else {
					flow *= 0.0;
				}
			}
			if (flow < 0) flow = 0;
			return flow;
		}

		_startIntro() {
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.introTotal = this.cfg.intro_dur > 0 ? this.cfg.intro_dur : SAND_DEFAULTS.intro_dur;
			this.introTicks = this.introTotal;
			this.grains = [];
			this._resetPile();
			if (this.cfg.intro_pile > 0) this._seedPile(this.cfg.intro_pile);
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = this.cfg.ending_dur > 0 ? this.cfg.ending_dur : SAND_DEFAULTS.ending_dur;
			const linger = Math.max(0, this.cfg.ending_linger);
			this.endingTotal = Math.max(1, this.endingFade + linger);
			this.endingTicks = this.endingTotal;
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
			if (lip > this.h - 8) lip = this.h - 8;
			return { lip, left, right, center };
		}

		_containerGeometry() {
			let brim = Math.round(this.cfg.container_y * (this.h - 1));
			if (brim < 8) brim = 8;
			if (brim > this.h - 4) brim = this.h - 4;
			let depth = Math.round(this.cfg.container_depth);
			if (depth < 3) depth = 3;
			if (depth > this.h - brim - 1) depth = this.h - brim - 1;
			if (depth < 2) depth = 2;
			const bottom = brim + depth;
			let half = Math.round(this.cfg.container_span * this.w * 0.5);
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

		_resetPile() {
			const c = this._containerGeometry();
			const cols = Math.max(1, c.right - c.left + 1);
			this.pile = new Array(cols).fill(0);
			this.pileLeft = c.left;
		}

		_seedPile(fillFraction) {
			const c = this._containerGeometry();
			const depth = c.bottom - c.brim;
			if (depth <= 0) return;
			const level = clamp01(fillFraction) * depth;
			const cols = Math.max(1, c.right - c.left + 1);
			if (this.pile.length !== cols || this.pileLeft !== c.left) {
				this.pile = new Array(cols).fill(0);
				this.pileLeft = c.left;
			}
			for (let i = 0; i < cols; i++) {
				const dist = Math.abs(i - (cols - 1) * 0.5);
				const falloff = Math.max(0, 1 - dist / (cols * 0.5 + 0.001));
				this.pile[i] = level * (0.7 + 0.3 * falloff);
			}
		}

		_spawnGrains() {
			const flow = this._flowLevel();
			if (flow <= 0.001) return;
			if (this.grains.length >= this.cfg.grain_max) return;
			const rate = this.cfg.emit_rate * flow;
			if (rate <= 0) return;
			let count = Math.floor(rate);
			const frac = rate - count;
			if (this.rng() < frac) count++;
			if (count <= 0) return;
			const pipe = this._pipeGeometry();
			for (let i = 0; i < count && this.grains.length < this.cfg.grain_max; i++) {
				this._spawnOneGrain(pipe.lip, pipe.center);
			}
		}

		_spawnOneGrain(lipRow, pipeCenter) {
			const col = pipeCenter + (this.rng() * 2 - 1) * Math.max(0.4, this.cfg.stream_spread * 0.5);
			const row = lipRow + 1;
			const vCol = (this.rng() * 2 - 1) * 0.18 * this.cfg.stream_spread;
			const vRow = 0.35 + this.rng() * 0.25;
			const hue = ((this.cfg.hue + (this.rng() * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
			const light = this.cfg.lmin + this.rng() * (this.cfg.lmax - this.cfg.lmin);
			const color = hslToRGB(hue, this.cfg.sat, light);
			const bright = 0.75 + this.rng() * 0.25;
			const maxLife = jitterInt(this.rng, Math.max(40, this.h * 2), 0.25);
			this.grains.push({ row, col, vRow, vCol, life: maxLife, maxLife, color, bright });
		}

		_stepGrains() {
			if (!this.grains.length) {
				if (this.cfg.splatter_p > 0 && this._flowLevel() > 0.05 && this.rng() < this.cfg.splatter_p * 0.15) {
					this._spawnSplatter();
				}
				return;
			}
			const alive = [];
			const gravity = this.cfg.gravity;
			const drag = this.cfg.drag;
			const jitter = this.cfg.spread * 0.18;
			const c = this._containerGeometry();
			const wallTop = c.bottom;
			for (const g of this.grains) {
				g.vRow += gravity;
				if (drag > 0) g.vCol *= 1 - drag * 0.4;
				if (jitter > 0) g.vCol += (this.rng() * 2 - 1) * jitter;
				g.row += g.vRow;
				g.col += g.vCol;
				g.life--;

				const gridCol = Math.round(g.col);
				if (gridCol >= c.left && gridCol <= c.right) {
					const idx = gridCol - this.pileLeft;
					if (idx >= 0 && idx < this.pile.length) {
						const surfaceRow = wallTop - this.pile[idx];
						if (g.row >= surfaceRow) {
							this._depositGrain(idx);
							continue;
						}
					}
					if (gridCol === c.left - 1 || gridCol === c.right + 1) {
						g.vCol = Math.abs(g.vCol);
						if (gridCol === c.right + 1) g.vCol = -g.vCol;
					}
				}

				if (g.life <= 0 || g.row >= this.h + 2) continue;
				alive.push(g);
			}
			this.grains = alive;

			if (this.cfg.splatter_p > 0 && this._flowLevel() > 0.05 && this.grains.length < this.cfg.grain_max) {
				if (this.rng() < this.cfg.splatter_p * 0.15) this._spawnSplatter();
			}
		}

		_depositGrain(idx) {
			if (idx < 0 || idx >= this.pile.length) return;
			this.pile[idx] += 1.0;
			const c = this._containerGeometry();
			const maxH = (c.bottom - c.brim) + 2;
			if (this.pile[idx] > maxH) this.pile[idx] = maxH;
		}

		_settlePile() {
			if (this.pile.length <= 1) return;
			const repose = Math.max(0.5, this.cfg.repose);
			const passes = Math.max(1, this.cfg.settle);
			for (let p = 0; p < passes; p++) {
				let moved = false;
				if ((p + this.tick) % 2 === 0) {
					for (let i = 0; i < this.pile.length - 1; i++) {
						if (this._tryFlow(i, i + 1, repose)) moved = true;
					}
					for (let i = this.pile.length - 1; i > 0; i--) {
						if (this._tryFlow(i, i - 1, repose)) moved = true;
					}
				} else {
					for (let i = this.pile.length - 1; i > 0; i--) {
						if (this._tryFlow(i, i - 1, repose)) moved = true;
					}
					for (let i = 0; i < this.pile.length - 1; i++) {
						if (this._tryFlow(i, i + 1, repose)) moved = true;
					}
				}
				if (!moved) break;
			}
		}

		_tryFlow(src, dst, repose) {
			if (src < 0 || src >= this.pile.length || dst < 0 || dst >= this.pile.length) return false;
			const delta = this.pile[src] - this.pile[dst];
			if (delta <= repose) return false;
			const move = (delta - repose) * 0.5;
			if (move < 0.05) return false;
			this.pile[src] -= move;
			this.pile[dst] += move;
			return true;
		}

		_applyEndingDrain() {
			if (this.endingTicks <= 0) return;
			const progress = this._phaseProgress(this.endingTotal, this.endingTicks);
			const target = clamp01(this.cfg.ending_residue);
			for (let i = 0; i < this.pile.length; i++) {
				this.pile[i] = this.pile[i] * (1 - 0.04 * progress) + target * this.pile[i] * 0.04 * progress;
			}
		}

		_spawnSplatter() {
			if (this.grains.length >= this.cfg.grain_max) return;
			const c = this._containerGeometry();
			if (c.right <= c.left) return;
			const pipe = this._pipeGeometry();
			const idx = pipe.center - this.pileLeft;
			if (idx < 0 || idx >= this.pile.length) return;
			const row = c.bottom - this.pile[idx];
			const col = pipe.center + (this.rng() * 2 - 1) * this.cfg.stream_spread * 1.4;
			const vRow = -(0.35 + this.rng() * 0.3);
			const vCol = (this.rng() * 2 - 1) * 0.55;
			const hue = ((this.cfg.hue + (this.rng() * 2 - 1) * this.cfg.hue_sp * 0.7) % 360 + 360) % 360;
			const light = clamp01(this.cfg.lmax * (0.85 + this.rng() * 0.15));
			const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.85), light);
			const maxLife = jitterInt(this.rng, 22, 0.3);
			this.grains.push({ row, col, vRow, vCol, life: maxLife, maxLife, color, bright: 0.95 });
		}

		_paintContainer() {
			const { brim, bottom, left, right } = this._containerGeometry();
			const wall = this._wallThick();
			const wallHue = ((this.cfg.pipe_hue % 360) + 360) % 360;
			const wallC = hslToRGB(wallHue, 0.4, this.cfg.pipe_light);
			const wallC2 = hslToRGB(wallHue, 0.32, clamp01(this.cfg.pipe_light * 0.7));
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
				const highlight = hslToRGB(wallHue, 0.3, clamp01(this.cfg.pipe_light * 1.4));
				for (let w = 0; w < wall; w++) {
					this._paintMax(brim - 1, left - 1 - w, highlight);
					this._paintMax(brim - 1, right + 1 + w, highlight);
				}
			}
		}

		_paintPile() {
			if (!this.pile.length) return;
			const { bottom, left, right } = this._containerGeometry();
			if (right <= left) return;
			const hue = ((this.cfg.hue % 360) + 360) % 360;
			for (let i = 0; i < this.pile.length; i++) {
				const col = this.pileLeft + i;
				if (col < left || col > right) continue;
				const h = this.pile[i];
				if (h <= 0) continue;
				let topRow = bottom - Math.round(h);
				if (topRow < 0) topRow = 0;
				for (let y = topRow; y <= bottom; y++) {
					const depth = bottom - y;
					const frac = h > 0 ? depth / h : 0;
					const ridge = 1 - clamp01(frac);
					const grain = 0.5 + 0.5 * Math.sin(col * 0.81 + depth * 0.37);
					const localHue = ((hue + (grain - 0.5) * this.cfg.hue_sp * 0.6) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) *
						(0.25 + 0.55 * ridge + 0.18 * grain));
					const color = hslToRGB(localHue, clamp01(this.cfg.sat * 0.92), light);
					this._paintMax(y, col, color);
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
			for (let x = pipe.left - 1; x <= pipe.right + 1; x++) {
				if (x < 0 || x >= this.w) continue;
				this._paintMax(pipe.lip, x, rim);
			}
		}

		_paintGrains() {
			for (const g of this.grains) {
				const fade = clamp01(g.life / Math.max(1, g.maxLife));
				if (fade <= 0) continue;
				const row = Math.round(g.row);
				const col = Math.round(g.col);
				const bright = g.bright * (0.5 + 0.5 * fade);
				this._paintMax(row, col, {
					r: Math.floor(g.color.r * bright),
					g: Math.floor(g.color.g * bright),
					b: Math.floor(g.color.b * bright),
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

	api.presets['sand'] = [
		{
			key: 'small-trickle',
			label: 'small trickle',
			config: {
				intro_trickle: 0.15,
				intro_pile: 0.02,
				ending_residue: 0.7,
				pipe_x: 0.5,
				pipe_width: 5,
				stream_spread: 0.8,
				container_y: 0.66,
				container_span: 0.34,
				container_depth: 14,
				emit_rate: 0.5,
				gravity: 0.075,
				drag: 0.04,
				spread: 0.04,
				splatter_p: 0.08,
				grain_max: 48,
				repose: 1.4,
				settle: 4,
				hue: 42,
				hue_sp: 8,
				sat: 0.55,
				lmin: 0.38,
				lmax: 0.78,
				pipe_hue: 24,
				pipe_light: 0.32,
				calm_p: 0.0008,
				calm_mult: 0.3,
			},
		},
		{
			key: 'steady-pour',
			label: 'steady pour',
			config: {
				intro_trickle: 0.22,
				intro_pile: 0.08,
				ending_residue: 0.5,
				pipe_x: 0.5,
				pipe_width: 6,
				stream_spread: 1.4,
				container_y: 0.62,
				container_span: 0.42,
				container_depth: 16,
				emit_rate: 1.6,
				gravity: 0.085,
				drag: 0.04,
				spread: 0.06,
				splatter_p: 0.18,
				grain_max: 96,
				repose: 1.6,
				settle: 6,
				hue: 38,
				hue_sp: 12,
				sat: 0.6,
				lmin: 0.36,
				lmax: 0.78,
				pipe_hue: 22,
				pipe_light: 0.32,
				surge_p: 0.0006,
				surge_mult: 1.7,
			},
		},
		{
			key: 'heavy-fill',
			label: 'heavy fill',
			config: {
				intro_trickle: 0.3,
				intro_pile: 0.18,
				ending_residue: 0.55,
				pipe_x: 0.5,
				pipe_width: 8,
				stream_spread: 2.2,
				container_y: 0.58,
				container_span: 0.5,
				container_depth: 20,
				emit_rate: 3.2,
				gravity: 0.1,
				drag: 0.05,
				spread: 0.1,
				splatter_p: 0.32,
				grain_max: 180,
				repose: 1.8,
				settle: 8,
				hue: 34,
				hue_sp: 14,
				sat: 0.65,
				lmin: 0.34,
				lmax: 0.82,
				pipe_hue: 20,
				pipe_light: 0.34,
				surge_p: 0.0014,
				surge_mult: 2.1,
				surge_dur: 80,
			},
		},
		{
			key: 'overflow-study',
			label: 'overflow study',
			config: {
				intro_trickle: 0.3,
				intro_pile: 0.55,
				ending_residue: 0.6,
				pipe_x: 0.5,
				pipe_width: 6,
				stream_spread: 1.6,
				container_y: 0.7,
				container_span: 0.34,
				container_depth: 10,
				wall_thick: 1,
				emit_rate: 2.4,
				gravity: 0.09,
				drag: 0.04,
				spread: 0.08,
				splatter_p: 0.4,
				grain_max: 140,
				repose: 1.2,
				settle: 8,
				hue: 36,
				hue_sp: 16,
				sat: 0.62,
				lmin: 0.36,
				lmax: 0.84,
				pipe_hue: 22,
				pipe_light: 0.34,
				surge_p: 0.0009,
				surge_mult: 1.8,
			},
		},
	];
	api.effects['sand'] = Sand;
})(window.AmbienceSim);
