'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB, positiveMod } = api._helpers;

	const DEFAULTS = {
		intro_dur: 60,
		intro_glow: 0.14,
		ending_dur: 70,
		ending_linger: 24,
		ending_glow: 0.10,
		bottle_x: 0.5,
		bottle_top: 0.10,
		bottle_bottom: 0.86,
		bottle_width: 34,
		neck_width: 12,
		base_height: 6,
		blob_count: 4,
		rise_speed: 0.018,
		viscosity: 0.55,
		min_radius: 3.0,
		max_radius: 6.5,
		glow: 0.62,
		hue: 8,
		hue_sp: 18,
		sat: 0.78,
		lmin: 0.18,
		lmax: 0.92,
		rise_p: 0,
		merge_p: 0,
		split_p: 0,
		surface_pop_p: 0,
		quiet_flow_p: 0,
		rise_dur: 60,
		merge_dur: 20,
		split_dur: 20,
		surface_pop_dur: 18,
		quiet_flow_dur: 140,
		quiet_flow_mult: 0.35,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.bottle_x <= 0) c.bottle_x = DEFAULTS.bottle_x;
		c.bottle_x = clamp01(c.bottle_x);
		if (c.bottle_top <= 0) c.bottle_top = DEFAULTS.bottle_top;
		c.bottle_top = clamp01(c.bottle_top);
		if (c.bottle_bottom <= 0) c.bottle_bottom = DEFAULTS.bottle_bottom;
		c.bottle_bottom = clamp01(c.bottle_bottom);
		if (c.bottle_bottom <= c.bottle_top + 0.05) {
			c.bottle_bottom = Math.min(0.96, c.bottle_top + 0.4);
		}
		if (c.bottle_width <= 0) c.bottle_width = DEFAULTS.bottle_width;
		if (c.neck_width <= 0) c.neck_width = DEFAULTS.neck_width;
		if (c.neck_width > c.bottle_width) c.neck_width = c.bottle_width;
		if (c.base_height <= 0) c.base_height = DEFAULTS.base_height;
		if (c.blob_count < 1) c.blob_count = 1;
		if (c.rise_speed <= 0) c.rise_speed = DEFAULTS.rise_speed;
		if (c.viscosity <= 0) c.viscosity = DEFAULTS.viscosity;
		c.viscosity = clamp01(c.viscosity);
		if (c.min_radius <= 0) c.min_radius = DEFAULTS.min_radius;
		if (c.max_radius <= 0) c.max_radius = DEFAULTS.max_radius;
		if (c.max_radius < c.min_radius) [c.min_radius, c.max_radius] = [c.max_radius, c.min_radius];
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.hue < 0) c.hue = DEFAULTS.hue;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.rise_dur <= 0) c.rise_dur = DEFAULTS.rise_dur;
		if (c.merge_dur <= 0) c.merge_dur = DEFAULTS.merge_dur;
		if (c.split_dur <= 0) c.split_dur = DEFAULTS.split_dur;
		if (c.surface_pop_dur <= 0) c.surface_pop_dur = DEFAULTS.surface_pop_dur;
		if (c.quiet_flow_dur <= 0) c.quiet_flow_dur = DEFAULTS.quiet_flow_dur;
		if (c.quiet_flow_mult <= 0) c.quiet_flow_mult = DEFAULTS.quiet_flow_mult;
		c.quiet_flow_mult = clamp01(c.quiet_flow_mult);
		return c;
	}

	class LavaLamp {
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

		triggerEvent(name) {
			const rng = this._eventRng(name.length + 53);
			const blobCount = Math.max(1, Math.round(this.cfg.blob_count));
			switch (name) {
				case 'blob-rise':
					this.timers.rise = jitterInt(rng, this.cfg.rise_dur, 0.3);
					this.values.rise_blob = rng.intn(blobCount);
					this.values.rise_seed = rng() * 1024;
					return true;
				case 'blob-merge':
					this.timers.merge = jitterInt(rng, this.cfg.merge_dur, 0.3);
					this.values.merge_a = rng.intn(blobCount);
					this.values.merge_b = (this.values.merge_a + 1 + rng.intn(Math.max(1, blobCount - 1))) % blobCount;
					this.values.merge_seed = rng() * 1024;
					return true;
				case 'blob-split':
					this.timers.split = jitterInt(rng, this.cfg.split_dur, 0.3);
					this.values.split_blob = rng.intn(blobCount);
					this.values.split_seed = rng() * 1024;
					return true;
				case 'surface-pop':
					this.timers.surface_pop = jitterInt(rng, this.cfg.surface_pop_dur, 0.3);
					this.values.surface_pop_blob = rng.intn(blobCount);
					this.values.surface_pop_seed = rng() * 1024;
					return true;
				case 'quiet-flow':
					this.timers.quiet_flow = jitterInt(rng, this.cfg.quiet_flow_dur, 0.3);
					this.values.quiet_flow_mult = this.cfg.quiet_flow_mult;
					return true;
				case 'intro':
					this.timers.rise = 0;
					this.timers.merge = 0;
					this.timers.split = 0;
					this.timers.surface_pop = 0;
					this.timers.quiet_flow = 0;
					this.timers.ending = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.rise = 0;
					this.timers.merge = 0;
					this.timers.split = 0;
					this.timers.surface_pop = 0;
					this.timers.quiet_flow = 0;
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
			if (!this.timers.quiet_flow || this.timers.quiet_flow <= 0) delete this.values.quiet_flow_mult;
		}

		// Bottle silhouette: half-width at vertical fraction t (0=top, 1=bottom).
		// Smooth s-curve from neck_width at the top to bottle_width at the
		// shoulder, holding bottle_width through the body and rounding at the
		// base. This is what gives the recognizable lamp profile.
		_bottleHalfWidth(t) {
			t = clamp01(t);
			const neckHalf = Math.max(2, this.cfg.neck_width * 0.5);
			const bodyHalf = Math.max(neckHalf, this.cfg.bottle_width * 0.5);
			// Top section (0..0.18): cap rounding from a slightly narrower top to neckHalf.
			// Shoulder (0.18..0.34): smooth transition from neckHalf to bodyHalf.
			// Body (0.34..0.85): hold at bodyHalf with a gentle bell-curve bulge.
			// Base round (0.85..1): taper back to ~80% bodyHalf for the pedestal seam.
			let half;
			if (t < 0.05) {
				const u = t / 0.05;
				half = neckHalf * (0.55 + 0.45 * u);
			} else if (t < 0.22) {
				half = neckHalf;
			} else if (t < 0.4) {
				const u = (t - 0.22) / 0.18;
				const ease = u * u * (3 - 2 * u);
				half = neckHalf + (bodyHalf - neckHalf) * ease;
			} else if (t < 0.85) {
				const bellT = (t - 0.4) / 0.45;
				const bell = 1 - 0.04 * Math.cos(bellT * Math.PI);
				half = bodyHalf * bell;
			} else {
				const u = (t - 0.85) / 0.15;
				const ease = u * u * (3 - 2 * u);
				half = bodyHalf - (bodyHalf - bodyHalf * 0.78) * ease;
			}
			return half;
		}

		_bottleBounds() {
			const top = Math.max(2, Math.round(this.cfg.bottle_top * (this.h - 1)));
			const bottom = Math.max(top + 6, Math.round(this.cfg.bottle_bottom * (this.h - 1)));
			const center = Math.round(this.cfg.bottle_x * (this.w - 1));
			return { top, bottom, center };
		}

		_pressure() {
			let level = 1;
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
			if (this.timers.quiet_flow > 0) {
				level *= 0.7;
			}
			return Math.max(0.05, level);
		}

		// Speed multiplier for blob motion. Quiet flow slows the cycle;
		// intro/ending suppress motion to a near-still state.
		_speedScale() {
			let s = 1;
			if (this.timers.quiet_flow > 0) {
				const mult = this.values.quiet_flow_mult || this.cfg.quiet_flow_mult;
				s *= mult;
			}
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				s *= 0.15 + 0.85 * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				s *= 1 - 0.9 * progress;
			}
			return Math.max(0.02, s);
		}

		// Blob i's deterministic state at the current tick. Returns {x, y, r, hue, alive}.
		// Position is a smooth rise-and-fall along the bottle interior. Each blob has
		// its own phase, cycle period, x-offset and radius derived from seed+i. The
		// motion is a slow cosine arc — viscous, never bouncy.
		_blob(i, blobCount, bounds) {
			const speed = this._speedScale();
			const cycleBase = 1 / Math.max(0.001, this.cfg.rise_speed);
			const cycle = cycleBase * (0.7 + this._hash(70 + i) * 0.6) * (1 + this.cfg.viscosity * 0.6);
			const phase = this._hash(140 + i) * Math.PI * 2;
			const t = (this.tick * speed) / cycle * Math.PI * 2 + phase;
			// Gentle stagger so the lamp doesn't pulse in unison
			const stagger = (i / Math.max(1, blobCount)) * Math.PI * 2;
			const arc = 0.5 - 0.5 * Math.cos(t + stagger);

			// Vertical bounds — keep blobs within the bottle interior.
			const margin = 2;
			const innerTop = bounds.top + 4;
			const innerBottom = bounds.bottom - this.cfg.base_height - margin;
			const span = Math.max(8, innerBottom - innerTop);
			let y = innerBottom - arc * span;

			// Per-blob horizontal drift, constrained so we don't punch through walls.
			const driftFreq = 0.02 + this._hash(210 + i) * 0.04;
			const driftPhase = this._hash(280 + i) * Math.PI * 2;
			const driftAmt = (this._hash(350 + i) * 2 - 1) * 0.55;

			// Surface pop: when fired against this blob, push it down and flatten it.
			let popFlatten = 0;
			let popHueBoost = 0;
			let popOffset = 0;
			if (this.timers.surface_pop > 0 && Math.round(this.values.surface_pop_blob || 0) === i) {
				const total = Math.max(1, Math.round(this.cfg.surface_pop_dur));
				const progress = this._phaseProgress(total, this.timers.surface_pop);
				popFlatten = Math.sin(progress * Math.PI) * 0.4;
				popHueBoost = (1 - progress) * 0.2;
				popOffset = -Math.sin(progress * Math.PI) * 4;
				y += popOffset;
			}

			// Rise highlight: brighten the chosen blob during a rise event.
			let riseGlow = 0;
			if (this.timers.rise > 0 && Math.round(this.values.rise_blob || 0) === i) {
				const total = Math.max(1, Math.round(this.cfg.rise_dur));
				const progress = this._phaseProgress(total, this.timers.rise);
				riseGlow = Math.sin(progress * Math.PI) * 0.45;
			}

			// Compute x by sampling bottle width at this y, then offsetting.
			const tFrac = clamp01((y - bounds.top) / Math.max(1, bounds.bottom - bounds.top));
			const half = this._bottleHalfWidth(tFrac);
			const wallSlack = Math.max(1, half - 2.5);
			const xOffset = Math.sin(this.tick * driftFreq * speed + driftPhase) * driftAmt * wallSlack;
			const x = bounds.center + xOffset;

			// Radius: stable per-blob value pulsing slightly with the cycle so the
			// lamp visually breathes. Merge events temporarily inflate one of the
			// pair; split events temporarily shrink.
			const baseR = this.cfg.min_radius + this._hash(420 + i) * (this.cfg.max_radius - this.cfg.min_radius);
			let r = baseR + Math.sin(t * 1.05) * 0.35;
			if (this.timers.merge > 0) {
				const a = Math.round(this.values.merge_a || 0);
				const b = Math.round(this.values.merge_b || 0);
				if (i === a || i === b) {
					const total = Math.max(1, Math.round(this.cfg.merge_dur));
					const progress = this._phaseProgress(total, this.timers.merge);
					const env = Math.sin(progress * Math.PI);
					r += env * (i === a ? 1.6 : -1.0);
				}
			}
			if (this.timers.split > 0 && Math.round(this.values.split_blob || 0) === i) {
				const total = Math.max(1, Math.round(this.cfg.split_dur));
				const progress = this._phaseProgress(total, this.timers.split);
				const env = Math.sin(progress * Math.PI);
				r -= env * 0.9;
			}
			if (popFlatten > 0) {
				r += popFlatten * 0.8;
			}
			if (r < 1.2) r = 1.2;

			const hueShift = (this._hash(490 + i) * 2 - 1) * this.cfg.hue_sp;
			const hue = ((this.cfg.hue + hueShift + popHueBoost * this.cfg.hue_sp + 360) % 360 + 360) % 360;

			return { x, y, r, hue, riseGlow, popFlatten };
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const bg = ctx.createLinearGradient(0, 0, 0, canvasH);
				bg.addColorStop(0, '#08060a');
				bg.addColorStop(1, '#120c0e');
				ctx.fillStyle = bg;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const bounds = this._bottleBounds();
			const pressure = this._pressure();
			const blobCount = Math.max(1, Math.round(this.cfg.blob_count));

			// 1. Pedestal / heat-source slab beneath the bottle.
			const baseTop = bounds.bottom - Math.max(1, Math.round(this.cfg.base_height));
			const pedestalRow0 = bounds.bottom + 1;
			const pedestalRow1 = Math.min(this.h - 1, pedestalRow0 + 3);
			const pedestalCol = (() => {
				const half = Math.round(this._bottleHalfWidth(1) + 4);
				return { left: bounds.center - half, right: bounds.center + half };
			})();
			for (let y = pedestalRow0; y <= pedestalRow1; y++) {
				const fade = (y - pedestalRow0) / Math.max(1, pedestalRow1 - pedestalRow0);
				const c = hslToRGB(((this.cfg.hue + 350) % 360 + 360) % 360,
					clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * (0.55 - fade * 0.25)));
				for (let x = pedestalCol.left; x <= pedestalCol.right; x++) {
					if (x < 0 || x >= this.w) continue;
					ctx.fillStyle = `rgb(${c.r},${c.g},${c.b})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// 2. Bottle interior (dim warm liquid bg) — render as filled rows
			//    using the smooth half-width function.
			const interiorHue = ((this.cfg.hue + 350) % 360 + 360) % 360;
			for (let y = bounds.top; y <= bounds.bottom; y++) {
				const t = (y - bounds.top) / Math.max(1, bounds.bottom - bounds.top);
				const half = this._bottleHalfWidth(t);
				const left = Math.floor(bounds.center - half + 0.5);
				const right = Math.ceil(bounds.center + half - 0.5);
				const verticalGrad = (y - bounds.top) / Math.max(1, bounds.bottom - bounds.top);
				const baseLight = clamp01(this.cfg.lmin * (0.45 + verticalGrad * 0.55) * pressure);
				const liq = hslToRGB(interiorHue, clamp01(this.cfg.sat * 0.4), baseLight);
				for (let x = left; x <= right; x++) {
					if (x < 0 || x >= this.w) continue;
					ctx.fillStyle = `rgb(${liq.r},${liq.g},${liq.b})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// 3. Heat-source warm glow at the base — radial gradient.
			const glowStrength = clamp01(this.cfg.glow * pressure);
			const glowR = Math.max(20, Math.min(canvasW, canvasH) * (0.08 + glowStrength * 0.18));
			const glowX = bounds.center * sx;
			const glowY = baseTop * sy;
			const glowGrad = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			const glowCore = hslToRGB(((this.cfg.hue + 4) % 360 + 360) % 360,
				clamp01(this.cfg.sat * 0.95),
				clamp01(this.cfg.lmax * (0.7 + glowStrength * 0.25)));
			const glowOuter = hslToRGB(((this.cfg.hue + 350) % 360 + 360) % 360,
				clamp01(this.cfg.sat),
				clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.35));
			glowGrad.addColorStop(0, `rgba(${glowCore.r},${glowCore.g},${glowCore.b},${0.4 + glowStrength * 0.4})`);
			glowGrad.addColorStop(0.45, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},${0.18 + glowStrength * 0.22})`);
			glowGrad.addColorStop(1, `rgba(${glowOuter.r},${glowOuter.g},${glowOuter.b},0)`);
			ctx.fillStyle = glowGrad;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			// 4. Compute blob list + render via metaball field within the bottle.
			const blobs = [];
			for (let i = 0; i < blobCount; i++) {
				blobs.push(this._blob(i, blobCount, bounds));
			}

			// Iterate every pixel in the bottle interior, summing field values.
			// Pixels with field >= 1 belong to a blob; we tint by the dominant
			// blob's hue for that pixel.
			for (let y = bounds.top; y <= bounds.bottom; y++) {
				const tFrac = (y - bounds.top) / Math.max(1, bounds.bottom - bounds.top);
				const half = this._bottleHalfWidth(tFrac);
				const left = Math.floor(bounds.center - half + 0.5);
				const right = Math.ceil(bounds.center + half - 0.5);
				for (let x = left; x <= right; x++) {
					if (x < 0 || x >= this.w) continue;
					let field = 0;
					let bestI = -1;
					let bestContribution = 0;
					for (let i = 0; i < blobs.length; i++) {
						const b = blobs[i];
						const dx = x - b.x;
						const dy = y - b.y;
						const d2 = dx * dx + dy * dy + 0.001;
						const contrib = (b.r * b.r) / d2;
						field += contrib;
						if (contrib > bestContribution) {
							bestContribution = contrib;
							bestI = i;
						}
					}
					if (field < 0.55) continue;
					const blob = blobs[bestI];
					// Height-based brightness — top of the bottle is hotter to
					// the eye, base is dimmer, with a small additive from rise/pop.
					const heightFrac = clamp01((blob.y - bounds.top) / Math.max(1, bounds.bottom - bounds.top));
					const heatLift = (1 - heightFrac) * 0.35; // higher up reads as cooler
					const coreFalloff = clamp01((field - 0.55) / 1.4);
					const rim = field < 1 ? clamp01((field - 0.55) / 0.45) : 1;
					const hotness = clamp01(0.55 + coreFalloff * 0.55 - heatLift + blob.riseGlow * 0.6);
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.45 + 0.55 * hotness));
					const color = hslToRGB(blob.hue, this.cfg.sat, light);
					const alpha = clamp01(0.45 + 0.5 * rim) * pressure;
					ctx.fillStyle = `rgba(${color.r},${color.g},${color.b},${alpha})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// 5. Bottle silhouette — soft glass walls, slightly tinted by hue.
			const wallHue = ((this.cfg.hue + 350) % 360 + 360) % 360;
			const wall = hslToRGB(wallHue, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 0.85));
			const wallHi = hslToRGB(wallHue, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 1.4));
			for (let y = bounds.top; y <= bounds.bottom; y++) {
				const t = (y - bounds.top) / Math.max(1, bounds.bottom - bounds.top);
				const half = this._bottleHalfWidth(t);
				const left = Math.round(bounds.center - half);
				const right = Math.round(bounds.center + half);
				if (left >= 0) {
					ctx.fillStyle = `rgb(${wall.r},${wall.g},${wall.b})`;
					ctx.fillRect(Math.floor(left * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
				if (right < this.w) {
					ctx.fillStyle = `rgb(${wall.r},${wall.g},${wall.b})`;
					ctx.fillRect(Math.floor(right * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
				// Subtle highlight on the left wall (looks like glass).
				if (left + 1 >= 0 && left + 1 < this.w) {
					ctx.fillStyle = `rgba(${wallHi.r},${wallHi.g},${wallHi.b},0.4)`;
					ctx.fillRect(Math.floor((left + 1) * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// 6. Bottle cap — a small flat disk at the top.
			const cap = hslToRGB(wallHue, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmin * 1.1));
			const capHalf = Math.round(this._bottleHalfWidth(0) + 1);
			const capRow0 = Math.max(0, bounds.top - 2);
			for (let y = capRow0; y <= bounds.top; y++) {
				for (let x = bounds.center - capHalf; x <= bounds.center + capHalf; x++) {
					if (x < 0 || x >= this.w) continue;
					ctx.fillStyle = `rgb(${cap.r},${cap.g},${cap.b})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// 7. Pedestal stand — wider footing under the heat slab.
			const standRow0 = pedestalRow1 + 1;
			const standRow1 = Math.min(this.h - 1, standRow0 + 1);
			const standHalf = pedestalCol.right - pedestalCol.left + 4;
			const standC = hslToRGB(wallHue, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 0.7));
			for (let y = standRow0; y <= standRow1; y++) {
				for (let x = bounds.center - Math.round(standHalf * 0.5); x <= bounds.center + Math.round(standHalf * 0.5); x++) {
					if (x < 0 || x >= this.w) continue;
					ctx.fillStyle = `rgb(${standC.r},${standC.g},${standC.b})`;
					ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), ceilSx, ceilSy);
				}
			}

			// 8. Surface-pop ripple — when active, a small bright ring near the
			//    targeted blob's apex. Adds a subtle visual punctuation.
			if (this.timers.surface_pop > 0) {
				const idx = Math.round(this.values.surface_pop_blob || 0);
				const target = blobs[idx];
				if (target) {
					const total = Math.max(1, Math.round(this.cfg.surface_pop_dur));
					const progress = this._phaseProgress(total, this.timers.surface_pop);
					const env = Math.sin(progress * Math.PI);
					const radius = target.r * (1 + env * 1.4);
					const ringColor = hslToRGB(target.hue, this.cfg.sat, clamp01(this.cfg.lmax * 0.92));
					const px = target.x * sx;
					const py = target.y * sy;
					ctx.strokeStyle = `rgba(${ringColor.r},${ringColor.g},${ringColor.b},${0.3 * env})`;
					ctx.lineWidth = Math.max(1, Math.round(sy));
					ctx.beginPath();
					ctx.arc(px, py, radius * sx, 0, Math.PI * 2);
					ctx.stroke();
				}
			}
		}
	}

	api.presets['lava-lamp'] = [
		{
			key: 'classic-red',
			label: 'classic red',
			config: {
				bottle_x: 0.5,
				bottle_top: 0.10,
				bottle_bottom: 0.86,
				bottle_width: 34,
				neck_width: 12,
				base_height: 6,
				blob_count: 4,
				rise_speed: 0.018,
				viscosity: 0.55,
				min_radius: 3.0,
				max_radius: 6.5,
				glow: 0.65,
				hue: 8,
				hue_sp: 14,
				sat: 0.82,
				lmin: 0.18,
				lmax: 0.92,
				rise_p: 0.0006,
				merge_p: 0.0004,
				split_p: 0.0003,
				surface_pop_p: 0.0006,
				quiet_flow_p: 0.0004,
			},
		},
		{
			key: 'cool-blue',
			label: 'cool blue',
			config: {
				bottle_x: 0.5,
				bottle_top: 0.10,
				bottle_bottom: 0.86,
				bottle_width: 32,
				neck_width: 11,
				base_height: 6,
				blob_count: 3,
				rise_speed: 0.015,
				viscosity: 0.6,
				min_radius: 3.2,
				max_radius: 6.0,
				glow: 0.55,
				hue: 220,
				hue_sp: 16,
				sat: 0.72,
				lmin: 0.18,
				lmax: 0.86,
				rise_p: 0.0005,
				merge_p: 0.0003,
				split_p: 0.0003,
				surface_pop_p: 0.0005,
				quiet_flow_p: 0.0006,
			},
		},
		{
			key: 'green-goo',
			label: 'green goo',
			config: {
				bottle_x: 0.5,
				bottle_top: 0.10,
				bottle_bottom: 0.86,
				bottle_width: 36,
				neck_width: 14,
				base_height: 6,
				blob_count: 5,
				rise_speed: 0.022,
				viscosity: 0.45,
				min_radius: 2.8,
				max_radius: 6.2,
				glow: 0.6,
				hue: 130,
				hue_sp: 22,
				sat: 0.78,
				lmin: 0.16,
				lmax: 0.88,
				rise_p: 0.0008,
				merge_p: 0.0006,
				split_p: 0.0004,
				surface_pop_p: 0.0008,
				quiet_flow_p: 0.0003,
			},
		},
		{
			key: 'slow-drift',
			label: 'slow drift',
			config: {
				bottle_x: 0.5,
				bottle_top: 0.08,
				bottle_bottom: 0.88,
				bottle_width: 34,
				neck_width: 12,
				base_height: 6,
				blob_count: 3,
				rise_speed: 0.009,
				viscosity: 0.85,
				min_radius: 3.4,
				max_radius: 7.0,
				glow: 0.5,
				hue: 32,
				hue_sp: 12,
				sat: 0.7,
				lmin: 0.18,
				lmax: 0.84,
				rise_p: 0.0003,
				merge_p: 0.0002,
				split_p: 0.0001,
				surface_pop_p: 0.0003,
				quiet_flow_p: 0.0008,
				quiet_flow_mult: 0.25,
				quiet_flow_dur: 200,
			},
		},
	];
	api.effects['lava-lamp'] = LavaLamp;
})(window.AmbienceSim);
