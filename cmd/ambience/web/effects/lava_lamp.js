'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	const DEFAULTS = {
		intro_dur: 60,
		intro_glow: 0.12,
		intro_warmup: 24,
		ending_dur: 70,
		ending_linger: 28,
		ending_glow: 0.08,
		bottle_x: 0.5,
		bottle_top: 0.10,
		bottle_bottom: 0.92,
		bottle_width: 0.32,
		bottle_neck: 0.18,
		liquid_hue: 14,
		liquid_sat: 0.42,
		liquid_light: 0.22,
		hue: 6,
		hue_sp: 16,
		sat: 0.86,
		lmin: 0.26,
		lmax: 0.92,
		glow: 0.55,
		blob_count: 4,
		blob_size: 4.5,
		blob_size_sp: 1.6,
		rise_speed: 0.18,
		viscosity: 0.78,
		wobble: 0.55,
		blob_rise_p: 0,
		blob_merge_p: 0,
		blob_split_p: 0,
		surface_pop_p: 0,
		quiet_flow_p: 0,
		blob_rise_dur: 60,
		blob_merge_dur: 36,
		blob_split_dur: 32,
		surface_pop_dur: 28,
		quiet_flow_dur: 140,
		quiet_flow_mult: 0.45,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.intro_warmup < 0) c.intro_warmup = 0;
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		c.bottle_x = clamp01(c.bottle_x);
		c.bottle_top = clamp01(c.bottle_top);
		c.bottle_bottom = clamp01(c.bottle_bottom);
		if (c.bottle_bottom <= c.bottle_top) c.bottle_bottom = Math.min(0.98, c.bottle_top + 0.4);
		if (c.bottle_width <= 0) c.bottle_width = DEFAULTS.bottle_width;
		if (c.bottle_neck <= 0) c.bottle_neck = DEFAULTS.bottle_neck;
		if (c.liquid_sat < 0) c.liquid_sat = 0;
		if (c.liquid_light <= 0) c.liquid_light = DEFAULTS.liquid_light;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.glow <= 0) c.glow = DEFAULTS.glow;
		if (c.blob_count <= 0) c.blob_count = DEFAULTS.blob_count;
		if (c.blob_size <= 0) c.blob_size = DEFAULTS.blob_size;
		if (c.blob_size_sp < 0) c.blob_size_sp = 0;
		if (c.rise_speed <= 0) c.rise_speed = DEFAULTS.rise_speed;
		if (c.viscosity <= 0) c.viscosity = DEFAULTS.viscosity;
		if (c.wobble < 0) c.wobble = 0;
		if (c.blob_rise_dur <= 0) c.blob_rise_dur = DEFAULTS.blob_rise_dur;
		if (c.blob_merge_dur <= 0) c.blob_merge_dur = DEFAULTS.blob_merge_dur;
		if (c.blob_split_dur <= 0) c.blob_split_dur = DEFAULTS.blob_split_dur;
		if (c.surface_pop_dur <= 0) c.surface_pop_dur = DEFAULTS.surface_pop_dur;
		if (c.quiet_flow_dur <= 0) c.quiet_flow_dur = DEFAULTS.quiet_flow_dur;
		if (c.quiet_flow_mult <= 0) c.quiet_flow_mult = DEFAULTS.quiet_flow_mult;
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
			const rng = this._eventRng(name.length + 131);
			switch (name) {
				case 'blob-rise':
					this.timers.blob_rise = jitterInt(rng, this.cfg.blob_rise_dur, 0.3);
					this.values.blob_rise_seed = rng() * 1024;
					return true;
				case 'blob-merge':
					this.timers.blob_merge = jitterInt(rng, this.cfg.blob_merge_dur, 0.3);
					this.values.blob_merge_seed = rng() * 1024;
					return true;
				case 'blob-split':
					this.timers.blob_split = jitterInt(rng, this.cfg.blob_split_dur, 0.3);
					this.values.blob_split_seed = rng() * 1024;
					return true;
				case 'surface-pop':
					this.timers.surface_pop = jitterInt(rng, this.cfg.surface_pop_dur, 0.3);
					this.values.surface_pop_seed = rng() * 1024;
					return true;
				case 'quiet-flow':
					this.timers.quiet_flow = jitterInt(rng, this.cfg.quiet_flow_dur, 0.3);
					return true;
				case 'intro':
					this.timers.blob_rise = 0;
					this.timers.blob_merge = 0;
					this.timers.blob_split = 0;
					this.timers.surface_pop = 0;
					this.timers.quiet_flow = 0;
					this.timers.ending = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur + Math.max(0, this.cfg.intro_warmup)));
					this.values.intro_total = this.timers.intro;
					this.values.intro_warmup = Math.max(0, this.cfg.intro_warmup);
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.blob_rise = 0;
					this.timers.blob_merge = 0;
					this.timers.blob_split = 0;
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
		}

		// Heat factor: 0..~1, multiplied through intro/ending/quiet windows.
		_heatLevel() {
			let level = 1;
			if (this.timers.quiet_flow > 0) level *= this.cfg.quiet_flow_mult;
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
			return Math.max(0.04, level);
		}

		// Bottle profile half-width at vertical fraction ny in [0,1].
		// Wide at the base, narrowing toward a small top cap with a subtle belly.
		_bottleHalfAt(ny, maxHalf, neckHalf) {
			ny = clamp01(ny);
			// Top cap (very narrow) + neck taper
			if (ny < 0.06) return Math.max(1, neckHalf * 0.85);
			let base;
			if (ny < 0.22) {
				const t = (ny - 0.06) / 0.16;
				base = neckHalf + (maxHalf * 0.78 - neckHalf) * t;
			} else if (ny < 0.82) {
				// belly: slight bulge
				const t = (ny - 0.22) / 0.6;
				const bulge = Math.sin(Math.PI * t) * 0.06;
				base = maxHalf * 0.78 + (maxHalf - maxHalf * 0.78) * (0.55 + 0.45 * Math.sin(Math.PI * t)) + bulge * maxHalf;
			} else {
				// flare back to narrower base cap
				const t = (ny - 0.82) / 0.18;
				base = maxHalf * (1 - 0.16 * t);
			}
			return Math.max(1, base);
		}

		_drawCircle(ctx, sx, sy, cx, cy, radiusGrid, color, alpha) {
			const px = cx * sx;
			const py = cy * sy;
			const rx = Math.max(0.6, radiusGrid * sx);
			const ry = Math.max(0.6, radiusGrid * sy);
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.beginPath();
			ctx.ellipse(px, py, rx, ry, 0, 0, Math.PI * 2);
			ctx.fill();
			ctx.globalAlpha = 1;
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const room = ctx.createLinearGradient(0, 0, 0, canvasH);
				room.addColorStop(0, '#080610');
				room.addColorStop(0.5, '#0c0a14');
				room.addColorStop(1, '#100b14');
				ctx.fillStyle = room;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const cfg = this.cfg;
			const heat = this._heatLevel();

			const bottleCenterX = cfg.bottle_x * this.w;
			const bottleTopY = cfg.bottle_top * this.h;
			const bottleBottomY = cfg.bottle_bottom * this.h;
			const bottleHeight = Math.max(1, bottleBottomY - bottleTopY);
			const maxHalf = Math.max(2, cfg.bottle_width * this.w * 0.5);
			const neckHalf = Math.max(1, maxHalf * cfg.bottle_neck);

			// Cap dims (top and bottom hardware silhouettes)
			const capHeight = Math.max(2, this.h * 0.04);
			const baseHeight = Math.max(2, this.h * 0.05);

			// Heat halo behind the base — strong warm radial glow.
			{
				const baseY = bottleBottomY * sy;
				const baseR = Math.max(40, Math.min(canvasW, canvasH) * (0.16 + heat * 0.22));
				const haloX = bottleCenterX * sx;
				const halo = ctx.createRadialGradient(haloX, baseY, 0, haloX, baseY, baseR);
				const haloHue = (cfg.hue + 6) % 360;
				const haloCore = hslToRGB(haloHue, clamp01(cfg.sat), clamp01(cfg.lmax * (0.55 + heat * 0.4)));
				const haloOuter = hslToRGB((cfg.hue + 350) % 360, clamp01(cfg.sat * 0.7), clamp01(cfg.lmin + (cfg.lmax - cfg.lmin) * 0.3));
				halo.addColorStop(0, `rgba(${haloCore.r},${haloCore.g},${haloCore.b},${0.3 + heat * 0.45 * cfg.glow})`);
				halo.addColorStop(0.5, `rgba(${haloOuter.r},${haloOuter.g},${haloOuter.b},${0.12 + heat * 0.18 * cfg.glow})`);
				halo.addColorStop(1, `rgba(${haloOuter.r},${haloOuter.g},${haloOuter.b},0)`);
				ctx.fillStyle = halo;
				ctx.fillRect(haloX - baseR, baseY - baseR, baseR * 2, baseR * 2);
			}

			// Build the bottle silhouette path so we can clip the warm liquid +
			// blobs inside it. ctx.clip uses the current path.
			const bottlePath = new Path2D();
			const profile = [];
			const profileSteps = Math.max(28, Math.round(bottleHeight));
			for (let i = 0; i <= profileSteps; i++) {
				const ny = i / profileSteps;
				const yPx = (bottleTopY + ny * bottleHeight) * sy;
				const half = this._bottleHalfAt(ny, maxHalf, neckHalf);
				profile.push({ ny, yPx, half });
				if (i === 0) bottlePath.moveTo((bottleCenterX - half) * sx, yPx);
				else bottlePath.lineTo((bottleCenterX - half) * sx, yPx);
			}
			for (let i = profile.length - 1; i >= 0; i--) {
				const p = profile[i];
				bottlePath.lineTo((bottleCenterX + p.half) * sx, p.yPx);
			}
			bottlePath.closePath();

			// Subtle outer glow tint of the bottle silhouette before clipping.
			ctx.save();
			ctx.fillStyle = `rgba(${Math.round(20 + heat * 28)},${Math.round(8 + heat * 18)},${Math.round(14 + heat * 14)},${0.55})`;
			ctx.fill(bottlePath);
			ctx.restore();

			// Clip to bottle interior for the warm liquid + heat shading + blobs.
			ctx.save();
			ctx.clip(bottlePath);

			// Warm liquid background — top→bottom gradient, brighter near heat.
			const liquid = ctx.createLinearGradient(0, bottleTopY * sy, 0, bottleBottomY * sy);
			const liquidTop = hslToRGB(cfg.liquid_hue, clamp01(cfg.liquid_sat * 0.55), clamp01(cfg.liquid_light * 0.5));
			const liquidMid = hslToRGB(cfg.liquid_hue, clamp01(cfg.liquid_sat), clamp01(cfg.liquid_light));
			const liquidHot = hslToRGB((cfg.liquid_hue + 8) % 360, clamp01(cfg.liquid_sat * 1.1), clamp01(cfg.liquid_light + 0.18 * heat));
			liquid.addColorStop(0, `rgb(${liquidTop.r},${liquidTop.g},${liquidTop.b})`);
			liquid.addColorStop(0.55, `rgb(${liquidMid.r},${liquidMid.g},${liquidMid.b})`);
			liquid.addColorStop(1, `rgb(${liquidHot.r},${liquidHot.g},${liquidHot.b})`);
			ctx.fillStyle = liquid;
			ctx.fillRect(0, bottleTopY * sy - 2, canvasW, bottleHeight * sy + 4);

			// Heat-source pool at the very base inside the bottle.
			{
				const baseY = bottleBottomY * sy;
				const poolR = Math.max(20, maxHalf * 0.95) * sx;
				const pool = ctx.createRadialGradient(bottleCenterX * sx, baseY, 0, bottleCenterX * sx, baseY, poolR);
				const core = hslToRGB(cfg.hue, clamp01(cfg.sat), clamp01(cfg.lmax * (0.62 + heat * 0.32)));
				const edge = hslToRGB((cfg.hue + 350) % 360, clamp01(cfg.sat * 0.7), clamp01(cfg.lmin + (cfg.lmax - cfg.lmin) * 0.28));
				pool.addColorStop(0, `rgba(${core.r},${core.g},${core.b},${0.45 + heat * 0.4 * cfg.glow})`);
				pool.addColorStop(0.6, `rgba(${edge.r},${edge.g},${edge.b},${0.18 + heat * 0.2 * cfg.glow})`);
				pool.addColorStop(1, `rgba(${edge.r},${edge.g},${edge.b},0)`);
				ctx.fillStyle = pool;
				ctx.fillRect(0, baseY - poolR, canvasW, poolR);
			}

			// Blobs.
			const count = Math.max(1, Math.round(cfg.blob_count));
			const speedBase = cfg.rise_speed / Math.max(0.2, cfg.viscosity);
			const quietBoost = this.timers.quiet_flow > 0 ? cfg.quiet_flow_mult : 1;
			const riseBoost = this.timers.blob_rise > 0 ? 1.55 : 1;
			const popActive = this.timers.surface_pop > 0;
			const mergeActive = this.timers.blob_merge > 0;
			const splitActive = this.timers.blob_split > 0;
			const introT = this.timers.intro > 0
				? this._phaseProgress(this.values.intro_total || cfg.intro_dur, this.timers.intro)
				: 1;
			const endingT = this.timers.ending > 0
				? this._phaseProgress(this.values.ending_total || (cfg.ending_dur + cfg.ending_linger), this.timers.ending)
				: 0;

			for (let i = 0; i < count; i++) {
				const sizeJitter = (this._hash(7100 + i) * 2 - 1) * cfg.blob_size_sp;
				let radius = Math.max(1.4, cfg.blob_size + sizeJitter);
				const phase = this._hash(7200 + i) * Math.PI * 2;
				const pace = speedBase * (0.65 + this._hash(7300 + i) * 0.7) * quietBoost
					* (i === 0 ? riseBoost : 1);

				// y normalized to bottle interior (0=top, 1=bottom).
				let cyc = (this.tick * pace * 0.022 + phase) % (Math.PI * 2);
				if (cyc < 0) cyc += Math.PI * 2;
				let yNorm = 0.5 + 0.42 * Math.sin(cyc);

				// Intro: blobs hug the base; splay outward as warmup completes.
				if (introT < 1) {
					yNorm = 0.95 - (0.95 - yNorm) * (introT * introT);
				}
				// Ending: pull all blobs back toward the base.
				if (endingT > 0) {
					yNorm = yNorm + (0.94 - yNorm) * (endingT * endingT);
				}
				// Surface-pop: flatten the highest blob near the top briefly.
				if (popActive && i === 0) {
					yNorm = Math.min(yNorm, 0.08 + this._hash(7400) * 0.06);
				}

				const cy = bottleTopY + yNorm * bottleHeight;
				const halfHere = this._bottleHalfAt(yNorm, maxHalf, neckHalf);
				const wobblePhase = Math.sin(this.tick * 0.018 * (0.6 + this._hash(7500 + i) * 0.7) + i * 1.3);
				const wobbleAmp = Math.max(0, Math.min(halfHere - radius - 0.5, cfg.wobble * (halfHere - radius) * 0.7));
				const cx = bottleCenterX + wobblePhase * wobbleAmp;

				// Heat brightness depends on proximity to the base.
				const heatFromBase = clamp01(yNorm * 0.85 + heat * 0.4);
				const hueShift = (this._hash(7600 + i) * 2 - 1) * cfg.hue_sp * 0.6;
				const blobHue = ((cfg.hue + hueShift) + 360) % 360;
				const lightCore = clamp01(cfg.lmin + (cfg.lmax - cfg.lmin) * (0.55 + heatFromBase * 0.4));
				const lightRim = clamp01(cfg.lmin + (cfg.lmax - cfg.lmin) * (0.18 + heatFromBase * 0.18));
				const sat = clamp01(cfg.sat * (0.85 + heatFromBase * 0.18));

				let highlightBoost = 0;
				if (mergeActive && i < 2) highlightBoost = 0.16;
				if (splitActive && i === 0) highlightBoost = 0.22;
				if (this.timers.blob_rise > 0 && i === 0) highlightBoost = Math.max(highlightBoost, 0.18);

				// soft outer halo
				const haloColor = hslToRGB(blobHue, sat * 0.85, lightRim);
				this._drawCircle(ctx, sx, sy, cx, cy, radius * 1.65, `rgba(${haloColor.r},${haloColor.g},${haloColor.b},${0.18 + highlightBoost * 0.4})`, 1);
				// rim
				const rimColor = hslToRGB(blobHue, sat, lightRim);
				this._drawCircle(ctx, sx, sy, cx, cy, radius * 1.18, `rgba(${rimColor.r},${rimColor.g},${rimColor.b},${0.55})`, 1);
				// body
				const body = hslToRGB(blobHue, sat, clamp01(lightCore - 0.12 + highlightBoost * 0.08));
				this._drawCircle(ctx, sx, sy, cx, cy, radius * 0.92, `rgb(${body.r},${body.g},${body.b})`, 0.96);
				// inner highlight (slight off-center shimmer)
				const highlight = hslToRGB(blobHue, clamp01(sat * 0.7), clamp01(lightCore + 0.18 + highlightBoost));
				this._drawCircle(ctx, sx, sy, cx - radius * 0.2, cy - radius * 0.28, radius * 0.42, `rgba(${highlight.r},${highlight.g},${highlight.b},0.78)`, 1);

				// Split: render a small offspring blob trailing the donor.
				if (splitActive && i === 0) {
					const splitProgress = this._phaseProgress(Math.max(1, cfg.blob_split_dur), this.timers.blob_split);
					const offset = (1 - splitProgress) * radius * 1.4;
					const childR = radius * (0.45 + (1 - splitProgress) * 0.2);
					const childCy = cy + offset + 1;
					const childCx = cx + (this._hash(7700) * 2 - 1) * radius * 0.4;
					this._drawCircle(ctx, sx, sy, childCx, childCy, childR, `rgb(${body.r},${body.g},${body.b})`, 0.85);
					this._drawCircle(ctx, sx, sy, childCx - childR * 0.3, childCy - childR * 0.3, childR * 0.4, `rgba(${highlight.r},${highlight.g},${highlight.b},0.7)`, 1);
				}
			}

			// Surface pop visual — a flattened bright slick at the top of the bottle.
			if (popActive) {
				const popProgress = this._phaseProgress(Math.max(1, cfg.surface_pop_dur), this.timers.surface_pop);
				const slickAlpha = 0.55 * (1 - popProgress);
				const slickHue = (cfg.hue + 4) % 360;
				const slick = hslToRGB(slickHue, clamp01(cfg.sat * 0.9), clamp01(cfg.lmax * 0.85));
				const slickHalf = this._bottleHalfAt(0.06, maxHalf, neckHalf);
				ctx.fillStyle = `rgba(${slick.r},${slick.g},${slick.b},${slickAlpha})`;
				ctx.beginPath();
				ctx.ellipse(bottleCenterX * sx, (bottleTopY + 1.5) * sy, slickHalf * sx, Math.max(2, sy * 1.4), 0, 0, Math.PI * 2);
				ctx.fill();
			}

			ctx.restore();

			// Bottle highlight stroke — thin glassy edge.
			ctx.save();
			ctx.lineWidth = Math.max(1, sx * 0.5);
			ctx.strokeStyle = `rgba(245,232,210,${0.18 + heat * 0.18})`;
			ctx.stroke(bottlePath);
			// Inner shadow stroke
			ctx.lineWidth = Math.max(1, sx * 0.35);
			ctx.strokeStyle = `rgba(8,5,8,0.55)`;
			ctx.stroke(bottlePath);
			ctx.restore();

			// Top cap and bottom base — small chrome-y silhouettes.
			const capHalf = neckHalf * 1.35;
			const baseHalf = maxHalf * 1.05;
			const capColor = hslToRGB(40, 0.12, 0.34);
			const capShadow = hslToRGB(40, 0.18, 0.18);
			ctx.fillStyle = `rgb(${capShadow.r},${capShadow.g},${capShadow.b})`;
			ctx.fillRect((bottleCenterX - capHalf) * sx, (bottleTopY - capHeight) * sy, capHalf * 2 * sx, capHeight * sy);
			ctx.fillStyle = `rgb(${capColor.r},${capColor.g},${capColor.b})`;
			ctx.fillRect((bottleCenterX - capHalf * 0.92) * sx, (bottleTopY - capHeight + 0.5) * sy, capHalf * 1.84 * sx, Math.max(2, sy));

			const baseColor = hslToRGB(36, 0.18, 0.28);
			const baseShadow = hslToRGB(28, 0.22, 0.14);
			ctx.fillStyle = `rgb(${baseShadow.r},${baseShadow.g},${baseShadow.b})`;
			ctx.fillRect((bottleCenterX - baseHalf) * sx, bottleBottomY * sy, baseHalf * 2 * sx, baseHeight * sy);
			ctx.fillStyle = `rgb(${baseColor.r},${baseColor.g},${baseColor.b})`;
			ctx.fillRect((bottleCenterX - baseHalf * 0.94) * sx, bottleBottomY * sy + 1, baseHalf * 1.88 * sx, Math.max(2, sy));
			// Heat slot in the base — warm sliver
			const slot = hslToRGB(cfg.hue, clamp01(cfg.sat), clamp01(cfg.lmax * (0.55 + heat * 0.35)));
			ctx.fillStyle = `rgba(${slot.r},${slot.g},${slot.b},${0.5 + heat * 0.3})`;
			ctx.fillRect((bottleCenterX - baseHalf * 0.55) * sx, (bottleBottomY + baseHeight * 0.55) * sy, baseHalf * 1.1 * sx, Math.max(1, sy * 0.9));
		}
	}

	api.presets['lava-lamp'] = [
		{
			key: 'classic-red',
			label: 'classic red',
			config: {
				hue: 6,
				hue_sp: 14,
				sat: 0.88,
				lmin: 0.26,
				lmax: 0.94,
				liquid_hue: 12,
				liquid_sat: 0.5,
				liquid_light: 0.22,
				glow: 0.6,
				blob_count: 4,
				blob_size: 4.6,
				blob_size_sp: 1.4,
				rise_speed: 0.18,
				viscosity: 0.78,
				wobble: 0.5,
				blob_rise_p: 0.0014,
				blob_merge_p: 0.0008,
				surface_pop_p: 0.001,
				quiet_flow_p: 0.0006,
			},
		},
		{
			key: 'cool-blue',
			label: 'cool blue',
			config: {
				hue: 212,
				hue_sp: 22,
				sat: 0.78,
				lmin: 0.28,
				lmax: 0.9,
				liquid_hue: 198,
				liquid_sat: 0.36,
				liquid_light: 0.18,
				glow: 0.42,
				blob_count: 4,
				blob_size: 4.2,
				blob_size_sp: 1.2,
				rise_speed: 0.16,
				viscosity: 0.82,
				wobble: 0.42,
				blob_rise_p: 0.001,
				blob_merge_p: 0.0006,
				blob_split_p: 0.0006,
				surface_pop_p: 0.0008,
			},
		},
		{
			key: 'green-goo',
			label: 'green goo',
			config: {
				hue: 116,
				hue_sp: 24,
				sat: 0.84,
				lmin: 0.24,
				lmax: 0.88,
				liquid_hue: 140,
				liquid_sat: 0.44,
				liquid_light: 0.18,
				glow: 0.5,
				blob_count: 5,
				blob_size: 5,
				blob_size_sp: 1.8,
				rise_speed: 0.16,
				viscosity: 0.86,
				wobble: 0.65,
				blob_rise_p: 0.0012,
				blob_merge_p: 0.0012,
				blob_split_p: 0.0008,
				surface_pop_p: 0.0009,
				quiet_flow_p: 0.0005,
			},
		},
		{
			key: 'slow-drift',
			label: 'slow drift',
			config: {
				hue: 28,
				hue_sp: 18,
				sat: 0.7,
				lmin: 0.24,
				lmax: 0.84,
				liquid_hue: 22,
				liquid_sat: 0.32,
				liquid_light: 0.16,
				glow: 0.38,
				blob_count: 3,
				blob_size: 5.2,
				blob_size_sp: 1.2,
				rise_speed: 0.10,
				viscosity: 0.92,
				wobble: 0.32,
				blob_rise_p: 0.0006,
				blob_merge_p: 0.0004,
				surface_pop_p: 0.0005,
				quiet_flow_p: 0.0014,
				quiet_flow_mult: 0.35,
			},
		},
	];
	api.effects['lava-lamp'] = LavaLamp;
})(window.AmbienceSim);
