'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	const DEFAULTS = {
		intro_dur: 60,
		intro_glow: 0.18,
		ending_dur: 80,
		ending_linger: 30,
		ending_glow: 0.12,
		horizon: 0.92,
		bottle_top: 0.08,
		bottle_width: 0.32,
		bottle_curve: 0.55,
		neck_width: 0.18,
		blob_count: 4,
		blob_size: 6.5,
		rise_speed: 0.42,
		viscosity: 0.55,
		heat: 0.62,
		hue: 8,
		hue_sp: 14,
		sat: 0.82,
		lmin: 0.22,
		lmax: 0.88,
		bg_hue: 18,
		bg_sat: 0.32,
		bg_light: 0.16,
		rise_p: 0,
		merge_p: 0,
		split_p: 0,
		surface_pop_p: 0,
		quiet_p: 0,
		rise_dur: 80,
		rise_mult: 1.6,
		merge_dur: 50,
		split_dur: 50,
		surface_dur: 40,
		quiet_dur: 180,
		quiet_mult: 0.45,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		c.horizon = clamp01(c.horizon);
		if (c.bottle_top < 0) c.bottle_top = 0;
		c.bottle_top = clamp01(c.bottle_top);
		if (c.bottle_width <= 0) c.bottle_width = DEFAULTS.bottle_width;
		c.bottle_curve = clamp01(c.bottle_curve);
		if (c.neck_width <= 0) c.neck_width = DEFAULTS.neck_width;
		if (c.blob_count < 1) c.blob_count = 1;
		if (c.blob_size <= 0) c.blob_size = DEFAULTS.blob_size;
		if (c.rise_speed <= 0) c.rise_speed = DEFAULTS.rise_speed;
		if (c.viscosity <= 0) c.viscosity = DEFAULTS.viscosity;
		if (c.heat <= 0) c.heat = DEFAULTS.heat;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.bg_sat < 0) c.bg_sat = 0;
		if (c.bg_light <= 0) c.bg_light = DEFAULTS.bg_light;
		if (c.rise_dur <= 0) c.rise_dur = DEFAULTS.rise_dur;
		if (c.rise_mult <= 0) c.rise_mult = DEFAULTS.rise_mult;
		if (c.merge_dur <= 0) c.merge_dur = DEFAULTS.merge_dur;
		if (c.split_dur <= 0) c.split_dur = DEFAULTS.split_dur;
		if (c.surface_dur <= 0) c.surface_dur = DEFAULTS.surface_dur;
		if (c.quiet_dur <= 0) c.quiet_dur = DEFAULTS.quiet_dur;
		if (c.quiet_mult <= 0) c.quiet_mult = DEFAULTS.quiet_mult;
		return c;
	}

	class LavaLamp {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.seed = Number(seed || Date.now()) >>> 0;
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
			if (typeof snap.seed === 'number') this.seed = snap.seed >>> 0;
			if (snap.gridW > 0 && snap.gridH > 0) {
				this.w = snap.gridW;
				this.h = snap.gridH;
			}
		}

		_eventRng(salt) {
			return makeRNG(((this.seed >>> 0) ^ (((this.tick + salt) * 2654435761) >>> 0)) >>> 0);
		}

		_phaseProgress(total, left) {
			if (left <= 1 || total <= 1) return 1;
			const elapsed = total - left;
			if (elapsed <= 0) return 0;
			return clamp01(elapsed / Math.max(1, total - 1));
		}

		_hash(index) {
			const x = Math.sin((this.seed * 0.000001 + index * 12.9898) * 43758.5453);
			return x - Math.floor(x);
		}

		triggerEvent(name) {
			const rng = this._eventRng(name.length + 53);
			const count = Math.max(1, Math.round(this.cfg.blob_count));
			switch (name) {
				case 'blob-rise':
					this.timers.rise = jitterInt(rng, this.cfg.rise_dur, 0.3);
					this.values.rise_gain = this.cfg.rise_mult * (0.85 + rng() * 0.3);
					this.values.rise_slot = Math.floor(rng() * count);
					return true;
				case 'blob-merge':
					this.timers.merge = jitterInt(rng, this.cfg.merge_dur, 0.3);
					this.values.merge_slot = Math.floor(rng() * count);
					return true;
				case 'blob-split':
					this.timers.split = jitterInt(rng, this.cfg.split_dur, 0.3);
					this.values.split_slot = Math.floor(rng() * count);
					return true;
				case 'surface-pop':
					this.timers.surface = jitterInt(rng, this.cfg.surface_dur, 0.3);
					this.values.surface_slot = Math.floor(rng() * count);
					return true;
				case 'quiet-flow':
					this.timers.quiet = jitterInt(rng, this.cfg.quiet_dur, 0.2);
					return true;
				case 'intro':
					this.timers = { intro: Math.max(1, Math.round(this.cfg.intro_dur)) };
					this.values = { intro_total: this.timers.intro };
					return true;
				case 'ending': {
					const total = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.timers = { ending: total };
					this.values = { ending_total: total };
					return true;
				}
			}
			return false;
		}

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (!this.timers.rise || this.timers.rise <= 0) this.values.rise_gain = 1;
		}

		// Returns the current motion multiplier based on lifecycle and quiet timers.
		// 0..1 during intro warm-up, 0..1 fading during ending; quiet_mult during
		// quiet windows; otherwise 1.
		_motion() {
			let m = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				m *= this.cfg.intro_glow + (1 - this.cfg.intro_glow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				m *= 1 - (1 - this.cfg.ending_glow) * progress;
			}
			if (this.timers.quiet > 0) m *= this.cfg.quiet_mult;
			return clamp01(m);
		}

		_heatLevel() {
			let m = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				m *= this.cfg.intro_glow + (1 - this.cfg.intro_glow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				m *= 1 - (1 - this.cfg.ending_glow) * progress;
			}
			return clamp01(m);
		}

		// Returns (centerX, halfWidth) for a given row. Outside the bottle returns
		// halfWidth <= 0. The silhouette is a smooth bottle: cap on top, narrow
		// neck, then a curved bulge belly to the base.
		_bottleAt(row, capRow, baseRow, centerX, neckHalf, bellyHalf) {
			if (row < capRow || row > baseRow) return { center: centerX, half: -1 };
			const span = Math.max(1, baseRow - capRow);
			const t = (row - capRow) / span; // 0 at top, 1 at base
			// Two-stage curve: shoulder (narrow neck) and belly (full width).
			// Smoothstep from neck to belly across the upper third.
			const shoulder = clamp01(t / 0.32); // 0 at neck, 1 by ~⅓ down
			const ease = shoulder * shoulder * (3 - 2 * shoulder);
			let half = neckHalf + (bellyHalf - neckHalf) * ease;
			// Slight pinch near the base for a bottle foot.
			if (t > 0.92) {
				const k = (t - 0.92) / 0.08;
				half *= 1 - 0.18 * k * k;
			}
			return { center: centerX, half };
		}

		_blobs() {
			const count = Math.max(1, Math.min(8, Math.round(this.cfg.blob_count)));
			const out = [];
			const motion = this._motion();
			const speed = this.cfg.rise_speed * motion;
			const visc = this.cfg.viscosity;
			// Settle target during ending — blobs gradually pulled toward the base.
			let endingPull = 0;
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				endingPull = this._phaseProgress(total, this.timers.ending);
			}
			for (let i = 0; i < count; i++) {
				const phase = (i / count) * Math.PI * 2 + this._hash(800 + i) * Math.PI * 2;
				const period = 6.2 + this._hash(900 + i) * 4.2;
				// y oscillates from near top (-1) to near bottom (+1). Higher
				// viscosity dampens overshoot so the curve looks gooey.
				const wobble = Math.sin(this.tick * 0.014 * speed + phase) * (1.04 - 0.18 * visc);
				let y = wobble + Math.sin(this.tick * 0.04 * speed * 0.5 + phase * 1.7) * 0.18;
				// Each blob spends more time near the ends — emphasize tails.
				y = y >= 0
					? Math.pow(y, 0.85)
					: -Math.pow(-y, 0.85);
				let x = Math.sin(this.tick * 0.022 * speed + phase * 0.7) * 0.55;
				let size = this.cfg.blob_size * (0.85 + this._hash(1100 + i) * 0.4);

				// Active rise event: chosen blob is pushed faster toward the top.
				if (this.timers.rise > 0 && this.values.rise_slot === i) {
					const total = Math.max(1, Math.round(this.cfg.rise_dur));
					const progress = this._phaseProgress(total, this.timers.rise);
					const surge = Math.sin(progress * Math.PI); // 0..1..0
					const gain = this.values.rise_gain || this.cfg.rise_mult;
					y -= surge * 0.95 * (gain - 1) * 0.6;
					size *= 1 + surge * 0.18;
				}
				// Surface pop: blob flattens at the top before sinking again.
				if (this.timers.surface > 0 && this.values.surface_slot === i) {
					const total = Math.max(1, Math.round(this.cfg.surface_dur));
					const progress = this._phaseProgress(total, this.timers.surface);
					const flatten = Math.sin(progress * Math.PI);
					y = -1.0 + (1 - flatten) * (y + 1.0); // pulled up
					size *= 1 + 0.12 * flatten;
				}
				// Split: chosen blob pulses outward in size before normalizing.
				if (this.timers.split > 0 && this.values.split_slot === i) {
					const total = Math.max(1, Math.round(this.cfg.split_dur));
					const progress = this._phaseProgress(total, this.timers.split);
					const pulse = Math.sin(progress * Math.PI);
					size *= 1 + 0.35 * pulse;
				}
				// Merge: target blob inflates and pulls toward the merge_slot+1
				// position (so a "neighbor" appears to drift in).
				if (this.timers.merge > 0 && this.values.merge_slot === i) {
					const total = Math.max(1, Math.round(this.cfg.merge_dur));
					const progress = this._phaseProgress(total, this.timers.merge);
					const pulse = Math.sin(progress * Math.PI);
					size *= 1 + 0.45 * pulse;
				}
				// Ending: pull every blob toward the base so the lamp settles.
				if (endingPull > 0) {
					y = y * (1 - endingPull) + 0.95 * endingPull;
					size *= 1 - 0.25 * endingPull;
				}

				out.push({
					i,
					x: clamp01((x + 1) * 0.5) * 2 - 1, // keep in [-1,1]
					y: Math.max(-1.05, Math.min(1.05, y)),
					size,
					period,
				});
			}
			return out;
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#0a0612');
				sky.addColorStop(0.55, '#0c0810');
				sky.addColorStop(1, '#100a12');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.max(1, Math.ceil(sx));
			const ceilSy = Math.max(1, Math.ceil(sy));

			const capRow = Math.max(2, Math.round(this.h * this.cfg.bottle_top));
			const baseRow = Math.min(this.h - 2, Math.round(this.h * this.cfg.horizon));
			const centerX = Math.floor(this.w * 0.5);
			const bellyHalf = Math.max(4, Math.round(this.cfg.bottle_width * this.w));
			const neckHalf = Math.max(2, Math.round(this.cfg.neck_width * this.w * 0.5));
			const interiorTop = capRow + 2;
			const interiorBot = baseRow - 1;

			const heat = this._heatLevel();

			// Bottle silhouette: walls + cap + base.
			const wallHue = (this.cfg.hue + 340) % 360;
			const wall = hslToRGB(wallHue, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.55));
			const wallEdge = hslToRGB(wallHue, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.85));
			const cap = hslToRGB((this.cfg.hue + 30) % 360, clamp01(this.cfg.sat * 0.4), clamp01(this.cfg.lmin * 1.1));

			// Cap (top piece slightly wider than the neck).
			const capW = neckHalf + 2;
			for (let dx = -capW; dx <= capW; dx++) {
				const col = centerX + dx;
				if (col < 0 || col >= this.w) continue;
				for (let y = Math.max(0, capRow - 3); y < capRow; y++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1,
						`rgb(${cap.r},${cap.g},${cap.b})`, 1);
				}
			}

			// Bottle base disk (a soft ellipse beneath the silhouette).
			for (let dx = -bellyHalf - 2; dx <= bellyHalf + 2; dx++) {
				const col = centerX + dx;
				if (col < 0 || col >= this.w) continue;
				for (let y = baseRow; y < this.h; y++) {
					const ratio = (y - baseRow) / Math.max(1, this.h - baseRow);
					const ground = hslToRGB((this.cfg.hue + 350) % 360,
						clamp01(this.cfg.sat * 0.16),
						clamp01(this.cfg.lmin * (0.32 + ratio * 0.4)));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1,
						`rgb(${ground.r},${ground.g},${ground.b})`, 1);
				}
			}

			// Bottle interior fill (dim warm liquid). Walk each row inside the
			// silhouette and paint the interior color, then paint the wall edges.
			const bgHue = ((this.cfg.bg_hue % 360) + 360) % 360;
			for (let y = capRow; y <= baseRow; y++) {
				const { half } = this._bottleAt(y, capRow, baseRow, centerX, neckHalf, bellyHalf);
				if (half <= 0) continue;
				const halfPix = Math.max(1, Math.round(half));
				// Vertical gradient — slightly darker at the bottom for depth.
				const t = (y - capRow) / Math.max(1, baseRow - capRow);
				const interiorLight = clamp01(this.cfg.bg_light * (1.05 - 0.18 * t));
				const interior = hslToRGB(bgHue, clamp01(this.cfg.bg_sat),
					interiorLight);
				for (let dx = -halfPix; dx <= halfPix; dx++) {
					const col = centerX + dx;
					if (col < 0 || col >= this.w) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1,
						`rgb(${interior.r},${interior.g},${interior.b})`, 1);
				}
				// Wall edges.
				const leftCol = centerX - halfPix;
				const rightCol = centerX + halfPix;
				if (leftCol - 1 >= 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, leftCol - 1, y, 1, 1,
						`rgb(${wall.r},${wall.g},${wall.b})`, 1);
				}
				if (rightCol + 1 < this.w) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, rightCol + 1, y, 1, 1,
						`rgb(${wall.r},${wall.g},${wall.b})`, 1);
				}
				if (leftCol >= 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, leftCol, y, 1, 1,
						`rgb(${wallEdge.r},${wallEdge.g},${wallEdge.b})`, clamp01(0.45));
				}
				if (rightCol < this.w) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, rightCol, y, 1, 1,
						`rgb(${wallEdge.r},${wallEdge.g},${wallEdge.b})`, clamp01(0.45));
				}
			}

			// Heat glow — radial gradient at the bottle base.
			const glowR = Math.max(canvasW, canvasH) * (0.18 + 0.18 * this.cfg.heat * heat);
			const glowX = centerX * sx;
			const glowY = (baseRow - 1) * sy;
			const glowGrad = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			const glowCore = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat),
				clamp01(this.cfg.lmax * (0.55 + 0.4 * this.cfg.heat * heat)));
			const glowOuter = hslToRGB((this.cfg.hue + 350) % 360, clamp01(this.cfg.sat * 0.7),
				clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.4));
			glowGrad.addColorStop(0, `rgba(${glowCore.r},${glowCore.g},${glowCore.b},${0.45 * heat + 0.18})`);
			glowGrad.addColorStop(0.5, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},${0.18 * heat})`);
			glowGrad.addColorStop(1, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},0)`);
			ctx.fillStyle = glowGrad;
			ctx.fillRect(0, (baseRow - 6) * sy, canvasW, canvasH - (baseRow - 6) * sy);

			// Blobs: metaball-style field across interior cells.
			const blobs = this._blobs();
			const interiorH = Math.max(1, interiorBot - interiorTop);
			// Interior column extents per blob (in cell space).
			const blobCells = blobs.map((b) => {
				const blobYrow = interiorTop + (b.y + 1) * 0.5 * interiorH;
				const { half } = this._bottleAt(Math.round(blobYrow), capRow, baseRow, centerX, neckHalf, bellyHalf);
				const usableHalf = Math.max(2, half - 1);
				const blobX = centerX + b.x * usableHalf * 0.6;
				return {
					x: blobX,
					y: blobYrow,
					r: b.size,
					i: b.i,
				};
			});

			const threshold = 0.85;
			for (let y = interiorTop; y <= interiorBot; y++) {
				const { half } = this._bottleAt(y, capRow, baseRow, centerX, neckHalf, bellyHalf);
				if (half <= 0) continue;
				const halfPix = Math.max(1, Math.round(half) - 1);
				for (let dx = -halfPix; dx <= halfPix; dx++) {
					const col = centerX + dx;
					if (col < 0 || col >= this.w) continue;
					let field = 0;
					let hueAccum = 0;
					let hueWeight = 0;
					for (const b of blobCells) {
						const dy = y - b.y;
						const ddx = col - b.x;
						const dsq = ddx * ddx + dy * dy + 0.001;
						const r2 = b.r * b.r;
						const contrib = r2 / dsq;
						field += contrib;
						const blobHue = (this.cfg.hue + (this._hash(2300 + b.i) * 2 - 1) * this.cfg.hue_sp) % 360;
						hueAccum += contrib * blobHue;
						hueWeight += contrib;
					}
					if (field <= threshold) continue;
					const intensity = clamp01((field - threshold) / 1.4);
					const hueAvg = hueWeight > 0 ? hueAccum / hueWeight : this.cfg.hue;
					const heatBoost = 1 - clamp01((y - interiorTop) / interiorH);
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) *
						(0.45 + 0.5 * intensity + 0.18 * heatBoost * this.cfg.heat * heat));
					const blob = hslToRGB(((hueAvg % 360) + 360) % 360,
						clamp01(this.cfg.sat * (0.95 + 0.05 * intensity)),
						light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1,
						`rgb(${blob.r},${blob.g},${blob.b})`, 1);
				}
			}

			// Subtle vertical highlight — a one-cell column near the left edge of
			// the belly so the bottle reads as glass instead of a flat strip.
			for (let y = capRow + 2; y <= baseRow - 1; y++) {
				const { half } = this._bottleAt(y, capRow, baseRow, centerX, neckHalf, bellyHalf);
				if (half <= 0) continue;
				const col = centerX - Math.max(1, Math.round(half) - 1);
				if (col < 0 || col >= this.w) continue;
				const t = (y - capRow) / Math.max(1, baseRow - capRow);
				const alpha = clamp01(0.2 + 0.25 * (1 - t));
				const hi = hslToRGB((this.cfg.hue + 30) % 360,
					clamp01(this.cfg.sat * 0.25),
					clamp01(this.cfg.lmax * 0.7));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, y, 1, 1,
					`rgb(${hi.r},${hi.g},${hi.b})`, alpha);
			}
		}

		_fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, w, h, color, alpha) {
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy),
				Math.max(1, Math.ceil(w * sx || ceilSx)),
				Math.max(1, Math.ceil(h * sy || ceilSy)));
			ctx.globalAlpha = 1;
		}
	}

	api.presets['lava-lamp'] = [
		{
			key: 'classic-red',
			label: 'classic red',
			config: {
				horizon: 0.92,
				bottle_top: 0.08,
				bottle_width: 0.32,
				bottle_curve: 0.55,
				neck_width: 0.18,
				blob_count: 4,
				blob_size: 6.5,
				rise_speed: 0.42,
				viscosity: 0.55,
				heat: 0.62,
				hue: 8,
				hue_sp: 12,
				sat: 0.85,
				lmin: 0.22,
				lmax: 0.9,
				bg_hue: 22,
				bg_sat: 0.32,
				bg_light: 0.16,
				rise_p: 0.0008,
				merge_p: 0.0004,
				split_p: 0.0003,
				surface_pop_p: 0.0006,
				quiet_p: 0.0002,
			},
		},
		{
			key: 'cool-blue',
			label: 'cool blue',
			config: {
				horizon: 0.92,
				bottle_top: 0.08,
				bottle_width: 0.3,
				bottle_curve: 0.55,
				neck_width: 0.18,
				blob_count: 4,
				blob_size: 6.0,
				rise_speed: 0.36,
				viscosity: 0.6,
				heat: 0.5,
				hue: 210,
				hue_sp: 18,
				sat: 0.7,
				lmin: 0.2,
				lmax: 0.86,
				bg_hue: 220,
				bg_sat: 0.28,
				bg_light: 0.14,
				rise_p: 0.0006,
				merge_p: 0.0003,
				split_p: 0.0002,
				surface_pop_p: 0.0006,
				quiet_p: 0.0003,
			},
		},
		{
			key: 'green-goo',
			label: 'green goo',
			config: {
				horizon: 0.92,
				bottle_top: 0.08,
				bottle_width: 0.34,
				bottle_curve: 0.5,
				neck_width: 0.2,
				blob_count: 5,
				blob_size: 7.0,
				rise_speed: 0.4,
				viscosity: 0.7,
				heat: 0.55,
				hue: 110,
				hue_sp: 22,
				sat: 0.78,
				lmin: 0.22,
				lmax: 0.84,
				bg_hue: 130,
				bg_sat: 0.34,
				bg_light: 0.15,
				rise_p: 0.001,
				merge_p: 0.0006,
				split_p: 0.0004,
				surface_pop_p: 0.0008,
				quiet_p: 0.0002,
			},
		},
		{
			key: 'slow-drift',
			label: 'slow drift',
			config: {
				horizon: 0.93,
				bottle_top: 0.09,
				bottle_width: 0.3,
				bottle_curve: 0.6,
				neck_width: 0.16,
				blob_count: 3,
				blob_size: 7.5,
				rise_speed: 0.22,
				viscosity: 0.85,
				heat: 0.5,
				hue: 32,
				hue_sp: 16,
				sat: 0.78,
				lmin: 0.22,
				lmax: 0.84,
				bg_hue: 24,
				bg_sat: 0.3,
				bg_light: 0.15,
				rise_p: 0.0003,
				merge_p: 0.0002,
				split_p: 0.0001,
				surface_pop_p: 0.0003,
				quiet_p: 0.0008,
				quiet_dur: 280,
				quiet_mult: 0.35,
			},
		},
	];
	api.effects['lava-lamp'] = LavaLamp;
})(window.AmbienceSim);
