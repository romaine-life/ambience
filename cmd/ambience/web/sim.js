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
		}

		render(ctx, canvasW, canvasH, opts) {
			switch (this.kind) {
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
	const effects = { rain: Rain, dust: Dust, fireflies: Fireflies, waterfall: Waterfall, snow: Snow };
	const presets = {
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

	global.AmbienceSim = { Rain, Dust, Fireflies, Waterfall, Snow, subscribe, applyDefaults, hslToRGB, effects, presets };
})(window);
