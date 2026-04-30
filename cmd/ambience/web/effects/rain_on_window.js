'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	// RainOnWindow — droplets nucleate on a pane, grow on contact, and run
	// downward when they exceed a critical mass. Mirrors
	// sim/rain_on_window.go: same per-slot state machine
	// (empty → forming → holding → falling) and lifecycle so authority and
	// browser stay in sync on which slot is currently running. Inner
	// shimmer + light-catch shapes free-run per client.
	const ROW_DEFAULTS = {
		intro_dur: 50, intro_burst: 0.55, intro_bg_ramp: 0.35,
		ending_dur: 80, ending_linger: 30, ending_residue: 0.18,
		slot_count: 36, form_dur: 70, hold_dur: 220, fall_speed: 0.7,
		drop_min_size: 1.2, drop_max_size: 3.4, crit_size: 2.6,
		trail_len: 18, frame_thick: 2,
		bg_hue: 30, bg_sat: 0.45, bg_light: 0.22, bg_glow: 0.6,
		drop_hue: 210, drop_sat: 0.18, hl_light: 0.85, sh_light: 0.18,
		form_p: 0, fall_p: 0, gust_p: 0, quiet_p: 0,
		gust_dur: 50, gust_skew: 0.6, quiet_dur: 90, quiet_mult: 0.25,
	};

	const ROW_STATE_EMPTY = 0;
	const ROW_STATE_FORMING = 1;
	const ROW_STATE_HOLDING = 2;
	const ROW_STATE_FALLING = 3;

	function applyRowDefaults(cfg) {
		const c = Object.assign({}, ROW_DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = ROW_DEFAULTS.intro_dur;
		c.intro_burst = clamp01(c.intro_burst);
		c.intro_bg_ramp = clamp01(c.intro_bg_ramp);
		if (c.ending_dur <= 0) c.ending_dur = ROW_DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_residue = clamp01(c.ending_residue);
		if (c.slot_count <= 0) c.slot_count = ROW_DEFAULTS.slot_count;
		if (c.slot_count > 96) c.slot_count = 96;
		if (c.form_dur <= 0) c.form_dur = ROW_DEFAULTS.form_dur;
		if (c.hold_dur <= 0) c.hold_dur = ROW_DEFAULTS.hold_dur;
		if (c.fall_speed <= 0) c.fall_speed = ROW_DEFAULTS.fall_speed;
		if (c.drop_min_size <= 0) c.drop_min_size = ROW_DEFAULTS.drop_min_size;
		if (c.drop_max_size <= 0) c.drop_max_size = ROW_DEFAULTS.drop_max_size;
		if (c.drop_max_size < c.drop_min_size) [c.drop_min_size, c.drop_max_size] = [c.drop_max_size, c.drop_min_size];
		if (c.crit_size <= 0) c.crit_size = ROW_DEFAULTS.crit_size;
		if (c.trail_len <= 0) c.trail_len = ROW_DEFAULTS.trail_len;
		if (c.frame_thick < 0) c.frame_thick = 0;
		if (c.bg_sat <= 0) c.bg_sat = ROW_DEFAULTS.bg_sat;
		if (c.bg_light <= 0) c.bg_light = ROW_DEFAULTS.bg_light;
		if (c.bg_glow <= 0) c.bg_glow = ROW_DEFAULTS.bg_glow;
		if (c.drop_sat < 0) c.drop_sat = 0;
		if (c.hl_light <= 0) c.hl_light = ROW_DEFAULTS.hl_light;
		if (c.sh_light <= 0) c.sh_light = ROW_DEFAULTS.sh_light;
		if (c.form_p < 0) c.form_p = 0;
		if (c.fall_p < 0) c.fall_p = 0;
		if (c.gust_p < 0) c.gust_p = 0;
		if (c.quiet_p < 0) c.quiet_p = 0;
		if (c.gust_dur <= 0) c.gust_dur = ROW_DEFAULTS.gust_dur;
		if (c.quiet_dur <= 0) c.quiet_dur = ROW_DEFAULTS.quiet_dur;
		if (c.quiet_mult <= 0) c.quiet_mult = ROW_DEFAULTS.quiet_mult;
		return c;
	}

	function emptyDrop() {
		return { state: ROW_STATE_EMPTY, x: 0, y: 0, size: 0, target: 0, phaseLeft: 0, phaseTotal: 0, startY: 0, velX: 0 };
	}

	class RainOnWindow {
		constructor(w, h, cfg, seed) {
			this.kind = 'rain-on-window';
			this.w = w;
			this.h = h;
			this.cfg = applyRowDefaults(cfg);
			this.seed = Number(seed || Date.now());
			this.rng = makeRNG(this.seed);
			this.tick = 0;
			this.drops = [];
			for (let i = 0; i < this.cfg.slot_count; i++) this.drops.push(emptyDrop());
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.gustTicks = 0;
			this.gustTotal = 0;
			this.gustDir = 0;
			this.quietTicks = 0;
			this.quietTotal = 0;
		}

		setConfig(cfg) {
			const next = applyRowDefaults(Object.assign({}, this.cfg, cfg));
			if (next.slot_count !== this.drops.length) {
				const resized = [];
				for (let i = 0; i < next.slot_count; i++) {
					resized.push(i < this.drops.length ? this.drops[i] : emptyDrop());
				}
				this.drops = resized;
			}
			this.cfg = next;
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || 0;
			const drops = state.drops || state.Drops;
			if (Array.isArray(drops)) {
				if (drops.length !== this.drops.length) {
					this.drops = [];
					for (let i = 0; i < drops.length; i++) this.drops.push(emptyDrop());
				}
				for (let i = 0; i < drops.length; i++) {
					const src = drops[i] || {};
					const d = this.drops[i];
					d.state = (src.s != null ? src.s : src.state) | 0;
					d.x = +(src.x || 0);
					d.y = +(src.y || 0);
					d.size = +(src.sz != null ? src.sz : (src.size || 0));
					d.target = +(src.tgt != null ? src.tgt : (src.target || 0));
					d.phaseLeft = (src.pl != null ? src.pl : (src.phaseLeft || 0)) | 0;
					d.phaseTotal = (src.pt != null ? src.pt : (src.phaseTotal || 0)) | 0;
					d.startY = +(src.sy != null ? src.sy : (src.startY || 0));
					d.velX = +(src.vx != null ? src.vx : (src.velX || 0));
				}
			}
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			this.gustTicks = state.gustTicks || 0;
			this.gustTotal = state.gustTotal || 0;
			this.gustDir = state.gustDir || 0;
			this.quietTicks = state.quietTicks || 0;
			this.quietTotal = state.quietTotal || 0;
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
				case 'drop-form': {
					const idx = this._pickEmpty();
					if (idx >= 0) this._spawn(idx, false);
					return true;
				}
				case 'drop-fall': {
					const idx = this._pickHolding();
					if (idx >= 0) this._startFall(idx);
					return true;
				}
				case 'drop-merge': {
					const pair = this._pickMergePair();
					if (pair) this._mergeDrops(pair[0], pair[1]);
					return true;
				}
				case 'wind-gust':
					this._startGust();
					return true;
				case 'quiet-pane':
					this.quietTicks = jitterInt(this.rng, this.cfg.quiet_dur, 0.3);
					this.quietTotal = this.quietTicks;
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
			if (this.gustTicks > 0) {
				this.gustTicks--;
				if (this.gustTicks === 0) {
					this.gustDir = 0;
					this.gustTotal = 0;
				}
			}
			if (this.quietTicks > 0) {
				this.quietTicks--;
				if (this.quietTicks === 0) this.quietTotal = 0;
			}
			this._advanceDrops();
			// Roll-and-spawn passes happen on the authority; the client just
			// replays trigger commands.
		}

		render(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);

			const bgScale = this._bgScale();
			const bgHue = ((this.cfg.bg_hue % 360) + 360) % 360;
			const innerLight = clamp01(this.cfg.bg_light * (0.6 + bgScale * 0.7));
			const edgeLight = clamp01(this.cfg.bg_light * (0.18 + bgScale * 0.4));

			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				ctx.fillStyle = '#050608';
				ctx.fillRect(0, 0, canvasW, canvasH);
			}
			// Diffuse background glow — read as warm interior or cool city.
			const cx = canvasW * 0.5;
			const cy = canvasH * 0.42;
			const radius = Math.max(canvasW, canvasH) * 0.85;
			const grad = ctx.createRadialGradient(cx, cy, radius * 0.05, cx, cy, radius);
			const inner = hslToRGB(bgHue, this.cfg.bg_sat, innerLight);
			const edge = hslToRGB(((bgHue + 8) % 360 + 360) % 360, clamp01(this.cfg.bg_sat * 0.7), edgeLight);
			grad.addColorStop(0, `rgba(${inner.r},${inner.g},${inner.b},${0.85 * bgScale + 0.1})`);
			grad.addColorStop(0.6, `rgba(${edge.r},${edge.g},${edge.b},${0.55 * bgScale + 0.05})`);
			grad.addColorStop(1, `rgba(0,0,0,${1 - 0.4 * bgScale})`);
			ctx.fillStyle = grad;
			ctx.fillRect(0, 0, canvasW, canvasH);

			// Soft window mullions / blur halo behind the glass — extra hint
			// of warm glow centered on the room's light source.
			const halo = ctx.createRadialGradient(canvasW * 0.32, canvasH * 0.28, 0, canvasW * 0.32, canvasH * 0.28, canvasW * 0.5);
			const haloC = hslToRGB(bgHue, this.cfg.bg_sat, clamp01(this.cfg.bg_light * (1 + bgScale * 1.2)));
			halo.addColorStop(0, `rgba(${haloC.r},${haloC.g},${haloC.b},${0.32 * this.cfg.bg_glow * bgScale})`);
			halo.addColorStop(1, `rgba(${haloC.r},${haloC.g},${haloC.b},0)`);
			ctx.fillStyle = halo;
			ctx.fillRect(0, 0, canvasW, canvasH);

			// Window-frame silhouette so the framing reads as glass, not just
			// a textured background.
			this._paintFrame(ctx, sx, sy, ceilSx, ceilSy);

			const dropAlpha = this._dropAlpha();
			for (let i = 0; i < this.drops.length; i++) {
				const d = this.drops[i];
				if (d.state === ROW_STATE_EMPTY) continue;
				if (d.state === ROW_STATE_FALLING) {
					this._paintTrail(ctx, sx, sy, ceilSx, ceilSy, d, dropAlpha);
				}
				this._paintDrop(ctx, sx, sy, ceilSx, ceilSy, d, dropAlpha);
			}
		}

		_paintFrame(ctx, sx, sy, ceilSx, ceilSy) {
			const t = Math.max(0, this.cfg.frame_thick);
			if (t <= 0) return;
			const frameC = hslToRGB(((this.cfg.bg_hue + 200) % 360 + 360) % 360, 0.18, 0.07);
			const colStr = `rgb(${frameC.r},${frameC.g},${frameC.b})`;
			// Top, bottom, left, right bands.
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, 0, this.w, t, colStr, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, this.h - t, this.w, t, colStr, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, 0, t, this.h, colStr, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, this.w - t, 0, t, this.h, colStr, 1);
		}

		_paintDrop(ctx, sx, sy, ceilSx, ceilSy, d, alphaScale) {
			const size = Math.max(0.4, d.size);
			const cxCell = d.x;
			const cyCell = d.y;
			const radius = Math.ceil(size + 0.6);
			const dropHue = ((this.cfg.drop_hue % 360) + 360) % 360;
			const baseLight = clamp01((this.cfg.bg_light * 0.5 + this.cfg.hl_light * 0.5));
			const shadow = hslToRGB(dropHue, clamp01(this.cfg.drop_sat * 0.8), this.cfg.sh_light);
			const body = hslToRGB(dropHue, this.cfg.drop_sat, baseLight);
			const highlight = hslToRGB(((dropHue + 12) % 360 + 360) % 360, clamp01(this.cfg.drop_sat * 0.6), this.cfg.hl_light);
			for (let dy = -radius; dy <= radius; dy++) {
				for (let dx = -radius; dx <= radius; dx++) {
					const rx = dx + 0.5;
					const ry = dy + 0.5;
					const dist = Math.sqrt(rx * rx + ry * ry);
					if (dist > size + 0.5) continue;
					const inset = clamp01(1 - dist / Math.max(0.5, size + 0.5));
					if (inset <= 0) continue;
					let r, g, b, alpha;
					// Shadow rim along the bottom-right; highlight along the top-left.
					const rim = clamp01((dist - (size - 0.6)) / 0.9);
					const lightX = clamp01(0.5 - rx / (size + 1));
					const lightY = clamp01(0.5 - ry / (size + 1));
					const hl = clamp01(lightX * 0.55 + lightY * 0.55 - rim * 0.6);
					if (rim > 0.55 && rx > 0 && ry > 0) {
						r = shadow.r; g = shadow.g; b = shadow.b;
						alpha = clamp01(0.7 * alphaScale * inset);
					} else if (hl > 0.45) {
						r = highlight.r; g = highlight.g; b = highlight.b;
						alpha = clamp01(0.85 * alphaScale * inset);
					} else {
						r = body.r; g = body.g; b = body.b;
						alpha = clamp01(0.55 * alphaScale * inset + 0.12);
					}
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(cxCell + dx), Math.round(cyCell + dy), 1, 1, `rgb(${r},${g},${b})`, alpha);
				}
			}
		}

		_paintTrail(ctx, sx, sy, ceilSx, ceilSy, d, alphaScale) {
			const trailMax = Math.max(2, this.cfg.trail_len);
			const dropHue = ((this.cfg.drop_hue % 360) + 360) % 360;
			const trailC = hslToRGB(dropHue, clamp01(this.cfg.drop_sat * 0.7), clamp01(this.cfg.bg_light * 1.1));
			const length = Math.min(trailMax, Math.max(2, d.y - d.startY + 1));
			const headY = d.y;
			for (let k = 1; k <= length; k++) {
				const fade = 1 - k / Math.max(1, length);
				if (fade < 0.05) continue;
				const ty = headY - k;
				if (ty < 0) break;
				// Trail sways slightly with the drop's velX so a gust skews
				// the streak too.
				const sway = -d.velX * 0.4 * (k / length);
				const tx = d.x + sway + (this._hash(7300 + Math.round(d.x * 13) + k) - 0.5) * 0.4;
				const widthShape = clamp01(fade * 0.85 + 0.15);
				const halfWidth = Math.max(0, Math.round(d.size * 0.5 * widthShape));
				for (let dx = -halfWidth; dx <= halfWidth; dx++) {
					const radial = halfWidth === 0 ? 1 : 1 - Math.abs(dx) / (halfWidth + 0.6);
					if (radial <= 0) continue;
					const alpha = clamp01(fade * radial * alphaScale * 0.6);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(tx + dx), Math.round(ty), 1, 1, `rgb(${trailC.r},${trailC.g},${trailC.b})`, alpha);
				}
				// Occasional residue droplet so the trail breaks into beads
				// rather than a clean line.
				if (k > 2 && this._hash(7400 + Math.round(d.x * 17) + k) > 0.78) {
					const resAlpha = clamp01(fade * 0.45 * alphaScale);
					const resC = hslToRGB(dropHue, this.cfg.drop_sat, this.cfg.hl_light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(tx), Math.round(ty), 1, 1, `rgb(${resC.r},${resC.g},${resC.b})`, resAlpha);
				}
			}
		}

		_pickEmpty() {
			const open = [];
			for (let i = 0; i < this.drops.length; i++) {
				if (this.drops[i].state === ROW_STATE_EMPTY) open.push(i);
			}
			if (open.length === 0) return -1;
			return open[Math.floor(this.rng() * open.length)];
		}

		_pickHolding() {
			const list = [];
			for (let i = 0; i < this.drops.length; i++) {
				if (this.drops[i].state === ROW_STATE_HOLDING) list.push(i);
			}
			if (list.length === 0) {
				for (let i = 0; i < this.drops.length; i++) {
					const d = this.drops[i];
					if (d.state === ROW_STATE_FORMING && d.size >= this.cfg.crit_size * 0.6) list.push(i);
				}
			}
			if (list.length === 0) return -1;
			return list[Math.floor(this.rng() * list.length)];
		}

		_pickMergePair() {
			const radius = Math.max(3, this.cfg.drop_max_size * 1.4);
			const pairs = [];
			for (let i = 0; i < this.drops.length; i++) {
				const di = this.drops[i];
				if (di.state !== ROW_STATE_FORMING && di.state !== ROW_STATE_HOLDING) continue;
				for (let j = i + 1; j < this.drops.length; j++) {
					const dj = this.drops[j];
					if (dj.state !== ROW_STATE_FORMING && dj.state !== ROW_STATE_HOLDING) continue;
					if (Math.abs(di.x - dj.x) > radius) continue;
					if (Math.abs(di.y - dj.y) > radius) continue;
					pairs.push([i, j]);
				}
			}
			if (pairs.length === 0) return null;
			return pairs[Math.floor(this.rng() * pairs.length)];
		}

		_spawn(idx, fromIntro) {
			if (idx < 0 || idx >= this.drops.length) return;
			const target = this.cfg.drop_min_size + this.rng() * Math.max(0, this.cfg.drop_max_size - this.cfg.drop_min_size);
			let formDur = jitterInt(this.rng, this.cfg.form_dur, 0.3);
			if (fromIntro) formDur = jitterInt(this.rng, Math.max(1, Math.floor(this.cfg.form_dur / 2)), 0.4);
			const frame = this.cfg.frame_thick + 1;
			const usableW = Math.max(4, this.w - 2 * frame);
			const usableH = Math.max(8, this.h * 0.7 - 2 * frame);
			let x = frame + this.rng() * usableW;
			const y = frame + this.rng() * usableH;
			if (this.gustTicks > 0) {
				const bias = this.gustDir * this.cfg.gust_skew;
				const norm = (x - frame) / Math.max(1, usableW);
				x = frame + clamp01(norm + bias * 0.18) * usableW;
			}
			this.drops[idx] = {
				state: ROW_STATE_FORMING,
				x: x,
				y: y,
				size: Math.max(0.4, this.cfg.drop_min_size * 0.4),
				target: target,
				phaseLeft: formDur,
				phaseTotal: formDur,
				startY: y,
				velX: 0,
			};
		}

		_startFall(idx) {
			const d = this.drops[idx];
			if (!d || d.state === ROW_STATE_EMPTY || d.state === ROW_STATE_FALLING) return;
			if (d.size < this.cfg.crit_size) d.size = this.cfg.crit_size;
			d.state = ROW_STATE_FALLING;
			d.phaseLeft = 0;
			d.phaseTotal = 0;
			d.startY = d.y;
			d.velX = 0;
			if (this.gustTicks > 0) {
				d.velX = this.gustDir * this.cfg.gust_skew * (0.6 + this.rng() * 0.4);
			}
		}

		_mergeDrops(a, b) {
			if (a === b) return;
			const da = this.drops[a];
			const db = this.drops[b];
			if (!da || !db) return;
			let keep = a, gone = b;
			if (db.y > da.y) { keep = b; gone = a; }
			const merged = this.drops[keep];
			const combined = Math.sqrt(da.size * da.size + db.size * db.size);
			merged.size = Math.min(combined, Math.max(this.cfg.drop_max_size, this.cfg.crit_size * 1.2));
			merged.x = (da.x * da.size + db.x * db.size) / Math.max(0.001, da.size + db.size);
			if (merged.state === ROW_STATE_FORMING) {
				const phase = jitterInt(this.rng, Math.max(1, Math.floor(this.cfg.form_dur / 3)), 0.3);
				merged.phaseLeft = phase;
				merged.phaseTotal = phase;
			}
			this.drops[gone] = emptyDrop();
			if (merged.size >= this.cfg.crit_size) this._startFall(keep);
		}

		_startGust() {
			const dur = jitterInt(this.rng, this.cfg.gust_dur, 0.3);
			this.gustTicks = dur;
			this.gustTotal = dur;
			this.gustDir = this.rng() < 0.5 ? -1 : 1;
			for (let i = 0; i < this.drops.length; i++) {
				if (this.drops[i].state === ROW_STATE_FALLING) {
					this.drops[i].velX += this.gustDir * this.cfg.gust_skew * (0.5 + this.rng() * 0.4);
				}
			}
		}

		_startIntro() {
			this.introTotal = Math.max(1, this.cfg.intro_dur);
			this.introTicks = this.introTotal;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.gustTicks = 0;
			this.quietTicks = 0;
			for (let i = 0; i < this.drops.length; i++) this.drops[i] = emptyDrop();
			const burst = Math.min(this.drops.length, Math.round(this.drops.length * clamp01(this.cfg.intro_burst)));
			for (let i = 0; i < burst; i++) this._spawn(i, true);
		}

		_startEnding() {
			this.gustTicks = 0;
			this.quietTicks = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = Math.max(1, this.cfg.ending_dur);
			this.endingTotal = Math.max(1, this.endingFade + Math.max(0, this.cfg.ending_linger));
			this.endingTicks = this.endingTotal;
		}

		_advanceDrops() {
			for (let i = 0; i < this.drops.length; i++) {
				const d = this.drops[i];
				if (d.state === ROW_STATE_EMPTY) continue;
				if (d.state === ROW_STATE_FORMING) {
					if (d.phaseLeft > 0) d.phaseLeft--;
					const total = Math.max(1, d.phaseTotal);
					const progress = clamp01(1 - d.phaseLeft / total);
					const start = Math.max(0.4, this.cfg.drop_min_size * 0.4);
					d.size = start + (d.target - start) * progress;
					if (d.phaseLeft <= 0) {
						d.state = ROW_STATE_HOLDING;
						const holdDur = jitterInt(this.rng, this.cfg.hold_dur, 0.4);
						d.phaseLeft = holdDur;
						d.phaseTotal = holdDur;
						d.size = d.target;
					}
				} else if (d.state === ROW_STATE_HOLDING) {
					if (d.phaseLeft > 0) d.phaseLeft--;
					if (d.size < this.cfg.crit_size) {
						d.size += (this.cfg.crit_size - d.size) * 0.0035;
					}
					if (d.phaseLeft <= 0 || d.size >= this.cfg.crit_size) {
						this._startFall(i);
					}
				} else if (d.state === ROW_STATE_FALLING) {
					d.y += this.cfg.fall_speed * (0.85 + Math.max(0, d.size - this.cfg.crit_size) * 0.45);
					d.x += d.velX;
					d.velX *= 0.92;
					if (this.gustTicks > 0) d.x += this.gustDir * this.cfg.gust_skew * 0.05;
					const frame = this.cfg.frame_thick + 1;
					const left = frame;
					const right = Math.max(left + 1, this.w - frame);
					if (d.x < left) { d.x = left; d.velX = Math.abs(d.velX) * 0.5; }
					if (d.x > right) { d.x = right; d.velX = -Math.abs(d.velX) * 0.5; }
					if (d.y > this.h - frame) {
						this.drops[i] = emptyDrop();
						continue;
					}
				}
			}
		}

		_bgScale() {
			let scale = 1;
			if (this.introTicks > 0 && this.introTotal > 0) {
				const progress = clamp01(1 - this.introTicks / this.introTotal);
				scale *= this.cfg.intro_bg_ramp + (1 - this.cfg.intro_bg_ramp) * progress;
			}
			if (this.endingTicks > 0 && this.endingTotal > 0) {
				const progress = clamp01(1 - this.endingTicks / this.endingTotal);
				scale *= 1 - 0.7 * progress;
			}
			return clamp01(scale);
		}

		_dropAlpha() {
			let alpha = 1;
			if (this.introTicks > 0 && this.introTotal > 0) {
				const progress = clamp01(1 - this.introTicks / this.introTotal);
				alpha *= 0.55 + 0.45 * progress;
			}
			if (this.endingTicks > 0 && this.endingTotal > 0) {
				const progress = clamp01(1 - this.endingTicks / this.endingTotal);
				alpha *= clamp01(this.cfg.ending_residue + (1 - this.cfg.ending_residue) * (1 - progress));
			}
			return clamp01(alpha);
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

	api.presets['rain-on-window'] = [
		{
			key: 'quiet-city',
			label: 'quiet city',
			config: {
				slot_count: 28,
				form_dur: 90,
				hold_dur: 280,
				fall_speed: 0.55,
				drop_min_size: 1.1,
				drop_max_size: 2.8,
				crit_size: 2.4,
				trail_len: 16,
				frame_thick: 2,
				bg_hue: 215,
				bg_sat: 0.35,
				bg_light: 0.18,
				bg_glow: 0.45,
				drop_hue: 215,
				drop_sat: 0.18,
				hl_light: 0.78,
				sh_light: 0.14,
				form_p: 0,
				gust_p: 0.0008,
			},
		},
		{
			key: 'evening-downpour',
			label: 'evening downpour',
			config: {
				slot_count: 56,
				form_dur: 50,
				hold_dur: 140,
				fall_speed: 1.0,
				drop_min_size: 1.4,
				drop_max_size: 3.6,
				crit_size: 2.4,
				trail_len: 26,
				frame_thick: 2,
				bg_hue: 28,
				bg_sat: 0.55,
				bg_light: 0.22,
				bg_glow: 0.7,
				drop_hue: 30,
				drop_sat: 0.22,
				hl_light: 0.86,
				sh_light: 0.18,
				gust_p: 0.0015,
				gust_skew: 0.85,
			},
		},
		{
			key: 'neon-street',
			label: 'neon street',
			config: {
				slot_count: 40,
				form_dur: 70,
				hold_dur: 200,
				fall_speed: 0.78,
				drop_min_size: 1.2,
				drop_max_size: 3.2,
				crit_size: 2.5,
				trail_len: 22,
				frame_thick: 2,
				bg_hue: 295,
				bg_sat: 0.7,
				bg_light: 0.24,
				bg_glow: 0.85,
				drop_hue: 200,
				drop_sat: 0.32,
				hl_light: 0.92,
				sh_light: 0.16,
				gust_p: 0.001,
				quiet_p: 0.0005,
			},
		},
		{
			key: 'gentle-drizzle',
			label: 'gentle drizzle',
			config: {
				slot_count: 22,
				form_dur: 110,
				hold_dur: 320,
				fall_speed: 0.45,
				drop_min_size: 1.0,
				drop_max_size: 2.4,
				crit_size: 2.1,
				trail_len: 12,
				frame_thick: 2,
				bg_hue: 36,
				bg_sat: 0.45,
				bg_light: 0.2,
				bg_glow: 0.55,
				drop_hue: 36,
				drop_sat: 0.16,
				hl_light: 0.82,
				sh_light: 0.16,
				quiet_p: 0.0012,
			},
		},
	];
	api.effects['rain-on-window'] = RainOnWindow;
})(window.AmbienceSim);
