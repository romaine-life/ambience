'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	// BurningTrees — slow ambient row of trees that ignite, burn, and resolve
	// into ash. Mirrors sim/burning_trees.go: same tree state machine
	// (alive → igniting → burning → ashing → ash) and lifecycle so authority
	// and browser stay in sync on which tree is currently on fire even though
	// inner flame turbulence renders independently per client.
	const BURNING_TREES_DEFAULTS = {
		intro_dur: 60, intro_growth: 0.18,
		ending_dur: 80, ending_linger: 30, ending_ash: 0.35,
		tree_count: 9, tree_width: 7, tree_min_h: 8, tree_max_h: 16,
		baseline: 0.86, canopy: 0.62,
		ignite_dur: 30, burn_dur: 220, ash_dur: 80, spread_p: 0.012,
		flame_h: 9, flicker: 0.7, ember_rate: 0.32, glow: 0.45, smoke: 0.45,
		canopy_hue: 118, flame_hue: 22, hue_sp: 14, sat: 0.62, lmin: 0.18, lmax: 0.82,
		ignite_p: 0, flare_p: 0, lull_p: 0,
		flare_dur: 36, flare_mult: 1.7, lull_dur: 60, lull_mult: 0.55,
	};

	const BTREE_STATE_ALIVE = 0;
	const BTREE_STATE_IGNITING = 1;
	const BTREE_STATE_BURNING = 2;
	const BTREE_STATE_ASHING = 3;
	const BTREE_STATE_ASH = 4;

	function applyBurningTreesDefaults(cfg) {
		const c = Object.assign({}, BURNING_TREES_DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = BURNING_TREES_DEFAULTS.intro_dur;
		c.intro_growth = clamp01(c.intro_growth);
		if (c.ending_dur <= 0) c.ending_dur = BURNING_TREES_DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_ash = clamp01(c.ending_ash);
		if (c.tree_count <= 0) c.tree_count = BURNING_TREES_DEFAULTS.tree_count;
		if (c.tree_count > 24) c.tree_count = 24;
		if (c.tree_width <= 0) c.tree_width = BURNING_TREES_DEFAULTS.tree_width;
		if (c.tree_min_h <= 0) c.tree_min_h = BURNING_TREES_DEFAULTS.tree_min_h;
		if (c.tree_max_h <= 0) c.tree_max_h = BURNING_TREES_DEFAULTS.tree_max_h;
		if (c.tree_max_h < c.tree_min_h) [c.tree_min_h, c.tree_max_h] = [c.tree_max_h, c.tree_min_h];
		if (c.baseline <= 0) c.baseline = BURNING_TREES_DEFAULTS.baseline;
		if (c.canopy <= 0) c.canopy = BURNING_TREES_DEFAULTS.canopy;
		if (c.ignite_dur <= 0) c.ignite_dur = BURNING_TREES_DEFAULTS.ignite_dur;
		if (c.burn_dur <= 0) c.burn_dur = BURNING_TREES_DEFAULTS.burn_dur;
		if (c.ash_dur <= 0) c.ash_dur = BURNING_TREES_DEFAULTS.ash_dur;
		if (c.spread_p < 0) c.spread_p = 0;
		if (c.flame_h <= 0) c.flame_h = BURNING_TREES_DEFAULTS.flame_h;
		if (c.flicker <= 0) c.flicker = BURNING_TREES_DEFAULTS.flicker;
		if (c.ember_rate < 0) c.ember_rate = 0;
		if (c.glow <= 0) c.glow = BURNING_TREES_DEFAULTS.glow;
		if (c.smoke < 0) c.smoke = 0;
		if (c.canopy_hue === 0) c.canopy_hue = BURNING_TREES_DEFAULTS.canopy_hue;
		if (c.flame_hue < 0) c.flame_hue = BURNING_TREES_DEFAULTS.flame_hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = BURNING_TREES_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = BURNING_TREES_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = BURNING_TREES_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.ignite_p < 0) c.ignite_p = 0;
		if (c.flare_p < 0) c.flare_p = 0;
		if (c.lull_p < 0) c.lull_p = 0;
		if (c.flare_dur <= 0) c.flare_dur = BURNING_TREES_DEFAULTS.flare_dur;
		if (c.flare_mult <= 0) c.flare_mult = BURNING_TREES_DEFAULTS.flare_mult;
		if (c.lull_dur <= 0) c.lull_dur = BURNING_TREES_DEFAULTS.lull_dur;
		if (c.lull_mult <= 0) c.lull_mult = BURNING_TREES_DEFAULTS.lull_mult;
		return c;
	}

	class BurningTrees {
		constructor(w, h, cfg, seed) {
			this.kind = 'burning-trees';
			this.w = w;
			this.h = h;
			this.cfg = applyBurningTreesDefaults(cfg);
			this.seed = Number(seed || Date.now());
			this.rng = makeRNG(this.seed);
			this.tick = 0;
			this.states = new Uint8Array(this.cfg.tree_count);
			this.phaseLeft = new Int32Array(this.cfg.tree_count);
			this.phaseTotal = new Int32Array(this.cfg.tree_count);
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.flareTicks = 0;
			this.flareGain = 1;
			this.lullTicks = 0;
		}

		setConfig(cfg) {
			const next = applyBurningTreesDefaults(Object.assign({}, this.cfg, cfg));
			if (next.tree_count !== this.states.length) {
				this.states = new Uint8Array(next.tree_count);
				this.phaseLeft = new Int32Array(next.tree_count);
				this.phaseTotal = new Int32Array(next.tree_count);
			}
			this.cfg = next;
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || 0;
			const states = state.States || state.states;
			if (states) {
				let bytes;
				if (typeof states === 'string') {
					const bin = atob(states);
					bytes = new Uint8Array(bin.length);
					for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i) & 0xff;
				} else {
					bytes = Uint8Array.from(states);
				}
				if (bytes.length !== this.states.length) {
					this.states = new Uint8Array(bytes.length);
					this.phaseLeft = new Int32Array(bytes.length);
					this.phaseTotal = new Int32Array(bytes.length);
				}
				this.states.set(bytes);
			}
			const phaseLeft = state.phaseLeft || state.PhaseLeft;
			if (Array.isArray(phaseLeft) && phaseLeft.length === this.phaseLeft.length) {
				for (let i = 0; i < phaseLeft.length; i++) this.phaseLeft[i] = phaseLeft[i] | 0;
			}
			const phaseTotal = state.phaseTotal || state.PhaseTotal;
			if (Array.isArray(phaseTotal) && phaseTotal.length === this.phaseTotal.length) {
				for (let i = 0; i < phaseTotal.length; i++) this.phaseTotal[i] = phaseTotal[i] | 0;
			}
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			this.flareTicks = state.flareTicks || 0;
			this.flareGain = state.flareGain || 1;
			this.lullTicks = state.lullTicks || 0;
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
				case 'ignite': {
					const idx = this._pickHealthy();
					if (idx >= 0) this._ignite(idx);
					return true;
				}
				case 'flare':
					this.flareTicks = jitterInt(this.rng, this.cfg.flare_dur, 0.3);
					this.flareGain = Math.max(1, this.cfg.flare_mult * (0.85 + this.rng() * 0.3));
					return true;
				case 'lull':
					this.lullTicks = jitterInt(this.rng, this.cfg.lull_dur, 0.3);
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
			if (this.flareTicks > 0) {
				this.flareTicks--;
				if (this.flareTicks === 0) this.flareGain = 1;
			}
			if (this.lullTicks > 0) this.lullTicks--;
			this._advanceTrees();
			// Spread/spawn rolls happen on the authority only — clients don't
			// fire those, they just replay the resulting trigger commands.
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				const burnSky = this._anyBurning();
				if (burnSky) {
					sky.addColorStop(0, '#0a0712');
					sky.addColorStop(0.55, '#1a0f10');
					sky.addColorStop(1, '#150705');
				} else {
					sky.addColorStop(0, '#0d1a18');
					sky.addColorStop(0.55, '#152724');
					sky.addColorStop(1, '#1d2419');
				}
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const baseRow = Math.max(8, Math.min(this.h - 2, Math.floor(this.h * this.cfg.baseline)));

			// Soil — flat ground band beneath the trees.
			for (let y = baseRow; y < this.h; y++) {
				const ratio = (y - baseRow) / Math.max(1, this.h - baseRow);
				const dirt = hslToRGB((this.cfg.canopy_hue + 12) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * (0.4 + ratio * 0.6)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${dirt.r},${dirt.g},${dirt.b})`, 1);
			}

			const introProgress = this._introProgress();
			const endingProgress = this._endingProgress();
			const ashLinger = this._ashLingerLevel();
			const intensity = this._intensityLevel();

			// Per-tree paint pass — the tree row is the meat of the effect.
			const n = this.states.length || 1;
			const rowHalf = Math.max(1, this.cfg.tree_width * 0.5);
			const rowSpan = this.w / n;
			for (let i = 0; i < n; i++) {
				const cx = Math.floor(rowSpan * (i + 0.5));
				const treeRng = this._treeNoise(i);
				const heightFrac = 0.5 + 0.5 * treeRng[0];
				const fullH = Math.max(2, Math.round(this.cfg.tree_min_h + (this.cfg.tree_max_h - this.cfg.tree_min_h) * heightFrac));
				let stateH = fullH;
				if (this.introTicks > 0) {
					const grow = this.cfg.intro_growth + (1 - this.cfg.intro_growth) * introProgress;
					stateH = Math.max(2, Math.round(fullH * grow));
				}
				const halfW = Math.max(1, Math.round(rowHalf + treeRng[1] * 1.5));
				const state = this.states[i];
				const burnEnv = this._burnEnvelope(i);
				this._paintTree(ctx, sx, sy, ceilSx, ceilSy, cx, baseRow, stateH, halfW, i, treeRng, state, burnEnv, ashLinger, intensity, endingProgress);
			}

			// Vignette + warm wash if anything is on fire.
			if (this._anyActiveFlame()) {
				const center = canvasW * 0.5;
				const vignY = Math.floor(baseRow * sy);
				const radius = Math.max(canvasW, canvasH) * 0.55;
				const wash = ctx.createRadialGradient(center, vignY, radius * 0.2, center, vignY, radius);
				const tint = hslToRGB(this.cfg.flame_hue, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmax * 0.7));
				wash.addColorStop(0, `rgba(${tint.r},${tint.g},${tint.b},${0.06 + intensity * 0.05})`);
				wash.addColorStop(1, 'rgba(0,0,0,0)');
				ctx.fillStyle = wash;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}
		}

		// _paintTree draws one tree at column cx with stump on baseRow. Trees
		// blend through the burn lifecycle so a tree halfway through burning
		// still has a visible (but sparser, charring) canopy with flame inside.
		_paintTree(ctx, sx, sy, ceilSx, ceilSy, cx, baseRow, stateH, halfW, i, treeRng, state, burnEnv, ashLinger, intensity, endingProgress) {
			const trunkH = Math.max(2, Math.round(stateH * 0.32));
			const canopyH = Math.max(2, stateH - trunkH);
			const burning = state === BTREE_STATE_IGNITING || state === BTREE_STATE_BURNING || state === BTREE_STATE_ASHING;
			const canopyAlive = state === BTREE_STATE_ALIVE;
			const charProgress = this._charProgress(state, burnEnv);

			// Trunk.
			const trunkBase = hslToRGB((this.cfg.canopy_hue + 28) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.95 - charProgress * 0.06));
			const trunkChar = hslToRGB(20, 0.18, clamp01(0.04 + (1 - charProgress) * 0.08));
			for (let y = 0; y < trunkH; y++) {
				const row = baseRow - 1 - y;
				if (row < 0) break;
				const trunkOffset = Math.round((treeRng[2] - 0.5) * 1.6 * y / Math.max(1, trunkH));
				for (let dx = -1; dx <= 1; dx++) {
					const col = cx + dx + trunkOffset;
					const c = charProgress >= 0.5 ? trunkChar : trunkBase;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${c.r},${c.g},${c.b})`, 1);
				}
			}

			// Canopy or charred silhouette. Cone-ish silhouette with hashed leaf cells.
			const canopyTop = baseRow - trunkH - canopyH;
			const canopyAlpha = canopyAlive ? 1 : (state === BTREE_STATE_IGNITING ? 0.95 : (state === BTREE_STATE_BURNING ? 0.75 - burnEnv * 0.4 : (state === BTREE_STATE_ASHING ? 0.4 * (1 - burnEnv) : ashLinger)));
			if (canopyAlpha > 0.02) {
				const baseHueJitter = (treeRng[3] - 0.5) * this.cfg.hue_sp;
				const canopyHue = ((this.cfg.canopy_hue + baseHueJitter) % 360 + 360) % 360;
				const charHue = 20 + (treeRng[4] - 0.5) * 10;
				const canopyLight = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.42 + treeRng[5] * 0.22));
				const charLight = clamp01(0.08 + (1 - charProgress) * 0.06);
				for (let y = 0; y < canopyH; y++) {
					const row = canopyTop + y;
					const yFrac = y / Math.max(1, canopyH - 1);
					const widthShape = Math.max(1, Math.round(halfW * (0.35 + 0.65 * yFrac)));
					for (let dx = -widthShape; dx <= widthShape; dx++) {
						const col = cx + dx;
						const filled = this._canopyHash(i, dx, y, this.cfg.canopy);
						if (!filled) continue;
						let r, g, b, alpha;
						if (charProgress > 0.85 || state === BTREE_STATE_ASH) {
							const c = hslToRGB(charHue, 0.18, charLight);
							r = c.r; g = c.g; b = c.b; alpha = canopyAlpha * 0.8;
						} else if (charProgress > 0.05) {
							const blend = clamp01(charProgress * 1.05);
							const fresh = hslToRGB(canopyHue, this.cfg.sat, canopyLight * (1 - blend * 0.4));
							const charred = hslToRGB(charHue, 0.16, charLight + 0.06);
							r = Math.round(fresh.r * (1 - blend) + charred.r * blend);
							g = Math.round(fresh.g * (1 - blend) + charred.g * blend);
							b = Math.round(fresh.b * (1 - blend) + charred.b * blend);
							alpha = canopyAlpha;
						} else {
							const c = hslToRGB(canopyHue, this.cfg.sat, canopyLight);
							r = c.r; g = c.g; b = c.b; alpha = canopyAlpha;
						}
						alpha = clamp01(alpha * (0.85 + (treeRng[6] - 0.5) * 0.2));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${r},${g},${b})`, alpha);
					}
				}
			}

			// Active flame inside / above the canopy while igniting/burning/ashing.
			if (burning) {
				const flameBase = baseRow - trunkH - Math.round(canopyH * 0.4);
				this._paintFlame(ctx, sx, sy, ceilSx, ceilSy, cx, flameBase, halfW, i, burnEnv, intensity, state);
				if (this.cfg.ember_rate > 0) {
					this._paintEmbers(ctx, sx, sy, ceilSx, ceilSy, cx, flameBase, halfW, i, burnEnv, intensity, state);
				}
				if (this.cfg.glow > 0) {
					this._paintGlow(ctx, sx, sy, cx, baseRow - trunkH - Math.round(canopyH * 0.5), halfW, burnEnv, intensity, state);
				}
			}
			// Smoke trails for burning + ashing trees.
			if ((burning || (state === BTREE_STATE_ASH && ashLinger > 0.2)) && this.cfg.smoke > 0) {
				this._paintSmoke(ctx, sx, sy, ceilSx, ceilSy, cx, canopyTop, halfW, i, burnEnv, state);
			}
		}

		_paintFlame(ctx, sx, sy, ceilSx, ceilSy, cx, anchorRow, halfW, i, burnEnv, intensity, state) {
			const speed = this.tick * 0.18;
			const intensityMix = intensity * (state === BTREE_STATE_IGNITING ? 0.55 + burnEnv * 0.45 : (state === BTREE_STATE_ASHING ? 0.25 + burnEnv * 0.4 : 0.7 + burnEnv * 0.3));
			const flameH = Math.max(2, Math.round(this.cfg.flame_h * (0.5 + intensityMix * 0.7)));
			const flameW = Math.max(1, Math.round(halfW * (0.55 + intensityMix * 0.4)));
			for (let dx = -flameW; dx <= flameW; dx++) {
				const nx = Math.abs(dx) / Math.max(1, flameW);
				const widthShape = Math.max(0, 1 - Math.pow(nx, 1.4));
				if (widthShape <= 0.05) continue;
				const pulse = 0.78 + 0.22 * Math.sin(speed * 1.4 + dx * 0.6 + this._hash(8000 + i * 41 + dx) * 6);
				const colH = Math.max(1, Math.round(flameH * widthShape * pulse));
				for (let y = 0; y < colH; y++) {
					const lift = y / Math.max(1, colH);
					const taper = 1 - lift;
					const sway = Math.sin(speed * 1.9 + dx * 0.3 + y * 0.22 + this._hash(8200 + i * 53 + y) * 6) * this.cfg.flicker * taper * 0.6;
					const col = Math.round(cx + dx + sway);
					const row = Math.round(anchorRow - y);
					if (row < 0) break;
					const hueWiggle = (this._hash(8400 + i * 19 + y * 7 + dx) - 0.5) * this.cfg.hue_sp * 0.4;
					const hue = ((this.cfg.flame_hue - lift * this.cfg.hue_sp * 0.4 + hueWiggle) % 360 + 360) % 360;
					const sat = clamp01(this.cfg.sat * (0.85 + taper * 0.18));
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.22 + taper * 0.78));
					const alpha = clamp01((0.18 + taper * 0.55) * (0.4 + widthShape * 0.6) * intensityMix);
					const color = hslToRGB(hue, sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if (widthShape > 0.4 && lift < 0.55 && (dx + y) % 2 === 0) {
						const core = hslToRGB((this.cfg.flame_hue + 8) % 360, clamp01(this.cfg.sat * 0.7), clamp01(this.cfg.lmax * (0.7 + taper * 0.25)));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${core.r},${core.g},${core.b})`, alpha * 0.55);
					}
				}
			}
		}

		_paintEmbers(ctx, sx, sy, ceilSx, ceilSy, cx, anchorRow, halfW, i, burnEnv, intensity, state) {
			const intensityMix = intensity * (state === BTREE_STATE_BURNING ? 1 : 0.55);
			const count = Math.max(1, Math.round(this.cfg.ember_rate * 6 * intensityMix));
			const maxRise = Math.max(6, Math.round(this.cfg.flame_h * 1.6 + 6));
			for (let k = 0; k < count; k++) {
				const cycle = maxRise + 6 + Math.floor(this._hash(9000 + i * 23 + k) * 12);
				const phase = positiveMod(this.tick * 0.3 * (0.7 + this._hash(9100 + i * 17 + k) * 0.6) + this._hash(9200 + i * 13 + k) * cycle, cycle);
				if (phase > maxRise) continue;
				const fade = 1 - phase / Math.max(1, maxRise);
				const drift = (this._hash(9300 + i * 11 + k) * 2 - 1) * (1.4 + phase * 0.06);
				const col = Math.round(cx + drift);
				const row = Math.round(anchorRow - 1 - phase);
				if (row < 0) continue;
				const hue = ((this.cfg.flame_hue + (this._hash(9400 + i * 7 + k) - 0.5) * 14) + 360) % 360;
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.55 + fade * 0.4));
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.85), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, clamp01((0.18 + fade * 0.6) * intensityMix));
			}
		}

		_paintGlow(ctx, sx, sy, cx, anchorRow, halfW, burnEnv, intensity, state) {
			const stage = state === BTREE_STATE_BURNING ? 1 : (state === BTREE_STATE_IGNITING ? 0.55 : 0.35);
			const strength = clamp01(this.cfg.glow * stage * intensity * (0.6 + burnEnv * 0.45));
			if (strength < 0.05) return;
			const glowX = cx * sx;
			const glowY = anchorRow * sy;
			const radius = Math.max(20, halfW * sx * (4 + strength * 6));
			const grad = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, radius);
			const core = hslToRGB((this.cfg.flame_hue + 6) % 360, clamp01(this.cfg.sat), clamp01(this.cfg.lmax * 0.85));
			const outer = hslToRGB((this.cfg.flame_hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.7), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.5));
			grad.addColorStop(0, `rgba(${core.r},${core.g},${core.b},${0.32 + strength * 0.28})`);
			grad.addColorStop(0.5, `rgba(${outer.r},${outer.g},${outer.b},${0.14 + strength * 0.18})`);
			grad.addColorStop(1, `rgba(${outer.r},${outer.g},${outer.b},0)`);
			ctx.fillStyle = grad;
			ctx.fillRect(glowX - radius, glowY - radius, radius * 2, radius * 2);
		}

		_paintSmoke(ctx, sx, sy, ceilSx, ceilSy, cx, canopyTop, halfW, i, burnEnv, state) {
			const stage = state === BTREE_STATE_BURNING ? 1 : (state === BTREE_STATE_IGNITING ? 0.55 : (state === BTREE_STATE_ASHING ? 0.85 : 0.45));
			const strength = clamp01(this.cfg.smoke * stage);
			if (strength < 0.05) return;
			const maxRise = Math.max(6, Math.round(canopyTop * 0.7));
			const puffCount = Math.max(2, Math.round(3 + strength * 6));
			for (let k = 0; k < puffCount; k++) {
				const cycle = maxRise + 8 + Math.floor(this._hash(10000 + i * 31 + k) * 18);
				const phase = positiveMod(this.tick * 0.12 * (0.7 + this._hash(10100 + i * 27 + k) * 0.5) + this._hash(10200 + i * 19 + k) * cycle, cycle);
				if (phase > maxRise) continue;
				const fade = 1 - phase / Math.max(1, maxRise);
				const drift = Math.sin(this.tick * 0.04 + i + k) * (1.5 + phase * 0.1) + (this._hash(10300 + i * 13 + k) - 0.5) * halfW * 0.6;
				const col = Math.round(cx + drift);
				const row = Math.round(canopyTop - 1 - phase);
				if (row < 0) continue;
				const tint = hslToRGB((this.cfg.flame_hue + 14) % 360, clamp01(this.cfg.sat * 0.18), clamp01(0.18 + fade * 0.4));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${tint.r},${tint.g},${tint.b})`, clamp01(0.12 + fade * 0.45 * strength));
			}
		}

		_pickHealthy() {
			const list = [];
			for (let i = 0; i < this.states.length; i++) {
				if (this.states[i] === BTREE_STATE_ALIVE) list.push(i);
			}
			if (list.length === 0) return -1;
			return list[Math.floor(this.rng() * list.length)];
		}

		_ignite(idx) {
			if (idx < 0 || idx >= this.states.length) return;
			if (this.states[idx] !== BTREE_STATE_ALIVE) return;
			const dur = jitterInt(this.rng, this.cfg.ignite_dur, 0.25);
			this.states[idx] = BTREE_STATE_IGNITING;
			this.phaseLeft[idx] = dur;
			this.phaseTotal[idx] = dur;
		}

		_advanceTrees() {
			for (let i = 0; i < this.states.length; i++) {
				if (this.states[i] === BTREE_STATE_ALIVE) continue;
				if (this.phaseLeft[i] > 0) this.phaseLeft[i]--;
				if (this.phaseLeft[i] > 0) continue;
				switch (this.states[i]) {
					case BTREE_STATE_IGNITING: {
						const dur = jitterInt(this.rng, this.cfg.burn_dur, 0.2);
						this.states[i] = BTREE_STATE_BURNING;
						this.phaseLeft[i] = dur;
						this.phaseTotal[i] = dur;
						break;
					}
					case BTREE_STATE_BURNING: {
						const dur = jitterInt(this.rng, this.cfg.ash_dur, 0.25);
						this.states[i] = BTREE_STATE_ASHING;
						this.phaseLeft[i] = dur;
						this.phaseTotal[i] = dur;
						break;
					}
					case BTREE_STATE_ASHING:
						this.states[i] = BTREE_STATE_ASH;
						this.phaseLeft[i] = 0;
						this.phaseTotal[i] = 0;
						break;
				}
			}
		}

		_startIntro() {
			this.states.fill(BTREE_STATE_ALIVE);
			this.phaseLeft.fill(0);
			this.phaseTotal.fill(0);
			this.flareTicks = 0;
			this.flareGain = 1;
			this.lullTicks = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.introTotal = Math.max(1, this.cfg.intro_dur);
			this.introTicks = this.introTotal;
		}

		_startEnding() {
			this.flareTicks = 0;
			this.flareGain = 1;
			this.lullTicks = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = Math.max(1, this.cfg.ending_dur);
			this.endingTotal = Math.max(1, this.endingFade + Math.max(0, this.cfg.ending_linger));
			this.endingTicks = this.endingTotal;
		}

		_introProgress() {
			if (this.introTicks <= 0 || this.introTotal <= 0) return 1;
			const elapsed = this.introTotal - this.introTicks;
			return clamp01(elapsed / Math.max(1, this.introTotal - 1));
		}

		_endingProgress() {
			if (this.endingTicks <= 0 || this.endingTotal <= 0) return 0;
			const elapsed = this.endingTotal - this.endingTicks;
			return clamp01(elapsed / Math.max(1, this.endingTotal - 1));
		}

		_ashLingerLevel() {
			if (this.endingTicks <= 0) return 1;
			return clamp01(this.cfg.ending_ash + (1 - this.cfg.ending_ash) * (1 - this._endingProgress()));
		}

		_intensityLevel() {
			let level = 1;
			if (this.flareTicks > 0) level *= this.flareGain || this.cfg.flare_mult;
			if (this.lullTicks > 0) level *= this.cfg.lull_mult;
			if (this.endingTicks > 0) level *= 1 - this._endingProgress() * 0.85;
			if (this.introTicks > 0) level *= 0.4 + 0.6 * this._introProgress();
			return Math.max(0.05, level);
		}

		_burnEnvelope(idx) {
			if (this.phaseTotal[idx] <= 0) return 0;
			const progress = clamp01((this.phaseTotal[idx] - this.phaseLeft[idx]) / Math.max(1, this.phaseTotal[idx] - 1));
			switch (this.states[idx]) {
				case BTREE_STATE_IGNITING:
					return progress;
				case BTREE_STATE_BURNING:
					return Math.sin(progress * Math.PI) * 0.5 + 0.6;
				case BTREE_STATE_ASHING:
					return 0.6 * (1 - progress);
				default:
					return 0;
			}
		}

		_charProgress(state, burnEnv) {
			switch (state) {
				case BTREE_STATE_ALIVE: return 0;
				case BTREE_STATE_IGNITING: return Math.min(0.45, burnEnv * 0.45);
				case BTREE_STATE_BURNING: return clamp01(0.45 + burnEnv * 0.35);
				case BTREE_STATE_ASHING: return clamp01(0.8 + (1 - burnEnv) * 0.2);
				case BTREE_STATE_ASH: return 1;
				default: return 0;
			}
		}

		_canopyHash(treeIdx, dx, y, density) {
			const v = this._hash((treeIdx + 1) * 1009 + dx * 71 + y * 13);
			return v < density;
		}

		_anyBurning() {
			for (let i = 0; i < this.states.length; i++) {
				if (this.states[i] === BTREE_STATE_BURNING || this.states[i] === BTREE_STATE_IGNITING) return true;
			}
			return false;
		}

		_anyActiveFlame() {
			for (let i = 0; i < this.states.length; i++) {
				const s = this.states[i];
				if (s === BTREE_STATE_BURNING || s === BTREE_STATE_IGNITING || s === BTREE_STATE_ASHING) return true;
			}
			return false;
		}

		_treeNoise(idx) {
			return [
				this._hash(7000 + idx * 31),
				this._hash(7100 + idx * 31),
				this._hash(7200 + idx * 31),
				this._hash(7300 + idx * 31),
				this._hash(7400 + idx * 31),
				this._hash(7500 + idx * 31),
				this._hash(7600 + idx * 31),
			];
		}

		_hash(index) {
			const x = Math.sin((this.seed * 0.000001 + index * 12.9898) * 43758.5453);
			return x - Math.floor(x);
		}

		_fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, w, h, color, alpha) {
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), Math.max(1, Math.ceil(w * sx || ceilSx)), Math.max(1, Math.ceil(h * sy || ceilSy)));
			ctx.globalAlpha = 1;
		}
	}

	api.presets['burning-trees'] = [
		{
			key: 'single-ignition',
			label: 'single ignition',
			config: {
				tree_count: 8,
				tree_width: 7,
				tree_min_h: 8,
				tree_max_h: 14,
				baseline: 0.86,
				canopy: 0.62,
				ignite_dur: 28,
				burn_dur: 240,
				ash_dur: 90,
				spread_p: 0,
				flame_h: 8,
				flicker: 0.62,
				ember_rate: 0.22,
				glow: 0.42,
				smoke: 0.38,
				canopy_hue: 122,
				flame_hue: 22,
				hue_sp: 12,
				sat: 0.6,
				lmin: 0.18,
				lmax: 0.8,
				ignite_p: 0.0008,
				flare_p: 0.0006,
			},
		},
		{
			key: 'slow-spread',
			label: 'slow spread',
			config: {
				tree_count: 10,
				tree_width: 7,
				tree_min_h: 9,
				tree_max_h: 16,
				baseline: 0.86,
				canopy: 0.62,
				ignite_dur: 32,
				burn_dur: 220,
				ash_dur: 80,
				spread_p: 0.012,
				flame_h: 9,
				flicker: 0.7,
				ember_rate: 0.32,
				glow: 0.45,
				smoke: 0.45,
				canopy_hue: 118,
				flame_hue: 22,
				hue_sp: 14,
				sat: 0.62,
				lmin: 0.18,
				lmax: 0.82,
				ignite_p: 0.001,
				flare_p: 0.0008,
				lull_p: 0.0008,
			},
		},
		{
			key: 'smoldering-line',
			label: 'smoldering line',
			config: {
				tree_count: 12,
				tree_width: 6.5,
				tree_min_h: 7,
				tree_max_h: 13,
				baseline: 0.84,
				canopy: 0.5,
				ignite_dur: 40,
				burn_dur: 320,
				ash_dur: 140,
				spread_p: 0.006,
				flame_h: 6,
				flicker: 0.5,
				ember_rate: 0.42,
				glow: 0.35,
				smoke: 0.7,
				canopy_hue: 110,
				flame_hue: 18,
				hue_sp: 18,
				sat: 0.55,
				lmin: 0.16,
				lmax: 0.7,
				ignite_p: 0.0006,
				lull_p: 0.001,
				ending_ash: 0.55,
			},
		},
		{
			key: 'active-burn',
			label: 'active burn',
			config: {
				tree_count: 12,
				tree_width: 7,
				tree_min_h: 10,
				tree_max_h: 18,
				baseline: 0.86,
				canopy: 0.7,
				ignite_dur: 22,
				burn_dur: 180,
				ash_dur: 60,
				spread_p: 0.028,
				flame_h: 12,
				flicker: 0.95,
				ember_rate: 0.55,
				glow: 0.7,
				smoke: 0.5,
				canopy_hue: 116,
				flame_hue: 18,
				hue_sp: 22,
				sat: 0.78,
				lmin: 0.2,
				lmax: 0.92,
				ignite_p: 0.002,
				flare_p: 0.0018,
				flare_mult: 2.1,
			},
		},
	];
	api.effects['burning-trees'] = BurningTrees;
})(window.AmbienceSim);
