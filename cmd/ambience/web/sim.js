// sim.js — JS port of ambience/sim/rain.go.
//
// Clients run their own Rain sim locally. The server broadcasts config
// changes + trigger events; clients apply those via setConfig / triggerEvent.
// Step() advances the local sim one tick; drops + splashes are rendered into
// an internal grid that render() paints to a canvas.
//
// Clients do NOT roll for discrete events — that's the server's job. Clients
// only advance timers and physics. This keeps all clients in rough agreement
// on when events happen (frame-level sync is not guaranteed in v1).

(function (global) {
	'use strict';

	const DEFAULTS = {
		wind: 0,
		wind_jit: 0,
		speed: 1.0,
		speed_jit: 0,
		intro_style: 0,
		intro_dur: 60,
		intro_sparse: 8,
		intro_open: 0.08,
		intro_seed: 4,
		ending_style: 0,
		ending_dur: 60,
		ending_linger: 20,
		ending_splashes: 3,
		streak: 5,
		fade: 0.88,
		spawn: 5,
		burst: 1,
		hue: 210,
		hue_sp: 0,
		sat: 0.6,
		lmin: 0.55,
		lmax: 0.85,
		layers: 1,
		lbal: 0.4,
		hue_drift: 0,
		wind_drift: 0,
		downpour_p: 0,
		calm_p: 0,
		gust_p: 0,
		splash_p: 0,
		downpour_dur: 60,
		downpour_mult: 4,
		calm_dur: 50,
		gust_dur: 30,
		gust_str: 1.5,
		splash_size: 4,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.speed <= 0) c.speed = DEFAULTS.speed;
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		if (c.intro_sparse < 1) c.intro_sparse = DEFAULTS.intro_sparse;
		if (c.intro_open <= 0) c.intro_open = DEFAULTS.intro_open;
		if (c.intro_open > 1) c.intro_open = 1;
		if (c.intro_seed < 0) c.intro_seed = 0;
		if (c.intro_seed === 0) c.intro_seed = DEFAULTS.intro_seed;
		if (c.ending_style === 0 && c.ending_dur === 0 && c.ending_linger === 0 && c.ending_splashes === 0) {
			c.ending_dur = DEFAULTS.ending_dur;
			c.ending_linger = DEFAULTS.ending_linger;
			c.ending_splashes = DEFAULTS.ending_splashes;
		} else {
			if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
			if (c.ending_linger < 0) c.ending_linger = 0;
			if (c.ending_splashes < 0) c.ending_splashes = 0;
		}
		if (c.spawn <= 0) c.spawn = DEFAULTS.spawn;
		if (c.burst <= 0) c.burst = DEFAULTS.burst;
		if (c.streak <= 0) c.streak = DEFAULTS.streak;
		if (c.fade <= 0) c.fade = DEFAULTS.fade;
		if (c.layers <= 0) c.layers = 1;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		return c;
	}

	// Deterministic RNG (Mulberry32). Same seed → same sequence across clients.
	function makeRNG(seed) {
		let state = seed >>> 0;
		const rng = () => {
			state = (state + 0x6D2B79F5) | 0;
			let t = state;
			t = Math.imul(t ^ (t >>> 15), t | 1);
			t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
			return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
		};
		rng.intn = (n) => (n <= 0 ? 0 : Math.floor(rng() * n));
		return rng;
	}

	function jitterInt(rng, base, spread) {
		const f = base * (1 + spread * (rng() * 2 - 1));
		return Math.max(1, Math.round(f));
	}

	function clamp01(v) {
		return Math.max(0, Math.min(1, v));
	}

	// HSL → RGBA, matches sim/rain.go's hslToRGB.
	function hslToRGB(h, s, l) {
		const c = (1 - Math.abs(2 * l - 1)) * s;
		const hp = h / 60;
		const x = c * (1 - Math.abs((hp % 2) - 1));
		let rp = 0, gp = 0, bp = 0;
		if (hp < 1)      { rp = c; gp = x; bp = 0; }
		else if (hp < 2) { rp = x; gp = c; bp = 0; }
		else if (hp < 3) { rp = 0; gp = c; bp = x; }
		else if (hp < 4) { rp = 0; gp = x; bp = c; }
		else if (hp < 5) { rp = x; gp = 0; bp = c; }
		else             { rp = c; gp = 0; bp = x; }
		const m = l - c / 2;
		const clamp = (v) => Math.max(0, Math.min(1, v));
		return {
			r: Math.round(clamp(rp + m) * 255),
			g: Math.round(clamp(gp + m) * 255),
			b: Math.round(clamp(bp + m) * 255),
		};
	}

	class Rain {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.cfg = applyDefaults(cfg);
			this.rng = makeRNG(seed || Date.now());
			this.tick = 0;

			// Flat pixel buffer: w*h*3 bytes (RGB). 0,0,0 = empty.
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.drops = [];
			this.splashes = [];

			// Event-timer state
			this.downpourTicks = 0;
			this.downpourMult = 0;
			this.calmTicks = 0;
			this.gustTicks = 0;
			this.gustWind = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.endingSplashLeft = 0;
			this.endingSplashTotal = 0;
		}

		setConfig(cfg) {
			this.cfg = applyDefaults(Object.assign({}, this.cfg, cfg));
		}

		// Apply an atmosphere-authoritative initial state (from /snapshot).
		// The outer envelope is effect-agnostic; Rain-specific replica state
		// lives under snapshot.state.
		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.downpourTicks = state.downpourTicks || state.downpourLeft || 0;
			this.downpourMult = state.downpourMult || 0;
			this.calmTicks = state.calmTicks || state.calmLeft || 0;
			this.gustTicks = state.gustTicks || state.gustLeft || 0;
			this.gustWind = state.gustWind || 0;
			this.introTicks = state.introTicks || state.introLeft || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || state.endingLeft || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			this.endingSplashLeft = state.endingSplashLeft || 0;
			this.endingSplashTotal = state.endingSplashTotal || 0;
			if (typeof snap.seed === 'number') this.rng = makeRNG(snap.seed);
			// Adopt the server's grid dims so drops transfer 1:1. The
			// canvas render() scales whatever resolution we have to fit,
			// so shifting from the local default (e.g. 200×100) to the
			// server's (e.g. 160×80) is imperceptible.
			if (snap.gridW > 0 && snap.gridH > 0 &&
				(snap.gridW !== this.w || snap.gridH !== this.h)) {
				this.w = snap.gridW;
				this.h = snap.gridH;
				this.grid = new Uint8ClampedArray(this.w * this.h * 3);
			}
			if (Array.isArray(state.drops)) {
				this.drops = state.drops.map(d => ({
					row: d.row,
					col: d.col,
					color: d.color,
					vRow: d.vRow,
					vCol: d.vCol,
					streakLen: d.streakLen,
				}));
			}
			if (Array.isArray(state.splashes)) {
				this.splashes = state.splashes.map(s => ({
					row: s.row,
					col: s.col,
					age: s.age,
					maxAge: s.maxAge,
					maxRadius: s.maxRadius,
					color: s.color,
				}));
			}
		}

		// Trigger a discrete event — same semantics as server's TriggerEvent.
		// Clients only invoke this in response to server commands.
		triggerEvent(name) {
			const c = this.cfg;
			switch (name) {
				case 'downpour':
					this.downpourTicks = jitterInt(this.rng, c.downpour_dur, 0.3);
					this.downpourMult = c.downpour_mult;
					return true;
				case 'calm':
					this.calmTicks = jitterInt(this.rng, c.calm_dur, 0.3);
					return true;
				case 'gust':
					this.gustTicks = jitterInt(this.rng, c.gust_dur, 0.3);
					{
						const sign = this.rng() < 0.5 ? -1 : 1;
						this.gustWind = sign * c.gust_str * (0.7 + this.rng() * 0.6);
					}
					return true;
				case 'splash':
					this._spawnSplash();
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

			// 1. Decrement event timers (we don't roll for new events — server does).
			if (this.downpourTicks > 0) this.downpourTicks--;
			if (this.calmTicks > 0) this.calmTicks--;
			if (this.gustTicks > 0) this.gustTicks--;
			else this.gustWind = 0;

			// 2. Clear grid.
			this.grid.fill(0);

			// 3. Paint splashes.
			this._paintSplashes();

			// 4. Spawn drops. While an intro is active it owns the start pattern.
			if (this.introTicks > 0) {
				this._stepIntro();
			} else if (this.endingTicks > 0) {
				this._stepEnding();
			} else {
				let spawnEvery = this.cfg.spawn;
				if (this.downpourTicks > 0 && this.downpourMult > 1) {
					spawnEvery = Math.max(1, Math.floor(spawnEvery / this.downpourMult));
				}
				if (this.calmTicks === 0 && this.rng.intn(spawnEvery) === 0) {
					let burst = 1;
					if (this.cfg.burst > 1) burst = 1 + this.rng.intn(this.cfg.burst);
					for (let i = 0; i < burst; i++) this._spawnDrop();
				}
			}

			// 5. Advance + paint + cull drops.
			const alive = [];
			for (const d of this.drops) {
				d.row += d.vRow;
				d.col += d.vCol;
				this._paintDrop(d);
				const tailRow = d.row - (d.streakLen - 1) * d.vRow;
				if (tailRow < this.h && d.row > -d.streakLen) alive.push(d);
			}
			this.drops = alive;

			// 6. Age splashes.
			const sAlive = [];
			for (const s of this.splashes) {
				s.age++;
				if (s.age < s.maxAge) sAlive.push(s);
			}
			this.splashes = sAlive;
		}

		// Paint the grid onto a canvas context, scaled to fill (canvasW, canvasH).
		// opts: { transparent: true } — clear canvas to transparent instead of
		//        filling with the default dark background. Use when rendering
		//        as an overlay layer on top of other content.
		//       { bg: '#RRGGBB' } — use a custom background color.
		//        Defaults to '#0a0a0a' when neither transparent nor bg is set.
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

		// --- internals ---

		_currentHue() {
			let hue = this.cfg.hue;
			if (this.cfg.hue_drift > 0) {
				hue += this.cfg.hue_drift * Math.sin(this.tick * 0.02);
			}
			return ((hue % 360) + 360) % 360;
		}

		_currentWind() {
			let w = this.cfg.wind;
			if (this.cfg.wind_drift > 0) {
				w += this.cfg.wind_drift * Math.sin(this.tick * 0.013 + 1.7);
			}
			w += this.gustWind;
			return w;
		}

		_introStyle() {
			const style = this.cfg.intro_style | 0;
			if (style < 0 || style > 3) return 0;
			return style;
		}

		_introProgress() {
			return this._phaseProgress(this.introTotal, this.introTicks);
		}

		_phaseProgress(total, left) {
			if (left <= 1 || total <= 1) return 1;
			const elapsed = total - left;
			if (elapsed <= 0) return 0;
			return Math.max(0, Math.min(1, elapsed / (total - 1)));
		}

		_introRange(progress) {
			const style = this._introStyle();
			if (style === 0) return [0, this.w];
			let open = this.cfg.intro_open;
			if (!(open > 0)) open = DEFAULTS.intro_open;
			open = Math.max(0, Math.min(1, open));
			let width = (open + (1 - open) * Math.max(0, Math.min(1, progress))) * this.w;
			if (width < 1) width = 1;
			switch (style) {
				case 1:
					return [0, Math.min(this.w, width)];
				case 2: {
					const center = this.w / 2;
					const half = width / 2;
					return [Math.max(0, center - half), Math.min(this.w, center + half)];
				}
				case 3:
					return [Math.max(0, this.w - width), this.w];
				default:
					return [0, this.w];
			}
		}

		_endingStyle() {
			const style = this.cfg.ending_style | 0;
			if (style < 0 || style > 3) return 0;
			return style;
		}

		_endingRange(progress) {
			const style = this._endingStyle();
			if (style === 0) return [0, this.w];
			let width = (1 - Math.max(0, Math.min(1, progress))) * this.w;
			if (width < 1) width = 1;
			switch (style) {
				case 1:
					return [0, Math.min(this.w, width)];
				case 2: {
					const center = this.w / 2;
					const half = width / 2;
					return [Math.max(0, center - half), Math.min(this.w, center + half)];
				}
				case 3:
					return [Math.max(0, this.w - width), this.w];
				default:
					return [0, this.w];
			}
		}

		_setPixel(gr, gc, r, g, b) {
			if (gr < 0 || gr >= this.h || gc < 0 || gc >= this.w) return;
			const i = (gr * this.w + gc) * 3;
			this.grid[i] = r;
			this.grid[i + 1] = g;
			this.grid[i + 2] = b;
		}

		_spawnDropAt(colValue) {
			const c = this.cfg;
			const isBG = c.layers >= 2 && this.rng() < c.lbal;

			const sJit = (this.rng() * 2 - 1) * c.speed_jit;
			const wJit = (this.rng() * 2 - 1) * c.wind_jit;
			let effSpeed = c.speed * (1 + sJit);
			let effWind = this._currentWind() + wJit * c.wind;
			if (effSpeed < 0.1) effSpeed = 0.1;
			if (isBG) effSpeed *= 0.6;

			const hJit = (this.rng() * 2 - 1) * c.hue_sp;
			const hue = ((this._currentHue() + hJit) % 360 + 360) % 360;
			const t = this.rng();
			let lightness = c.lmin + t * (c.lmax - c.lmin);
			if (isBG) lightness *= 0.65;
			const col = hslToRGB(hue, c.sat, lightness);

			let streak = c.streak;
			if (isBG) streak = Math.max(2, Math.floor(streak / 2));

			this.drops.push({
				row: 0,
				col: Math.max(0, Math.min(this.w - 1, colValue)),
				color: col,
				vRow: effSpeed,
				vCol: effWind * effSpeed,
				streakLen: streak,
			});
		}

		_spawnDrop() {
			this._spawnDropAt(this.rng() * this.w);
		}

		_spawnIntroDrop(progress) {
			const [minCol, maxCol] = this._introRange(progress);
			let col = minCol;
			if (maxCol > minCol) col += this.rng() * (maxCol - minCol);
			this._spawnDropAt(col);
		}

		_spawnEndingDrop(progress) {
			const [minCol, maxCol] = this._endingRange(progress);
			let col = minCol;
			if (maxCol > minCol) col += this.rng() * (maxCol - minCol);
			this._spawnDropAt(col);
		}

		_startIntro() {
			this.downpourTicks = 0;
			this.downpourMult = 0;
			this.calmTicks = 0;
			this.gustTicks = 0;
			this.gustWind = 0;
			this.drops = [];
			this.splashes = [];
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.endingSplashLeft = 0;
			this.endingSplashTotal = 0;
			this.introTotal = Math.max(1, this.cfg.intro_dur | 0);
			this.introTicks = this.introTotal;
			for (let i = 0; i < this.cfg.intro_seed; i++) {
				this._spawnIntroDrop(0);
			}
		}

		_stepIntro() {
			const progress = this._introProgress();
			const sparse = Math.max(1, this.cfg.intro_sparse);
			const factor = 1 + (sparse - 1) * (1 - progress);
			const effectiveSpawn = Math.max(1, Math.round(this.cfg.spawn * factor));
			if (this.rng.intn(effectiveSpawn) === 0) {
				let burst = 1;
				if (this.cfg.burst > 1) burst = 1 + this.rng.intn(this.cfg.burst);
				for (let i = 0; i < burst; i++) this._spawnIntroDrop(progress);
			}
			this.introTicks--;
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.downpourTicks = 0;
			this.downpourMult = 0;
			this.calmTicks = 0;
			this.gustTicks = 0;
			this.gustWind = 0;
			this.endingFade = Math.max(1, this.cfg.ending_dur | 0);
			const linger = Math.max(0, this.cfg.ending_linger | 0);
			this.endingTotal = this.endingFade + linger;
			this.endingTicks = this.endingTotal;
			this.endingSplashTotal = Math.max(0, this.cfg.ending_splashes | 0);
			this.endingSplashLeft = this.endingSplashTotal;
		}

		_stepEnding() {
			const totalProgress = this._phaseProgress(this.endingTotal, this.endingTicks);
			if (this.endingSplashLeft > 0 && this.endingSplashTotal > 0) {
				const targetDone = Math.floor(Math.pow(totalProgress, 1.8) * this.endingSplashTotal);
				let done = this.endingSplashTotal - this.endingSplashLeft;
				while (done < targetDone && this.endingSplashLeft > 0) {
					this._spawnSplash();
					this.endingSplashLeft--;
					done++;
				}
			}

			const elapsed = this.endingTotal - this.endingTicks;
			if (elapsed < this.endingFade) {
				const fadeProgress = Math.max(0, Math.min(1, elapsed / Math.max(1, this.endingFade - 1)));
				const factor = 1 + 18 * fadeProgress * fadeProgress;
				const effectiveSpawn = Math.max(1, Math.round(this.cfg.spawn * factor));
				if (this.rng.intn(effectiveSpawn) === 0) {
					this._spawnEndingDrop(fadeProgress);
				}
			}

			this.endingTicks--;
			if (this.endingTicks < 0) this.endingTicks = 0;
		}

		_paintDrop(d) {
			for (let i = 0; i < d.streakLen; i++) {
				const row = d.row - i * d.vRow;
				const col = d.col - i * d.vCol;
				const gr = Math.floor(row);
				const gc = Math.round(col);
				if (gr < 0 || gr >= this.h || gc < 0 || gc >= this.w) continue;
				const brightness = Math.pow(this.cfg.fade, i);
				this._setPixel(gr, gc,
					Math.floor(d.color.r * brightness),
					Math.floor(d.color.g * brightness),
					Math.floor(d.color.b * brightness));
			}
		}

		_spawnSplash() {
			const c = this.cfg;
			if (c.splash_size <= 0) return;
			const radius = jitterInt(this.rng, c.splash_size, 0.3);
			const hJit = (this.rng() * 2 - 1) * c.hue_sp;
			const hue = ((this._currentHue() + hJit) % 360 + 360) % 360;
			const col = hslToRGB(hue, c.sat, c.lmax);
			this.splashes.push({
				row: this.rng.intn(this.h),
				col: this.rng.intn(this.w),
				age: 0,
				maxAge: radius * 2,
				maxRadius: radius,
				color: col,
			});
		}

		_paintSplashes() {
			for (const s of this.splashes) {
				const t = s.age / s.maxAge;
				const radius = t * s.maxRadius;
				const alpha = 1 - t;
				const rr = Math.floor(s.color.r * alpha);
				const gg = Math.floor(s.color.g * alpha);
				const bb = Math.floor(s.color.b * alpha);
				let steps = Math.floor(2 * Math.PI * radius);
				if (steps < 8) steps = 8;
				for (let i = 0; i < steps; i++) {
					const theta = (2 * Math.PI * i) / steps;
					const gc = s.col + Math.round(radius * Math.cos(theta));
					const gr = s.row + Math.round(radius * Math.sin(theta));
					this._setPixel(gr, gc, rr, gg, bb);
				}
			}
		}
	}

	const FIREFLIES_DEFAULTS = {
		drift: 0.18,
		wander: 0.4,
		spawn: 3,
		max: 44,
		hue: 72,
		hue_sp: 18,
		sat: 0.55,
		lmin: 0.45,
		lmax: 0.9,
		layers: 2,
		lbal: 0.45,
		blink_burst_p: 0,
		cluster_shift_p: 0,
		calm_p: 0,
		blink_burst_dur: 55,
		blink_burst_mult: 1.6,
		cluster_shift_dur: 75,
		cluster_pull: 0.65,
		calm_dur: 60,
	};

	function applyFirefliesDefaults(cfg) {
		const c = Object.assign({}, FIREFLIES_DEFAULTS, cfg || {});
		if (c.drift <= 0) c.drift = FIREFLIES_DEFAULTS.drift;
		if (c.wander <= 0) c.wander = FIREFLIES_DEFAULTS.wander;
		if (c.spawn <= 0) c.spawn = FIREFLIES_DEFAULTS.spawn;
		if (c.max <= 0) c.max = FIREFLIES_DEFAULTS.max;
		if (c.sat <= 0) c.sat = FIREFLIES_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = FIREFLIES_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = FIREFLIES_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.layers <= 0) c.layers = FIREFLIES_DEFAULTS.layers;
		if (c.lbal <= 0) c.lbal = FIREFLIES_DEFAULTS.lbal;
		if (c.blink_burst_dur <= 0) c.blink_burst_dur = FIREFLIES_DEFAULTS.blink_burst_dur;
		if (c.blink_burst_mult <= 0) c.blink_burst_mult = FIREFLIES_DEFAULTS.blink_burst_mult;
		if (c.cluster_shift_dur <= 0) c.cluster_shift_dur = FIREFLIES_DEFAULTS.cluster_shift_dur;
		if (c.cluster_pull <= 0) c.cluster_pull = FIREFLIES_DEFAULTS.cluster_pull;
		if (c.calm_dur <= 0) c.calm_dur = FIREFLIES_DEFAULTS.calm_dur;
		return c;
	}

	class Fireflies {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.cfg = applyFirefliesDefaults(cfg);
			this.rng = makeRNG(seed || Date.now());
			this.tick = 0;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.fireflies = [];
			this.blinkBurstTicks = 0;
			this.calmTicks = 0;
			this.clusterShiftTicks = 0;
			this.clusterRow = h * 0.5;
			this.clusterCol = w * 0.5;
		}

		setConfig(cfg) {
			this.cfg = applyFirefliesDefaults(Object.assign({}, this.cfg, cfg));
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.blinkBurstTicks = state.blinkBurstTicks || state.blinkBurstLeft || 0;
			this.calmTicks = state.calmTicks || state.calmLeft || 0;
			this.clusterShiftTicks = state.clusterShiftTicks || state.clusterShiftLeft || 0;
			if (typeof state.clusterRow === 'number') this.clusterRow = state.clusterRow;
			if (typeof state.clusterCol === 'number') this.clusterCol = state.clusterCol;
			if (typeof snap.seed === 'number') this.rng = makeRNG(snap.seed);
			if (snap.gridW > 0 && snap.gridH > 0 &&
				(snap.gridW !== this.w || snap.gridH !== this.h)) {
				this.w = snap.gridW;
				this.h = snap.gridH;
				this.grid = new Uint8ClampedArray(this.w * this.h * 3);
			}
			if (Array.isArray(state.fireflies)) {
				this.fireflies = state.fireflies.map(ff => ({
					row: ff.row,
					col: ff.col,
					vRow: ff.vRow,
					vCol: ff.vCol,
					color: ff.color,
					phase: ff.phase,
					blinkRate: ff.blinkRate,
					background: !!ff.background,
				}));
			}
		}

		triggerEvent(name) {
			const c = this.cfg;
			switch (name) {
				case 'blink-burst':
					this.blinkBurstTicks = jitterInt(this.rng, c.blink_burst_dur, 0.3);
					return true;
				case 'cluster-shift':
					this.clusterShiftTicks = jitterInt(this.rng, c.cluster_shift_dur, 0.3);
					this.clusterRow = this.rng() * this.h;
					this.clusterCol = this.rng() * this.w;
					return true;
				case 'calm':
					this.calmTicks = jitterInt(this.rng, c.calm_dur, 0.3);
					return true;
			}
			return false;
		}

		step() {
			this.tick++;
			if (this.blinkBurstTicks > 0) this.blinkBurstTicks--;
			if (this.calmTicks > 0) this.calmTicks--;
			if (this.clusterShiftTicks > 0) this.clusterShiftTicks--;

			this.grid.fill(0);

			let spawnEvery = this.cfg.spawn;
			if (this.calmTicks > 0) spawnEvery *= 2;
			if (spawnEvery < 1) spawnEvery = 1;
			if (this.fireflies.length < this.cfg.max && this.rng.intn(spawnEvery) === 0) {
				this._spawnFirefly();
			}

			for (const ff of this.fireflies) {
				this._stepFirefly(ff);
				this._paintFirefly(ff);
			}
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

		_stepFirefly(ff) {
			const wander = this.cfg.wander * 0.02;
			ff.vCol += (this.rng() * 2 - 1) * wander;
			ff.vRow += (this.rng() * 2 - 1) * wander * 0.7;
			let maxSpeed = this.cfg.drift * 2.2;
			if (ff.background) maxSpeed *= 0.7;
			ff.vCol = Math.max(-maxSpeed, Math.min(maxSpeed, ff.vCol));
			ff.vRow = Math.max(-maxSpeed, Math.min(maxSpeed, ff.vRow));
			if (this.clusterShiftTicks > 0 && this.cfg.cluster_pull > 0) {
				ff.vCol += (this.clusterCol - ff.col) * this.cfg.cluster_pull * 0.0008;
				ff.vRow += (this.clusterRow - ff.row) * this.cfg.cluster_pull * 0.0005;
			}
			ff.col += ff.vCol;
			ff.row += ff.vRow;
			while (ff.col < 0) ff.col += this.w;
			while (ff.col >= this.w) ff.col -= this.w;
			while (ff.row < 0) ff.row += this.h;
			while (ff.row >= this.h) ff.row -= this.h;
			ff.phase += ff.blinkRate;
		}

		_paintFirefly(ff) {
			const gr = Math.round(ff.row);
			const gc = Math.round(ff.col);
			if (gr < 0 || gr >= this.h || gc < 0 || gc >= this.w) return;
			const base = (Math.sin(ff.phase) + 1) * 0.5;
			let glow = 0.15 + 0.85 * base * base;
			if (this.blinkBurstTicks > 0) glow *= this.cfg.blink_burst_mult;
			if (this.calmTicks > 0) glow *= 0.7;
			if (ff.background) glow *= 0.75;
			glow = Math.max(0, Math.min(1, glow));
			this._setPixel(gr, gc,
				Math.floor(ff.color.r * glow),
				Math.floor(ff.color.g * glow),
				Math.floor(ff.color.b * glow));
		}

		_setPixel(gr, gc, r, g, b) {
			if (gr < 0 || gr >= this.h || gc < 0 || gc >= this.w) return;
			const i = (gr * this.w + gc) * 3;
			this.grid[i] = r;
			this.grid[i + 1] = g;
			this.grid[i + 2] = b;
		}

		_spawnFirefly() {
			const c = this.cfg;
			const isBG = c.layers >= 2 && this.rng() < c.lbal;
			let speed = c.drift * (0.55 + this.rng() * 0.9);
			if (isBG) speed *= 0.6;
			const hue = ((c.hue + (this.rng() * 2 - 1) * c.hue_sp) % 360 + 360) % 360;
			let lightness = c.lmin + this.rng() * (c.lmax - c.lmin);
			if (isBG) lightness *= 0.82;
			const color = hslToRGB(hue, c.sat, lightness);
			this.fireflies.push({
				row: this.rng() * this.h,
				col: this.rng() * this.w,
				vRow: (this.rng() * 2 - 1) * speed * 0.5,
				vCol: (this.rng() * 2 - 1) * speed,
				color: color,
				phase: this.rng() * 2 * Math.PI,
				blinkRate: 0.04 + this.rng() * 0.07,
				background: isBG,
			});
		}
	}

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

	const DUST_DEFAULTS = {
		intro_dur: 60,
		intro_haze: 0.12,
		intro_push: 1.5,
		ending_dur: 60,
		ending_linger: 20,
		ending_residue: 0.08,
		drift: 0.45,
		wander: 0.35,
		spawn: 4,
		burst: 2,
		max: 56,
		trail: 3,
		fade: 0.72,
		hue: 32,
		hue_sp: 10,
		sat: 0.35,
		lmin: 0.32,
		lmax: 0.72,
		layers: 2,
		lbal: 0.45,
		gust_p: 0,
		calm_p: 0,
		gust_dur: 50,
		gust_mult: 1.8,
		gust_front: 18,
		calm_dur: 65,
		calm_mult: 0.4,
	};

	function applyDustDefaults(cfg) {
		const c = Object.assign({}, DUST_DEFAULTS, cfg || {});
		if (c.intro_dur === 0 && c.intro_haze === 0 && c.intro_push === 0) {
			c.intro_dur = DUST_DEFAULTS.intro_dur;
			c.intro_haze = DUST_DEFAULTS.intro_haze;
			c.intro_push = DUST_DEFAULTS.intro_push;
		} else {
			if (c.intro_dur <= 0) c.intro_dur = DUST_DEFAULTS.intro_dur;
			if (c.intro_haze < 0) c.intro_haze = 0;
			if (c.intro_push <= 0) c.intro_push = DUST_DEFAULTS.intro_push;
		}
		c.intro_haze = clamp01(c.intro_haze);
		if (c.ending_dur === 0 && c.ending_linger === 0 && c.ending_residue === 0) {
			c.ending_dur = DUST_DEFAULTS.ending_dur;
			c.ending_linger = DUST_DEFAULTS.ending_linger;
			c.ending_residue = DUST_DEFAULTS.ending_residue;
		} else {
			if (c.ending_dur <= 0) c.ending_dur = DUST_DEFAULTS.ending_dur;
			if (c.ending_linger < 0) c.ending_linger = 0;
			if (c.ending_residue < 0) c.ending_residue = 0;
		}
		c.ending_residue = clamp01(c.ending_residue);
		if (c.drift === 0) c.drift = DUST_DEFAULTS.drift;
		if (c.wander <= 0) c.wander = DUST_DEFAULTS.wander;
		if (c.spawn <= 0) c.spawn = DUST_DEFAULTS.spawn;
		if (c.burst <= 0) c.burst = DUST_DEFAULTS.burst;
		if (c.max <= 0) c.max = DUST_DEFAULTS.max;
		if (c.trail <= 0) c.trail = DUST_DEFAULTS.trail;
		if (c.fade <= 0) c.fade = DUST_DEFAULTS.fade;
		if (c.hue === 0) c.hue = DUST_DEFAULTS.hue;
		if (c.hue_sp <= 0) c.hue_sp = DUST_DEFAULTS.hue_sp;
		if (c.sat <= 0) c.sat = DUST_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DUST_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DUST_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.layers <= 0) c.layers = DUST_DEFAULTS.layers;
		if (c.lbal <= 0) c.lbal = DUST_DEFAULTS.lbal;
		if (c.gust_dur <= 0) c.gust_dur = DUST_DEFAULTS.gust_dur;
		if (c.gust_mult <= 0) c.gust_mult = DUST_DEFAULTS.gust_mult;
		if (c.gust_front <= 0) c.gust_front = DUST_DEFAULTS.gust_front;
		if (c.calm_dur <= 0) c.calm_dur = DUST_DEFAULTS.calm_dur;
		if (c.calm_mult <= 0) c.calm_mult = DUST_DEFAULTS.calm_mult;
		return c;
	}

	class Dust {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.cfg = applyDustDefaults(cfg);
			this.rng = makeRNG(seed || Date.now());
			this.tick = 0;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.motes = [];
			this.gustTicks = 0;
			this.calmTicks = 0;
			this.gustCenter = h * 0.5;
			this.gustPush = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
		}

		setConfig(cfg) {
			const prev = this.cfg;
			const next = applyDustDefaults(Object.assign({}, this.cfg, cfg));
			if (prev && next.drift !== prev.drift) {
				const delta = next.drift - prev.drift;
				for (const mote of this.motes) {
					mote.vCol += delta * (mote.background ? 0.72 : 1);
				}
			}
			this.cfg = next;
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || snap.tick || 0;
			this.gustTicks = state.gustTicks || 0;
			this.calmTicks = state.calmTicks || 0;
			if (typeof state.gustCenter === 'number') this.gustCenter = state.gustCenter;
			if (typeof state.gustPush === 'number') this.gustPush = state.gustPush;
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
			}
			this.motes = Array.isArray(state.motes) ? state.motes.map(m => ({
				row: m.row,
				col: m.col,
				vRow: m.vRow,
				vCol: m.vCol,
				life: m.life,
				maxLife: m.maxLife,
				trail: m.trail,
				color: m.color,
				background: !!m.background,
			})) : [];
		}

		triggerEvent(name) {
			switch (name) {
				case 'gust':
					this._startGust(this.cfg.gust_mult);
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
			if (this.gustTicks > 0) this.gustTicks--;
			else this.gustPush = 0;
			if (this.calmTicks > 0) this.calmTicks--;
			const introActive = this.introTicks > 0;
			const endingActive = this.endingTicks > 0;

			if (!introActive && !endingActive) {
				if (this.gustTicks === 0 && this.rng() < this.cfg.gust_p) {
					this._startGust(this.cfg.gust_mult);
				}
				if (this.calmTicks === 0 && this.rng() < this.cfg.calm_p) {
					this.calmTicks = jitterInt(this.rng, this.cfg.calm_dur, 0.3);
				}
			}

			this.grid.fill(0);
			this._paintHaze();
			this._spawnStep();
			this._stepMotes();
			this._paintMotes();

			if (introActive) this.introTicks = Math.max(0, this.introTicks - 1);
			if (endingActive) this.endingTicks = Math.max(0, this.endingTicks - 1);
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

		_startGust(mult) {
			this.gustTicks = jitterInt(this.rng, this.cfg.gust_dur, 0.3);
			this.gustCenter = this.h > 1 ? this.rng() * (this.h - 1) : 0;
			let dir = this.cfg.drift;
			if (Math.abs(dir) < 0.05) {
				dir = this.rng() < 0.5 ? -0.35 : 0.35;
			}
			const sign = dir < 0 ? -1 : 1;
			this.gustPush = sign * Math.max(0.18, Math.abs(dir)) * mult * (0.7 + this.rng() * 0.6);
		}

		_startIntro() {
			this.calmTicks = 0;
			this.gustTicks = 0;
			this.gustPush = 0;
			this.motes = [];
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.introTotal = this.cfg.intro_dur > 0 ? this.cfg.intro_dur : DUST_DEFAULTS.intro_dur;
			this.introTicks = this.introTotal;
			this._startGust(Math.max(0.2, this.cfg.intro_push));
		}

		_startEnding() {
			this.introTicks = 0;
			this.introTotal = 0;
			this.calmTicks = 0;
			this.gustTicks = 0;
			this.gustPush = 0;
			this.endingFade = this.cfg.ending_dur > 0 ? this.cfg.ending_dur : DUST_DEFAULTS.ending_dur;
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

		_densityLevel() {
			let level = 1.0;
			if (this.gustTicks > 0) level *= 1.25;
			if (this.calmTicks > 0) level *= this.cfg.calm_mult;
			if (this.introTicks > 0) {
				const progress = this._phaseProgress(this.introTotal, this.introTicks);
				level *= this.cfg.intro_haze + (1 - this.cfg.intro_haze) * progress;
			}
			if (this.endingTicks > 0) {
				const progress = this._phaseProgress(this.endingTotal, this.endingTicks);
				level *= 1 - (1 - this.cfg.ending_residue) * progress;
			}
			return Math.max(0.05, level);
		}

		_gustInfluence(row) {
			if (this.gustTicks <= 0) return 0;
			const half = Math.max(2, this.cfg.gust_front * 0.5);
			const dist = Math.abs(row - this.gustCenter);
			if (dist >= half) return 0;
			return 1 - dist / half;
		}

		_spawnStep() {
			if (this.motes.length >= this.cfg.max) return;
			const level = this._densityLevel();
			let spawnEvery = Math.round(this.cfg.spawn / Math.max(0.15, level));
			if (spawnEvery < 1) spawnEvery = 1;
			let attempts = 1;
			if (level > 1) {
				attempts += Math.floor(level);
				if (this.rng() < (level - Math.floor(level))) attempts++;
			}
			for (let i = 0; i < attempts && this.motes.length < this.cfg.max; i++) {
				if (this.rng.intn(spawnEvery) !== 0) continue;
				let burst = 1;
				if (this.cfg.burst > 1) burst = 1 + this.rng.intn(this.cfg.burst);
				if (this.gustTicks > 0 && this.rng() < 0.35) burst++;
				for (let j = 0; j < burst && this.motes.length < this.cfg.max; j++) {
					this._spawnMote();
				}
			}
		}

		_spawnRow() {
			if (this.h <= 1) return 0;
			if (this.gustTicks > 0) {
				const half = Math.max(2, this.cfg.gust_front * 0.5);
				return Math.max(0, Math.min(this.h - 1, this.gustCenter + (this.rng() * 2 - 1) * half * 0.9));
			}
			const center = this.h * 0.58 + Math.sin(this.tick * 0.017) * this.h * 0.08;
			const spread = Math.max(3, this.h * 0.28);
			return Math.max(0, Math.min(this.h - 1, center + (this.rng() * 2 - 1) * spread));
		}

		_spawnMote() {
			const isBG = this.cfg.layers >= 2 && this.rng() < this.cfg.lbal;
			const row = this._spawnRow();
			const gustInfluence = this._gustInfluence(row);
			let drift = this.cfg.drift;
			if (isBG) drift *= 0.72;
			let vCol = drift * (0.7 + this.rng() * 0.6) + this.gustPush * gustInfluence * (0.45 + this.rng() * 0.25);
			vCol += (this.rng() * 2 - 1) * this.cfg.wander * 0.05;
			let vRow = (this.rng() * 2 - 1) * (0.03 + this.cfg.wander * 0.12);
			if (isBG) vRow *= 0.7;
			let trail = this.cfg.trail;
			if (isBG) trail = Math.max(1, trail - 1);
			const hue = ((this.cfg.hue + (this.rng() * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
			let light = this.cfg.lmin + this.rng() * (this.cfg.lmax - this.cfg.lmin);
			if (isBG) light *= 0.78;
			const color = hslToRGB(hue, this.cfg.sat, light);
			const speed = Math.abs(vCol) + 0.15;
			let lifeBase = Math.round(Math.max(24, this.w) / Math.max(0.12, speed));
			lifeBase = Math.max(18, Math.min(260, lifeBase));
			const life = jitterInt(this.rng, lifeBase, 0.3);
			const edgePad = trail + 2;
			let col = this.rng() * Math.max(1, this.w - 1);
			if (vCol > 0.08) col = -edgePad + this.rng() * edgePad;
			else if (vCol < -0.08) col = (this.w - 1) + this.rng() * edgePad;
			this.motes.push({
				row,
				col,
				vRow,
				vCol,
				life,
				maxLife: life,
				trail,
				color,
				background: isBG,
			});
		}

		_stepMotes() {
			if (!this.motes.length) return;
			const alive = [];
			for (const mote of this.motes) {
				let wander = this.cfg.wander * 0.018;
				if (mote.background) wander *= 0.75;
				mote.vCol += (this.rng() * 2 - 1) * wander;
				mote.vRow += (this.rng() * 2 - 1) * wander * 0.8;
				let target = this.cfg.drift;
				if (mote.background) target *= 0.72;
				if (this.calmTicks > 0) target *= 0.88;
				if (this.gustTicks > 0) {
					const influence = this._gustInfluence(mote.row);
					target += this.gustPush * (0.55 + 0.45 * influence);
					mote.vRow += (this.rng() * 2 - 1) * influence * this.cfg.wander * 0.03;
				}
				mote.vCol += (target - mote.vCol) * 0.18;
				mote.vRow = Math.max(-0.28, Math.min(0.28, mote.vRow));
				let maxCol = Math.max(0.18, Math.abs(target) * 2.4 + 0.15);
				if (mote.background) maxCol *= 0.8;
				mote.vCol = Math.max(-maxCol, Math.min(maxCol, mote.vCol));
				mote.col += mote.vCol;
				mote.row += mote.vRow;
				while (mote.row < 0) mote.row += Math.max(1, this.h);
				while (mote.row >= this.h) mote.row -= Math.max(1, this.h);
				mote.life--;
				if (mote.life > 0 && mote.col >= -mote.trail - 2 && mote.col < this.w + mote.trail + 2) {
					alive.push(mote);
				}
			}
			this.motes = alive;
		}

		_paintHaze() {
			const level = this._densityLevel();
			if (level <= 0 || this.w <= 0 || this.h <= 0) return;
			const center = this.h * 0.58 + Math.sin(this.tick * 0.013) * this.h * 0.04;
			const spread = Math.max(4, this.h * 0.24);
			for (let y = 0; y < this.h; y++) {
				const rowInfluence = 1 - Math.abs(y - center) / spread;
				if (rowInfluence <= 0) continue;
				const gustRow = this._gustInfluence(y);
				for (let x = 0; x < this.w; x++) {
					const wave = 0.5 + 0.5 * Math.sin(x * 0.09 + y * 0.17 + this.tick * 0.04);
					let strength = rowInfluence * level * (0.02 + 0.05 * wave);
					if (gustRow > 0) {
						const sweep = 0.6 + 0.4 * Math.sin(x * 0.12 - this.tick * 0.06);
						strength += gustRow * 0.04 * sweep;
					}
					if (strength < 0.028) continue;
					const hue = ((this.cfg.hue - 6 + Math.sin(x * 0.03) * this.cfg.hue_sp * 0.2) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin * (0.18 + 0.6 * strength));
					const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.35), light);
					this._paintMax(y, x, color);
				}
			}
		}

		_paintMotes() {
			for (const mote of this.motes) {
				const lifeFade = clamp01(mote.life / Math.max(1, mote.maxLife));
				if (lifeFade <= 0) continue;
				const tail = Math.max(1, mote.trail);
				for (let i = 0; i < tail; i++) {
					const row = mote.row - i * mote.vRow * 0.75;
					const col = mote.col - i * mote.vCol * 0.75;
					const bright = Math.pow(this.cfg.fade, i) * (0.35 + 0.65 * lifeFade) * (mote.background ? 0.78 : 1);
					this._paintMax(Math.round(row), Math.round(col), {
						r: Math.floor(mote.color.r * bright),
						g: Math.floor(mote.color.g * bright),
						b: Math.floor(mote.color.b * bright),
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

	const PROCEDURAL_DEFAULTS = {
		aurora: {
			intro_dur: 70,
			intro_glow: 0.18,
			ending_dur: 80,
			ending_linger: 20,
			ending_glow: 0.05,
			intensity: 0.56,
			speed: 0.11,
			drift: 0.08,
			bands: 3,
			thickness: 9,
			wave_amp: 6,
			wave_freq: 0.16,
			curtain_len: 15,
			hue: 138,
			hue_sp: 26,
			sat: 0.72,
			lmin: 0.2,
			lmax: 0.74,
			brighten_p: 0,
			shift_p: 0,
			fade_p: 0,
			brighten_dur: 42,
			brighten_mult: 1.45,
			shift_dur: 64,
			shift_amt: 1.1,
			fade_dur: 58,
			fade_mult: 0.6,
		},
		'wheat-field': {
			intro_dur: 60,
			intro_breeze: 0.16,
			ending_dur: 70,
			ending_linger: 20,
			ending_sway: 0.08,
			density: 0.48,
			speed: 0.12,
			drift: 0.16,
			sway: 0.68,
			wave_freq: 0.18,
			field_top: 0.62,
			stalk_h: 18,
			layers: 3,
			hue: 46,
			hue_sp: 18,
			sat: 0.64,
			lmin: 0.3,
			lmax: 0.76,
			gust_p: 0,
			calm_p: 0,
			gust_dur: 50,
			gust_mult: 1.85,
			calm_dur: 72,
			calm_mult: 0.4,
		},
		beach: {
			intro_dur: 55,
			intro_tide: 0.18,
			ending_dur: 65,
			ending_linger: 18,
			ending_wet: 0.1,
			shoreline: 0.58,
			tide_amp: 6,
			wave_amp: 2.4,
			wave_freq: 0.18,
			speed: 0.1,
			slope: 0.16,
			foam: 0.36,
			shimmer: 0.22,
			hue: 198,
			hue_sp: 16,
			sat: 0.5,
			lmin: 0.28,
			lmax: 0.82,
			high_tide_p: 0,
			low_tide_p: 0,
			foam_burst_p: 0,
			high_tide_dur: 60,
			high_tide_push: 1.4,
			low_tide_dur: 58,
			low_tide_pull: 1.2,
			foam_burst_dur: 34,
			foam_burst_mult: 1.9,
		},
		campfire: {
			intro_dur: 45,
			intro_glow: 0.14,
			ending_dur: 60,
			ending_linger: 24,
			ending_glow: 0.08,
			flame_height: 14,
			flame_width: 10,
			flame_speed: 0.12,
			flicker: 0.72,
			ember_rate: 0.26,
			ember_speed: 0.62,
			glow: 0.54,
			hue: 24,
			hue_sp: 18,
			sat: 0.82,
			lmin: 0.32,
			lmax: 0.94,
			crackle_p: 0,
			lull_p: 0,
			crackle_dur: 36,
			crackle_mult: 1.85,
			lull_dur: 68,
			lull_mult: 0.55,
		},
		windmill: {
			intro_dur: 45,
			intro_turn: 0.12,
			ending_dur: 60,
			ending_linger: 20,
			ending_turn: 0.05,
			turn_speed: 0.08,
			blade_len: 14,
			blade_width: 1.8,
			tower_height: 20,
			tower_width: 6,
			horizon: 0.72,
			glow: 0.18,
			hue: 28,
			hue_sp: 18,
			sat: 0.42,
			lmin: 0.18,
			lmax: 0.82,
			gust_p: 0,
			lull_p: 0,
			gust_dur: 50,
			gust_mult: 1.9,
			lull_dur: 72,
			lull_mult: 0.45,
		},
		starfield: {
			intro_dur: 50,
			intro_density: 0.08,
			ending_dur: 60,
			ending_linger: 16,
			ending_density: 0.03,
			density: 0.22,
			speed: 0.12,
			drift: 0.04,
			layers: 3,
			size: 1,
			hue: 218,
			hue_sp: 18,
			sat: 0.18,
			lmin: 0.55,
			lmax: 0.95,
			shooting_star_p: 0,
			twinkle_burst_p: 0,
			shooting_star_dur: 26,
			shooting_star_mult: 1.8,
			twinkle_burst_dur: 42,
			twinkle_burst_mult: 1.7,
		},
		'autumn-leaves': {
			intro_dur: 55,
			intro_density: 0.12,
			ending_dur: 60,
			ending_linger: 18,
			ending_density: 0.04,
			density: 0.24,
			speed: 0.44,
			drift: 0.18,
			sway: 0.86,
			layers: 2,
			size: 1.2,
			hue: 28,
			hue_sp: 24,
			sat: 0.62,
			lmin: 0.38,
			lmax: 0.78,
			gust_p: 0,
			lull_p: 0,
			swirl_p: 0,
			gust_dur: 48,
			gust_mult: 1.9,
			lull_dur: 72,
			lull_mult: 0.35,
			swirl_dur: 52,
			swirl_pull: 1.15,
		},
		snow: {
			intro_dur: 60,
			intro_density: 0.16,
			ending_dur: 70,
			ending_linger: 22,
			ending_density: 0.08,
			density: 0.32,
			speed: 0.48,
			drift: 0.08,
			sway: 0.42,
			layers: 3,
			size: 1,
			hue: 210,
			hue_sp: 12,
			sat: 0.16,
			lmin: 0.74,
			lmax: 0.98,
			gust_p: 0,
			calm_p: 0,
			gust_dur: 55,
			gust_mult: 1.85,
			calm_dur: 80,
			calm_mult: 0.42,
		},
	};

	function applyProceduralDefaults(kind, cfg) {
		const base = PROCEDURAL_DEFAULTS[kind] || {};
		const c = Object.assign({}, base, cfg || {});
		switch (kind) {
			case 'wheat-field':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_breeze = clamp01(c.intro_breeze);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_sway = clamp01(c.ending_sway);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.sway <= 0) c.sway = base.sway;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.field_top <= 0) c.field_top = base.field_top;
				if (c.stalk_h <= 0) c.stalk_h = base.stalk_h;
				if (c.layers < 1) c.layers = base.layers;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'beach':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_tide = clamp01(c.intro_tide);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_wet = clamp01(c.ending_wet);
				if (c.shoreline <= 0) c.shoreline = base.shoreline;
				if (c.tide_amp <= 0) c.tide_amp = base.tide_amp;
				if (c.wave_amp <= 0) c.wave_amp = base.wave_amp;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.foam <= 0) c.foam = base.foam;
				if (c.shimmer <= 0) c.shimmer = base.shimmer;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.high_tide_dur <= 0) c.high_tide_dur = base.high_tide_dur;
				if (c.high_tide_push <= 0) c.high_tide_push = base.high_tide_push;
				if (c.low_tide_dur <= 0) c.low_tide_dur = base.low_tide_dur;
				if (c.low_tide_pull <= 0) c.low_tide_pull = base.low_tide_pull;
				if (c.foam_burst_dur <= 0) c.foam_burst_dur = base.foam_burst_dur;
				if (c.foam_burst_mult <= 0) c.foam_burst_mult = base.foam_burst_mult;
				break;
			case 'campfire':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_glow = clamp01(c.intro_glow);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_glow = clamp01(c.ending_glow);
				if (c.flame_height <= 0) c.flame_height = base.flame_height;
				if (c.flame_width <= 0) c.flame_width = base.flame_width;
				if (c.flame_speed <= 0) c.flame_speed = base.flame_speed;
				if (c.flicker <= 0) c.flicker = base.flicker;
				if (c.ember_rate <= 0) c.ember_rate = base.ember_rate;
				if (c.ember_speed <= 0) c.ember_speed = base.ember_speed;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.crackle_dur <= 0) c.crackle_dur = base.crackle_dur;
				if (c.crackle_mult <= 0) c.crackle_mult = base.crackle_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				break;
			case 'windmill':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_turn = clamp01(c.intro_turn);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_turn = clamp01(c.ending_turn);
				if (c.turn_speed <= 0) c.turn_speed = base.turn_speed;
				if (c.blade_len <= 0) c.blade_len = base.blade_len;
				if (c.blade_width <= 0) c.blade_width = base.blade_width;
				if (c.tower_height <= 0) c.tower_height = base.tower_height;
				if (c.tower_width <= 0) c.tower_width = base.tower_width;
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				break;
			case 'aurora':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_glow = clamp01(c.intro_glow);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_glow = clamp01(c.ending_glow);
				if (c.intensity <= 0) c.intensity = base.intensity;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.bands < 1) c.bands = base.bands;
				if (c.thickness <= 0) c.thickness = base.thickness;
				if (c.wave_amp <= 0) c.wave_amp = base.wave_amp;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.curtain_len <= 0) c.curtain_len = base.curtain_len;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.brighten_dur <= 0) c.brighten_dur = base.brighten_dur;
				if (c.brighten_mult <= 0) c.brighten_mult = base.brighten_mult;
				if (c.shift_dur <= 0) c.shift_dur = base.shift_dur;
				if (c.shift_amt <= 0) c.shift_amt = base.shift_amt;
				if (c.fade_dur <= 0) c.fade_dur = base.fade_dur;
				if (c.fade_mult <= 0) c.fade_mult = base.fade_mult;
				break;
			case 'starfield':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_density = clamp01(c.intro_density);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_density = clamp01(c.ending_density);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.layers < 1) c.layers = base.layers;
				if (c.size <= 0) c.size = base.size;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.shooting_star_dur <= 0) c.shooting_star_dur = base.shooting_star_dur;
				if (c.shooting_star_mult <= 0) c.shooting_star_mult = base.shooting_star_mult;
				if (c.twinkle_burst_dur <= 0) c.twinkle_burst_dur = base.twinkle_burst_dur;
				if (c.twinkle_burst_mult <= 0) c.twinkle_burst_mult = base.twinkle_burst_mult;
				break;
			case 'autumn-leaves':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_density = clamp01(c.intro_density);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_density = clamp01(c.ending_density);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.layers < 1) c.layers = base.layers;
				if (c.size <= 0) c.size = base.size;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				if (c.swirl_dur <= 0) c.swirl_dur = base.swirl_dur;
				if (c.swirl_pull <= 0) c.swirl_pull = base.swirl_pull;
				break;
			case 'snow':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_density = clamp01(c.intro_density);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_density = clamp01(c.ending_density);
				if (c.density <= 0) c.density = base.density;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.layers < 1) c.layers = base.layers;
				if (c.size <= 0) c.size = base.size;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.gust_dur <= 0) c.gust_dur = base.gust_dur;
				if (c.gust_mult <= 0) c.gust_mult = base.gust_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
		}
		return c;
	}

	function positiveMod(value, mod) {
		if (mod === 0) return 0;
		return ((value % mod) + mod) % mod;
	}

	class ProceduralScene {
		constructor(kind, w, h, cfg, seed) {
			this.kind = kind;
			this.w = w;
			this.h = h;
			this.seed = Number(seed || Date.now());
			this.tick = 0;
			this.timers = {};
			this.values = {};
			this.cfg = applyProceduralDefaults(kind, cfg);
		}

		setConfig(cfg) {
			this.cfg = applyProceduralDefaults(this.kind, Object.assign({}, this.cfg, cfg));
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

		triggerEvent(name) {
			switch (this.kind) {
				case 'wheat-field':
					return this._triggerWheatField(name);
				case 'beach':
					return this._triggerBeach(name);
				case 'campfire':
					return this._triggerCampfire(name);
				case 'windmill':
					return this._triggerWindmill(name);
				case 'aurora':
					return this._triggerAurora(name);
				case 'starfield':
					return this._triggerStarfield(name);
				case 'autumn-leaves':
					return this._triggerAutumnLeaves(name);
				case 'snow':
					return this._triggerSnow(name);
			}
			return false;
		}

		step() {
			this.tick++;
			for (const key of Object.keys(this.timers)) {
				if (this.timers[key] > 0) this.timers[key]--;
			}
			if (this.kind === 'snow' && (!this.timers.gust || this.timers.gust <= 0)) {
				this.values.gust_push = 0;
			}
			if (this.kind === 'autumn-leaves') {
				if (!this.timers.gust || this.timers.gust <= 0) this.values.gust_push = 0;
				if (!this.timers.swirl || this.timers.swirl <= 0) this.values.swirl_spin = 0;
			}
			if (this.kind === 'aurora') {
				if (!this.timers.brighten || this.timers.brighten <= 0) this.values.brighten_gain = 0;
				if (!this.timers.shift || this.timers.shift <= 0) {
					this.values.shift_push = 0;
					this.values.shift_seed = 0;
				}
			}
			if (this.kind === 'wheat-field' && (!this.timers.gust || this.timers.gust <= 0)) {
				this.values.gust_push = 0;
			}
			if (this.kind === 'beach') {
				if ((!this.timers['high-tide'] || this.timers['high-tide'] <= 0) && (!this.timers['low-tide'] || this.timers['low-tide'] <= 0)) {
					this.values.tide_bias = 0;
				}
				if (!this.timers['foam-burst'] || this.timers['foam-burst'] <= 0) {
					this.values.foam_gain = 1;
				}
			}
			if (this.kind === 'campfire' && (!this.timers.crackle || this.timers.crackle <= 0)) {
				this.values.crackle_gain = 1;
			}
			if (this.kind === 'windmill' && (!this.timers.gust || this.timers.gust <= 0)) {
				this.values.gust_gain = 1;
			}
		}

		render(ctx, canvasW, canvasH, opts) {
			switch (this.kind) {
				case 'wheat-field':
					return this._renderWheatField(ctx, canvasW, canvasH, opts);
				case 'beach':
					return this._renderBeach(ctx, canvasW, canvasH, opts);
				case 'campfire':
					return this._renderCampfire(ctx, canvasW, canvasH, opts);
				case 'windmill':
					return this._renderWindmill(ctx, canvasW, canvasH, opts);
				case 'aurora':
					return this._renderAurora(ctx, canvasW, canvasH, opts);
				case 'starfield':
					return this._renderStarfield(ctx, canvasW, canvasH, opts);
				case 'autumn-leaves':
					return this._renderAutumnLeaves(ctx, canvasW, canvasH, opts);
				case 'snow':
					return this._renderSnow(ctx, canvasW, canvasH, opts);
			}
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

		_fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, w, h, color, alpha) {
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), Math.max(1, Math.ceil(w * sx || ceilSx)), Math.max(1, Math.ceil(h * sy || ceilSy)));
			ctx.globalAlpha = 1;
		}

		_triggerSnow(name) {
			const rng = this._eventRng(name.length + 17);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.5 ? -1 : 1) * this.cfg.gust_mult * (0.45 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.values.gust_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerAutumnLeaves(name) {
			const rng = this._eventRng(name.length + 29);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.5 ? -1 : 1) * this.cfg.gust_mult * (0.5 + rng() * 0.7);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'swirl':
					this.timers.swirl = jitterInt(rng, this.cfg.swirl_dur, 0.3);
					this.values.swirl_spin = (rng() < 0.5 ? -1 : 1) * this.cfg.swirl_pull * (0.65 + rng() * 0.45);
					this.values.swirl_row = Math.max(8, this.h / 3) + rng() * Math.max(1, this.h / 2);
					this.values.swirl_col = rng() * this.w;
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.swirl = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.values.swirl_spin = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.swirl = 0;
					this.values.gust_push = 0;
					this.values.swirl_spin = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerWheatField(name) {
			const rng = this._eventRng(name.length + 47);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_push = (rng() < 0.35 ? -1 : 1) * this.cfg.gust_mult * (0.55 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.gust_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.calm = 0;
					this.values.gust_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerBeach(name) {
			const rng = this._eventRng(name.length + 61);
			switch (name) {
				case 'high-tide':
					this.timers['high-tide'] = jitterInt(rng, this.cfg.high_tide_dur, 0.3);
					this.timers['low-tide'] = 0;
					this.values.tide_bias = this.cfg.high_tide_push * (0.65 + rng() * 0.55);
					return true;
				case 'low-tide':
					this.timers['low-tide'] = jitterInt(rng, this.cfg.low_tide_dur, 0.3);
					this.timers['high-tide'] = 0;
					this.values.tide_bias = -this.cfg.low_tide_pull * (0.65 + rng() * 0.55);
					return true;
				case 'foam-burst':
					this.timers['foam-burst'] = jitterInt(rng, this.cfg.foam_burst_dur, 0.3);
					this.values.foam_gain = this.cfg.foam_burst_mult * (0.85 + rng() * 0.35);
					return true;
				case 'intro':
					this.timers['high-tide'] = 0;
					this.timers['low-tide'] = 0;
					this.timers['foam-burst'] = 0;
					this.timers.ending = 0;
					this.values.tide_bias = 0;
					this.values.foam_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['high-tide'] = 0;
					this.timers['low-tide'] = 0;
					this.timers['foam-burst'] = 0;
					this.values.tide_bias = 0;
					this.values.foam_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerCampfire(name) {
			const rng = this._eventRng(name.length + 67);
			switch (name) {
				case 'crackle':
					this.timers.crackle = jitterInt(rng, this.cfg.crackle_dur, 0.3);
					this.values.crackle_gain = this.cfg.crackle_mult * (0.75 + rng() * 0.5);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'intro':
					this.timers.crackle = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.crackle_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.crackle = 0;
					this.timers.lull = 0;
					this.values.crackle_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerWindmill(name) {
			const rng = this._eventRng(name.length + 71);
			switch (name) {
				case 'gust':
					this.timers.gust = jitterInt(rng, this.cfg.gust_dur, 0.3);
					this.values.gust_gain = this.cfg.gust_mult * (0.75 + rng() * 0.45);
					return true;
				case 'lull':
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					return true;
				case 'intro':
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.gust_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.gust = 0;
					this.timers.lull = 0;
					this.values.gust_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerAurora(name) {
			const rng = this._eventRng(name.length + 53);
			switch (name) {
				case 'brighten':
					this.timers.brighten = jitterInt(rng, this.cfg.brighten_dur, 0.3);
					this.values.brighten_gain = this.cfg.brighten_mult * (0.85 + rng() * 0.35);
					return true;
				case 'shift':
					this.timers.shift = jitterInt(rng, this.cfg.shift_dur, 0.3);
					this.values.shift_push = (rng() < 0.5 ? -1 : 1) * this.cfg.shift_amt * (0.55 + rng() * 0.55);
					this.values.shift_seed = rng() * Math.PI * 2;
					return true;
				case 'fade':
					this.timers.fade = jitterInt(rng, this.cfg.fade_dur, 0.3);
					return true;
				case 'intro':
					this.timers.brighten = 0;
					this.timers.shift = 0;
					this.timers.fade = 0;
					this.timers.ending = 0;
					this.values.brighten_gain = 0;
					this.values.shift_push = 0;
					this.values.shift_seed = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.brighten = 0;
					this.timers.shift = 0;
					this.timers.fade = 0;
					this.values.brighten_gain = 0;
					this.values.shift_push = 0;
					this.values.shift_seed = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerStarfield(name) {
			const rng = this._eventRng(name.length + 41);
			switch (name) {
				case 'shooting-star':
					this.timers['shooting-star'] = jitterInt(rng, this.cfg.shooting_star_dur, 0.3);
					this.values['shooting-star_total'] = this.timers['shooting-star'];
					this.values.shooting_dir = rng() < 0.5 ? -1 : 1;
					this.values.shooting_row = 6 + rng() * Math.max(4, this.h / 3);
					this.values.shooting_start = rng() * this.w;
					return true;
				case 'twinkle-burst':
					this.timers['twinkle-burst'] = jitterInt(rng, this.cfg.twinkle_burst_dur, 0.3);
					return true;
				case 'intro':
					this.timers.ending = 0;
					this.timers['shooting-star'] = 0;
					this.timers['twinkle-burst'] = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['shooting-star'] = 0;
					this.timers['twinkle-burst'] = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_densityLevelSnow() {
			let level = this.cfg.density;
			if (this.timers.gust > 0) level *= 1.28;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.02, level);
		}

		_densityLevelAutumnLeaves() {
			let level = this.cfg.density;
			if (this.timers.gust > 0) level *= 1.22;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.015, level);
		}

		_densityLevelStarfield() {
			let level = this.cfg.density;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_density + (1 - this.cfg.intro_density) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_density) * progress;
			}
			return Math.max(0.02, level);
		}

		_intensityLevelAurora() {
			let level = this.cfg.intensity;
			if (this.timers.brighten > 0) level *= this.values.brighten_gain || this.cfg.brighten_mult;
			if (this.timers.fade > 0) level *= this.cfg.fade_mult;
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
			return Math.max(0.02, level);
		}

		_motionLevelWheatField() {
			let level = this.cfg.sway;
			if (this.timers.gust > 0) level *= 1 + Math.abs(this.values.gust_push || this.cfg.gust_mult) * 0.35;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_breeze + (1 - this.cfg.intro_breeze) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_sway) * progress;
			}
			return Math.max(0.05, level);
		}

		_tideLevelBeach() {
			let level = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_tide + (1 - this.cfg.intro_tide) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_wet) * progress;
			}
			return Math.max(0.05, level);
		}

		_flameLevelCampfire() {
			let level = 1;
			if (this.timers.crackle > 0) level *= this.values.crackle_gain || this.cfg.crackle_mult;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
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
			return Math.max(0.05, level);
		}

		_rotationLevelWindmill() {
			let level = 1;
			if (this.timers.gust > 0) level *= this.values.gust_gain || this.cfg.gust_mult;
			if (this.timers.lull > 0) level *= this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_turn + (1 - this.cfg.intro_turn) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_turn) * progress;
			}
			return Math.max(0.03, level);
		}

		_renderSnow(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#09111d');
				sky.addColorStop(0.58, '#102033');
				sky.addColorStop(1, '#17263a');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.8);

			const moonX = canvasW * (0.16 + this._hash(401) * 0.18);
			const moonY = canvasH * (0.14 + this._hash(402) * 0.08);
			const moonR = Math.max(12, Math.min(canvasW, canvasH) * 0.035);
			const moon = ctx.createRadialGradient(moonX, moonY, 0, moonX, moonY, moonR * 2.6);
			moon.addColorStop(0, 'rgba(225, 234, 255, 0.18)');
			moon.addColorStop(1, 'rgba(225, 234, 255, 0)');
			ctx.fillStyle = moon;
			ctx.fillRect(0, 0, canvasW, canvasH);

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const hue = ((this.cfg.hue - 8) % 360 + 360) % 360;
				const light = clamp01(0.06 + 0.2 * ratio);
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.55), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
			}

			const treeCount = 13;
			for (let i = 0; i < treeCount; i++) {
				const center = Math.floor((i + 0.5) * this.w / treeCount + (this._hash(500 + i) - 0.5) * 6);
				const trunkH = 1 + Math.floor(this._hash(530 + i) * 2);
				const crownH = 8 + Math.floor(this._hash(560 + i) * 9);
				const maxHalf = 2 + Math.floor(this._hash(590 + i) * 4);
				const hue = ((this.cfg.hue - 26) % 360 + 360) % 360;
				const treeColor = hslToRGB(hue, clamp01(this.cfg.sat * 0.6), 0.18 + this._hash(620 + i) * 0.08);
				for (let row = 0; row < crownH; row++) {
					const width = Math.max(1, maxHalf - Math.floor(row / 2));
					const y = groundRow - crownH + row;
					for (let dx = -width; dx <= width; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center + dx, y, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
					}
				}
				for (let row = 0; row < trunkH; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center, groundRow + row - trunkH, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
				}
			}

			const density = this._densityLevelSnow();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(8, Math.round(this.w * density * (0.35 + layerRatio * 0.8)));
				const baseSpeed = this.cfg.speed * (0.4 + layerRatio * 0.85);
				const drift = this.cfg.drift * (0.35 + layerRatio * 0.65) + (this.values.gust_push || 0) * 0.035 * (0.5 + layerRatio * 0.65);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1000 + i;
					const baseX = this._hash(1000 + idx) * this.w;
					const baseY = this._hash(2000 + idx) * Math.max(1, groundRow - 2);
					const sway = (this._hash(3000 + idx) * 2 - 1) * this.cfg.sway * (1.4 + layerRatio * 2.4);
					const fall = baseY + this.tick * baseSpeed * (0.75 + this._hash(4000 + idx) * 0.5);
					const row = positiveMod(fall, Math.max(1, groundRow - 2));
					const col = positiveMod(baseX + this.tick * drift + Math.sin(this.tick * 0.035 + idx * 0.19) * sway, this.w);
					const hue = ((this.cfg.hue + (this._hash(5000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.35 + 0.55 * (0.3 + layerRatio * 0.7)));
					const alpha = clamp01(0.35 + 0.55 * (0.25 + layerRatio * 0.75));
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}
		}

		_renderAutumnLeaves(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#20170f');
				sky.addColorStop(0.52, '#58422b');
				sky.addColorStop(1, '#7c6042');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.82);

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const ground = hslToRGB(38, 0.35, 0.1 + ratio * 0.16);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${ground.r},${ground.g},${ground.b})`, 1);
			}

			for (let x = 0; x < this.w; x++) {
				const canopyDepth = 4 + Math.floor(this._hash(6100 + x) * 6);
				const shade = hslToRGB(24 + this._hash(6200 + x) * 18, 0.45, 0.14 + this._hash(6300 + x) * 0.08);
				for (let y = 0; y < canopyDepth; y++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${shade.r},${shade.g},${shade.b})`, 0.75);
				}
			}

			const density = this._densityLevelAutumnLeaves();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(6, Math.round(this.w * density * (0.28 + layerRatio * 0.62)));
				const baseSpeed = this.cfg.speed * (0.34 + layerRatio * 0.72);
				const drift = this.cfg.drift * (0.45 + layerRatio * 0.65) + (this.values.gust_push || 0) * 0.04 * (0.5 + layerRatio * 0.7);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio * 0.8));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1000 + i;
					const baseX = this._hash(7000 + idx) * this.w;
					const baseY = this._hash(8000 + idx) * Math.max(1, groundRow - 2);
					const flutter = (this._hash(9000 + idx) * 2 - 1) * this.cfg.sway * (2.4 + layerRatio * 2.8);
					let row = positiveMod(baseY + this.tick * baseSpeed * (0.7 + this._hash(10000 + idx) * 0.55), Math.max(1, groundRow - 2));
					let col = positiveMod(baseX + this.tick * drift + Math.sin(this.tick * 0.04 + idx * 0.23) * flutter, this.w);
					if (this.timers.swirl > 0) {
						const sr = this.values.swirl_row || this.h * 0.55;
						const sc = this.values.swirl_col || this.w * 0.5;
						const angle = Math.atan2(row - sr, col - sc) + (this.values.swirl_spin || 0) * 0.015;
						const radius = Math.hypot(col - sc, row - sr);
						col = positiveMod(sc + Math.cos(angle) * radius, this.w);
						row = Math.max(0, Math.min(groundRow - 2, sr + Math.sin(angle) * radius * 0.94));
					}
					const hue = ((this.cfg.hue + (this._hash(11000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + this._hash(12000 + idx) * 0.7));
					const alpha = clamp01(0.4 + layerRatio * 0.45);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if ((idx + this.tick) % 3 === 0) {
						const accent = hslToRGB((hue + 12) % 360, clamp01(this.cfg.sat * 0.85), clamp01(light * 1.08));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col) + (this._hash(13000 + idx) < 0.5 ? 1 : 0), Math.round(row), 1, size > 1 ? 1 : 0.8, `rgb(${accent.r},${accent.g},${accent.b})`, alpha * 0.8);
					}
				}
			}
		}

		_renderStarfield(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#050912');
				sky.addColorStop(0.6, '#090f20');
				sky.addColorStop(1, '#0b1128');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const density = this._densityLevelStarfield();
			const layers = Math.max(1, Math.round(this.cfg.layers));
			const burst = this.timers['twinkle-burst'] > 0 ? this.cfg.twinkle_burst_mult : 1;

			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const layerCount = Math.max(10, Math.round(this.w * density * (0.4 + layerRatio * 1.2)));
				const speed = this.cfg.speed * (0.18 + layerRatio * 0.82);
				const drift = this.cfg.drift * (0.25 + layerRatio * 0.9);
				const size = Math.max(1, Math.round(this.cfg.size + layerRatio));
				for (let i = 0; i < layerCount; i++) {
					const idx = layer * 1400 + i;
					const baseX = this._hash(15000 + idx) * this.w;
					const baseY = this._hash(16000 + idx) * this.h;
					const col = positiveMod(baseX + this.tick * drift * speed * 2, this.w);
					const row = baseY;
					const hue = ((this.cfg.hue + (this._hash(17000 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const twinkle = 0.4 + 0.6 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * (0.02 + this._hash(18000 + idx) * 0.03) + idx), 2);
					const light = clamp01((this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.3 + layerRatio * 0.7)) * twinkle * burst);
					const alpha = clamp01(0.35 + 0.25 * layerRatio + 0.25 * twinkle);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			if (this.timers['shooting-star'] > 0) {
				const total = Math.max(1, this.values['shooting-star_total'] || this.cfg.shooting_star_dur);
				const progress = 1 - (this.timers['shooting-star'] / total);
				const dir = this.values.shooting_dir || 1;
				const row = this.values.shooting_row || this.h * 0.25;
				const start = this.values.shooting_start || this.w * 0.25;
				const head = positiveMod(start + dir * progress * this.w * 0.6, this.w);
				for (let i = 0; i < 7; i++) {
					const fade = 1 - i / 7;
					const x = Math.round(head - dir * i * 1.5);
					const y = Math.round(row + i * 0.6);
					const light = clamp01(this.cfg.lmax * this.cfg.shooting_star_mult * fade * 0.55);
					const color = hslToRGB(this.cfg.hue - 8, clamp01(this.cfg.sat * 0.9), light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1 + (i === 0 ? 1 : 0), 1, `rgb(${color.r},${color.g},${color.b})`, fade);
				}
			}
		}

		_renderWheatField(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#16213f');
				sky.addColorStop(0.56, '#556785');
				sky.addColorStop(1, '#d2ad66');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const motion = this._motionLevelWheatField();
			const density = Math.max(0.08, this.cfg.density);
			const layers = Math.max(1, Math.round(this.cfg.layers));
			const gustPush = this.values.gust_push || 0;
			const fieldBase = Math.floor(this.h * this.cfg.field_top);

			const sunX = canvasW * (0.16 + this._hash(21000) * 0.18);
			const sunY = canvasH * (0.2 + this._hash(21001) * 0.06);
			const sunR = Math.max(18, Math.min(canvasW, canvasH) * 0.06);
			const sun = ctx.createRadialGradient(sunX, sunY, 0, sunX, sunY, sunR * 2.8);
			sun.addColorStop(0, 'rgba(255, 228, 153, 0.2)');
			sun.addColorStop(1, 'rgba(255, 228, 153, 0)');
			ctx.fillStyle = sun;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const hillColor = hslToRGB(38, 0.18, 0.3);
			const hillPoints = [];
			for (let i = 0; i <= 8; i++) {
				hillPoints.push(Math.floor(this.h * 0.56) - Math.floor(this._hash(21100 + i) * 5) - Math.floor((0.5 + 0.5 * Math.sin(i * 0.9 + this._hash(21200 + i) * 3)) * 5));
			}
			ctx.fillStyle = `rgb(${hillColor.r},${hillColor.g},${hillColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * 8;
				const idx = Math.min(7, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const y = hillPoints[idx] + (hillPoints[idx + 1] - hillPoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(y * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const groundGrad = ctx.createLinearGradient(0, Math.floor(fieldBase * sy), 0, canvasH);
			groundGrad.addColorStop(0, '#b28d45');
			groundGrad.addColorStop(0.45, '#be9c56');
			groundGrad.addColorStop(1, '#8e6e32');
			ctx.fillStyle = groundGrad;
			ctx.fillRect(0, Math.floor(fieldBase * sy), canvasW, canvasH - Math.floor(fieldBase * sy));

			for (let layer = 0; layer < layers; layer++) {
				const layerRatio = layers === 1 ? 1 : layer / (layers - 1);
				const amp = motion * (2.4 + layerRatio * 4.8);
				const speed = this.cfg.speed * (0.4 + layerRatio * 0.65);
				const drift = this.cfg.drift * (0.25 + layerRatio * 0.75) + gustPush * 0.04 * (0.5 + layerRatio * 0.5);
				const topBase = fieldBase - this.cfg.stalk_h * (0.28 + layerRatio * 0.46);
				const tipChance = clamp01(density * (0.4 + layerRatio * 0.22));
				for (let x = 0; x < this.w;) {
					const clumpSeed = layer * 4000 + x;
					const width = Math.max(1, Math.min(4, Math.round(1 + this._hash(21700 + clumpSeed) * (1.2 + layerRatio * 2.4))));
					const sampleX = Math.min(this.w - 1, x + width * 0.5);
					const idx = layer * 2000 + sampleX;
					const wave = Math.sin(sampleX * this.cfg.wave_freq * (0.85 + layerRatio * 0.25) + this.tick * speed + layer * 1.7);
					const subWave = Math.sin(sampleX * this.cfg.wave_freq * 0.42 - this.tick * speed * 0.62 + layer * 2.3);
					const lean = wave * amp + subWave * amp * 0.32 + Math.sin(this.tick * 0.012 + sampleX * 0.05) * drift * 3.2;
					const top = Math.max(0, Math.min(this.h - 2, topBase + lean + this._hash(21300 + idx) * 2));
					const depth = Math.max(6, Math.round(this.cfg.stalk_h * (0.48 + layerRatio * 0.42)));
					const hue = ((this.cfg.hue + (this._hash(21400 + idx) * 2 - 1) * this.cfg.hue_sp) % 360 + 360) % 360;
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.18 + layerRatio * 0.6));
					const alpha = clamp01(0.34 + layerRatio * 0.24);
					const color = hslToRGB(hue, this.cfg.sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, Math.round(top), width, depth, `rgb(${color.r},${color.g},${color.b})`, alpha);
					const shadow = hslToRGB(hue, clamp01(this.cfg.sat * 0.72), clamp01(light * 0.76));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, Math.round(top), width, Math.max(1, Math.round(depth * 0.28)), `rgb(${shadow.r},${shadow.g},${shadow.b})`, clamp01(alpha * 0.55));

					if (this._hash(21500 + idx) < tipChance) {
						const tipHeight = 1 + Math.floor(this._hash(21600 + idx) * (1 + layerRatio * 3));
						const tipX = Math.max(0, Math.min(this.w - 1, Math.round(x + width * 0.5 + lean * 0.18)));
						const accent = hslToRGB((hue + 4) % 360, clamp01(this.cfg.sat * 0.82), clamp01(light * 1.08));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, tipX, Math.round(top) - tipHeight + 1, 1, tipHeight, `rgb(${accent.r},${accent.g},${accent.b})`, clamp01(alpha * 0.9));
					}
					x += width;
				}
			}
		}

		_renderWindmill(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue + 210) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.95));
				const skyMid = hslToRGB((this.cfg.hue + 248) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.32));
				const skyLow = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.78));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.58, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.min(this.h - 8, Math.floor(this.h * this.cfg.horizon)));
			const centerX = Math.floor(this.w * 0.58);
			const rotationLevel = this._rotationLevelWindmill();
			const angle = this.tick * this.cfg.turn_speed * rotationLevel + Math.PI * 0.08;
			const towerH = Math.max(10, Math.round(this.cfg.tower_height));
			const towerW = Math.max(3, Math.round(this.cfg.tower_width));
			const bladeLen = Math.max(5, Math.round(this.cfg.blade_len));
			const bladeWidth = Math.max(1, this.cfg.blade_width);

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 3) * sy), 0, Math.floor((horizon + 7) * sy));
			horizonGlow.addColorStop(0, `rgba(255, 214, 163, ${0.04 + this.cfg.glow * 0.12})`);
			horizonGlow.addColorStop(1, 'rgba(255, 214, 163, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 3) * sy), canvasW, Math.ceil(12 * sy));

			const hillRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const broad = Math.sin(x * 0.045 + 0.4) * 1.2 + Math.sin(x * 0.012 + 1.3) * 2.1;
				const mound = Math.exp(-Math.pow((x - centerX) / 18, 2)) * 6.2 + Math.exp(-Math.pow((x - this.w * 0.18) / 24, 2)) * 2.1;
				hillRows[x] = Math.round(horizon + broad - mound);
			}

			const hillColor = hslToRGB((this.cfg.hue + 205) % 360, clamp01(this.cfg.sat * 0.16), 0.08);
			ctx.fillStyle = `rgb(${hillColor.r},${hillColor.g},${hillColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				ctx.lineTo(Math.floor(x * sx), Math.floor(hillRows[x] * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const baseY = hillRows[Math.max(0, Math.min(this.w - 1, centerX))];
			const hubY = baseY - towerH + 2;
			const millColor = hslToRGB((this.cfg.hue + 208) % 360, clamp01(this.cfg.sat * 0.08), 0.1);
			for (let y = hubY; y <= baseY; y++) {
				const ratio = (y - hubY) / Math.max(1, baseY - hubY);
				const half = Math.max(1, Math.round((towerW * (0.38 + ratio * 0.62)) * 0.5));
				for (let dx = -half; dx <= half; dx++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, y, 1, 1, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);
				}
			}

			for (let dx = -Math.max(2, Math.round(towerW * 0.42)); dx <= Math.max(2, Math.round(towerW * 0.42)); dx++) {
				const roofY = hubY - 2 + Math.abs(dx) * 0.4;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, Math.round(roofY), 1, 1, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);
			}

			const windowGlow = hslToRGB(42, 0.72, clamp01(0.38 + this.cfg.glow * 0.5));
			const windowY = Math.round(hubY + towerH * 0.46);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX, windowY, 1, 2, `rgb(${windowGlow.r},${windowGlow.g},${windowGlow.b})`, clamp01(0.18 + this.cfg.glow * 0.58));

			const bladeColor = hslToRGB((this.cfg.hue + 210) % 360, clamp01(this.cfg.sat * 0.08), 0.13);
			for (let blade = 0; blade < 4; blade++) {
				const theta = angle + blade * Math.PI * 0.5;
				const px = -Math.sin(theta);
				const py = Math.cos(theta) * 0.88;
				for (let r = 1; r <= bladeLen; r++) {
					const fade = 1 - r / Math.max(1, bladeLen);
					const bx = centerX + Math.cos(theta) * r;
					const by = hubY + Math.sin(theta) * r * 0.88;
					const half = Math.max(0, Math.round(bladeWidth * fade * 0.55));
					for (let spread = -half; spread <= half; spread++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(bx + px * spread * 0.7), Math.round(by + py * spread * 0.7), 1, 1, `rgb(${bladeColor.r},${bladeColor.g},${bladeColor.b})`, 1);
					}
				}
			}

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - 1, hubY - 1, 3, 3, `rgb(${millColor.r},${millColor.g},${millColor.b})`, 1);

			const grassColor = hslToRGB((this.cfg.hue + 120) % 360, 0.16, 0.14);
			for (let x = 0; x < this.w; x += 2) {
				const top = hillRows[x];
				if ((x + this.tick) % 5 !== 0) continue;
				const sway = this.timers.gust > 0 ? (this.values.gust_gain || this.cfg.gust_mult) * 0.2 : this.timers.lull > 0 ? -this.cfg.lull_mult * 0.08 : 0.04;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + Math.round(Math.sin(this.tick * 0.05 + x * 0.1) * sway), top - 1, 1, 2, `rgb(${grassColor.r},${grassColor.g},${grassColor.b})`, 0.28);
			}
		}

		_renderCampfire(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#08111a');
				sky.addColorStop(0.62, '#0f1520');
				sky.addColorStop(1, '#16110c');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.84);
			const centerX = Math.floor(this.w * 0.5);
			const flameLevel = this._flameLevelCampfire();
			const crackleGain = this.values.crackle_gain || 1;
			const halfW = Math.max(2, Math.round(this.cfg.flame_width * 0.5));
			const flameH = Math.max(4, this.cfg.flame_height * (0.52 + flameLevel * 0.38));
			const speed = this.tick * this.cfg.flame_speed * 1.7;

			for (let y = groundRow; y < this.h; y++) {
				const ratio = (y - groundRow) / Math.max(1, this.h - groundRow);
				const ground = hslToRGB(18, 0.24, 0.08 + ratio * 0.12);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${ground.r},${ground.g},${ground.b})`, 1);
			}

			const glowStrength = clamp01(this.cfg.glow * (0.6 + flameLevel * 0.45));
			const glowX = centerX * sx;
			const glowY = groundRow * sy;
			const glowR = Math.max(28, Math.min(canvasW, canvasH) * (0.08 + glowStrength * 0.12));
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			glow.addColorStop(0, `rgba(255, 178, 82, ${0.24 + glowStrength * 0.22})`);
			glow.addColorStop(0.42, `rgba(255, 120, 44, ${0.12 + glowStrength * 0.12})`);
			glow.addColorStop(1, 'rgba(255, 120, 44, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			const vignette = ctx.createRadialGradient(glowX, glowY, glowR * 0.35, glowX, glowY, glowR * 2.4);
			vignette.addColorStop(0, 'rgba(0, 0, 0, 0)');
			vignette.addColorStop(1, 'rgba(0, 0, 0, 0.26)');
			ctx.fillStyle = vignette;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const logColor = hslToRGB(20, 0.46, 0.22);
			const logHighlight = hslToRGB(24, 0.44, 0.32);
			const logHalf = halfW + 2;
			for (let dx = -logHalf; dx <= logHalf; dx++) {
				const rowA = groundRow + 1 + Math.round(dx * 0.12);
				const rowB = groundRow + 1 - Math.round(dx * 0.1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowA, 1, 1, `rgb(${logColor.r},${logColor.g},${logColor.b})`, 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowB, 1, 1, `rgb(${logColor.r},${logColor.g},${logColor.b})`, 0.82);
				if ((dx + logHalf) % 3 === 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, rowA, 1, 1, `rgb(${logHighlight.r},${logHighlight.g},${logHighlight.b})`, 0.34);
				}
			}

			for (let dx = -halfW; dx <= halfW; dx++) {
				const coalHeat = 0.45 + 0.55 * Math.pow(0.5 + 0.5 * Math.sin(speed * 1.8 + dx * 0.8), 2);
				const coal = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.88), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.22 + coalHeat * 0.45)));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, groundRow, 1, 1, `rgb(${coal.r},${coal.g},${coal.b})`, 0.28 + coalHeat * 0.42);
			}

			for (let x = -halfW; x <= halfW; x++) {
				const nx = Math.abs(x) / Math.max(1, halfW);
				const widthShape = Math.max(0, 1 - Math.pow(nx, 1.32));
				if (widthShape <= 0.04) continue;
				const pulse = 0.8 + 0.2 * Math.sin(speed * 1.3 + x * 0.7 + this._hash(26000 + x + halfW) * 5);
				const columnH = Math.max(2, Math.round(flameH * widthShape * pulse));
				for (let y = 0; y < columnH; y++) {
					const lift = y / Math.max(1, columnH);
					const taper = 1 - lift;
					const sway = Math.sin(speed * 2.1 + x * 0.35 + y * 0.24 + this._hash(26100 + y * 31 + x + 400) * 6) * this.cfg.flicker * taper * 0.72;
					const col = Math.round(centerX + x + sway);
					const row = Math.round(groundRow - 1 - y);
					const hue = ((this.cfg.hue - lift * this.cfg.hue_sp * 0.34 + (this._hash(26200 + x * 17 + y + 700) * 2 - 1) * this.cfg.hue_sp * 0.08) % 360 + 360) % 360;
					const sat = clamp01(this.cfg.sat * (0.88 + taper * 0.16));
					const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.18 + taper * 0.8));
					const alpha = clamp01((0.12 + taper * 0.56) * (0.36 + widthShape * 0.64) * (0.72 + flameLevel * 0.18));
					const color = hslToRGB(hue, sat, light);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					if (widthShape > 0.35 && lift < 0.6 && (x + y) % 2 === 0) {
						const core = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.74), clamp01(this.cfg.lmax * (0.72 + taper * 0.24)));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${core.r},${core.g},${core.b})`, alpha * 0.52);
					}
				}
			}

			const emberCount = Math.max(4, Math.round(this.cfg.flame_width * (0.8 + this.cfg.ember_rate * 3.8) * (this.timers.crackle > 0 ? 0.92 + crackleGain * 0.3 : 1)));
			const maxRise = Math.max(10, Math.round(this.cfg.flame_height * 2.1 + this.cfg.ember_speed * 12));
			for (let i = 0; i < emberCount; i++) {
				const cycle = maxRise + 8 + Math.floor(this._hash(27000 + i) * 12);
				const progress = positiveMod(this.tick * this.cfg.ember_speed * (0.7 + this._hash(27100 + i) * 0.7) + this._hash(27200 + i) * cycle, cycle);
				if (progress > maxRise) continue;
				const rise = progress;
				const fade = 1 - rise / Math.max(1, maxRise);
				const drift = (this._hash(27300 + i) * 2 - 1) * (1.2 + rise * 0.08) + Math.sin(speed + i * 0.7) * 0.6;
				const col = Math.round(centerX + drift);
				const row = Math.round(groundRow - 2 - rise);
				if (row < 1) continue;
				const size = fade > 0.72 && this.timers.crackle > 0 && this._hash(27400 + i) > 0.5 ? 2 : 1;
				const hue = ((this.cfg.hue - 6 + this._hash(27500 + i) * 10) % 360 + 360) % 360;
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.42 + fade * 0.5));
				const color = hslToRGB(hue, clamp01(this.cfg.sat * 0.8), light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, size, 1, `rgb(${color.r},${color.g},${color.b})`, clamp01((0.16 + fade * 0.68) * (0.78 + Math.max(0, crackleGain - 1) * 0.16)));
			}
		}

		_renderBeach(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#f4b17a');
				sky.addColorStop(0.38, '#f8d6a9');
				sky.addColorStop(0.68, '#cfe3e6');
				sky.addColorStop(1, '#8bb4c4');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.floor(this.h * 0.34));
			const tideLevel = this._tideLevelBeach();
			const tideBias = this.values.tide_bias || 0;
			const foamGain = this.values.foam_gain || 1;
			const tidePhase = this.tick * this.cfg.speed * 0.08;
			const baseShore = this.h * this.cfg.shoreline + Math.sin(tidePhase) * this.cfg.tide_amp * tideLevel * 0.34 + tideBias * 1.6;

			const sunX = canvasW * (0.16 + this._hash(24000) * 0.18);
			const sunY = canvasH * (0.18 + this._hash(24001) * 0.08);
			const sunR = Math.max(22, Math.min(canvasW, canvasH) * 0.085);
			const sun = ctx.createRadialGradient(sunX, sunY, 0, sunX, sunY, sunR * 2.8);
			sun.addColorStop(0, 'rgba(255, 239, 187, 0.38)');
			sun.addColorStop(0.34, 'rgba(255, 224, 168, 0.2)');
			sun.addColorStop(1, 'rgba(255, 224, 168, 0)');
			ctx.fillStyle = sun;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const haze = ctx.createLinearGradient(0, canvasH * 0.18, 0, canvasH * 0.6);
			haze.addColorStop(0, 'rgba(255, 246, 224, 0)');
			haze.addColorStop(1, 'rgba(255, 246, 224, 0.14)');
			ctx.fillStyle = haze;
			ctx.fillRect(0, canvasH * 0.16, canvasW, canvasH * 0.44);

			const waterTop = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmax * 0.74));
			const waterMid = hslToRGB(this.cfg.hue, this.cfg.sat, clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.44));
			const waterDeep = hslToRGB((this.cfg.hue + 6) % 360, clamp01(this.cfg.sat * 1.06), clamp01(this.cfg.lmin * 0.8));
			const waterGrad = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			waterGrad.addColorStop(0, `rgb(${waterTop.r},${waterTop.g},${waterTop.b})`);
			waterGrad.addColorStop(0.46, `rgb(${waterMid.r},${waterMid.g},${waterMid.b})`);
			waterGrad.addColorStop(1, `rgb(${waterDeep.r},${waterDeep.g},${waterDeep.b})`);
			ctx.fillStyle = waterGrad;
			ctx.fillRect(0, Math.floor(horizon * sy), canvasW, canvasH - Math.floor(horizon * sy));

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 1) * sy), 0, Math.floor((horizon + 4) * sy));
			horizonGlow.addColorStop(0, 'rgba(255, 247, 221, 0.35)');
			horizonGlow.addColorStop(1, 'rgba(255, 247, 221, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 1) * sy), canvasW, Math.ceil(7 * sy));

			for (let band = 0; band < 3; band++) {
				const y = Math.floor((horizon + 2 + band * 3 + Math.sin(tidePhase * 1.8 + band) * 1.2) * sy);
				ctx.fillStyle = `rgba(${waterTop.r},${waterTop.g},${waterTop.b},${0.08 + band * 0.03})`;
				ctx.fillRect(0, y, canvasW, Math.max(1, Math.ceil(sy)));
			}

			const shoreRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const nx = x / Math.max(1, this.w - 1);
				const slopeOffset = (nx - 0.5) * this.cfg.slope * this.h * 0.34;
				const wave = Math.sin(x * this.cfg.wave_freq + tidePhase * 2.2);
				const backwash = Math.sin(x * this.cfg.wave_freq * 0.46 - tidePhase * 1.6 + 0.8);
				const chop = Math.sin(x * this.cfg.wave_freq * 1.85 + tidePhase * 3.1 + this._hash(24100 + x) * 3);
				const shore = baseShore + slopeOffset + wave * this.cfg.wave_amp * (0.14 + tideLevel * 0.1) + backwash * this.cfg.wave_amp * 0.08 + chop * this.cfg.wave_amp * 0.03;
				shoreRows[x] = Math.max(horizon + 3, Math.min(this.h - 4, shore));
			}
			for (let pass = 0; pass < 2; pass++) {
				for (let x = 1; x < this.w - 1; x++) {
					shoreRows[x] = (shoreRows[x - 1] + shoreRows[x] * 2 + shoreRows[x + 1]) / 4;
				}
			}

			const sandTop = hslToRGB(39, 0.54, 0.8);
			const sandMid = hslToRGB(36, 0.48, 0.68);
			const sandLow = hslToRGB(33, 0.42, 0.54);
			const drawSandPath = () => {
				ctx.beginPath();
				ctx.moveTo(0, canvasH);
				for (let x = 0; x < this.w; x++) {
					ctx.lineTo(Math.floor(x * sx), Math.floor(shoreRows[x] * sy));
				}
				ctx.lineTo(canvasW, canvasH);
				ctx.closePath();
			};
			drawSandPath();
			const sandGrad = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			sandGrad.addColorStop(0, `rgb(${sandTop.r},${sandTop.g},${sandTop.b})`);
			sandGrad.addColorStop(0.55, `rgb(${sandMid.r},${sandMid.g},${sandMid.b})`);
			sandGrad.addColorStop(1, `rgb(${sandLow.r},${sandLow.g},${sandLow.b})`);
			ctx.fillStyle = sandGrad;
			ctx.fill();

			const duneColor = hslToRGB(31, 0.32, 0.42);
			const dunePoints = [];
			for (let i = 0; i <= 6; i++) {
				dunePoints.push(Math.floor(this.h * 0.74) + Math.floor(this._hash(24200 + i) * 5) + Math.floor(Math.sin(i * 1.08 + this._hash(24300 + i) * 2) * 2));
			}
			ctx.fillStyle = `rgba(${duneColor.r},${duneColor.g},${duneColor.b},0.16)`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * 6;
				const idx = Math.min(5, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const y = dunePoints[idx] + (dunePoints[idx + 1] - dunePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(y * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const wetColor = hslToRGB(34, 0.34, 0.34 + clamp01(0.22 + tideLevel * 0.36) * 0.12);
			const foamColor = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 1.02));
			const shimmerColor = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmax * 0.96));
			const shimmerLevel = clamp01(this.cfg.shimmer * (0.6 + tideLevel * 0.5));

			for (let x = 0; x < this.w; x++) {
				const shore = shoreRows[x];
				const surfRow = Math.round(shore);
				const wetBand = Math.max(2, Math.round(2 + tideLevel * 3 + Math.max(0, foamGain - 1) * 1.4));
				const foamBand = Math.max(1, Math.round(1 + this.cfg.foam * 2.8 + Math.max(0, foamGain - 1) * 1.4));

				for (let row = surfRow; row < Math.min(this.h, surfRow + wetBand); row++) {
					const fade = 1 - (row - shore) / Math.max(1, wetBand);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${wetColor.r},${wetColor.g},${wetColor.b})`, clamp01(0.14 + fade * 0.32));
				}

				for (let i = 0; i < foamBand; i++) {
					const row = surfRow - i;
					const pulse = 0.55 + 0.45 * Math.sin(this.tick * 0.05 + x * 0.18 + i * 0.9);
					const alpha = clamp01((0.12 + this.cfg.foam * 0.42) * foamGain * (0.5 + 0.5 * pulse));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, alpha);
				}

				if ((x + this.tick) % 2 === 0) {
					const depth = 0.18 + this._hash(24400 + x) * 0.56;
					const row = Math.max(horizon + 1, Math.floor(horizon + (shore - horizon) * depth));
					const width = 1 + Math.floor(this._hash(24500 + x) * 3);
					const blink = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.03 + x * 0.12), 2);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, width, 1, `rgb(${shimmerColor.r},${shimmerColor.g},${shimmerColor.b})`, clamp01((0.08 + shimmerLevel * 0.34) * blink));
				}

				if ((x + Math.floor(this.tick / 3)) % 7 === 0) {
					const pebbleRow = Math.min(this.h - 2, surfRow + wetBand + 1 + Math.floor(this._hash(24600 + x) * 8));
					const pebble = hslToRGB(34 + this._hash(24700 + x) * 10, 0.2, 0.4 + this._hash(24800 + x) * 0.12);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, pebbleRow, 1, 1, `rgb(${pebble.r},${pebble.g},${pebble.b})`, 0.22);
				}
			}
		}

		_renderAurora(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, '#02060f');
				sky.addColorStop(0.52, '#07101c');
				sky.addColorStop(1, '#0a1220');
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const groundRow = Math.floor(this.h * 0.82);
			const intensity = this._intensityLevelAurora();
			const bands = Math.max(1, Math.round(this.cfg.bands));
			const shiftPush = this.values.shift_push || 0;
			const shiftSeed = this.values.shift_seed || 0;

			const horizonGlow = ctx.createLinearGradient(0, canvasH * 0.5, 0, canvasH);
			horizonGlow.addColorStop(0, 'rgba(36, 84, 92, 0)');
			horizonGlow.addColorStop(1, `rgba(48, 168, 140, ${clamp01(0.18 + intensity * 0.16)})`);
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, canvasH * 0.48, canvasW, canvasH * 0.52);

			const baseGround = hslToRGB(212, 0.2, 0.04);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, groundRow - 1, this.w, this.h - groundRow + 1, `rgb(${baseGround.r},${baseGround.g},${baseGround.b})`, 1);

			const starCount = Math.max(12, Math.round(this.w * 0.18));
			for (let i = 0; i < starCount; i++) {
				const col = Math.floor(this._hash(19000 + i) * this.w);
				const row = Math.floor(this._hash(19100 + i) * Math.max(1, groundRow - 10));
				const twinkle = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * (0.018 + this._hash(19200 + i) * 0.02) + i), 2);
				const alpha = clamp01((0.14 + twinkle * 0.22) * (1 - Math.min(0.65, intensity * 0.55)));
				const color = hslToRGB(205 + this._hash(19300 + i) * 18, 0.18, 0.72 + this._hash(19400 + i) * 0.2);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
			}

			const ridgePoints = [];
			const ridgeSegments = 7;
			const ridgeColor = hslToRGB(210, 0.24, 0.055);
			for (let i = 0; i <= ridgeSegments; i++) {
				ridgePoints.push(groundRow - 4 - Math.floor(this._hash(19500 + i) * 6) - Math.floor((0.5 + 0.5 * Math.sin(i * 1.3 + this._hash(19600 + i) * 4)) * 4));
			}
			const ridgeCoords = [];
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const ridge = Math.round(ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased + Math.sin(x * 0.08 + shiftSeed) * 0.8);
				ridgeCoords.push({ x, ridge });
			}
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (const point of ridgeCoords) {
				ctx.lineTo(Math.floor(point.x * sx), Math.floor(point.ridge * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const treeCount = 12;
			for (let i = 0; i < treeCount; i++) {
				const center = Math.floor((i + 0.5) * this.w / treeCount + (this._hash(19900 + i) - 0.5) * 5);
				const trunkH = 1 + Math.floor(this._hash(20000 + i) * 2);
				const crownH = 5 + Math.floor(this._hash(20100 + i) * 5);
				const half = 1 + Math.floor(this._hash(20200 + i) * 2);
				const baseY = groundRow - 1 - Math.floor(this._hash(20300 + i) * 4);
				const treeColor = hslToRGB(210 + this._hash(20400 + i) * 10, 0.22, 0.045 + this._hash(20500 + i) * 0.02);
				for (let row = 0; row < crownH; row++) {
					const width = Math.max(1, half - Math.floor(row / 2));
					const y = baseY - crownH + row;
					for (let dx = -width; dx <= width; dx++) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center + dx, y, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
					}
				}
				for (let row = 0; row < trunkH; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, center, baseY - row, 1, 1, `rgb(${treeColor.r},${treeColor.g},${treeColor.b})`, 1);
				}
			}

			for (let band = 0; band < bands; band++) {
				const bandRatio = bands === 1 ? 0.5 : band / (bands - 1);
				const phase = this.tick * this.cfg.speed * (0.18 + bandRatio * 0.12) + band * 1.6 + shiftSeed;
				const amp = this.cfg.wave_amp * (0.8 + bandRatio * 0.35);
				const freq = this.cfg.wave_freq * (0.82 + bandRatio * 0.26);
				const thickness = this.cfg.thickness * (0.8 + bandRatio * 0.28);
				const curtain = this.cfg.curtain_len * (0.8 + bandRatio * 0.22);
				const baseY = this.h * (0.16 + bandRatio * 0.08) + Math.sin(this.tick * 0.01 + band * 0.8) * 1.1;
				const hueBase = ((this.cfg.hue + (bandRatio - 0.5) * this.cfg.hue_sp * 1.15 + shiftPush * 5) % 360 + 360) % 360;

				for (let x = 0; x < this.w; x++) {
					const nx = x / Math.max(1, this.w - 1);
					const arch = Math.sin(nx * Math.PI * (1.08 + bandRatio * 0.24) + band * 0.75);
					const wave = Math.sin(x * freq + phase + this.tick * this.cfg.drift * 0.04);
					const subWave = Math.sin(x * freq * 0.47 - phase * 0.62 + band * 2.1);
					const center = baseY + arch * amp * 0.72 + wave * amp * 0.52 + subWave * amp * 0.22 + shiftPush * Math.sin(x * 0.07 + phase) * 1.05;
					const startY = Math.max(0, Math.floor(center - thickness * 1.15));
					const endY = Math.min(groundRow - 2, Math.ceil(center + curtain));
					for (let y = startY; y <= endY; y++) {
						const dy = y - center;
						const core = Math.exp(-(dy * dy) / Math.max(1, thickness * thickness * 1.4));
						const tail = y >= center ? Math.exp(-(y - center) / Math.max(1, curtain)) : 0;
						const shimmer = 0.76 + 0.24 * Math.sin(this.tick * 0.03 + x * 0.1 + band * 1.7);
						const strength = (core * 0.9 + tail * 0.7) * intensity * shimmer * (0.58 + 0.42 * Math.max(0.2, arch));
						if (strength < 0.025) continue;
						const hue = ((hueBase + Math.sin(y * 0.18 + x * 0.06 + phase) * this.cfg.hue_sp * 0.32 + band * 6) % 360 + 360) % 360;
						const sat = clamp01(this.cfg.sat * (0.84 + core * 0.28));
						const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * Math.min(1, 0.24 + strength));
						const color = hslToRGB(hue, sat, light);
						const alpha = clamp01(strength * (0.34 + core * 0.46));
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
						if (core > 0.62 && y < groundRow - 3) {
							const accent = hslToRGB((hue + 12) % 360, clamp01(sat * 0.9), clamp01(light * 1.08));
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${accent.r},${accent.g},${accent.b})`, alpha * 0.45);
						}
					}
				}
			}
		}
	}

	class WheatField extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('wheat-field', w, h, cfg, seed);
		}
	}

	class Beach extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('beach', w, h, cfg, seed);
		}
	}

	class Campfire extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('campfire', w, h, cfg, seed);
		}
	}

	class Windmill extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('windmill', w, h, cfg, seed);
		}
	}

	class Aurora extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('aurora', w, h, cfg, seed);
		}
	}

	class Starfield extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('starfield', w, h, cfg, seed);
		}
	}

	class AutumnLeaves extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('autumn-leaves', w, h, cfg, seed);
		}
	}

	class Snow extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('snow', w, h, cfg, seed);
		}
	}

	// Subscribe to an SSE command stream, applying messages to one effect
	// instance. onReady is called once the initial snapshot has been applied.
	function subscribe(url, rain, onReady) {
		const es = new EventSource(url);
		es.addEventListener('message', (e) => {
			let cmd;
			try { cmd = JSON.parse(e.data); } catch (_) { return; }
			switch (cmd.kind) {
				case 'snapshot':
					try {
						const snap = typeof cmd.data === 'string' ? JSON.parse(cmd.data) : cmd.data;
						rain.restoreSnapshot(snap);
					} catch (err) { console.error('bad snapshot', err); }
					if (onReady) onReady();
					break;
				case 'config':
					try {
						const cfg = typeof cmd.data === 'string' ? JSON.parse(cmd.data) : cmd.data;
						rain.setConfig(cfg);
					} catch (err) { console.error('bad config', err); }
					break;
				case 'trigger':
					rain.triggerEvent(cmd.event);
					break;
			}
		});
		es.addEventListener('error', () => { /* auto-reconnect is built in */ });
		return es;
	}

	// Effect registry. Keyed by the effect type string broadcast in the
	// server's snapshot payload — the client looks up the constructor here
	// by name so new effects just register themselves and work without
	// client-side changes.
	const effects = { rain: Rain, dust: Dust, fireflies: Fireflies, waterfall: Waterfall, 'wheat-field': WheatField, beach: Beach, campfire: Campfire, windmill: Windmill, aurora: Aurora, snow: Snow, 'autumn-leaves': AutumnLeaves, starfield: Starfield };
	const presets = {
		'wheat-field': [
			{
				key: 'still-evening',
				label: 'still evening',
				config: {
					density: 0.4,
					speed: 0.07,
					drift: 0.05,
					sway: 0.34,
					wave_freq: 0.14,
					field_top: 0.64,
					stalk_h: 17,
					layers: 2,
					hue: 42,
					hue_sp: 12,
					sat: 0.56,
					lmin: 0.28,
					lmax: 0.7,
					calm_p: 0.001,
				},
			},
			{
				key: 'gentle-breeze',
				label: 'gentle breeze',
				config: {
					density: 0.48,
					speed: 0.12,
					drift: 0.14,
					sway: 0.68,
					wave_freq: 0.18,
					field_top: 0.62,
					stalk_h: 18,
					layers: 3,
					hue: 46,
					hue_sp: 18,
					sat: 0.64,
					lmin: 0.3,
					lmax: 0.76,
					gust_p: 0.0008,
				},
			},
			{
				key: 'rolling-field',
				label: 'rolling field',
				config: {
					density: 0.56,
					speed: 0.16,
					drift: 0.2,
					sway: 0.88,
					wave_freq: 0.16,
					field_top: 0.6,
					stalk_h: 20,
					layers: 3,
					hue: 48,
					hue_sp: 20,
					sat: 0.68,
					lmin: 0.3,
					lmax: 0.8,
					gust_p: 0.0012,
					gust_mult: 2.15,
				},
			},
			{
				key: 'windy-harvest',
				label: 'windy harvest',
				config: {
					density: 0.62,
					speed: 0.2,
					drift: 0.28,
					sway: 1.02,
					wave_freq: 0.21,
					field_top: 0.59,
					stalk_h: 22,
					layers: 4,
					hue: 44,
					hue_sp: 24,
					sat: 0.72,
					lmin: 0.32,
					lmax: 0.84,
					gust_p: 0.0016,
					gust_mult: 2.45,
					gust_dur: 66,
				},
			},
		],
		beach: [
			{
				key: 'still-shore',
				label: 'still shore',
				config: {
					shoreline: 0.56,
					tide_amp: 3.2,
					wave_amp: 1.3,
					wave_freq: 0.14,
					speed: 0.05,
					slope: 0.08,
					foam: 0.24,
					shimmer: 0.18,
					hue: 196,
					hue_sp: 10,
					sat: 0.42,
					lmin: 0.26,
					lmax: 0.78,
				},
			},
			{
				key: 'gentle-tide',
				label: 'gentle tide',
				config: {
					shoreline: 0.58,
					tide_amp: 6,
					wave_amp: 2.4,
					wave_freq: 0.18,
					speed: 0.1,
					slope: 0.16,
					foam: 0.36,
					shimmer: 0.22,
					hue: 198,
					hue_sp: 16,
					sat: 0.5,
					lmin: 0.28,
					lmax: 0.82,
					high_tide_p: 0.0008,
					low_tide_p: 0.0006,
				},
			},
			{
				key: 'foamy-edge',
				label: 'foamy edge',
				config: {
					shoreline: 0.6,
					tide_amp: 7.4,
					wave_amp: 3.1,
					wave_freq: 0.21,
					speed: 0.12,
					slope: 0.2,
					foam: 0.5,
					shimmer: 0.18,
					hue: 194,
					hue_sp: 18,
					sat: 0.54,
					lmin: 0.3,
					lmax: 0.84,
					high_tide_p: 0.0012,
					foam_burst_p: 0.0013,
					foam_burst_mult: 2.2,
				},
			},
			{
				key: 'wide-beach',
				label: 'wide beach',
				config: {
					shoreline: 0.52,
					tide_amp: 4.8,
					wave_amp: 1.8,
					wave_freq: 0.12,
					speed: 0.08,
					slope: -0.1,
					foam: 0.3,
					shimmer: 0.28,
					hue: 202,
					hue_sp: 14,
					sat: 0.44,
					lmin: 0.24,
					lmax: 0.78,
					low_tide_p: 0.0011,
				},
			},
		],
		campfire: [
			{
				key: 'small-fire',
				label: 'small fire',
				config: {
					flame_height: 9,
					flame_width: 7,
					flame_speed: 0.1,
					flicker: 0.56,
					ember_rate: 0.18,
					ember_speed: 0.52,
					glow: 0.4,
					hue: 22,
					hue_sp: 12,
					sat: 0.76,
					lmin: 0.28,
					lmax: 0.88,
				},
			},
			{
				key: 'steady-campfire',
				label: 'steady campfire',
				config: {
					flame_height: 14,
					flame_width: 10,
					flame_speed: 0.12,
					flicker: 0.72,
					ember_rate: 0.26,
					ember_speed: 0.62,
					glow: 0.54,
					hue: 24,
					hue_sp: 18,
					sat: 0.82,
					lmin: 0.32,
					lmax: 0.94,
					crackle_p: 0.0008,
				},
			},
			{
				key: 'crackling-fire',
				label: 'crackling fire',
				config: {
					flame_height: 16,
					flame_width: 11,
					flame_speed: 0.15,
					flicker: 0.92,
					ember_rate: 0.34,
					ember_speed: 0.78,
					glow: 0.62,
					hue: 21,
					hue_sp: 22,
					sat: 0.88,
					lmin: 0.34,
					lmax: 0.96,
					crackle_p: 0.0015,
					crackle_mult: 2.15,
					crackle_dur: 48,
				},
			},
			{
				key: 'late-embers',
				label: 'late embers',
				config: {
					intro_glow: 0.1,
					ending_glow: 0.14,
					flame_height: 8,
					flame_width: 8,
					flame_speed: 0.08,
					flicker: 0.42,
					ember_rate: 0.3,
					ember_speed: 0.48,
					glow: 0.34,
					hue: 18,
					hue_sp: 14,
					sat: 0.68,
					lmin: 0.24,
					lmax: 0.8,
					lull_p: 0.0014,
					lull_mult: 0.42,
				},
			},
		],
		windmill: [
			{
				key: 'still-dusk',
				label: 'still dusk',
				config: {
					intro_turn: 0.08,
					ending_turn: 0.04,
					turn_speed: 0.04,
					blade_len: 12,
					blade_width: 1.6,
					tower_height: 19,
					tower_width: 5.5,
					horizon: 0.74,
					glow: 0.22,
					hue: 26,
					hue_sp: 14,
					sat: 0.38,
					lmin: 0.16,
					lmax: 0.78,
				},
			},
			{
				key: 'steady-turning',
				label: 'steady turning',
				config: {
					turn_speed: 0.08,
					blade_len: 14,
					blade_width: 1.8,
					tower_height: 20,
					tower_width: 6,
					horizon: 0.72,
					glow: 0.18,
					hue: 28,
					hue_sp: 18,
					sat: 0.42,
					lmin: 0.18,
					lmax: 0.82,
					gust_p: 0.0006,
				},
			},
			{
				key: 'windy-hill',
				label: 'windy hill',
				config: {
					turn_speed: 0.12,
					blade_len: 15,
					blade_width: 2.1,
					tower_height: 21,
					tower_width: 6.5,
					horizon: 0.7,
					glow: 0.14,
					hue: 24,
					hue_sp: 20,
					sat: 0.4,
					lmin: 0.16,
					lmax: 0.8,
					gust_p: 0.0014,
					gust_mult: 2.2,
					gust_dur: 62,
				},
			},
			{
				key: 'silhouette-mill',
				label: 'silhouette mill',
				config: {
					turn_speed: 0.06,
					blade_len: 16,
					blade_width: 1.5,
					tower_height: 23,
					tower_width: 5,
					horizon: 0.76,
					glow: 0.1,
					hue: 222,
					hue_sp: 12,
					sat: 0.22,
					lmin: 0.12,
					lmax: 0.68,
					lull_p: 0.0012,
					lull_mult: 0.38,
				},
			},
		],
		aurora: [
			{
				key: 'green-veil',
				label: 'green veil',
				config: {
					intensity: 0.54,
					speed: 0.1,
					drift: 0.06,
					bands: 3,
					thickness: 9,
					wave_amp: 5.5,
					wave_freq: 0.15,
					curtain_len: 14,
					hue: 134,
					hue_sp: 18,
					sat: 0.7,
					lmin: 0.2,
					lmax: 0.72,
					shift_p: 0.0007,
				},
			},
			{
				key: 'cold-ribbons',
				label: 'cold ribbons',
				config: {
					intensity: 0.48,
					speed: 0.12,
					drift: 0.1,
					bands: 4,
					thickness: 7.5,
					wave_amp: 6.5,
					wave_freq: 0.18,
					curtain_len: 13,
					hue: 164,
					hue_sp: 34,
					sat: 0.66,
					lmin: 0.18,
					lmax: 0.76,
					shift_p: 0.0011,
					fade_p: 0.0005,
				},
			},
			{
				key: 'quiet-sky',
				label: 'quiet sky',
				config: {
					intensity: 0.34,
					speed: 0.07,
					drift: 0.03,
					bands: 2,
					thickness: 8.5,
					wave_amp: 4.5,
					wave_freq: 0.12,
					curtain_len: 11,
					hue: 142,
					hue_sp: 14,
					sat: 0.58,
					lmin: 0.16,
					lmax: 0.64,
					fade_p: 0.0008,
				},
			},
			{
				key: 'bright-aurora',
				label: 'bright aurora',
				config: {
					intensity: 0.72,
					speed: 0.14,
					drift: 0.12,
					bands: 4,
					thickness: 10,
					wave_amp: 7.2,
					wave_freq: 0.19,
					curtain_len: 18,
					hue: 136,
					hue_sp: 30,
					sat: 0.78,
					lmin: 0.22,
					lmax: 0.82,
					brighten_p: 0.0012,
					brighten_mult: 1.7,
					shift_p: 0.001,
				},
			},
		],
		starfield: [
			{
				key: 'still-night',
				label: 'still night',
				config: {
					density: 0.16,
					speed: 0.08,
					drift: 0.02,
					layers: 2,
					size: 1,
					hue: 214,
					hue_sp: 12,
					sat: 0.16,
					lmin: 0.5,
					lmax: 0.9,
				},
			},
			{
				key: 'soft-parallax',
				label: 'soft parallax',
				config: {
					density: 0.22,
					speed: 0.12,
					drift: 0.04,
					layers: 3,
					size: 1,
					hue: 218,
					hue_sp: 18,
					sat: 0.18,
					lmin: 0.55,
					lmax: 0.95,
					twinkle_burst_p: 0.0006,
				},
			},
			{
				key: 'meteor-watch',
				label: 'meteor watch',
				config: {
					density: 0.24,
					speed: 0.14,
					drift: 0.06,
					layers: 3,
					size: 1.2,
					hue: 214,
					hue_sp: 22,
					sat: 0.2,
					lmin: 0.56,
					lmax: 0.96,
					shooting_star_p: 0.0012,
					shooting_star_mult: 2.4,
				},
			},
			{
				key: 'cold-deep-space',
				label: 'cold deep space',
				config: {
					density: 0.2,
					speed: 0.09,
					drift: 0.03,
					layers: 4,
					size: 1,
					hue: 226,
					hue_sp: 26,
					sat: 0.22,
					lmin: 0.52,
					lmax: 0.94,
					twinkle_burst_p: 0.0009,
					twinkle_burst_mult: 1.9,
				},
			},
		],
		'autumn-leaves': [
			{
				key: 'few-leaves',
				label: 'few leaves',
				config: {
					density: 0.14,
					speed: 0.36,
					drift: 0.12,
					sway: 0.7,
					layers: 1,
					size: 1,
					hue: 24,
					hue_sp: 18,
					sat: 0.58,
					lmin: 0.36,
					lmax: 0.7,
					lull_p: 0.0014,
				},
			},
			{
				key: 'gentle-fall',
				label: 'gentle fall',
				config: {
					density: 0.24,
					speed: 0.44,
					drift: 0.18,
					sway: 0.86,
					layers: 2,
					size: 1.2,
					hue: 28,
					hue_sp: 24,
					sat: 0.62,
					lmin: 0.38,
					lmax: 0.78,
					gust_p: 0.0008,
				},
			},
			{
				key: 'windy-autumn',
				label: 'windy autumn',
				config: {
					density: 0.3,
					speed: 0.5,
					drift: 0.26,
					sway: 1.05,
					layers: 2,
					size: 1.4,
					hue: 22,
					hue_sp: 28,
					sat: 0.68,
					lmin: 0.36,
					lmax: 0.8,
					gust_p: 0.0016,
					gust_mult: 2.35,
				},
			},
			{
				key: 'swirl-study',
				label: 'swirl study',
				config: {
					density: 0.28,
					speed: 0.42,
					drift: 0.12,
					sway: 1.15,
					layers: 2,
					size: 1.4,
					hue: 30,
					hue_sp: 34,
					sat: 0.7,
					lmin: 0.4,
					lmax: 0.84,
					swirl_p: 0.0015,
					swirl_dur: 68,
					swirl_pull: 1.55,
				},
			},
		],
		snow: [
			{
				key: 'quiet-flurries',
				label: 'quiet flurries',
				config: {
					density: 0.2,
					speed: 0.38,
					drift: 0.04,
					sway: 0.35,
					layers: 2,
					size: 1,
					hue: 208,
					hue_sp: 8,
					sat: 0.12,
					lmin: 0.76,
					lmax: 0.96,
					calm_p: 0.0012,
				},
			},
			{
				key: 'pine-evening',
				label: 'pine evening',
				config: {
					density: 0.3,
					speed: 0.5,
					drift: 0.08,
					sway: 0.4,
					layers: 3,
					size: 1,
					hue: 214,
					hue_sp: 12,
					sat: 0.16,
					lmin: 0.74,
					lmax: 0.98,
					gust_p: 0.0008,
				},
			},
			{
				key: 'crosswind',
				label: 'crosswind',
				config: {
					density: 0.34,
					speed: 0.56,
					drift: 0.16,
					sway: 0.58,
					layers: 3,
					size: 1.2,
					hue: 206,
					hue_sp: 10,
					sat: 0.14,
					lmin: 0.72,
					lmax: 0.98,
					gust_p: 0.0015,
					gust_mult: 2.25,
					gust_dur: 68,
				},
			},
			{
				key: 'whiteout-edge',
				label: 'whiteout edge',
				config: {
					intro_density: 0.22,
					ending_density: 0.14,
					density: 0.52,
					speed: 0.7,
					drift: 0.12,
					sway: 0.74,
					layers: 4,
					size: 1.5,
					hue: 212,
					hue_sp: 16,
					sat: 0.18,
					lmin: 0.76,
					lmax: 1,
					gust_p: 0.0018,
					gust_mult: 2.8,
					gust_dur: 76,
					calm_p: 0.0003,
				},
			},
		],
		dust: [
			{
				key: 'lazy-dust',
				label: 'lazy dust',
				config: {
					drift: 0.28,
					wander: 0.22,
					spawn: 5,
					burst: 1,
					max: 42,
					trail: 2,
					fade: 0.68,
					hue: 28,
					hue_sp: 8,
					sat: 0.28,
					lmin: 0.28,
					lmax: 0.62,
					gust_p: 0.0006,
					calm_p: 0.0008,
					gust_front: 14,
					calm_mult: 0.35,
				},
			},
			{
				key: 'cross-breeze',
				label: 'cross breeze',
				config: {
					drift: 0.58,
					wander: 0.3,
					spawn: 4,
					burst: 2,
					max: 60,
					trail: 3,
					fade: 0.74,
					hue: 34,
					hue_sp: 10,
					sat: 0.36,
					lmin: 0.32,
					lmax: 0.72,
					gust_p: 0.0012,
					calm_p: 0.0004,
					gust_mult: 1.7,
					gust_front: 20,
				},
			},
			{
				key: 'dry-gusts',
				label: 'dry gusts',
				config: {
					intro_push: 2.1,
					drift: 0.78,
					wander: 0.42,
					spawn: 3,
					burst: 2,
					max: 72,
					trail: 4,
					fade: 0.76,
					hue: 26,
					hue_sp: 12,
					sat: 0.42,
					lmin: 0.34,
					lmax: 0.76,
					gust_p: 0.0018,
					gust_dur: 65,
					gust_mult: 2.25,
					gust_front: 24,
					calm_p: 0.0002,
				},
			},
			{
				key: 'dust-storm-edge',
				label: 'dust storm edge',
				config: {
					intro_haze: 0.22,
					ending_residue: 0.16,
					drift: 1.05,
					wander: 0.55,
					spawn: 2,
					burst: 3,
					max: 92,
					trail: 5,
					fade: 0.8,
					hue: 22,
					hue_sp: 16,
					sat: 0.48,
					lmin: 0.36,
					lmax: 0.8,
					gust_p: 0.002,
					gust_dur: 80,
					gust_mult: 2.8,
					gust_front: 30,
					calm_p: 0.0001,
				},
			},
		],
		waterfall: [
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
		],
	};

	global.AmbienceSim = { Rain, Dust, Fireflies, Waterfall, WheatField, Beach, Campfire, Windmill, Aurora, AutumnLeaves, Snow, Starfield, subscribe, applyDefaults, hslToRGB, effects, presets };
})(window);
