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
		lighthouse: {
			intro_dur: 50,
			intro_beam: 0.16,
			ending_dur: 65,
			ending_linger: 18,
			ending_beam: 0.08,
			sweep_speed: 0.08,
			beam_width: 0.22,
			beam_softness: 0.42,
			tower_height: 22,
			tower_width: 6.5,
			horizon: 0.74,
			haze: 0.14,
			glow: 0.22,
			hue: 214,
			hue_sp: 18,
			sat: 0.34,
			lmin: 0.12,
			lmax: 0.84,
			bright_pass_p: 0,
			fog_thicken_p: 0,
			calm_p: 0,
			bright_pass_dur: 42,
			bright_pass_mult: 1.75,
			fog_thicken_dur: 72,
			fog_thicken_mult: 1.85,
			calm_dur: 64,
			calm_mult: 0.55,
		},
		rowboat: {
			intro_dur: 50,
			intro_drift: 0.18,
			ending_dur: 65,
			ending_linger: 18,
			ending_ripple: 0.08,
			waterline: 0.58,
			drift_speed: 0.08,
			bob_amp: 1.2,
			wave_amp: 1.6,
			wave_freq: 0.16,
			ripple: 0.24,
			reflection: 0.22,
			boat_len: 14,
			boat_height: 3.5,
			hue: 206,
			hue_sp: 16,
			sat: 0.36,
			lmin: 0.16,
			lmax: 0.82,
			wake_p: 0,
			drift_p: 0,
			calm_p: 0,
			wake_dur: 40,
			wake_mult: 1.85,
			drift_dur: 58,
			drift_push: 1.3,
			calm_dur: 72,
			calm_mult: 0.5,
		},
		underwater: {
			intro_dur: 55,
			intro_reveal: 0.14,
			ending_dur: 70,
			ending_linger: 22,
			ending_murk: 0.08,
			density: 0.28,
			rise_speed: 0.42,
			drift: 0.1,
			sway: 0.54,
			weed_height: 20,
			weed_count: 11,
			caustics: 0.3,
			depth: 0.56,
			hue: 192,
			hue_sp: 18,
			sat: 0.42,
			lmin: 0.12,
			lmax: 0.82,
			bubble_burst_p: 0,
			current_shift_p: 0,
			calm_p: 0,
			bubble_burst_dur: 38,
			bubble_burst_mult: 1.9,
			current_shift_dur: 62,
			current_shift_push: 1.2,
			calm_dur: 74,
			calm_mult: 0.55,
		},
		volcano: {
			intro_dur: 55,
			intro_glow: 0.12,
			ending_dur: 70,
			ending_linger: 22,
			ending_embers: 0.08,
			horizon: 0.8,
			cone_height: 26,
			cone_width: 44,
			crater_width: 8,
			glow: 0.28,
			smoke: 0.22,
			ash: 0.18,
			hue: 18,
			hue_sp: 18,
			sat: 0.78,
			lmin: 0.08,
			lmax: 0.88,
			eruption_p: 0,
			smolder_p: 0,
			flare_p: 0,
			eruption_dur: 42,
			eruption_mult: 2.2,
			smolder_dur: 72,
			smolder_mult: 1.6,
			flare_dur: 28,
			flare_mult: 1.9,
		},
		train: {
			intro_dur: 50,
			intro_cue: 0.12,
			ending_dur: 70,
			ending_linger: 18,
			ending_clear: 0.08,
			horizon: 0.74,
			track_row: 0.82,
			train_height: 4.5,
			car_count: 6,
			car_gap: 1,
			speed: 1.05,
			smoke: 0.18,
			headlight: 0.24,
			hue: 220,
			hue_sp: 20,
			sat: 0.34,
			lmin: 0.08,
			lmax: 0.86,
			pass_p: 0,
			express_p: 0,
			quiet_gap_p: 0,
			pass_dur: 72,
			pass_mult: 1.15,
			express_dur: 46,
			express_mult: 1.9,
			quiet_gap_dur: 90,
			quiet_gap_mult: 0.45,
		},
		'mysterious-man': {
			intro_dur: 55,
			intro_reveal: 0.08,
			ending_dur: 70,
			ending_linger: 20,
			ending_shadow: 0.08,
			figure_x: 0.38,
			figure_scale: 1,
			lean: 0.08,
			contrast: 0.24,
			ember: 0.24,
			hold_angle: 0.22,
			smoke: 0.16,
			rise_speed: 0.72,
			drift: 0.08,
			hue: 216,
			hue_sp: 18,
			sat: 0.18,
			lmin: 0.04,
			lmax: 0.88,
			inhale_p: 0,
			exhale_p: 0,
			ash_fall_p: 0,
			lighter_flick_p: 0,
			inhale_dur: 34,
			inhale_mult: 1.8,
			exhale_dur: 46,
			exhale_mult: 1.65,
			ash_fall_dur: 24,
			ash_fall_mult: 1.4,
			lighter_flick_dur: 20,
			lighter_flick_mult: 2.25,
		},
		'burning-trees': {
			intro_dur: 55,
			intro_growth: 0.14,
			ending_dur: 72,
			ending_linger: 22,
			ending_ash: 0.1,
			horizon: 0.84,
			tree_count: 10,
			tree_height: 13,
			canopy: 0.78,
			spread: 1.35,
			flame: 0.28,
			embers: 0.18,
			smoke: 0.16,
			char: 0.42,
			hue: 112,
			hue_sp: 20,
			sat: 0.48,
			lmin: 0.06,
			lmax: 0.88,
			ignite_p: 0,
			flare_p: 0,
			lull_p: 0,
			ignite_dur: 76,
			ignite_span: 1.6,
			flare_dur: 38,
			flare_mult: 1.85,
			lull_dur: 54,
			lull_mult: 0.55,
		},
		sand: {
			intro_dur: 55,
			intro_trickle: 0.1,
			intro_fill: 0.02,
			ending_dur: 70,
			ending_linger: 20,
			ending_settle: 0.08,
			pipe_x: 0.28,
			pipe_width: 7,
			pipe_drop: 18,
			container_x: 0.58,
			container_width: 34,
			container_depth: 10,
			flow: 0.24,
			spread: 0.9,
			settle: 0.32,
			overflow: 0.28,
			hue: 38,
			hue_sp: 10,
			sat: 0.54,
			lmin: 0.16,
			lmax: 0.86,
			surge_p: 0,
			calm_p: 0,
			surge_dur: 42,
			surge_mult: 2.0,
			calm_dur: 60,
			calm_mult: 0.42,
		},
		'water-pipe': {
			intro_dur: 55,
			intro_flow: 0.10,
			intro_pool: 0.08,
			ending_dur: 70,
			ending_linger: 22,
			ending_ripple: 0.08,
			pipe_x: 0.22,
			pipe_width: 7,
			pipe_drop: 17,
			basin_x: 0.40,
			basin_width: 30,
			basin_depth: 9,
			flow: 0.24,
			stream: 1.05,
			ripple: 0.38,
			overflow: 0.30,
			foam: 0.22,
			hue: 196,
			hue_sp: 18,
			sat: 0.56,
			lmin: 0.12,
			lmax: 0.92,
			surge_p: 0,
			dry_up_p: 0,
			surge_dur: 36,
			surge_mult: 1.90,
			dry_up_dur: 50,
			dry_up_mult: 0.35,
		},
		tetris: {
			intro_dur: 60,
			intro_stack: 0,
			intro_cadence: 0.55,
			ending_dur: 70,
			ending_linger: 28,
			ending_fill: 0.84,
			well_x: 0.18,
			well_y: 0.13,
			cell_size: 3,
			spawn_every: 42,
			fall_every: 8,
			lock_delay: 4,
			glow: 0.18,
			ghost: 0.14,
			hue: 208,
			hue_sp: 24,
			sat: 0.56,
			lmin: 0.10,
			lmax: 0.92,
			new_piece_p: 0,
			lull_p: 0,
			new_piece_dur: 14,
			new_piece_cut: 0.25,
			lull_dur: 80,
			lull_mult: 1.8,
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
			case 'lighthouse':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_beam = clamp01(c.intro_beam);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_beam = clamp01(c.ending_beam);
				if (c.sweep_speed <= 0) c.sweep_speed = base.sweep_speed;
				if (c.beam_width <= 0) c.beam_width = base.beam_width;
				if (c.beam_softness <= 0) c.beam_softness = base.beam_softness;
				if (c.tower_height <= 0) c.tower_height = base.tower_height;
				if (c.tower_width <= 0) c.tower_width = base.tower_width;
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.haze <= 0) c.haze = base.haze;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.bright_pass_dur <= 0) c.bright_pass_dur = base.bright_pass_dur;
				if (c.bright_pass_mult <= 0) c.bright_pass_mult = base.bright_pass_mult;
				if (c.fog_thicken_dur <= 0) c.fog_thicken_dur = base.fog_thicken_dur;
				if (c.fog_thicken_mult <= 0) c.fog_thicken_mult = base.fog_thicken_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'rowboat':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_drift = clamp01(c.intro_drift);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_ripple = clamp01(c.ending_ripple);
				if (c.waterline <= 0) c.waterline = base.waterline;
				if (c.drift_speed <= 0) c.drift_speed = base.drift_speed;
				if (c.bob_amp <= 0) c.bob_amp = base.bob_amp;
				if (c.wave_amp <= 0) c.wave_amp = base.wave_amp;
				if (c.wave_freq <= 0) c.wave_freq = base.wave_freq;
				if (c.ripple <= 0) c.ripple = base.ripple;
				if (c.reflection <= 0) c.reflection = base.reflection;
				if (c.boat_len <= 0) c.boat_len = base.boat_len;
				if (c.boat_height <= 0) c.boat_height = base.boat_height;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.wake_dur <= 0) c.wake_dur = base.wake_dur;
				if (c.wake_mult <= 0) c.wake_mult = base.wake_mult;
				if (c.drift_dur <= 0) c.drift_dur = base.drift_dur;
				if (c.drift_push <= 0) c.drift_push = base.drift_push;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'underwater':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_reveal = clamp01(c.intro_reveal);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_murk = clamp01(c.ending_murk);
				if (c.density <= 0) c.density = base.density;
				if (c.rise_speed <= 0) c.rise_speed = base.rise_speed;
				if (c.sway <= 0) c.sway = base.sway;
				if (c.weed_height <= 0) c.weed_height = base.weed_height;
				if (c.weed_count < 1) c.weed_count = base.weed_count;
				if (c.caustics <= 0) c.caustics = base.caustics;
				if (c.depth <= 0) c.depth = base.depth;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.bubble_burst_dur <= 0) c.bubble_burst_dur = base.bubble_burst_dur;
				if (c.bubble_burst_mult <= 0) c.bubble_burst_mult = base.bubble_burst_mult;
				if (c.current_shift_dur <= 0) c.current_shift_dur = base.current_shift_dur;
				if (c.current_shift_push <= 0) c.current_shift_push = base.current_shift_push;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'volcano':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_glow = clamp01(c.intro_glow);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_embers = clamp01(c.ending_embers);
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.cone_height <= 0) c.cone_height = base.cone_height;
				if (c.cone_width <= 0) c.cone_width = base.cone_width;
				if (c.crater_width <= 0) c.crater_width = base.crater_width;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.smoke <= 0) c.smoke = base.smoke;
				if (c.ash <= 0) c.ash = base.ash;
				if (c.hue === 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.eruption_dur <= 0) c.eruption_dur = base.eruption_dur;
				if (c.eruption_mult <= 0) c.eruption_mult = base.eruption_mult;
				if (c.smolder_dur <= 0) c.smolder_dur = base.smolder_dur;
				if (c.smolder_mult <= 0) c.smolder_mult = base.smolder_mult;
				if (c.flare_dur <= 0) c.flare_dur = base.flare_dur;
				if (c.flare_mult <= 0) c.flare_mult = base.flare_mult;
				break;
			case 'train':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_cue = clamp01(c.intro_cue);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_clear = clamp01(c.ending_clear);
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.track_row <= 0) c.track_row = base.track_row;
				if (c.train_height <= 0) c.train_height = base.train_height;
				if (c.car_count < 3) c.car_count = base.car_count;
				if (c.car_gap <= 0) c.car_gap = base.car_gap;
				if (c.speed <= 0) c.speed = base.speed;
				if (c.smoke <= 0) c.smoke = base.smoke;
				if (c.headlight <= 0) c.headlight = base.headlight;
				if (c.hue <= 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.pass_dur <= 0) c.pass_dur = base.pass_dur;
				if (c.pass_mult <= 0) c.pass_mult = base.pass_mult;
				if (c.express_dur <= 0) c.express_dur = base.express_dur;
				if (c.express_mult <= 0) c.express_mult = base.express_mult;
				if (c.quiet_gap_dur <= 0) c.quiet_gap_dur = base.quiet_gap_dur;
				if (c.quiet_gap_mult <= 0) c.quiet_gap_mult = base.quiet_gap_mult;
				break;
			case 'mysterious-man':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_reveal = clamp01(c.intro_reveal);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_shadow = clamp01(c.ending_shadow);
				if (c.figure_x <= 0) c.figure_x = base.figure_x;
				if (c.figure_scale <= 0) c.figure_scale = base.figure_scale;
				if (c.contrast <= 0) c.contrast = base.contrast;
				if (c.ember <= 0) c.ember = base.ember;
				if (c.smoke <= 0) c.smoke = base.smoke;
				if (c.rise_speed <= 0) c.rise_speed = base.rise_speed;
				if (c.hue <= 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.inhale_dur <= 0) c.inhale_dur = base.inhale_dur;
				if (c.inhale_mult <= 0) c.inhale_mult = base.inhale_mult;
				if (c.exhale_dur <= 0) c.exhale_dur = base.exhale_dur;
				if (c.exhale_mult <= 0) c.exhale_mult = base.exhale_mult;
				if (c.ash_fall_dur <= 0) c.ash_fall_dur = base.ash_fall_dur;
				if (c.ash_fall_mult <= 0) c.ash_fall_mult = base.ash_fall_mult;
				if (c.lighter_flick_dur <= 0) c.lighter_flick_dur = base.lighter_flick_dur;
				if (c.lighter_flick_mult <= 0) c.lighter_flick_mult = base.lighter_flick_mult;
				break;
			case 'burning-trees':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_growth = clamp01(c.intro_growth);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_ash = clamp01(c.ending_ash);
				if (c.horizon <= 0) c.horizon = base.horizon;
				if (c.tree_count < 1) c.tree_count = base.tree_count;
				if (c.tree_height <= 0) c.tree_height = base.tree_height;
				c.canopy = clamp01(c.canopy);
				if (c.spread <= 0) c.spread = base.spread;
				if (c.flame <= 0) c.flame = base.flame;
				if (c.embers <= 0) c.embers = base.embers;
				if (c.smoke <= 0) c.smoke = base.smoke;
				c.char = clamp01(c.char);
				if (c.hue <= 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.ignite_dur <= 0) c.ignite_dur = base.ignite_dur;
				if (c.ignite_span <= 0) c.ignite_span = base.ignite_span;
				if (c.flare_dur <= 0) c.flare_dur = base.flare_dur;
				if (c.flare_mult <= 0) c.flare_mult = base.flare_mult;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 0) c.lull_mult = base.lull_mult;
				break;
			case 'sand':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_trickle = clamp01(c.intro_trickle);
				c.intro_fill = clamp01(c.intro_fill);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_settle = clamp01(c.ending_settle);
				if (c.pipe_x <= 0) c.pipe_x = base.pipe_x;
				if (c.pipe_width <= 0) c.pipe_width = base.pipe_width;
				if (c.pipe_drop <= 0) c.pipe_drop = base.pipe_drop;
				if (c.container_x <= 0) c.container_x = base.container_x;
				if (c.container_width <= 0) c.container_width = base.container_width;
				if (c.container_depth <= 0) c.container_depth = base.container_depth;
				if (c.flow <= 0) c.flow = base.flow;
				if (c.spread <= 0) c.spread = base.spread;
				if (c.settle <= 0) c.settle = base.settle;
				if (c.overflow <= 0) c.overflow = base.overflow;
				if (c.hue <= 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.surge_dur <= 0) c.surge_dur = base.surge_dur;
				if (c.surge_mult <= 0) c.surge_mult = base.surge_mult;
				if (c.calm_dur <= 0) c.calm_dur = base.calm_dur;
				if (c.calm_mult <= 0) c.calm_mult = base.calm_mult;
				break;
			case 'water-pipe':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				c.intro_flow = clamp01(c.intro_flow);
				c.intro_pool = clamp01(c.intro_pool);
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_ripple = clamp01(c.ending_ripple);
				if (c.pipe_x <= 0) c.pipe_x = base.pipe_x;
				if (c.pipe_width <= 0) c.pipe_width = base.pipe_width;
				if (c.pipe_drop <= 0) c.pipe_drop = base.pipe_drop;
				if (c.basin_x <= 0) c.basin_x = base.basin_x;
				if (c.basin_width <= 0) c.basin_width = base.basin_width;
				if (c.basin_depth <= 0) c.basin_depth = base.basin_depth;
				if (c.flow <= 0) c.flow = base.flow;
				if (c.stream <= 0) c.stream = base.stream;
				if (c.ripple <= 0) c.ripple = base.ripple;
				if (c.overflow <= 0) c.overflow = base.overflow;
				if (c.foam <= 0) c.foam = base.foam;
				if (c.hue <= 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.surge_dur <= 0) c.surge_dur = base.surge_dur;
				if (c.surge_mult <= 0) c.surge_mult = base.surge_mult;
				if (c.dry_up_dur <= 0) c.dry_up_dur = base.dry_up_dur;
				if (c.dry_up_mult <= 0) c.dry_up_mult = base.dry_up_mult;
				break;
			case 'tetris':
				if (c.intro_dur <= 0) c.intro_dur = base.intro_dur;
				if (c.intro_stack < 0) c.intro_stack = 0;
				if (c.intro_stack > 8) c.intro_stack = 8;
				c.intro_cadence = clamp01(c.intro_cadence);
				if (c.intro_cadence < 0.2) c.intro_cadence = base.intro_cadence;
				if (c.ending_dur <= 0) c.ending_dur = base.ending_dur;
				if (c.ending_linger < 0) c.ending_linger = 0;
				c.ending_fill = clamp01(c.ending_fill);
				if (c.ending_fill < 0.6) c.ending_fill = base.ending_fill;
				if (c.well_x <= 0) c.well_x = base.well_x;
				if (c.well_y <= 0) c.well_y = base.well_y;
				if (c.cell_size <= 0) c.cell_size = base.cell_size;
				if (c.spawn_every <= 0) c.spawn_every = base.spawn_every;
				if (c.fall_every <= 0) c.fall_every = base.fall_every;
				if (c.lock_delay < 1) c.lock_delay = base.lock_delay;
				if (c.glow <= 0) c.glow = base.glow;
				if (c.ghost < 0) c.ghost = 0;
				if (c.hue <= 0) c.hue = base.hue;
				if (c.hue_sp < 0) c.hue_sp = 0;
				if (c.sat <= 0) c.sat = base.sat;
				if (c.lmin <= 0) c.lmin = base.lmin;
				if (c.lmax <= 0) c.lmax = base.lmax;
				if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
				if (c.new_piece_dur <= 0) c.new_piece_dur = base.new_piece_dur;
				if (c.new_piece_cut <= 0) c.new_piece_cut = base.new_piece_cut;
				if (c.lull_dur <= 0) c.lull_dur = base.lull_dur;
				if (c.lull_mult <= 1) c.lull_mult = base.lull_mult;
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

	const TETRIS_COLS = 10;
	const TETRIS_ROWS = 20;
	const TETRIS_PIECES = [
		[
			[{ x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 3, y: 1 }],
			[{ x: 2, y: 0 }, { x: 2, y: 1 }, { x: 2, y: 2 }, { x: 2, y: 3 }],
			[{ x: 0, y: 2 }, { x: 1, y: 2 }, { x: 2, y: 2 }, { x: 3, y: 2 }],
			[{ x: 1, y: 0 }, { x: 1, y: 1 }, { x: 1, y: 2 }, { x: 1, y: 3 }],
		],
		[
			[{ x: 0, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }],
			[{ x: 1, y: 0 }, { x: 2, y: 0 }, { x: 1, y: 1 }, { x: 1, y: 2 }],
			[{ x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 2, y: 2 }],
			[{ x: 1, y: 0 }, { x: 1, y: 1 }, { x: 0, y: 2 }, { x: 1, y: 2 }],
		],
		[
			[{ x: 2, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }],
			[{ x: 1, y: 0 }, { x: 1, y: 1 }, { x: 1, y: 2 }, { x: 2, y: 2 }],
			[{ x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 0, y: 2 }],
			[{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 1, y: 1 }, { x: 1, y: 2 }],
		],
		[
			[{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }],
			[{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }],
			[{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }],
			[{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }],
		],
		[
			[{ x: 1, y: 0 }, { x: 2, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }],
			[{ x: 1, y: 0 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 2, y: 2 }],
			[{ x: 1, y: 1 }, { x: 2, y: 1 }, { x: 0, y: 2 }, { x: 1, y: 2 }],
			[{ x: 0, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }, { x: 1, y: 2 }],
		],
		[
			[{ x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }],
			[{ x: 1, y: 0 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 1, y: 2 }],
			[{ x: 0, y: 1 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 1, y: 2 }],
			[{ x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }, { x: 1, y: 2 }],
		],
		[
			[{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 1, y: 1 }, { x: 2, y: 1 }],
			[{ x: 2, y: 0 }, { x: 1, y: 1 }, { x: 2, y: 1 }, { x: 1, y: 2 }],
			[{ x: 0, y: 1 }, { x: 1, y: 1 }, { x: 1, y: 2 }, { x: 2, y: 2 }],
			[{ x: 1, y: 0 }, { x: 0, y: 1 }, { x: 1, y: 1 }, { x: 0, y: 2 }],
		],
	];

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
				case 'lighthouse':
					return this._triggerLighthouse(name);
				case 'rowboat':
					return this._triggerRowboat(name);
				case 'underwater':
					return this._triggerUnderwater(name);
				case 'volcano':
					return this._triggerVolcano(name);
				case 'train':
					return this._triggerTrain(name);
				case 'mysterious-man':
					return this._triggerMysteriousMan(name);
				case 'burning-trees':
					return this._triggerBurningTrees(name);
				case 'sand':
					return this._triggerSand(name);
				case 'water-pipe':
					return this._triggerWaterPipe(name);
				case 'tetris':
					return this._triggerTetris(name);
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
			if (this.kind === 'lighthouse') {
				if (!this.timers['bright-pass'] || this.timers['bright-pass'] <= 0) {
					this.values.bright_gain = 1;
				}
				if (!this.timers['fog-thicken'] || this.timers['fog-thicken'] <= 0) {
					this.values.fog_gain = 1;
				}
			}
			if (this.kind === 'rowboat') {
				if (!this.timers.wake || this.timers.wake <= 0) {
					this.values.wake_gain = 1;
				}
				if (!this.timers.drift || this.timers.drift <= 0) {
					this.values.drift_push = 0;
				}
			}
			if (this.kind === 'underwater') {
				if (!this.timers['bubble-burst'] || this.timers['bubble-burst'] <= 0) {
					this.values.bubble_gain = 1;
				}
				if (!this.timers['current-shift'] || this.timers['current-shift'] <= 0) {
					this.values.current_push = 0;
				}
			}
			if (this.kind === 'volcano') {
				if (!this.timers.eruption || this.timers.eruption <= 0) {
					this.values.eruption_gain = 1;
					this.values.eruption_total = 0;
					this.values.eruption_seed = 0;
					this.values.eruption_dir = 0;
				}
				if (!this.timers.smolder || this.timers.smolder <= 0) {
					this.values.smolder_gain = 1;
				}
				if (!this.timers.flare || this.timers.flare <= 0) {
					this.values.flare_gain = 1;
				}
			}
			if (this.kind === 'train') {
				if ((!this.timers.pass || this.timers.pass <= 0) && (!this.timers.express || this.timers.express <= 0)) {
					this.values.pass_gain = 1;
					this.values.express_gain = 1;
					this.values.train_total = 0;
					this.values.train_speed = 0;
					this.values.train_dir = 0;
					this.values.train_cars = 0;
					this.values.train_gap = 0;
					this.values.train_seed = 0;
					this.values.smoke_gain = 0;
					this.values.headlight_gain = 0;
				}
			}
			if (this.kind === 'mysterious-man') {
				if (!this.timers.inhale || this.timers.inhale <= 0) this.values.inhale_gain = 1;
				if (!this.timers.exhale || this.timers.exhale <= 0) {
					this.values.exhale_gain = 1;
					this.values.exhale_total = 0;
					this.values.exhale_seed = 0;
					this.values.exhale_dir = 0;
				}
				if (!this.timers['ash-fall'] || this.timers['ash-fall'] <= 0) {
					this.values.ash_gain = 1;
					this.values.ash_seed = 0;
					this.values.ash_total = 0;
				}
				if (!this.timers['lighter-flick'] || this.timers['lighter-flick'] <= 0) {
					this.values.lighter_gain = 1;
				}
			}
			if (this.kind === 'burning-trees') {
				this._stepBurningTreesChar();
				if (!this.timers.ignite || this.timers.ignite <= 0) {
					this.values.ignite_gain = 0;
					this.values.ignite_total = 0;
					this.values.ignite_center = 0;
					this.values.ignite_span = 0;
					this.values.ignite_seed = 0;
				}
				if (!this.timers.flare || this.timers.flare <= 0) {
					this.values.flare_gain = 1;
				}
				if (!this.timers.lull || this.timers.lull <= 0) {
					this.values.lull_gain = 1;
				}
				if (!this.timers.intro || this.timers.intro <= 0) {
					delete this.values.intro_total;
				}
				if (!this.timers.ending || this.timers.ending <= 0) {
					delete this.values.ending_total;
				}
				if (this.timers.ending > 0) {
					const treeCount = Math.max(1, Math.round(this.cfg.tree_count));
					for (let i = 0; i < treeCount; i++) {
						const key = `char_${i}`;
						if ((this.values[key] || 0) <= 0) continue;
						this.values[key] = clamp01(this.values[key] + this.cfg.char * 0.004);
					}
				}
			}
			if (this.kind === 'sand') {
				this._stepSandState();
				if (!this.timers.surge || this.timers.surge <= 0) this.values.surge_gain = 1;
				if (!this.timers.calm || this.timers.calm <= 0) this.values.calm_gain = 1;
				if (!this.timers.intro || this.timers.intro <= 0) delete this.values.intro_total;
				if (!this.timers.ending || this.timers.ending <= 0) delete this.values.ending_total;
			}
			if (this.kind === 'water-pipe') {
				this._stepWaterPipeState();
				if (!this.timers.surge || this.timers.surge <= 0) this.values.surge_gain = 1;
				if (!this.timers['dry-up'] || this.timers['dry-up'] <= 0) this.values.dry_up_gain = 1;
				if (!this.timers.intro || this.timers.intro <= 0) delete this.values.intro_total;
				if (!this.timers.ending || this.timers.ending <= 0) delete this.values.ending_total;
			}
			if (this.kind === 'tetris') {
				this._stepTetrisState();
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
				case 'lighthouse':
					return this._renderLighthouse(ctx, canvasW, canvasH, opts);
				case 'rowboat':
					return this._renderRowboat(ctx, canvasW, canvasH, opts);
				case 'underwater':
					return this._renderUnderwater(ctx, canvasW, canvasH, opts);
				case 'volcano':
					return this._renderVolcano(ctx, canvasW, canvasH, opts);
				case 'train':
					return this._renderTrain(ctx, canvasW, canvasH, opts);
				case 'mysterious-man':
					return this._renderMysteriousMan(ctx, canvasW, canvasH, opts);
				case 'burning-trees':
					return this._renderBurningTrees(ctx, canvasW, canvasH, opts);
				case 'sand':
					return this._renderSand(ctx, canvasW, canvasH, opts);
				case 'water-pipe':
					return this._renderWaterPipe(ctx, canvasW, canvasH, opts);
				case 'tetris':
					return this._renderTetris(ctx, canvasW, canvasH, opts);
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

		_triggerLighthouse(name) {
			const rng = this._eventRng(name.length + 79);
			switch (name) {
				case 'bright-pass':
					this.timers['bright-pass'] = jitterInt(rng, this.cfg.bright_pass_dur, 0.3);
					this.values.bright_gain = this.cfg.bright_pass_mult * (0.8 + rng() * 0.4);
					return true;
				case 'fog-thicken':
					this.timers['fog-thicken'] = jitterInt(rng, this.cfg.fog_thicken_dur, 0.3);
					this.timers.calm = 0;
					this.values.fog_gain = this.cfg.fog_thicken_mult * (0.8 + rng() * 0.45);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.timers['fog-thicken'] = 0;
					this.values.fog_gain = 1;
					return true;
				case 'intro':
					this.timers['bright-pass'] = 0;
					this.timers['fog-thicken'] = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.bright_gain = 1;
					this.values.fog_gain = 1;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['bright-pass'] = 0;
					this.timers['fog-thicken'] = 0;
					this.timers.calm = 0;
					this.values.bright_gain = 1;
					this.values.fog_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerRowboat(name) {
			const rng = this._eventRng(name.length + 83);
			switch (name) {
				case 'wake':
					this.timers.wake = jitterInt(rng, this.cfg.wake_dur, 0.3);
					this.values.wake_gain = this.cfg.wake_mult * (0.8 + rng() * 0.45);
					return true;
				case 'drift':
					this.timers.drift = jitterInt(rng, this.cfg.drift_dur, 0.3);
					this.values.drift_push = (rng() < 0.5 ? -1 : 1) * this.cfg.drift_push * (0.65 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					return true;
				case 'intro':
					this.timers.wake = 0;
					this.timers.drift = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.wake_gain = 1;
					this.values.drift_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.wake = 0;
					this.timers.drift = 0;
					this.timers.calm = 0;
					this.values.wake_gain = 1;
					this.values.drift_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerUnderwater(name) {
			const rng = this._eventRng(name.length + 89);
			switch (name) {
				case 'bubble-burst':
					this.timers['bubble-burst'] = jitterInt(rng, this.cfg.bubble_burst_dur, 0.3);
					this.values.bubble_gain = this.cfg.bubble_burst_mult * (0.8 + rng() * 0.45);
					return true;
				case 'current-shift':
					this.timers['current-shift'] = jitterInt(rng, this.cfg.current_shift_dur, 0.3);
					this.timers.calm = 0;
					this.values.current_push = (rng() < 0.5 ? -1 : 1) * this.cfg.current_shift_push * (0.55 + rng() * 0.55);
					return true;
				case 'calm':
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.timers['current-shift'] = 0;
					this.values.current_push = 0;
					return true;
				case 'intro':
					this.timers['bubble-burst'] = 0;
					this.timers['current-shift'] = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.bubble_gain = 1;
					this.values.current_push = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers['bubble-burst'] = 0;
					this.timers['current-shift'] = 0;
					this.timers.calm = 0;
					this.values.bubble_gain = 1;
					this.values.current_push = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerVolcano(name) {
			const rng = this._eventRng(name.length + 97);
			switch (name) {
				case 'eruption':
					this.timers.eruption = jitterInt(rng, this.cfg.eruption_dur, 0.3);
					this.values.eruption_gain = this.cfg.eruption_mult * (0.8 + rng() * 0.45);
					this.values.eruption_total = this.timers.eruption;
					this.values.eruption_seed = rng() * 1000;
					this.values.eruption_dir = rng() < 0.5 ? -1 : 1;
					return true;
				case 'smolder':
					this.timers.smolder = jitterInt(rng, this.cfg.smolder_dur, 0.3);
					this.values.smolder_gain = this.cfg.smolder_mult * (0.8 + rng() * 0.4);
					return true;
				case 'flare':
					this.timers.flare = jitterInt(rng, this.cfg.flare_dur, 0.3);
					this.values.flare_gain = this.cfg.flare_mult * (0.85 + rng() * 0.35);
					return true;
				case 'intro':
					this.timers.eruption = 0;
					this.timers.smolder = 0;
					this.timers.flare = 0;
					this.timers.ending = 0;
					this.values.eruption_gain = 1;
					this.values.smolder_gain = 1;
					this.values.flare_gain = 1;
					this.values.eruption_total = 0;
					this.values.eruption_seed = 0;
					this.values.eruption_dir = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.eruption = 0;
					this.timers.smolder = 0;
					this.timers.flare = 0;
					this.values.eruption_gain = 1;
					this.values.smolder_gain = 1;
					this.values.flare_gain = 1;
					this.values.eruption_total = 0;
					this.values.eruption_seed = 0;
					this.values.eruption_dir = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerTrain(name) {
			const rng = this._eventRng(name.length + 109);
			switch (name) {
				case 'pass':
					this.timers.express = 0;
					this.timers['quiet-gap'] = 0;
					this.timers.pass = jitterInt(rng, this.cfg.pass_dur, 0.3);
					this.values.pass_gain = this.cfg.pass_mult * (0.9 + rng() * 0.3);
					this.values.express_gain = 1;
					this.values.train_total = this.timers.pass;
					this.values.train_speed = this.cfg.speed * this.values.pass_gain;
					this.values.train_dir = rng() < 0.5 ? -1 : 1;
					this.values.train_cars = Math.max(3, Math.round(this.cfg.car_count) - 1 + Math.floor(rng() * 3));
					this.values.train_gap = Math.max(0.5, this.cfg.car_gap * (0.9 + rng() * 0.25));
					this.values.train_seed = rng() * 1000;
					this.values.smoke_gain = this.cfg.smoke * (0.8 + rng() * 0.5);
					this.values.headlight_gain = this.cfg.headlight * (0.9 + rng() * 0.35);
					return true;
				case 'express':
					this.timers.pass = 0;
					this.timers['quiet-gap'] = 0;
					this.timers.express = jitterInt(rng, this.cfg.express_dur, 0.3);
					this.values.pass_gain = 1;
					this.values.express_gain = this.cfg.express_mult * (0.95 + rng() * 0.35);
					this.values.train_total = this.timers.express;
					this.values.train_speed = this.cfg.speed * this.values.express_gain;
					this.values.train_dir = rng() < 0.5 ? -1 : 1;
					this.values.train_cars = Math.max(3, Math.round(this.cfg.car_count) - 2 + Math.floor(rng() * 3));
					this.values.train_gap = Math.max(0.5, this.cfg.car_gap * (0.75 + rng() * 0.15));
					this.values.train_seed = rng() * 1000;
					this.values.smoke_gain = this.cfg.smoke * (0.7 + rng() * 0.35);
					this.values.headlight_gain = this.cfg.headlight * (1.15 + rng() * 0.55);
					return true;
				case 'quiet-gap':
					this.timers['quiet-gap'] = jitterInt(rng, this.cfg.quiet_gap_dur, 0.3);
					return true;
				case 'intro':
					this.timers.pass = 0;
					this.timers.express = 0;
					this.timers['quiet-gap'] = 0;
					this.timers.ending = 0;
					this.values.pass_gain = 1;
					this.values.express_gain = 1;
					this.values.train_total = 0;
					this.values.train_speed = 0;
					this.values.train_dir = 0;
					this.values.train_cars = 0;
					this.values.train_gap = 0;
					this.values.train_seed = 0;
					this.values.smoke_gain = 0;
					this.values.headlight_gain = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.pass = 0;
					this.timers.express = 0;
					this.timers['quiet-gap'] = 0;
					this.values.pass_gain = 1;
					this.values.express_gain = 1;
					this.values.train_total = 0;
					this.values.train_speed = 0;
					this.values.train_dir = 0;
					this.values.train_cars = 0;
					this.values.train_gap = 0;
					this.values.train_seed = 0;
					this.values.smoke_gain = 0;
					this.values.headlight_gain = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerMysteriousMan(name) {
			const rng = this._eventRng(name.length + 121);
			switch (name) {
				case 'inhale':
					this.timers.exhale = 0;
					this.timers.inhale = jitterInt(rng, this.cfg.inhale_dur, 0.3);
					this.values.inhale_gain = this.cfg.inhale_mult * (0.85 + rng() * 0.35);
					this.values.exhale_gain = 1;
					return true;
				case 'exhale':
					this.timers.inhale = 0;
					this.timers.exhale = jitterInt(rng, this.cfg.exhale_dur, 0.3);
					this.values.inhale_gain = 1;
					this.values.exhale_gain = this.cfg.exhale_mult * (0.85 + rng() * 0.35);
					this.values.exhale_total = this.timers.exhale;
					this.values.exhale_seed = rng() * 1000;
					this.values.exhale_dir = rng() < 0.5 ? -1 : 1;
					return true;
				case 'ash-fall':
					this.timers['ash-fall'] = jitterInt(rng, this.cfg.ash_fall_dur, 0.3);
					this.values.ash_gain = this.cfg.ash_fall_mult * (0.8 + rng() * 0.35);
					this.values.ash_seed = rng() * 1000;
					this.values.ash_total = this.timers['ash-fall'];
					return true;
				case 'lighter-flick':
					this.timers['lighter-flick'] = jitterInt(rng, this.cfg.lighter_flick_dur, 0.3);
					this.values.lighter_gain = this.cfg.lighter_flick_mult * (0.9 + rng() * 0.4);
					return true;
				case 'intro':
					this.timers.inhale = 0;
					this.timers.exhale = 0;
					this.timers['ash-fall'] = 0;
					this.timers['lighter-flick'] = 0;
					this.timers.ending = 0;
					this.values.inhale_gain = 1;
					this.values.exhale_gain = 1;
					this.values.exhale_total = 0;
					this.values.lighter_gain = 1;
					this.values.exhale_seed = 0;
					this.values.exhale_dir = 0;
					this.values.ash_gain = 1;
					this.values.ash_seed = 0;
					this.values.ash_total = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.inhale = 0;
					this.timers.exhale = 0;
					this.timers['ash-fall'] = 0;
					this.timers['lighter-flick'] = 0;
					this.values.inhale_gain = 1;
					this.values.exhale_gain = 1;
					this.values.exhale_total = 0;
					this.values.lighter_gain = 1;
					this.values.exhale_seed = 0;
					this.values.exhale_dir = 0;
					this.values.ash_gain = 1;
					this.values.ash_seed = 0;
					this.values.ash_total = 0;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerBurningTrees(name) {
			const rng = this._eventRng(name.length + 133);
			switch (name) {
				case 'ignite': {
					this.timers.flare = 0;
					this.timers.lull = 0;
					this.timers.ignite = jitterInt(rng, this.cfg.ignite_dur, 0.3);
					this.values.flare_gain = 1;
					this.values.lull_gain = 1;
					this.values.ignite_gain = this.cfg.flame * (0.95 + rng() * 0.45);
					this.values.ignite_total = this.timers.ignite;
					const treeCount = Math.max(1, Math.round(this.cfg.tree_count));
					this.values.ignite_center = Math.floor(rng() * treeCount);
					const span = Math.max(0.75, this.cfg.ignite_span * (0.7 + rng() * 0.7));
					this.values.ignite_span = Math.min(Math.max(1, treeCount - 1), span);
					this.values.ignite_seed = rng() * 1000;
					return true;
				}
				case 'flare':
					if (!this.timers.ignite || this.timers.ignite <= 0) this._triggerBurningTrees('ignite');
					this.timers.lull = 0;
					this.timers.flare = jitterInt(rng, this.cfg.flare_dur, 0.3);
					this.values.lull_gain = 1;
					this.values.flare_gain = this.cfg.flare_mult * (0.85 + rng() * 0.35);
					return true;
				case 'lull':
					if (!this.timers.ignite || this.timers.ignite <= 0) this._triggerBurningTrees('ignite');
					this.timers.flare = 0;
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.3);
					this.values.flare_gain = 1;
					this.values.lull_gain = Math.max(0.15, this.cfg.lull_mult * (0.85 + rng() * 0.25));
					return true;
				case 'intro':
					this.timers.ignite = 0;
					this.timers.flare = 0;
					this.timers.lull = 0;
					this.timers.ending = 0;
					this.values.ignite_gain = 0;
					this.values.ignite_total = 0;
					this.values.ignite_center = 0;
					this.values.ignite_span = 0;
					this.values.ignite_seed = 0;
					this.values.flare_gain = 1;
					this.values.lull_gain = 1;
					this._clearBurningTreesChar();
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.ignite = 0;
					this.timers.flare = 0;
					this.timers.lull = 0;
					this.values.ignite_gain = 0;
					this.values.ignite_total = 0;
					this.values.ignite_center = 0;
					this.values.ignite_span = 0;
					this.values.ignite_seed = 0;
					this.values.flare_gain = 1;
					this.values.lull_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerSand(name) {
			const rng = this._eventRng(name.length + 145);
			switch (name) {
				case 'surge':
					this.timers.calm = 0;
					this.timers.surge = jitterInt(rng, this.cfg.surge_dur, 0.3);
					this.values.calm_gain = 1;
					this.values.surge_gain = this.cfg.surge_mult * (0.9 + rng() * 0.4);
					return true;
				case 'calm':
					this.timers.surge = 0;
					this.timers.calm = jitterInt(rng, this.cfg.calm_dur, 0.3);
					this.values.surge_gain = 1;
					this.values.calm_gain = Math.max(0.08, this.cfg.calm_mult * (0.85 + rng() * 0.25));
					return true;
				case 'intro':
					this.timers.surge = 0;
					this.timers.calm = 0;
					this.timers.ending = 0;
					this.values.surge_gain = 1;
					this.values.calm_gain = 1;
					this.values.fill_level = clamp01(this.cfg.intro_fill);
					this.values.spill_level = 0;
					this.values.surface_bias = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.surge = 0;
					this.timers.calm = 0;
					this.values.surge_gain = 1;
					this.values.calm_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerWaterPipe(name) {
			const rng = this._eventRng(name.length + 157);
			switch (name) {
				case 'surge':
					this.timers['dry-up'] = 0;
					this.timers.surge = jitterInt(rng, this.cfg.surge_dur, 0.3);
					this.values.dry_up_gain = 1;
					this.values.surge_gain = this.cfg.surge_mult * (0.9 + rng() * 0.35);
					return true;
				case 'dry-up':
					this.timers.surge = 0;
					this.timers['dry-up'] = jitterInt(rng, this.cfg.dry_up_dur, 0.3);
					this.values.surge_gain = 1;
					this.values.dry_up_gain = Math.max(0.08, this.cfg.dry_up_mult * (0.85 + rng() * 0.25));
					return true;
				case 'intro':
					this.timers.surge = 0;
					this.timers['dry-up'] = 0;
					this.timers.ending = 0;
					this.values.surge_gain = 1;
					this.values.dry_up_gain = 1;
					this.values.fill_level = clamp01(this.cfg.intro_pool);
					this.values.spill_level = 0;
					this.values.surface_bias = 0;
					this.values.ripple_phase = 0;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.surge = 0;
					this.timers['dry-up'] = 0;
					this.values.surge_gain = 1;
					this.values.dry_up_gain = 1;
					this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
					this.values.ending_total = this.timers.ending;
					return true;
			}
			return false;
		}

		_triggerTetris(name) {
			const rng = this._eventRng(name.length + 173);
			switch (name) {
				case 'new-piece':
					this.timers.lull = 0;
					this.timers['new-piece'] = jitterInt(rng, this.cfg.new_piece_dur, 0.25);
					this.values.lull_gain = 1;
					this.values.new_piece_cut = Math.max(0.05, this.cfg.new_piece_cut * (0.8 + rng() * 0.35));
					if ((this.values.piece_alive || 0) > 0.5) {
						if ((this.values.fall_cd || 0) > 1) this.values.fall_cd = 1;
						if ((this.values.lock_cd || 0) > 1) this.values.lock_cd = 1;
					} else {
						const spawn = Math.round(this.values.spawn_cd || 0);
						if (spawn > 1) this.values.spawn_cd = Math.max(1, Math.round(spawn * this.values.new_piece_cut));
					}
					return true;
				case 'lull':
					this.timers['new-piece'] = 0;
					this.timers.lull = jitterInt(rng, this.cfg.lull_dur, 0.25);
					this.values.new_piece_cut = this.cfg.new_piece_cut;
					this.values.lull_gain = Math.max(1.05, this.cfg.lull_mult * (0.9 + rng() * 0.3));
					if ((this.values.piece_alive || 0) <= 0.5) {
						let delay = Math.round(this.values.spawn_cd || 0);
						if (delay < 1) delay = Math.round(this.cfg.spawn_every);
						const target = Math.round(delay * this.values.lull_gain);
						if (target > delay) this.values.spawn_cd = target;
					}
					return true;
				case 'intro':
					this._startTetrisIntro();
					return true;
				case 'ending':
					this._startTetrisEnding();
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

		_beamLevelLighthouse() {
			let level = 1;
			if (this.timers['bright-pass'] > 0) level *= this.values.bright_gain || this.cfg.bright_pass_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_beam + (1 - this.cfg.intro_beam) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_beam) * progress;
			}
			return Math.max(0.05, level);
		}

		_rippleLevelRowboat() {
			let level = 1;
			if (this.timers.wake > 0) level *= this.values.wake_gain || this.cfg.wake_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_drift + (1 - this.cfg.intro_drift) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_ripple) * progress;
			}
			if (this.timers.drift > 0) {
				level *= 1 + Math.abs(this.values.drift_push || this.cfg.drift_push) * 0.22;
			}
			return Math.max(0.04, level);
		}

		_sceneLevelUnderwater() {
			let level = 1;
			if (this.timers['bubble-burst'] > 0) level *= this.values.bubble_gain || this.cfg.bubble_burst_mult;
			if (this.timers.calm > 0) level *= this.cfg.calm_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_reveal + (1 - this.cfg.intro_reveal) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_murk) * progress;
			}
			if (this.timers['current-shift'] > 0) {
				level *= 1 + Math.abs(this.values.current_push || this.cfg.current_shift_push) * 0.18;
			}
			return Math.max(0.04, level);
		}

		_heatLevelVolcano() {
			let level = 1;
			if (this.timers.smolder > 0) level *= this.values.smolder_gain || this.cfg.smolder_mult;
			if (this.timers.flare > 0) level *= this.values.flare_gain || this.cfg.flare_mult;
			if (this.timers.eruption > 0) level *= 1 + Math.max(0, (this.values.eruption_gain || this.cfg.eruption_mult) - 1) * 0.45;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_glow + (1 - this.cfg.intro_glow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_embers) * progress;
			}
			return Math.max(0.04, level);
		}

		_sceneLevelTrain() {
			let level = 1;
			if (this.timers.pass > 0) level *= this.values.pass_gain || this.cfg.pass_mult;
			if (this.timers.express > 0) level *= this.values.express_gain || this.cfg.express_mult;
			if (this.timers['quiet-gap'] > 0) level *= this.cfg.quiet_gap_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_cue + (1 - this.cfg.intro_cue) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_clear) * progress;
			}
			return Math.max(0.04, level);
		}

		_sceneLevelMysteriousMan() {
			let level = 1;
			if (this.timers.inhale > 0) level *= this.values.inhale_gain || this.cfg.inhale_mult;
			if (this.timers.exhale > 0) level *= 1 + Math.max(0, (this.values.exhale_gain || this.cfg.exhale_mult) - 1) * 0.3;
			if (this.timers['lighter-flick'] > 0) level *= this.values.lighter_gain || this.cfg.lighter_flick_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_reveal + (1 - this.cfg.intro_reveal) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_shadow) * progress;
			}
			return Math.max(0.03, level);
		}

		_clearBurningTreesChar() {
			const limit = Math.max(32, Math.round(this.cfg.tree_count || 0) + 4);
			for (let i = 0; i < limit; i++) delete this.values[`char_${i}`];
		}

		_stepBurningTreesChar() {
			if (!this.timers.ignite || this.timers.ignite <= 0) return;
			const treeCount = Math.max(1, Math.round(this.cfg.tree_count));
			const total = Math.max(1, this.values.ignite_total || 0);
			const progress = clamp01(1 - this.timers.ignite / total);
			const center = this.values.ignite_center || 0;
			const span = Math.max(0.75, this.values.ignite_span || this.cfg.ignite_span);
			const fireGain = this.values.ignite_gain > 0 ? this.values.ignite_gain : this.cfg.flame;
			let mod = 1;
			if (this.timers.flare > 0) {
				const gain = this.values.flare_gain > 0 ? this.values.flare_gain : this.cfg.flare_mult;
				mod *= 1 + Math.max(0, gain - 1) * 0.55;
			}
			if (this.timers.lull > 0) {
				const gain = this.values.lull_gain > 0 ? this.values.lull_gain : this.cfg.lull_mult;
				mod *= Math.max(0.18, gain);
			}
			const ramp = 0.45 + 0.55 * Math.sin(progress * Math.PI);
			const reach = span + this.cfg.spread * (0.15 + progress * 0.55);
			for (let i = 0; i < treeCount; i++) {
				const dist = Math.abs(i - center);
				let influence = 1 - dist / Math.max(1, reach + 0.75);
				if (influence <= 0) continue;
				influence = Math.pow(influence, 0.72);
				const delta = this.cfg.char * 0.022 * (0.55 + fireGain * 1.6) * mod * ramp * influence;
				const key = `char_${i}`;
				this.values[key] = clamp01((this.values[key] || 0) + delta);
			}
		}

		_sceneLevelBurningTrees() {
			let level = 1;
			if (this.timers.ignite > 0) level *= 0.72 + (this.values.ignite_gain || this.cfg.flame) * 1.6;
			if (this.timers.flare > 0) level *= this.values.flare_gain || this.cfg.flare_mult;
			if (this.timers.lull > 0) level *= this.values.lull_gain || this.cfg.lull_mult;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				level *= this.cfg.intro_growth + (1 - this.cfg.intro_growth) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				level *= 1 - (1 - this.cfg.ending_ash) * progress;
			}
			return Math.max(0.04, level);
		}

		_sandFlowLevel() {
			let flow = Math.max(0, this.cfg.flow);
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				flow = this.cfg.intro_trickle + (flow - this.cfg.intro_trickle) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				flow *= Math.max(0, 1 - progress);
			}
			if (this.timers.surge > 0) flow *= this.values.surge_gain || this.cfg.surge_mult;
			if (this.timers.calm > 0) flow *= this.values.calm_gain || this.cfg.calm_mult;
			return Math.max(0, flow);
		}

		_stepSandState() {
			const flow = this._sandFlowLevel();
			let fill = clamp01(this.values.fill_level || 0);
			let spill = clamp01(this.values.spill_level || 0);
			let bias = this.values.surface_bias || 0;

			const incoming = flow * (0.02 + this.cfg.spread * 0.012);
			fill += incoming * (0.8 + this.cfg.settle * 0.35);
			if (fill > 1) {
				spill += (fill - 1) * (0.6 + this.cfg.overflow * 0.5);
				fill = 1;
			}
			if (fill > 0.88) {
				spill += (fill - 0.88) * incoming * (0.4 + this.cfg.overflow * 0.8);
			}
			spill -= (0.004 + this.cfg.settle * 0.008) * Math.max(0.12, 1 - flow * 1.2);
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				spill += this.cfg.ending_settle * 0.006 * (1 - progress);
			}

			let targetBias = (this.cfg.pipe_x - this.cfg.container_x) / 0.22;
			targetBias = Math.max(-1, Math.min(1, targetBias));
			bias += (targetBias - bias) * (0.04 + flow * 0.08 + this.cfg.settle * 0.04);
			if (spill > 0.02) bias += (targetBias < 0 ? -1 : 1) * spill * 0.01;
			bias = Math.max(-1, Math.min(1, bias));

			this.values.fill_level = clamp01(fill);
			this.values.spill_level = clamp01(spill);
			this.values.surface_bias = bias;
		}

		_waterPipeFlowLevel() {
			let flow = Math.max(0, this.cfg.flow);
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				flow = this.cfg.intro_flow + (flow - this.cfg.intro_flow) * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				flow *= Math.max(0, 1 - progress);
			}
			if (this.timers.surge > 0) flow *= this.values.surge_gain || this.cfg.surge_mult;
			if (this.timers['dry-up'] > 0) flow *= this.values.dry_up_gain || this.cfg.dry_up_mult;
			return Math.max(0, flow);
		}

		_stepWaterPipeState() {
			const flow = this._waterPipeFlowLevel();
			let fill = clamp01(this.values.fill_level || 0);
			let spill = clamp01(this.values.spill_level || 0);
			let bias = this.values.surface_bias || 0;
			let ripplePhase = this.values.ripple_phase || 0;

			const incoming = flow * (0.016 + this.cfg.stream * 0.010);
			fill += incoming * (0.72 + this.cfg.ripple * 0.2);
			if (fill > 1) {
				spill += (fill - 1) * (0.7 + this.cfg.overflow * 0.45);
				fill = 1;
			}
			if (fill > 0.9) {
				spill += (fill - 0.9) * incoming * (0.3 + this.cfg.overflow * 0.9);
			}
			spill -= (0.002 + this.cfg.overflow * 0.006) * Math.max(0.1, 1 - flow * 0.9);
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				spill += this.cfg.ending_ripple * 0.004 * (1 - progress);
			}

			let targetBias = (this.cfg.pipe_x - this.cfg.basin_x) / 0.24;
			targetBias = Math.max(-1, Math.min(1, targetBias));
			bias += (targetBias - bias) * (0.03 + flow * 0.05 + this.cfg.ripple * 0.03);
			bias = Math.max(-1, Math.min(1, bias));

			ripplePhase += 0.12 + flow * (0.8 + this.cfg.stream * 0.4) + spill * 0.4;
			if (ripplePhase > Math.PI * 200) ripplePhase = positiveMod(ripplePhase, Math.PI * 2);

			this.values.fill_level = clamp01(fill);
			this.values.spill_level = clamp01(spill);
			this.values.surface_bias = bias;
			this.values.ripple_phase = ripplePhase;
		}

		_tetrisRng(key) {
			return makeRNG(((this.seed >>> 0) ^ (((key >>> 0) * 2654435761) >>> 0)) >>> 0);
		}

		_tetrisCellKey(row, col) {
			return `cell_${String(row).padStart(2, '0')}_${String(col).padStart(2, '0')}`;
		}

		_tetrisGetCell(row, col) {
			if (row < 0 || row >= TETRIS_ROWS || col < 0 || col >= TETRIS_COLS) return 0;
			return Math.round(this.values[this._tetrisCellKey(row, col)] || 0);
		}

		_tetrisSetCell(row, col, value) {
			if (row < 0 || row >= TETRIS_ROWS || col < 0 || col >= TETRIS_COLS) return;
			const key = this._tetrisCellKey(row, col);
			if (value <= 0) {
				delete this.values[key];
				return;
			}
			this.values[key] = value;
		}

		_tetrisClearBoard() {
			for (let row = 0; row < TETRIS_ROWS; row++) {
				for (let col = 0; col < TETRIS_COLS; col++) {
					delete this.values[this._tetrisCellKey(row, col)];
				}
			}
		}

		_tetrisSeedIntroStack() {
			const rows = Math.max(0, Math.round(this.cfg.intro_stack || 0));
			if (rows <= 0) return;
			for (let col = 0; col < TETRIS_COLS; col++) {
				const rng = this._tetrisRng(400 + col * 7);
				let height = rows - 1 + Math.floor(rng() * 3);
				height = Math.max(0, Math.min(rows, height));
				for (let depth = 0; depth < height; depth++) {
					const row = TETRIS_ROWS - 1 - depth;
					const kind = 1 + (col + depth + Math.floor(rng() * TETRIS_PIECES.length)) % TETRIS_PIECES.length;
					this._tetrisSetCell(row, col, kind);
				}
			}
		}

		_tetrisClearCurrentPiece() {
			this.values.piece_alive = 0;
			delete this.values.piece_kind;
			delete this.values.piece_rot;
			delete this.values.piece_x;
			delete this.values.piece_y;
			delete this.values.fall_cd;
			delete this.values.lock_cd;
		}

		_tetrisCurrentPiece() {
			const alive = (this.values.piece_alive || 0) > 0.5;
			if (!alive) return { alive: false, kind: 0, rot: 0, x: 0, y: 0 };
			return {
				alive: true,
				kind: Math.round(this.values.piece_kind || 0),
				rot: Math.round(this.values.piece_rot || 0),
				x: Math.round(this.values.piece_x || 0),
				y: Math.round(this.values.piece_y || 0),
			};
		}

		_tetrisSetCurrentPiece(kind, rot, x, y) {
			this.values.piece_alive = 1;
			this.values.piece_kind = kind;
			this.values.piece_rot = rot;
			this.values.piece_x = x;
			this.values.piece_y = y;
		}

		_tetrisPiecePoints(kind, rot) {
			const rotations = TETRIS_PIECES[Math.round(kind) - 1] || [];
			if (!rotations.length) return [];
			return rotations[positiveMod(Math.round(rot), rotations.length)];
		}

		_tetrisCollision(kind, rot, x, y) {
			for (const pt of this._tetrisPiecePoints(kind, rot)) {
				const col = x + pt.x;
				const row = y + pt.y;
				if (col < 0 || col >= TETRIS_COLS || row < 0 || row >= TETRIS_ROWS) return true;
				if (this._tetrisGetCell(row, col) > 0) return true;
			}
			return false;
		}

		_tetrisLockPiece(kind, rot, x, y) {
			for (const pt of this._tetrisPiecePoints(kind, rot)) {
				this._tetrisSetCell(y + pt.y, x + pt.x, kind);
			}
		}

		_tetrisUpdateBoardStats() {
			let filled = 0;
			let top = TETRIS_ROWS;
			for (let row = 0; row < TETRIS_ROWS; row++) {
				for (let col = 0; col < TETRIS_COLS; col++) {
					if (this._tetrisGetCell(row, col) <= 0) continue;
					filled++;
					if (row < top) top = row;
				}
			}
			this.values.fill_ratio = filled / (TETRIS_COLS * TETRIS_ROWS);
			this.values.stack_height = filled > 0 ? (TETRIS_ROWS - top) : 0;
		}

		_tetrisSpawnDelay() {
			let delay = Math.max(1, Math.round(this.cfg.spawn_every));
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				const cadence = this.cfg.intro_cadence + (1 - this.cfg.intro_cadence) * progress;
				delay = Math.max(1, Math.round(delay * Math.max(0.2, cadence)));
			}
			if (this.timers.lull > 0) {
				const gain = (this.values.lull_gain || 0) > 1 ? this.values.lull_gain : this.cfg.lull_mult;
				delay = Math.max(1, Math.round(delay * Math.max(1.05, gain)));
			}
			if (this.timers['new-piece'] > 0) {
				const cut = (this.values.new_piece_cut || 0) > 0 ? this.values.new_piece_cut : this.cfg.new_piece_cut;
				delay = Math.max(1, Math.round(delay * Math.max(0.05, cut)));
			}
			return delay;
		}

		_tetrisFallDelay() {
			let delay = Math.max(1, Math.round(this.cfg.fall_every));
			if (this.timers['new-piece'] > 0) delay = Math.max(1, Math.round(delay * 0.6));
			return delay;
		}

		_startTetrisIntro() {
			this.timers['new-piece'] = 0;
			this.timers.lull = 0;
			this.timers.ending = 0;
			this.values.lull_gain = 1;
			this.values.new_piece_cut = this.cfg.new_piece_cut;
			delete this.values.ended;
			delete this.values.ending_total;
			this._tetrisClearCurrentPiece();
			this._tetrisClearBoard();
			this._tetrisSeedIntroStack();
			this.values.piece_counter = 0;
			this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
			this.values.intro_total = this.timers.intro;
			this.values.spawn_cd = Math.max(1, Math.round(this.cfg.spawn_every * Math.max(0.2, this.cfg.intro_cadence)));
			this._tetrisUpdateBoardStats();
		}

		_startTetrisEnding() {
			this.timers.intro = 0;
			this.timers['new-piece'] = 0;
			this.timers.lull = 0;
			this.values.lull_gain = 1;
			delete this.values.intro_total;
			this._tetrisClearCurrentPiece();
			this.timers.ending = Math.max(1, Math.round(this.cfg.ending_dur + Math.max(0, this.cfg.ending_linger)));
			this.values.ending_total = this.timers.ending;
			this.values.ended = 1;
			this._tetrisUpdateBoardStats();
		}

		_spawnTetrisPiece() {
			const counter = Math.round(this.values.piece_counter || 0);
			const rng = this._tetrisRng(800 + counter * 17);
			const kind = 1 + Math.floor(rng() * TETRIS_PIECES.length);
			const rot = Math.floor(rng() * 4);
			const x = 3;
			const y = 0;
			if (this._tetrisCollision(kind, rot, x, y)) {
				this._startTetrisEnding();
				return false;
			}
			this.values.piece_counter = counter + 1;
			this._tetrisSetCurrentPiece(kind, rot, x, y);
			this.values.fall_cd = this._tetrisFallDelay();
			this.values.lock_cd = Math.max(1, Math.round(this.cfg.lock_delay));
			this.values.spawn_cd = 0;
			if (this.timers['new-piece'] > 0) this.timers['new-piece'] = 0;
			return true;
		}

		_stepTetrisState() {
			if (!this.timers.lull || this.timers.lull <= 0) this.values.lull_gain = 1;
			if (!this.timers['new-piece'] || this.timers['new-piece'] <= 0) delete this.values.new_piece_cut;
			if (!this.timers.intro || this.timers.intro <= 0) delete this.values.intro_total;
			if (!this.timers.ending || this.timers.ending <= 0) {
				delete this.values.ending_total;
				if ((this.values.ended || 0) > 0.5) {
					this._startTetrisIntro();
					return;
				}
			}
			if (this.timers.ending > 0) {
				this._tetrisUpdateBoardStats();
				return;
			}

			const piece = this._tetrisCurrentPiece();
			if (!piece.alive) {
				let spawnCD = Math.round(this.values.spawn_cd || 0);
				if (spawnCD > 0) {
					spawnCD--;
					this.values.spawn_cd = spawnCD;
				}
				if (spawnCD <= 0) this._spawnTetrisPiece();
			} else {
				let fallCD = Math.round(this.values.fall_cd || 0);
				let lockCD = Math.round(this.values.lock_cd || 0);
				if (this.timers['new-piece'] > 0 && fallCD > 1) fallCD = 1;
				if (fallCD > 0) {
					fallCD--;
					this.values.fall_cd = fallCD;
				} else if (!this._tetrisCollision(piece.kind, piece.rot, piece.x, piece.y + 1)) {
					this.values.piece_y = piece.y + 1;
					this.values.fall_cd = this._tetrisFallDelay();
					this.values.lock_cd = Math.max(1, Math.round(this.cfg.lock_delay));
				} else if (lockCD > 0) {
					lockCD--;
					this.values.lock_cd = lockCD;
				} else {
					this._tetrisLockPiece(piece.kind, piece.rot, piece.x, piece.y);
					this._tetrisClearCurrentPiece();
					this.values.spawn_cd = this._tetrisSpawnDelay();
					this._tetrisUpdateBoardStats();
					if ((this.values.fill_ratio || 0) >= Math.max(0.6, this.cfg.ending_fill)) {
						this._startTetrisEnding();
						return;
					}
				}
			}

			this._tetrisUpdateBoardStats();
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

		_renderLighthouse(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue + 358) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin * 0.92));
				const skyMid = hslToRGB(this.cfg.hue, this.cfg.sat, clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const skyLow = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.6 + 360) % 360, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.46));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(8, Math.min(this.h - 10, Math.floor(this.h * this.cfg.horizon)));
			const towerX = Math.floor(this.w * 0.18);
			const towerH = Math.max(10, Math.round(this.cfg.tower_height));
			const towerW = Math.max(3, Math.round(this.cfg.tower_width));
			const beamLevel = this._beamLevelLighthouse();
			const fogLevel = clamp01(this.cfg.haze * (this.timers['fog-thicken'] > 0 ? this.values.fog_gain || this.cfg.fog_thicken_mult : 1) * (this.timers.calm > 0 ? 0.82 : 1));
			const beamAngle = -0.26 + Math.sin(this.tick * this.cfg.sweep_speed * 0.06) * 0.78;
			const beamWidth = this.cfg.beam_width * (1 + fogLevel * 0.9);
			const beamSoftness = clamp01(this.cfg.beam_softness * (1 + fogLevel * 0.35));

			const seaTop = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.12));
			const seaLow = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin * 0.8));
			const sea = ctx.createLinearGradient(0, Math.floor(horizon * sy), 0, canvasH);
			sea.addColorStop(0, `rgb(${seaTop.r},${seaTop.g},${seaTop.b})`);
			sea.addColorStop(1, `rgb(${seaLow.r},${seaLow.g},${seaLow.b})`);
			ctx.fillStyle = sea;
			ctx.fillRect(0, Math.floor(horizon * sy), canvasW, canvasH - Math.floor(horizon * sy));

			const coastRows = new Array(this.w);
			for (let x = 0; x < this.w; x++) {
				const bluff = Math.exp(-Math.pow((x - towerX) / 18, 2)) * 7.5;
				const swell = Math.sin(x * 0.032 + 0.5) * 1.3 + Math.sin(x * 0.013 + 2.1) * 1.1;
				coastRows[x] = Math.round(horizon + swell - bluff);
			}
			const coastColor = hslToRGB((this.cfg.hue + 214) % 360, clamp01(this.cfg.sat * 0.12), 0.08);
			ctx.fillStyle = `rgb(${coastColor.r},${coastColor.g},${coastColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				ctx.lineTo(Math.floor(x * sx), Math.floor(coastRows[x] * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const oceanLine = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.35), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.3));
			for (let x = 0; x < this.w; x += 2) {
				if ((x + this.tick) % 5 !== 0) continue;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, horizon + 1 + Math.round(Math.sin(x * 0.07 + this.tick * 0.02)), 2, 1, `rgb(${oceanLine.r},${oceanLine.g},${oceanLine.b})`, 0.18);
			}

			const towerBase = coastRows[Math.max(0, Math.min(this.w - 1, towerX))];
			const lampY = towerBase - towerH + 2;
			const towerColor = hslToRGB((this.cfg.hue + 212) % 360, clamp01(this.cfg.sat * 0.08), 0.1);
			for (let y = lampY; y <= towerBase; y++) {
				const ratio = (y - lampY) / Math.max(1, towerBase - lampY);
				const half = Math.max(1, Math.round((towerW * (0.32 + ratio * 0.68)) * 0.5));
				for (let dx = -half; dx <= half; dx++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX + dx, y, 1, 1, `rgb(${towerColor.r},${towerColor.g},${towerColor.b})`, 1);
				}
			}
			for (let dx = -Math.max(2, Math.round(towerW * 0.6)); dx <= Math.max(2, Math.round(towerW * 0.6)); dx++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX + dx, lampY - 1 + Math.round(Math.abs(dx) * 0.15), 1, 1, `rgb(${towerColor.r},${towerColor.g},${towerColor.b})`, 1);
			}

			const lampGlow = hslToRGB(48, 0.68, clamp01(0.42 + this.cfg.glow * 0.34));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, towerX, lampY, 1, 1, `rgb(${lampGlow.r},${lampGlow.g},${lampGlow.b})`, clamp01(0.3 + this.cfg.glow * 0.7));

			const glowX = towerX * sx;
			const glowY = lampY * sy;
			const glowR = Math.max(18, Math.min(canvasW, canvasH) * (0.04 + this.cfg.glow * 0.05));
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR);
			glow.addColorStop(0, `rgba(255, 236, 184, ${0.16 + this.cfg.glow * 0.22})`);
			glow.addColorStop(1, 'rgba(255, 236, 184, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(glowX - glowR, glowY - glowR, glowR * 2, glowR * 2);

			const beamHue = (this.cfg.hue - 18 + 360) % 360;
			const beamBase = hslToRGB(beamHue, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 0.98));
			const beamCore = hslToRGB(44, 0.42, clamp01(this.cfg.lmax * 1.02));
			const angleDiff = (a, b) => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b)));
			for (let y = 0; y <= horizon + 5; y++) {
				for (let x = towerX + 1; x < this.w; x++) {
					const dx = x - towerX;
					const dy = y - lampY;
					if (dx <= 0) continue;
					const dist = Math.hypot(dx, dy);
					if (dist < 2) continue;
					const ang = Math.atan2(dy, dx);
					const diff = angleDiff(ang, beamAngle);
					if (diff > beamWidth) continue;
					const cone = 1 - diff / beamWidth;
					const edge = Math.pow(cone, Math.max(0.6, 1.8 - beamSoftness));
					const falloff = Math.pow(clamp01(1 - dist / (this.w * 0.92)), 0.72);
					const strength = edge * falloff * beamLevel;
					if (strength < 0.02) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${beamBase.r},${beamBase.g},${beamBase.b})`, clamp01(strength * (0.12 + fogLevel * 0.08)));
					if (diff < beamWidth * 0.34 && strength > 0.08) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${beamCore.r},${beamCore.g},${beamCore.b})`, clamp01(strength * 0.16));
					}
				}
			}

			const fog = ctx.createLinearGradient(0, Math.floor((horizon - 7) * sy), 0, Math.floor((horizon + 9) * sy));
			fog.addColorStop(0, `rgba(201, 214, 226, ${0.02 + fogLevel * 0.08})`);
			fog.addColorStop(1, 'rgba(201, 214, 226, 0)');
			ctx.fillStyle = fog;
			ctx.fillRect(0, Math.floor((horizon - 8) * sy), canvasW, Math.ceil(18 * sy));
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

		_renderRowboat(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 12 + 360) % 360, clamp01(this.cfg.sat * 0.45), clamp01(this.cfg.lmin + 0.02));
				const skyMid = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.22 + 360) % 360, clamp01(this.cfg.sat * 0.58), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.34));
				const skyLow = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.18) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.62));
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
			const waterline = Math.max(12, Math.min(this.h - 10, Math.floor(this.h * this.cfg.waterline)));
			const motion = this._rippleLevelRowboat();
			const driftPush = this.values.drift_push || 0;
			const phase = this.tick * this.cfg.drift_speed * 0.08;

			const glowX = canvasW * 0.74;
			const glowY = canvasH * 0.24;
			const glowR = Math.max(20, Math.min(canvasW, canvasH) * 0.09);
			const glow = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, glowR * 2.8);
			glow.addColorStop(0, 'rgba(255, 223, 174, 0.22)');
			glow.addColorStop(1, 'rgba(255, 223, 174, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const ridgeBase = waterline - Math.max(5, Math.round(this.h * 0.12));
			const shorelineY = Math.max(ridgeBase + 2, waterline - Math.max(2, Math.round(this.h * 0.04)));
			const ridgePoints = [];
			const ridgeSegments = 7;
			for (let i = 0; i <= ridgeSegments; i++) {
				const ridgeWave = Math.sin(i * 0.9 + this._hash(25100 + i) * 2.4) * 2.8;
				ridgePoints.push(Math.round(ridgeBase - Math.abs(ridgeWave) - this._hash(25200 + i) * 3));
			}
			const ridgeColor = hslToRGB((this.cfg.hue + 54) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin * 0.7));
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, Math.floor(shorelineY * sy));
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const ridgeY = ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(ridgeY * sy));
			}
			ctx.lineTo(canvasW, Math.floor(shorelineY * sy));
			ctx.closePath();
			ctx.fill();

			const treelineColor = hslToRGB((this.cfg.hue + 72) % 360, clamp01(this.cfg.sat * 0.2), clamp01(this.cfg.lmin * 0.52));
			for (let i = 0; i < 11; i++) {
				const col = Math.floor((i + 0.4) * this.w / 11 + (this._hash(25300 + i) - 0.5) * 6);
				const top = Math.round(ridgeBase - 1 - this._hash(25400 + i) * 4);
				const height = 2 + Math.floor(this._hash(25500 + i) * 3);
				for (let row = 0; row < height; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, top + row, 1, 1, `rgb(${treelineColor.r},${treelineColor.g},${treelineColor.b})`, 0.92);
				}
			}

			for (let y = waterline; y < this.h; y++) {
				const depth = (y - waterline) / Math.max(1, this.h - waterline);
				const hue = ((this.cfg.hue + depth * this.cfg.hue_sp * 0.22) % 360 + 360) % 360;
				const sat = clamp01(this.cfg.sat * (0.8 - depth * 0.22));
				const light = clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (0.36 - depth * 0.18));
				const color = hslToRGB(hue, sat, light);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, y, this.w, 1, `rgb(${color.r},${color.g},${color.b})`, 1);
			}

			const mist = ctx.createLinearGradient(0, Math.floor((shorelineY - 3) * sy), 0, Math.floor((waterline + 8) * sy));
			mist.addColorStop(0, 'rgba(255, 240, 220, 0.14)');
			mist.addColorStop(1, 'rgba(255, 240, 220, 0)');
			ctx.fillStyle = mist;
			ctx.fillRect(0, Math.floor((shorelineY - 3) * sy), canvasW, Math.ceil((waterline - shorelineY + 11) * sy));

			const surfaceColor = hslToRGB((this.cfg.hue - 6 + 360) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmax * 0.92));
			for (let x = 0; x < this.w; x++) {
				const wave = Math.sin(x * this.cfg.wave_freq + phase) * this.cfg.wave_amp;
				const subWave = Math.sin(x * this.cfg.wave_freq * 0.42 - phase * 1.7) * this.cfg.wave_amp * 0.36;
				const row = waterline + Math.round((wave + subWave) * motion * 0.22);
				const twinkle = 0.45 + 0.55 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.03 + x * 0.11), 2);
				const alpha = clamp01((0.05 + this.cfg.reflection * 0.16) * twinkle);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, alpha);
				if ((x + this.tick) % 6 === 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row + 2, 1, 1, `rgb(${surfaceColor.r},${surfaceColor.g},${surfaceColor.b})`, alpha * 0.45);
				}
			}

			const boatLen = Math.max(7, Math.round(this.cfg.boat_len));
			const boatHeight = Math.max(2, Math.round(this.cfg.boat_height));
			const boatX = Math.max(Math.floor(boatLen * 0.6), Math.min(this.w - Math.ceil(boatLen * 0.6), Math.round(this.w * 0.34 + Math.sin(phase * 1.6 + 0.8) * (2.4 + motion * 1.6) + driftPush * 1.8)));
			const bob = Math.sin(phase * 2.4 + 0.6) * this.cfg.bob_amp * motion * 0.52 + Math.sin(phase * 0.95 + 1.3) * 0.35;
			const hullBaseY = waterline - Math.round(bob * 0.55);
			const tilt = Math.sin(phase * 1.9 + 0.4) * motion * 0.9 + driftPush * 0.2;
			const hullColor = hslToRGB(24, 0.34, 0.22);
			const railColor = hslToRGB(31, 0.26, 0.36);
			const seatColor = hslToRGB(28, 0.18, 0.16);
			const hullRows = [];

			for (let row = 0; row < boatHeight; row++) {
				const t = boatHeight === 1 ? 0.5 : row / (boatHeight - 1);
				const arch = 1 - Math.abs(t * 2 - 1);
				const width = Math.max(4, Math.round(boatLen * (0.54 + arch * 0.42)));
				const y = hullBaseY - (boatHeight - 1 - row);
				const offset = Math.round((t - 0.5) * tilt * 1.8);
				const startX = Math.round(boatX - width / 2 + offset);
				hullRows.push({ startX, width, y });
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, startX, y, width, 1, `rgb(${hullColor.r},${hullColor.g},${hullColor.b})`, clamp01(0.78 + t * 0.2));
			}

			const topHull = hullRows[0];
			if (topHull && topHull.width > 3) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, topHull.startX + 1, topHull.y, topHull.width - 2, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.72);
			}
			const seatWidth = Math.max(2, Math.round(boatLen * 0.22));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - seatWidth / 2), hullBaseY - Math.max(1, Math.floor(boatHeight / 2)), seatWidth, 1, `rgb(${seatColor.r},${seatColor.g},${seatColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - boatLen * 0.46), hullBaseY - 1, 1, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX + boatLen * 0.44), hullBaseY - 1, 1, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.82);

			const shadowColor = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.9));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(boatX - boatLen * 0.5), waterline, boatLen, 1, `rgb(${shadowColor.r},${shadowColor.g},${shadowColor.b})`, 0.26);

			const reflectionColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmax * 0.58));
			const reflectionLevel = clamp01(this.cfg.reflection * (0.3 + motion * 0.22));
			for (let i = 0; i < hullRows.length; i++) {
				const row = hullRows[i];
				const distance = hullBaseY - row.y + 1;
				const wobble = Math.round(Math.sin(this.tick * 0.08 + i * 0.8 + row.startX * 0.03) * (0.5 + motion * 0.45));
				const reflY = hullBaseY + distance + Math.round(Math.sin(row.startX * this.cfg.wave_freq + phase) * motion * 0.35);
				const reflWidth = Math.max(2, row.width - 1 - Math.floor(distance / 2));
				const alpha = clamp01(reflectionLevel * (0.4 - distance * 0.045));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, row.startX + wobble, reflY, reflWidth, 1, `rgb(${reflectionColor.r},${reflectionColor.g},${reflectionColor.b})`, alpha);
			}

			const rippleColor = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.32), clamp01(this.cfg.lmax * 0.98));
			const wakeGain = this.timers.wake > 0 ? (this.values.wake_gain || this.cfg.wake_mult) : 1;
			const rippleBands = 4 + Math.round(this.cfg.ripple * 7);
			for (let band = 0; band < rippleBands; band++) {
				const centerX = Math.round(boatX + boatLen * 0.18 + band * (1.2 + wakeGain * 0.28));
				const half = Math.max(3, Math.round(boatLen * (0.24 + band * 0.14 + wakeGain * 0.03)));
				const centerY = waterline + Math.round(0.8 + band * 0.75 + Math.abs(Math.sin(phase * 4.2 + band * 0.9)) * 1.1);
				for (let dx = -half; dx <= half; dx++) {
					if ((dx + band + this.tick) % 2 !== 0) continue;
					const edge = 1 - Math.abs(dx) / Math.max(1, half);
					const waveY = centerY + Math.round(Math.sin(dx * 0.22 + this.tick * 0.08 + band * 0.7) * 0.7);
					const alpha = clamp01((0.08 + this.cfg.ripple * 0.24 * motion) * Math.pow(edge, 0.45));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, waveY, 1, 1, `rgb(${rippleColor.r},${rippleColor.g},${rippleColor.b})`, alpha);
				}
			}

			for (let band = 0; band < 3; band++) {
				const half = Math.max(2, Math.round(boatLen * (0.16 + band * 0.08)));
				const centerX = Math.round(boatX - boatLen * 0.2 - band * 1.2);
				const centerY = waterline + Math.round(band * 0.85 + Math.abs(Math.sin(phase * 3.6 + band)) * 0.9);
				for (let dx = -half; dx <= half; dx++) {
					if ((dx + band) % 2 !== 0) continue;
					const edge = 1 - Math.abs(dx) / Math.max(1, half);
					const alpha = clamp01((0.03 + this.cfg.ripple * 0.12 * motion) * Math.pow(edge, 0.65));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX + dx, centerY + Math.round(Math.sin(dx * 0.3 + this.tick * 0.06) * 0.5), 1, 1, `rgb(${rippleColor.r},${rippleColor.g},${rippleColor.b})`, alpha);
				}
			}
		}

		_renderUnderwater(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const top = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.58), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.54));
				const mid = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.82), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.28));
				const deep = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin * (0.72 - this.cfg.depth * 0.18)));
				const water = ctx.createLinearGradient(0, 0, 0, canvasH);
				water.addColorStop(0, `rgb(${top.r},${top.g},${top.b})`);
				water.addColorStop(0.46, `rgb(${mid.r},${mid.g},${mid.b})`);
				water.addColorStop(1, `rgb(${deep.r},${deep.g},${deep.b})`);
				ctx.fillStyle = water;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const level = this._sceneLevelUnderwater();
			const currentPush = this.timers['current-shift'] > 0 ? (this.values.current_push || this.cfg.current_shift_push) : 0;
			const floorBase = Math.max(Math.floor(this.h * 0.78), Math.min(this.h - 4, Math.floor(this.h * (0.84 + this.cfg.depth * 0.08))));
			const phase = this.tick * this.cfg.rise_speed * 0.04;

			const surfaceGlow = ctx.createRadialGradient(canvasW * 0.32, 0, 0, canvasW * 0.32, 0, Math.max(canvasW, canvasH) * 0.52);
			surfaceGlow.addColorStop(0, `rgba(210, 244, 238, ${clamp01(0.08 + this.cfg.caustics * 0.18 * level)})`);
			surfaceGlow.addColorStop(1, 'rgba(210, 244, 238, 0)');
			ctx.fillStyle = surfaceGlow;
			ctx.fillRect(0, 0, canvasW, canvasH);

			const beamCount = 4;
			for (let i = 0; i < beamCount; i++) {
				const sourceX = canvasW * (0.08 + i * 0.24 + this._hash(30100 + i) * 0.08);
				const spread = canvasW * (0.08 + this._hash(30200 + i) * 0.08);
				const bend = (currentPush * 12 + Math.sin(this.tick * 0.02 + i) * 10) * this.cfg.caustics;
				ctx.fillStyle = `rgba(210, 248, 242, ${clamp01(0.04 + this.cfg.caustics * 0.12 * level)})`;
				ctx.beginPath();
				ctx.moveTo(sourceX - spread * 0.2, 0);
				ctx.lineTo(sourceX + spread * 0.22, 0);
				ctx.lineTo(sourceX + spread + bend, canvasH * 0.7);
				ctx.lineTo(sourceX - spread * 0.65 + bend * 0.45, canvasH * 0.7);
				ctx.closePath();
				ctx.fill();
			}

			const causticColor = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmax * 0.96));
			for (let band = 0; band < 5; band++) {
				const baseY = Math.floor(this.h * (0.16 + band * 0.09));
				for (let x = 0; x < this.w; x++) {
					if ((x + band) % 2 !== 0) continue;
					const wave = Math.sin(x * 0.16 + phase * (1.2 + band * 0.18) + band) + Math.sin(x * 0.07 - phase * 1.4 + band * 1.7);
					const row = baseY + Math.round(wave * this.cfg.caustics * level * 1.3);
					const alpha = clamp01((0.03 + this.cfg.caustics * 0.18 * level) * (0.82 - band * 0.12));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, row, 1, 1, `rgb(${causticColor.r},${causticColor.g},${causticColor.b})`, alpha);
				}
			}

			const particulateColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmax * 0.8));
			const particulateCount = Math.max(18, Math.round(this.w * 0.14));
			for (let i = 0; i < particulateCount; i++) {
				const col = Math.floor(this._hash(30300 + i) * this.w);
				const row = Math.floor(this._hash(30400 + i) * Math.max(1, floorBase - 6));
				const blink = 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(this.tick * 0.018 + i * 0.6), 2);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, row, 1, 1, `rgb(${particulateColor.r},${particulateColor.g},${particulateColor.b})`, clamp01((0.04 + this.cfg.depth * 0.08) * blink));
			}

			const seabedPoints = [];
			const seabedSegments = 8;
			for (let i = 0; i <= seabedSegments; i++) {
				seabedPoints.push(Math.round(floorBase - Math.abs(Math.sin(i * 0.8 + this._hash(30500 + i) * 2.4)) * 2 - this._hash(30600 + i) * 2));
			}
			const seabedColor = hslToRGB((this.cfg.hue + 36) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 0.85));
			ctx.fillStyle = `rgb(${seabedColor.r},${seabedColor.g},${seabedColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * seabedSegments;
				const idx = Math.min(seabedSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const row = seabedPoints[idx] + (seabedPoints[idx + 1] - seabedPoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(row * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const weedColor = hslToRGB((this.cfg.hue - 36 + 360) % 360, clamp01(this.cfg.sat * 0.6), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.28));
			const weedAccent = hslToRGB((this.cfg.hue - 18 + 360) % 360, clamp01(this.cfg.sat * 0.48), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.4));
			const weedCount = Math.max(4, Math.round(this.cfg.weed_count));
			for (let i = 0; i < weedCount; i++) {
				const baseX = Math.floor((i + 0.35) * this.w / weedCount + (this._hash(30700 + i) - 0.5) * 5);
				const rootY = floorBase - 1 - Math.floor(this._hash(30800 + i) * 3);
				const fronds = 2 + Math.floor(this._hash(30900 + i) * 2);
				for (let f = 0; f < fronds; f++) {
					const height = Math.max(7, Math.round(this.cfg.weed_height * (0.58 + this._hash(31000 + i * 5 + f) * 0.5)));
					const offset = (f - (fronds - 1) / 2) * 1.2;
					const localPhase = this.tick * 0.035 * (0.8 + this._hash(31100 + i * 5 + f) * 0.4) + i * 0.7 + f * 0.4;
					for (let seg = 0; seg < height; seg++) {
						const progress = seg / Math.max(1, height - 1);
						const sway = Math.sin(localPhase + progress * 2.6) * this.cfg.sway * level * (1.1 + Math.abs(currentPush) * 0.55) + currentPush * progress * 1.4;
						const x = Math.round(baseX + offset + sway * progress * 1.2);
						const y = rootY - seg;
						const color = seg < height * 0.28 ? weedColor : weedAccent;
						const alpha = clamp01(0.3 + progress * 0.42);
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, seg < height * 0.25 ? 2 : 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					}
				}
			}

			const burstGain = this.timers['bubble-burst'] > 0 ? (this.values.bubble_gain || this.cfg.bubble_burst_mult) : 1;
			const bubbleDensity = Math.max(0.04, this.cfg.density * level);
			const bubbleCount = Math.max(12, Math.round(this.w * bubbleDensity * (0.44 + Math.max(0, burstGain - 1) * 0.18)));
			const bubbleColor = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmax * 0.98));
			for (let i = 0; i < bubbleCount; i++) {
				const baseX = this._hash(31200 + i) * this.w;
				const baseY = this._hash(31300 + i) * Math.max(6, floorBase - 6);
				const rise = baseY - this.tick * this.cfg.rise_speed * (0.55 + this._hash(31400 + i) * 0.7);
				const row = 1 + positiveMod(rise, Math.max(1, floorBase - 5));
				const drift = this.cfg.drift * (0.4 + this._hash(31500 + i) * 0.9) + currentPush * 0.05 * (0.45 + this._hash(31600 + i) * 0.55);
				const wobble = Math.sin(this.tick * 0.03 + i * 0.72) * this.cfg.sway * (0.6 + this._hash(31700 + i) * 0.5);
				const col = positiveMod(baseX + this.tick * drift + wobble, this.w);
				const size = this._hash(31800 + i) > 0.82 ? 2 : 1;
				const alpha = clamp01((0.22 + this._hash(31900 + i) * 0.28) * (0.8 + Math.max(0, burstGain - 1) * 0.14));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), size, size, `rgb(${bubbleColor.r},${bubbleColor.g},${bubbleColor.b})`, alpha);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(col), Math.round(row), 1, 1, `rgb(235,248,246)`, clamp01(alpha * 0.62));
			}
		}

		_renderVolcano(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB(228, 0.26, clamp01(this.cfg.lmin * 0.55));
				const skyMid = hslToRGB(250, 0.22, clamp01(this.cfg.lmin * 0.82));
				const skyLow = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const sky = ctx.createLinearGradient(0, 0, 0, canvasH);
				sky.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				sky.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				sky.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = sky;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const horizon = Math.max(12, Math.min(this.h - 6, Math.floor(this.h * this.cfg.horizon)));
			const centerX = Math.floor(this.w * 0.38);
			const coneWidth = Math.max(20, Math.round(this.cfg.cone_width));
			const coneHeight = Math.max(10, Math.round(this.cfg.cone_height));
			const craterWidth = Math.max(3, Math.round(this.cfg.crater_width));
			const leftBase = Math.max(0, centerX - Math.floor(coneWidth / 2));
			const rightBase = Math.min(this.w - 1, centerX + Math.floor(coneWidth / 2));
			const peakY = Math.max(4, horizon - coneHeight);
			const craterY = peakY + 2;
			const heat = this._heatLevelVolcano();
			const smolderGain = this.timers.smolder > 0 ? (this.values.smolder_gain || this.cfg.smolder_mult) : 1;
			const flareGain = this.timers.flare > 0 ? (this.values.flare_gain || this.cfg.flare_mult) : 1;
			const eruptionGain = this.timers.eruption > 0 ? (this.values.eruption_gain || this.cfg.eruption_mult) : 1;

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 10) * sy), 0, Math.floor((horizon + 8) * sy));
			horizonGlow.addColorStop(0, 'rgba(255, 120, 46, 0)');
			horizonGlow.addColorStop(1, `rgba(255, 120, 46, ${clamp01(0.1 + this.cfg.glow * heat * 0.18)})`);
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 10) * sy), canvasW, Math.ceil(20 * sy));

			const smokeBand = ctx.createLinearGradient(0, 0, 0, Math.floor(horizon * sy));
			smokeBand.addColorStop(0, 'rgba(120, 92, 112, 0)');
			smokeBand.addColorStop(1, `rgba(120, 92, 112, ${clamp01(0.08 + this.cfg.smoke * 0.12 * heat)})`);
			ctx.fillStyle = smokeBand;
			ctx.fillRect(0, 0, canvasW, Math.floor(horizon * sy));

			const groundColor = hslToRGB(26, 0.1, 0.1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, horizon, this.w, this.h - horizon, `rgb(${groundColor.r},${groundColor.g},${groundColor.b})`, 1);

			const mountainColor = hslToRGB(262, 0.12, 0.14);
			const mountainShadow = hslToRGB(266, 0.16, 0.09);
			ctx.fillStyle = `rgb(${mountainColor.r},${mountainColor.g},${mountainColor.b})`;
			ctx.beginPath();
			ctx.moveTo(Math.floor(leftBase * sx), Math.floor(horizon * sy));
			ctx.lineTo(Math.floor((centerX - craterWidth) * sx), Math.floor((peakY + 2) * sy));
			ctx.lineTo(Math.floor(centerX * sx), Math.floor((peakY + 5) * sy));
			ctx.lineTo(Math.floor((centerX + craterWidth) * sx), Math.floor((peakY + 2) * sy));
			ctx.lineTo(Math.floor(rightBase * sx), Math.floor(horizon * sy));
			ctx.closePath();
			ctx.fill();

			ctx.fillStyle = `rgb(${mountainShadow.r},${mountainShadow.g},${mountainShadow.b})`;
			ctx.beginPath();
			ctx.moveTo(Math.floor(centerX * sx), Math.floor((peakY + 4) * sy));
			ctx.lineTo(Math.floor(rightBase * sx), Math.floor(horizon * sy));
			ctx.lineTo(Math.floor((centerX + Math.floor(coneWidth * 0.18)) * sx), Math.floor(horizon * sy));
			ctx.closePath();
			ctx.fill();

			const foregroundRidge = hslToRGB(260, 0.18, 0.08);
			const ridgePoints = [];
			for (let i = 0; i <= 7; i++) {
				ridgePoints.push(Math.round(horizon + 1 + this._hash(32100 + i) * 3 + Math.sin(i * 1.1 + this._hash(32200 + i) * 2) * 1.4));
			}
			ctx.fillStyle = `rgb(${foregroundRidge.r},${foregroundRidge.g},${foregroundRidge.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * 7;
				const idx = Math.min(6, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const row = ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(row * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const glowHue = this.cfg.hue;
			const craterGlow = ctx.createRadialGradient(centerX * sx, craterY * sy, 0, centerX * sx, craterY * sy, Math.max(14, craterWidth * sx * 2.8));
			craterGlow.addColorStop(0, `rgba(255, 190, 96, ${clamp01(0.22 + this.cfg.glow * heat * flareGain * 0.32)})`);
			craterGlow.addColorStop(0.45, `rgba(255, 104, 48, ${clamp01(0.14 + this.cfg.glow * heat * 0.24)})`);
			craterGlow.addColorStop(1, 'rgba(255, 104, 48, 0)');
			ctx.fillStyle = craterGlow;
			ctx.fillRect(Math.floor((centerX - craterWidth * 3) * sx), Math.floor((peakY - 3) * sy), Math.ceil(craterWidth * 6 * sx), Math.ceil(12 * sy));

			const lavaCore = hslToRGB(glowHue, this.cfg.sat, clamp01(this.cfg.lmax * Math.min(1, 0.88 + (flareGain - 1) * 0.2)));
			const lavaLip = hslToRGB((glowHue + this.cfg.hue_sp * 0.4) % 360, clamp01(this.cfg.sat * 0.92), clamp01(this.cfg.lmax * 0.84));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - Math.floor(craterWidth / 2), craterY, craterWidth, 1, `rgb(${lavaCore.r},${lavaCore.g},${lavaCore.b})`, clamp01(0.72 + this.cfg.glow * 0.24 * heat));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - Math.floor(craterWidth / 2) - 1, craterY + 1, craterWidth + 2, 1, `rgb(${lavaLip.r},${lavaLip.g},${lavaLip.b})`, clamp01(0.28 + this.cfg.glow * 0.14 * heat));

			const smokeColor = hslToRGB(266, 0.1, 0.36);
			const smokeCount = Math.max(8, Math.round((4 + this.cfg.smoke * 20) * Math.max(0.5, heat)));
			for (let i = 0; i < smokeCount; i++) {
				const life = 32 + Math.floor(this._hash(32300 + i) * 36);
				const age = positiveMod(this.tick + i * 9, life);
				const progress = age / Math.max(1, life - 1);
				const rise = (6 + this.cfg.smoke * 20 + Math.max(0, smolderGain - 1) * 10) * progress;
				const spread = (this._hash(32400 + i) * 2 - 1) * (1.5 + progress * 4.5) + Math.sin(progress * Math.PI * 2 + i) * this.cfg.smoke * 1.2;
				const x = Math.round(centerX + spread);
				const y = Math.round(craterY - rise);
				const width = 1 + Math.floor(this._hash(32500 + i) * (1 + progress * 3));
				const alpha = clamp01((0.05 + this.cfg.smoke * 0.12 * smolderGain) * (1 - progress) * heat);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, width, 1 + (progress > 0.4 ? 1 : 0), `rgb(${smokeColor.r},${smokeColor.g},${smokeColor.b})`, alpha);
			}

			const emberBase = hslToRGB((glowHue + 6) % 360, clamp01(this.cfg.sat * 0.96), clamp01(this.cfg.lmax * 0.92));
			const emberAccent = hslToRGB((glowHue + 16) % 360, clamp01(this.cfg.sat * 0.84), clamp01(this.cfg.lmax));
			const emberCount = Math.max(8, Math.round((4 + this.cfg.ash * 18) * Math.max(0.55, heat)));
			for (let i = 0; i < emberCount; i++) {
				const life = 24 + Math.floor(this._hash(32600 + i) * 28);
				const age = positiveMod(this.tick + i * 7, life);
				const progress = age / Math.max(1, life - 1);
				const drift = (this._hash(32700 + i) * 2 - 1) * (1 + progress * 4) + Math.sin(this.tick * 0.03 + i) * 0.6;
				const x = Math.round(centerX + drift);
				const y = Math.round(craterY - progress * (3 + this.cfg.ash * 10) + progress * progress * (2 + this.cfg.ash * 6));
				const alpha = clamp01((0.1 + this.cfg.ash * 0.22) * (1 - progress) * heat);
				const color = (i + this.tick) % 3 === 0 ? emberAccent : emberBase;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
			}

			if (this.timers.eruption > 0) {
				const total = Math.max(1, this.values.eruption_total || this.cfg.eruption_dur);
				const progressBase = 1 - this.timers.eruption / total;
				const seed = Math.floor((this.values.eruption_seed || 0) * 10);
				const dir = this.values.eruption_dir || 1;
				const sparkCount = 22 + Math.round(this.cfg.ash * 24 + Math.max(0, eruptionGain - 1) * 14);
				for (let i = 0; i < sparkCount; i++) {
					const launch = this._hash(32800 + seed + i) * 0.22;
					const t = clamp01((progressBase - launch) / (0.46 + this._hash(32900 + seed + i) * 0.28));
					if (t <= 0 || t >= 1) continue;
					const spread = (this._hash(33000 + seed + i) * 2 - 1) * (2.5 + this._hash(33100 + seed + i) * 5.5);
					const height = (7 + this._hash(33200 + seed + i) * 12) * eruptionGain * 0.72;
					const gravity = 6 + this._hash(33300 + seed + i) * 5;
					const x = centerX + spread * t + dir * t * (2 + this._hash(33400 + seed + i) * 4);
					const y = craterY - height * t + gravity * t * t;
					const alpha = clamp01((1 - t) * (0.68 + this._hash(33500 + seed + i) * 0.28));
					const color = this._hash(33600 + seed + i) > 0.35 ? emberAccent : lavaCore;
					const size = this._hash(33700 + seed + i) > 0.72 ? 2 : 1;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(x), Math.round(y), size, size, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}

				const ventFlash = hslToRGB((glowHue + 8) % 360, clamp01(this.cfg.sat * 0.96), clamp01(this.cfg.lmax));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, centerX - Math.floor(craterWidth / 2) - 1, craterY - 1, craterWidth + 2, 2,
					`rgb(${ventFlash.r},${ventFlash.g},${ventFlash.b})`, clamp01(0.22 + (eruptionGain - 1) * 0.18));

				const burstGlow = ctx.createRadialGradient(centerX * sx, (craterY - 2) * sy, 0, centerX * sx, (craterY - 2) * sy, Math.max(24, craterWidth * sx * 2.8));
				burstGlow.addColorStop(0, `rgba(255, 174, 82, ${clamp01(0.26 + (eruptionGain - 1) * 0.22)})`);
				burstGlow.addColorStop(1, 'rgba(255, 174, 82, 0)');
				ctx.fillStyle = burstGlow;
				ctx.fillRect(Math.floor((centerX - craterWidth * 3.4) * sx), Math.floor((craterY - 12) * sy), Math.ceil(craterWidth * 6.8 * sx), Math.ceil(20 * sy));
			}
		}

		_renderTrain(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmin * 0.75));
				const skyMid = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.35) % 360, clamp01(this.cfg.sat * 0.38), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.16));
				const skyLow = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.9) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.3));
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
			const horizon = Math.max(8, Math.min(this.h - 12, Math.floor(this.h * this.cfg.horizon)));
			const railRow = Math.max(horizon + 6, Math.min(this.h - 6, Math.floor(this.h * this.cfg.track_row)));
			const sceneLevel = this._sceneLevelTrain();
			const headlightGain = this.values.headlight_gain || this.cfg.headlight;

			const horizonGlow = ctx.createLinearGradient(0, Math.floor((horizon - 8) * sy), 0, Math.floor((horizon + 8) * sy));
			horizonGlow.addColorStop(0, 'rgba(255, 184, 112, 0)');
			horizonGlow.addColorStop(1, `rgba(255, 184, 112, ${clamp01(0.08 + headlightGain * 0.16 * sceneLevel)})`);
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(0, Math.floor((horizon - 8) * sy), canvasW, Math.ceil(18 * sy));

			const ridgeColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 1.45));
			const ridgePoints = [];
			const ridgeSegments = 6;
			for (let i = 0; i <= ridgeSegments; i++) {
				ridgePoints.push(horizon - 1 - Math.floor(this._hash(33800 + i) * 5) - Math.floor((0.5 + 0.5 * Math.sin(i * 1.15 + this._hash(33900 + i) * 4)) * 3));
			}
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const row = ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(row * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const embankment = hslToRGB((this.cfg.hue + 18) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * 1.15));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, railRow + 2, this.w, this.h - railRow - 2, `rgb(${embankment.r},${embankment.g},${embankment.b})`, 1);

			const ballast = hslToRGB((this.cfg.hue + 24) % 360, clamp01(this.cfg.sat * 0.12), clamp01(this.cfg.lmin * 0.95));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, railRow, this.w, 3, `rgb(${ballast.r},${ballast.g},${ballast.b})`, 1);

			const railColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.25) % 360, clamp01(this.cfg.sat * 0.12), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.34));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, railRow, this.w, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.95);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, railRow + 2, this.w, 1, `rgb(${railColor.r},${railColor.g},${railColor.b})`, 0.75);

			const tieColor = hslToRGB((this.cfg.hue + 22) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * 0.78));
			for (let x = 0; x < this.w; x += 5) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, railRow + 1, 3, 1, `rgb(${tieColor.r},${tieColor.g},${tieColor.b})`, 0.82);
			}

			const poleColor = hslToRGB((this.cfg.hue + 18) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin * 1.18));
			for (let i = 0; i < 5; i++) {
				const col = Math.round((i + 0.8) * this.w / 5 + (this._hash(34000 + i) - 0.5) * 4);
				const poleTop = horizon - 5 - Math.floor(this._hash(34100 + i) * 4);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col, poleTop, 1, railRow - poleTop, `rgb(${poleColor.r},${poleColor.g},${poleColor.b})`, 0.8);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, col - 1, poleTop + 2, 3, 1, `rgb(${poleColor.r},${poleColor.g},${poleColor.b})`, 0.55);
			}

			const activeTimer = this.timers.express > 0 ? this.timers.express : (this.timers.pass > 0 ? this.timers.pass : 0);
			if (activeTimer > 0) {
				const total = Math.max(1, this.values.train_total || activeTimer);
				const progress = clamp01((1 - activeTimer / total) * Math.max(0.35, this.values.train_speed || this.cfg.speed));
				const dir = this.values.train_dir || 1;
				const cars = Math.max(3, Math.round(this.values.train_cars || this.cfg.car_count));
				const gap = Math.max(0.5, this.values.train_gap || this.cfg.car_gap);
				const trainHeight = Math.max(3, Math.round(this.cfg.train_height));
				const carW = Math.max(5, Math.round(trainHeight * 1.7));
				const locoW = carW + 3;
				const totalLen = locoW + cars * (carW + gap) + 6;
				const front = dir > 0
					? -2 + progress * (this.w + totalLen + 4)
					: this.w + 1 - progress * (this.w + totalLen + 4);
				const bodyRow = railRow - 1;
				const bodyTop = bodyRow - trainHeight + 1;
				const bodyColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.15) % 360, clamp01(this.cfg.sat * 0.2), clamp01(this.cfg.lmin * 1.28));
				const accentColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.5) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.22));
				const windowColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp) % 360, clamp01(this.cfg.sat * 0.42), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.7));
				const smokeGain = this.values.smoke_gain || this.cfg.smoke;
				const seed = Math.floor((this.values.train_seed || 0) * 10);
				const segmentStart = (offset, len) => dir > 0 ? Math.round(front - offset - len + 1) : Math.round(front + offset);
				const engineX = segmentStart(0, locoW);

				const headlightX = dir > 0 ? engineX + locoW - 1 : engineX;
				const beamWidth = 18 + Math.max(0, (this.values.express_gain || 1) - 1) * 6;
				const beam = ctx.createLinearGradient(
					Math.floor(headlightX * sx), Math.floor((bodyTop + 1) * sy),
					Math.floor((headlightX + dir * beamWidth) * sx), Math.floor((bodyTop + 1) * sy),
				);
				const beamAlpha = clamp01(0.12 + headlightGain * 0.22 * sceneLevel);
				if (dir > 0) {
					beam.addColorStop(0, `rgba(255, 232, 188, ${beamAlpha})`);
					beam.addColorStop(1, 'rgba(255, 232, 188, 0)');
				} else {
					beam.addColorStop(0, 'rgba(255, 232, 188, 0)');
					beam.addColorStop(1, `rgba(255, 232, 188, ${beamAlpha})`);
				}
				ctx.fillStyle = beam;
				ctx.fillRect(
					Math.floor((Math.min(headlightX, headlightX + dir * beamWidth) - 1) * sx),
					Math.floor((bodyTop - 1) * sy),
					Math.ceil((Math.abs(beamWidth) + 3) * sx),
					Math.ceil((trainHeight + 4) * sy),
				);

				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, engineX, bodyTop, locoW, trainHeight, `rgb(${bodyColor.r},${bodyColor.g},${bodyColor.b})`, 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, engineX + (dir > 0 ? 2 : 1), bodyTop + 1, 3, Math.max(1, trainHeight - 2), `rgb(${accentColor.r},${accentColor.g},${accentColor.b})`, 0.95);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, headlightX, bodyTop + 1, 1, 1, `rgb(${windowColor.r},${windowColor.g},${windowColor.b})`, 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, engineX + Math.floor(locoW * 0.45), bodyTop - 1, 1, 2, `rgb(${bodyColor.r},${bodyColor.g},${bodyColor.b})`, 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, engineX + Math.floor(locoW * 0.45) - 1, bodyTop + 1, 2, 1, `rgb(${windowColor.r},${windowColor.g},${windowColor.b})`, 0.75);

				for (let i = 0; i < cars; i++) {
					const offset = locoW + 1 + i * (carW + gap);
					const carX = segmentStart(offset, carW);
					const lift = (i % 2 === 0) ? 0 : 1;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, carX, bodyTop + lift, carW, trainHeight - lift, `rgb(${bodyColor.r},${bodyColor.g},${bodyColor.b})`, 0.94);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, carX, bodyTop + trainHeight - 1, carW, 1, `rgb(${accentColor.r},${accentColor.g},${accentColor.b})`, 0.8);
					const windowCount = Math.max(2, Math.floor((carW - 1) / 3));
					for (let w = 0; w < windowCount; w++) {
						const wx = carX + 1 + w * 3;
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, wx, bodyTop + 1 + lift, 1, 1, `rgb(${windowColor.r},${windowColor.g},${windowColor.b})`, 0.65);
					}
				}

				const wheelColor = hslToRGB((this.cfg.hue + 18) % 360, clamp01(this.cfg.sat * 0.06), clamp01(this.cfg.lmin * 0.5));
				for (let i = 0; i <= cars; i++) {
					const len = i === 0 ? locoW : carW;
					const offset = i === 0 ? 0 : locoW + 1 + (i - 1) * (carW + gap);
					const start = segmentStart(offset, len);
					for (let x = start + 1; x < start + len - 1; x += 3) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, railRow - 1, 1, 1, `rgb(${wheelColor.r},${wheelColor.g},${wheelColor.b})`, 1);
					}
				}

				const puffCount = Math.max(6, Math.round(4 + smokeGain * 16));
				for (let i = 0; i < puffCount; i++) {
					const life = 26 + Math.floor(this._hash(34200 + seed + i) * 28);
					const age = positiveMod(this.tick + i * 7, life);
					const t = age / Math.max(1, life - 1);
					const drift = dir * t * (4 + (this.values.train_speed || this.cfg.speed) * 2) + (this._hash(34300 + seed + i) * 2 - 1) * (1 + t * 4);
					const rise = t * (4 + smokeGain * 12);
					const x = Math.round(engineX + Math.floor(locoW * 0.45) - drift);
					const y = Math.round(bodyTop - 1 - rise);
					const alpha = clamp01((0.04 + smokeGain * 0.14) * (1 - t) * sceneLevel);
					const width = 1 + Math.floor(this._hash(34400 + seed + i) * (1 + t * 3));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, width, 1 + (t > 0.45 ? 1 : 0), `rgb(${railColor.r},${railColor.g},${railColor.b})`, alpha);
				}
			}
		}

		_renderMysteriousMan(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB(this.cfg.hue, clamp01(this.cfg.sat * 0.55), clamp01(this.cfg.lmin * 0.55));
				const skyMid = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.25) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.08));
				const skyLow = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.75) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.14));
				const bg = ctx.createLinearGradient(0, 0, 0, canvasH);
				bg.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				bg.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				bg.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = bg;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const floorRow = Math.max(10, Math.min(this.h - 6, Math.floor(this.h * 0.84)));
			const sceneLevel = this._sceneLevelMysteriousMan();
			const scale = Math.max(0.85, this.cfg.figure_scale) * 1.18;
			const baseX = Math.round(this.w * this.cfg.figure_x);
			const lean = this.cfg.lean * 6 * scale;
			const headR = Math.max(2, Math.round(3 * scale));
			const bodyH = Math.max(12, Math.round(18 * scale));
			const shoulderW = Math.max(6, Math.round(10 * scale));
			const shoulderY = floorRow - bodyH + Math.round(4 * scale);
			const headCx = baseX + lean * 0.35;
			const headCy = shoulderY - Math.round(2.2 * scale);
			const mouthX = headCx + Math.round(1.2 * scale);
			const mouthY = headCy + Math.round(0.8 * scale);
			const hold = this.cfg.hold_angle;
			const emberX = mouthX + Math.round(Math.cos(hold) * 1.5 * scale);
			const emberY = mouthY - Math.round(Math.sin(hold) * 1.2 * scale);
			const silhouetteColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.1) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 0.85));
			const edgeColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.2) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 1.15));
			const floorColor = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.14), clamp01(this.cfg.lmin * 1.2));
			const contrastAlpha = clamp01((0.58 + this.cfg.contrast * 1.5) * sceneLevel);
			const emberLevel = clamp01((this.cfg.ember + (this.timers.inhale > 0 ? Math.max(0, (this.values.inhale_gain || this.cfg.inhale_mult) - 1) * 0.18 : 0)) * sceneLevel);
			const lighterGain = this.timers['lighter-flick'] > 0 ? (this.values.lighter_gain || this.cfg.lighter_flick_mult) : 1;

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, floorRow, this.w, this.h - floorRow, `rgb(${floorColor.r},${floorColor.g},${floorColor.b})`, 1);

			const doorwayGlow = ctx.createRadialGradient((baseX - 2) * sx, (shoulderY + 2) * sy, 0, (baseX - 2) * sx, (shoulderY + 2) * sy, Math.max(24, 24 * sx));
			doorwayGlow.addColorStop(0, `rgba(255, 176, 108, ${clamp01(0.1 + emberLevel * 0.14 + Math.max(0, lighterGain - 1) * 0.08)})`);
			doorwayGlow.addColorStop(1, 'rgba(255, 176, 108, 0)');
			ctx.fillStyle = doorwayGlow;
			ctx.fillRect(Math.floor((baseX - 20) * sx), Math.floor((shoulderY - 16) * sy), Math.ceil(34 * sx), Math.ceil(34 * sy));

			const doorwayRect = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.12));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, baseX - 5, shoulderY - Math.round(5 * scale), 9, Math.round(12 * scale), `rgb(${doorwayRect.r},${doorwayRect.g},${doorwayRect.b})`, clamp01(0.18 * sceneLevel));

			const wallLight = ctx.createLinearGradient(0, Math.floor((floorRow - 14) * sy), 0, Math.floor(floorRow * sy));
			wallLight.addColorStop(0, 'rgba(255, 176, 108, 0)');
			wallLight.addColorStop(1, `rgba(255, 176, 108, ${clamp01(0.03 + emberLevel * 0.06)})`);
			ctx.fillStyle = wallLight;
			ctx.fillRect(0, Math.floor((floorRow - 14) * sy), canvasW, Math.ceil(14 * sy));

			ctx.fillStyle = `rgb(${silhouetteColor.r},${silhouetteColor.g},${silhouetteColor.b})`;
			ctx.globalAlpha = contrastAlpha;
			ctx.beginPath();
			ctx.moveTo(Math.floor((baseX - shoulderW * 0.9) * sx), Math.floor(floorRow * sy));
			ctx.lineTo(Math.floor((baseX - shoulderW * 0.85) * sx), Math.floor((shoulderY + 4 * scale) * sy));
			ctx.lineTo(Math.floor((baseX - shoulderW * 0.6) * sx), Math.floor((shoulderY + 1) * sy));
			ctx.lineTo(Math.floor((baseX - shoulderW * 0.15 + lean) * sx), Math.floor((shoulderY - 1) * sy));
			ctx.lineTo(Math.floor((baseX + shoulderW * 0.42 + lean * 0.8) * sx), Math.floor((shoulderY + 2) * sy));
			ctx.lineTo(Math.floor((baseX + shoulderW * 0.2) * sx), Math.floor(floorRow * sy));
			ctx.closePath();
			ctx.fill();
			ctx.beginPath();
			ctx.ellipse(headCx * sx, headCy * sy, Math.max(2, headR * sx), Math.max(2, (headR + 1) * sy), 0, 0, Math.PI * 2);
			ctx.fill();
			ctx.beginPath();
			ctx.moveTo(Math.floor((baseX - shoulderW * 0.05 + lean * 0.35) * sx), Math.floor((shoulderY + 1) * sy));
			ctx.lineTo(Math.floor((mouthX - 1.5 * scale) * sx), Math.floor((mouthY + 1.5 * scale) * sy));
			ctx.lineWidth = Math.max(2, Math.round(2 * sx));
			ctx.lineCap = 'round';
			ctx.strokeStyle = `rgb(${silhouetteColor.r},${silhouetteColor.g},${silhouetteColor.b})`;
			ctx.stroke();
			ctx.lineCap = 'butt';
			ctx.lineWidth = 1;
			ctx.globalAlpha = 1;

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, baseX - 1, floorRow - 1, 2, 1, `rgb(${edgeColor.r},${edgeColor.g},${edgeColor.b})`, contrastAlpha * 0.55);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, baseX - Math.round(shoulderW * 0.55), shoulderY + Math.round(1 * scale), Math.max(2, Math.round(2 * scale)), 1, `rgb(${edgeColor.r},${edgeColor.g},${edgeColor.b})`, contrastAlpha * 0.35);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(headCx - headR * 0.4), Math.round(headCy - headR * 0.1), Math.max(1, Math.round(headR * 0.7)), Math.max(1, Math.round(headR * 1.5)), `rgb(${edgeColor.r},${edgeColor.g},${edgeColor.b})`, contrastAlpha * 0.28);

			const emberColor = hslToRGB(18, 0.88, clamp01(this.cfg.lmax));
			const emberCore = hslToRGB(36, 0.8, clamp01(this.cfg.lmax * 0.96));
			const emberGlow = ctx.createRadialGradient(emberX * sx, emberY * sy, 0, emberX * sx, emberY * sy, Math.max(10, 6 * sx));
			emberGlow.addColorStop(0, `rgba(255, 196, 110, ${clamp01(0.18 + emberLevel * 0.34 + Math.max(0, lighterGain - 1) * 0.08)})`);
			emberGlow.addColorStop(1, 'rgba(255, 196, 110, 0)');
			ctx.fillStyle = emberGlow;
			ctx.fillRect(Math.floor((emberX - 4) * sx), Math.floor((emberY - 4) * sy), Math.ceil(8 * sx), Math.ceil(8 * sy));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, emberX, emberY, 1, 1, `rgb(${emberColor.r},${emberColor.g},${emberColor.b})`, clamp01(0.72 + emberLevel * 0.5));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, emberX, emberY, 1, 1, `rgb(${emberCore.r},${emberCore.g},${emberCore.b})`, clamp01(0.35 + emberLevel * 0.25));

			if (this.timers['lighter-flick'] > 0) {
				const flash = ctx.createRadialGradient((mouthX - 2) * sx, (mouthY + 1) * sy, 0, (mouthX - 2) * sx, (mouthY + 1) * sy, Math.max(16, 10 * sx));
				flash.addColorStop(0, `rgba(255, 178, 96, ${clamp01(0.16 + Math.max(0, lighterGain - 1) * 0.16)})`);
				flash.addColorStop(1, 'rgba(255, 178, 96, 0)');
				ctx.fillStyle = flash;
				ctx.fillRect(Math.floor((mouthX - 10) * sx), Math.floor((mouthY - 7) * sy), Math.ceil(20 * sx), Math.ceil(14 * sy));
			}

			const smokeTint = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.45) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.48));
			const smokeCount = Math.max(6, Math.round((4 + this.cfg.smoke * 18) * Math.max(0.45, sceneLevel)));
			const inhaleClamp = this.timers.inhale > 0 ? 0.7 : 1;
			for (let i = 0; i < smokeCount; i++) {
				const life = 28 + Math.floor(this._hash(34500 + i) * 30);
				const age = positiveMod(this.tick + i * 9, life);
				const t = age / Math.max(1, life - 1);
				const lift = t * (4 + this.cfg.rise_speed * 10) * inhaleClamp;
				const drift = (this.cfg.drift * 7 + (this._hash(34600 + i) * 2 - 1) * 2.2) * t;
				const wobble = Math.sin(this.tick * 0.03 + i) * (0.6 + t * 1.6);
				const x = Math.round(emberX + drift + wobble);
				const y = Math.round(emberY - lift);
				const width = 1 + Math.floor(this._hash(34700 + i) * (1 + t * 3));
				const alpha = clamp01((0.07 + this.cfg.smoke * 0.2) * (1 - t) * sceneLevel * inhaleClamp);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, width, 1 + (t > 0.45 ? 1 : 0), `rgb(${smokeTint.r},${smokeTint.g},${smokeTint.b})`, alpha);
			}

			if (this.timers.exhale > 0) {
				const total = Math.max(1, this.values.exhale_total || this.cfg.exhale_dur);
				const progress = 1 - this.timers.exhale / total;
				const plumeGain = this.values.exhale_gain || this.cfg.exhale_mult;
				const dir = this.values.exhale_dir || (this.cfg.drift >= 0 ? 1 : -1);
				const seed = Math.floor((this.values.exhale_seed || 0) * 10);
				const plumeGlow = ctx.createRadialGradient((emberX + dir * 6) * sx, (emberY - 1) * sy, 0, (emberX + dir * 6) * sx, (emberY - 1) * sy, Math.max(14, 9 * sx));
				plumeGlow.addColorStop(0, `rgba(${smokeTint.r},${smokeTint.g},${smokeTint.b}, ${clamp01(0.06 + plumeGain * 0.04)})`);
				plumeGlow.addColorStop(1, `rgba(${smokeTint.r},${smokeTint.g},${smokeTint.b}, 0)`);
				ctx.fillStyle = plumeGlow;
				ctx.fillRect(Math.floor((emberX - 3) * sx), Math.floor((emberY - 8) * sy), Math.ceil(18 * sx), Math.ceil(14 * sy));
				for (let i = 0; i < 18; i++) {
					const launch = this._hash(34800 + seed + i) * 0.2;
					const t = clamp01((progress - launch) / (0.55 + this._hash(34900 + seed + i) * 0.2));
					if (t <= 0 || t >= 1) continue;
					const x = emberX + dir * t * (6 + plumeGain * 6) + Math.sin(i + t * Math.PI * 2) * (1.5 + t * 2.5);
					const y = emberY - t * (1.5 + this.cfg.rise_speed * 3.4) + Math.sin(i * 0.8 + t * 6) * 0.8;
					const alpha = clamp01((1 - t) * (0.2 + this.cfg.smoke * 0.24) * plumeGain);
					const size = 1 + Math.floor(this._hash(35000 + seed + i) * (2 + t * 4));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(x), Math.round(y), size, 1 + (t > 0.4 ? 1 : 0), `rgb(${smokeTint.r},${smokeTint.g},${smokeTint.b})`, alpha);
				}
			}

			if (this.timers['ash-fall'] > 0) {
				const total = Math.max(1, this.values.ash_total || this.cfg.ash_fall_dur);
				const progress = 1 - this.timers['ash-fall'] / total;
				const ashGain = this.values.ash_gain || this.cfg.ash_fall_mult;
				const wobble = Math.sin(progress * Math.PI * 4 + (this.values.ash_seed || 0)) * 0.8;
				const x = Math.round(emberX + wobble);
				const y = Math.round(emberY + progress * (4 + ashGain * 7));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${emberColor.r},${emberColor.g},${emberColor.b})`, clamp01((1 - progress) * 0.8));
			}
		}

		_renderBurningTrees(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 34 + 360) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.45));
				const skyMid = hslToRGB((this.cfg.hue - 16 + 360) % 360, clamp01(this.cfg.sat * 0.14), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.09));
				const skyLow = hslToRGB(22, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const bg = ctx.createLinearGradient(0, 0, 0, canvasH);
				bg.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				bg.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				bg.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = bg;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const treeCount = Math.max(1, Math.round(this.cfg.tree_count));
			const sceneLevel = this._sceneLevelBurningTrees();
			const groundRow = Math.max(10, Math.min(this.h - 6, Math.floor(this.h * this.cfg.horizon)));
			const activeTotal = Math.max(1, this.values.ignite_total || this.cfg.ignite_dur);
			const activeProgress = this.timers.ignite > 0 ? clamp01(1 - this.timers.ignite / activeTotal) : 0;
			const activeCenter = Number.isFinite(this.values.ignite_center) ? this.values.ignite_center : Math.floor(treeCount / 2);
			const activeSpan = Math.max(0.75, this.values.ignite_span || this.cfg.ignite_span);
			const igniteSeed = Math.floor((this.values.ignite_seed || 0) * 10);
			const flareBoost = this.timers.flare > 0 ? (this.values.flare_gain || this.cfg.flare_mult) : 1;
			const lullClamp = this.timers.lull > 0 ? Math.max(0.18, this.values.lull_gain || this.cfg.lull_mult) : 1;
			const focusX = ((activeCenter + 1) / (treeCount + 1)) * this.w;

			const horizonGlow = ctx.createRadialGradient(focusX * sx, Math.floor((groundRow - 4) * sy), 0, focusX * sx, Math.floor((groundRow - 4) * sy), Math.max(42, 32 * sx));
			horizonGlow.addColorStop(0, `rgba(255, 138, 72, ${clamp01(0.08 + this.cfg.flame * 0.34 * sceneLevel)})`);
			horizonGlow.addColorStop(1, 'rgba(255, 138, 72, 0)');
			ctx.fillStyle = horizonGlow;
			ctx.fillRect(Math.floor((focusX - 24) * sx), Math.floor((groundRow - 18) * sy), Math.ceil(48 * sx), Math.ceil(28 * sy));

			const smokeWash = ctx.createLinearGradient(0, Math.floor((groundRow - 20) * sy), 0, Math.floor((groundRow + 2) * sy));
			smokeWash.addColorStop(0, 'rgba(36, 32, 28, 0)');
			smokeWash.addColorStop(1, `rgba(36, 32, 28, ${clamp01(0.16 + this.cfg.smoke * 0.32 * sceneLevel)})`);
			ctx.fillStyle = smokeWash;
			ctx.fillRect(0, Math.floor((groundRow - 20) * sy), canvasW, Math.ceil(24 * sy));

			const ridgeColor = hslToRGB((this.cfg.hue + 18) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.85));
			const ridgePoints = [];
			const ridgeSegments = 7;
			for (let i = 0; i <= ridgeSegments; i++) {
				ridgePoints.push(groundRow - Math.floor(this._hash(35100 + i) * 3) - Math.floor((0.5 + 0.5 * Math.sin(i * 1.1 + this._hash(35200 + i) * 3.2)) * 2));
			}
			ctx.fillStyle = `rgb(${ridgeColor.r},${ridgeColor.g},${ridgeColor.b})`;
			ctx.beginPath();
			ctx.moveTo(0, canvasH);
			for (let x = 0; x < this.w; x++) {
				const pos = (x / Math.max(1, this.w - 1)) * ridgeSegments;
				const idx = Math.min(ridgeSegments - 1, Math.floor(pos));
				const frac = pos - idx;
				const eased = frac * frac * (3 - 2 * frac);
				const row = ridgePoints[idx] + (ridgePoints[idx + 1] - ridgePoints[idx]) * eased;
				ctx.lineTo(Math.floor(x * sx), Math.floor(row * sy));
			}
			ctx.lineTo(canvasW, canvasH);
			ctx.closePath();
			ctx.fill();

			const fieldColor = hslToRGB((this.cfg.hue + 6) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmin * 1.05));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, groundRow, this.w, this.h - groundRow, `rgb(${fieldColor.r},${fieldColor.g},${fieldColor.b})`, 1);

			const healthyLeaf = hslToRGB((this.cfg.hue - 6 + 360) % 360, clamp01(this.cfg.sat * 0.5), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.24));
			const healthyShade = hslToRGB((this.cfg.hue - 14 + 360) % 360, clamp01(this.cfg.sat * 0.38), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.14));
			const trunkColor = hslToRGB((this.cfg.hue + 26) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.72));
			const charColor = hslToRGB(18, 0.08, clamp01(this.cfg.lmin * 0.52));
			const emberColor = hslToRGB(18, 0.88, clamp01(this.cfg.lmax * 0.96));
			const emberCore = hslToRGB(34, 0.8, clamp01(this.cfg.lmax));
			const smokeTint = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.5) % 360, clamp01(this.cfg.sat * 0.12), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.48));
			const ashTint = hslToRGB((this.cfg.hue + 26) % 360, clamp01(this.cfg.sat * 0.06), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.62));
			const spacing = this.w / (treeCount + 1);
			const sources = [];

			for (let i = 0; i < treeCount; i++) {
				const x = Math.round((i + 1) * spacing + (this._hash(35300 + i) - 0.5) * spacing * 0.26);
				const baseY = groundRow - 1 - Math.floor(this._hash(35400 + i) * 2);
				const treeH = Math.max(8, Math.round(this.cfg.tree_height * (0.78 + this._hash(35500 + i) * 0.42)));
				const trunkH = Math.max(3, Math.round(treeH * (0.28 + this._hash(35600 + i) * 0.08)));
				const char = clamp01(this.values[`char_${i}`] || 0);
				let heat = 0;
				if (this.timers.ignite > 0) {
					const dist = Math.abs(i - activeCenter);
					let influence = 1 - dist / Math.max(1, activeSpan + this.cfg.spread * (0.15 + activeProgress * 0.55) + 0.75);
					if (influence > 0) {
						influence = Math.pow(influence, 0.72);
						const fireBase = 0.45 + (this.values.ignite_gain || this.cfg.flame) * 2.2;
						heat = clamp01((0.22 + influence * 0.78) * fireBase * (0.72 + 0.28 * Math.sin(activeProgress * Math.PI)) * flareBoost * lullClamp);
					}
				}
				const crownScale = 1 - char * 0.55;
				const crownH = Math.max(3, Math.round(treeH * (0.5 + this.cfg.canopy * 0.45) * crownScale));
				const crownHalf = Math.max(2, Math.round((1.5 + this.cfg.canopy * 3.2) * (treeH / 10) * (1 - char * 0.38)));
				const shadowWidth = Math.max(2, crownHalf * 2 + 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x - Math.floor(shadowWidth / 2), baseY + 1, shadowWidth, 1, `rgb(${charColor.r},${charColor.g},${charColor.b})`, clamp01(0.14 + char * 0.18));

				const trunkDrawH = Math.max(2, Math.round(trunkH * (1 - Math.min(0.7, char * 0.45))));
				for (let row = 0; row < trunkDrawH; row++) {
					const y = baseY - row;
					const color = char > 0.45 ? charColor : trunkColor;
					const alpha = clamp01(0.72 + char * 0.18);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1 + (row < trunkDrawH * 0.22 ? 1 : 0), 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}

				for (let row = 0; row < crownH; row++) {
					const progress = row / Math.max(1, crownH - 1);
					const width = Math.max(1, Math.round(crownHalf * (1 - progress * 0.72))) + (row < crownH * 0.3 ? 1 : 0);
					const y = baseY - trunkDrawH - crownH + row + 1;
					for (let dx = -width; dx <= width; dx++) {
						const edge = 1 - Math.abs(dx) / Math.max(1, width);
						const burnMask = clamp01(char * 0.78 + heat * (0.1 + edge * 0.42) * (1 - progress * 0.42));
						if (burnMask > 0.88 && ((dx + row + i) % 3 === 0)) continue;
						const baseColor = burnMask > 0.48
							? charColor
							: (row < crownH * 0.45 ? healthyLeaf : healthyShade);
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + dx, y, 1, 1, `rgb(${baseColor.r},${baseColor.g},${baseColor.b})`, clamp01(0.7 + edge * 0.22));
						const flameHere = heat > 0.08 && row >= crownH * 0.14 && edge > 0.22 && ((dx + row + this.tick + i) % 2 === 0);
						if (flameHere) {
							const flameAlpha = clamp01((0.18 + this.cfg.flame * 0.42) * heat * sceneLevel * (0.9 - progress * 0.3));
							const flameColor = row < crownH * 0.36 ? emberCore : emberColor;
							this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + dx, y, 1, 1, `rgb(${flameColor.r},${flameColor.g},${flameColor.b})`, flameAlpha);
							if (flameAlpha > 0.16) {
								this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + dx, y - 1, 1, 1, `rgb(${emberCore.r},${emberCore.g},${emberCore.b})`, flameAlpha * 0.65);
							}
							if (progress > 0.18 && this._hash(35700 + i * 23 + row * 7 + dx + igniteSeed) > 0.5) {
								this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + dx, y - 1, 1, 1, `rgb(${emberCore.r},${emberCore.g},${emberCore.b})`, flameAlpha * 0.55);
							}
						}
					}
				}

				if (heat > 0.08) {
					const glow = ctx.createRadialGradient(x * sx, (baseY - crownH * 0.35) * sy, 0, x * sx, (baseY - crownH * 0.35) * sy, Math.max(16, 9 * sx));
					glow.addColorStop(0, `rgba(255, 152, 74, ${clamp01(0.08 + heat * 0.22)})`);
					glow.addColorStop(1, 'rgba(255, 152, 74, 0)');
					ctx.fillStyle = glow;
					ctx.fillRect(Math.floor((x - 6) * sx), Math.floor((baseY - crownH - 4) * sy), Math.ceil(12 * sx), Math.ceil((crownH + 8) * sy));

					const tongues = 3 + Math.round(heat * 4);
					for (let tongue = 0; tongue < tongues; tongue++) {
						const offset = -1 + tongue - Math.floor(tongues / 2);
						const flicker = this._hash(36500 + i * 17 + tongue * 5 + igniteSeed);
						const flameH = 1 + Math.floor(flicker * (2 + heat * 3));
						const flameY = baseY - Math.floor(1 + flicker * (2 + heat * 3));
						const flameColor = tongue % 2 === 0 ? emberCore : emberColor;
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + offset, flameY - flameH + 1, 1, flameH, `rgb(${flameColor.r},${flameColor.g},${flameColor.b})`, clamp01(0.3 + heat * 0.5));
					}
				}

				if (char > 0.58 || this.timers.ending > 0) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x - 1, baseY - Math.min(2, trunkDrawH - 1), 3, 1, `rgb(${charColor.r},${charColor.g},${charColor.b})`, clamp01(0.72 + char * 0.16));
				}

				sources.push({
					x,
					y: baseY - trunkDrawH - Math.max(2, Math.round(crownH * 0.42)),
					heat,
					char,
				});
			}

			for (const source of sources) {
				const sourceStrength = Math.max(source.heat, source.char * 0.65);
				if (sourceStrength <= 0.04) continue;
				const puffCount = Math.max(2, Math.round((2 + this.cfg.smoke * 8) * (0.45 + sourceStrength * 1.4)));
				for (let i = 0; i < puffCount; i++) {
					const life = 34 + Math.floor(this._hash(35800 + source.x * 11 + i) * 34);
					const age = positiveMod(this.tick + i * 9 + Math.floor(source.x * 2), life);
					const t = age / Math.max(1, life - 1);
					const drift = (this._hash(35900 + source.x * 13 + i) * 2 - 1) * (0.8 + t * 4.5);
					const lift = t * (5 + this.cfg.smoke * 16 + sourceStrength * 7);
					const x = Math.round(source.x + drift + Math.sin(this.tick * 0.03 + i + source.x * 0.08) * (0.4 + t * 1.8));
					const y = Math.round(source.y - lift);
					const alpha = clamp01((0.03 + this.cfg.smoke * 0.18) * (1 - t) * (0.55 + sourceStrength) * sceneLevel);
					const size = 1 + Math.floor(this._hash(36000 + source.x * 17 + i) * (1 + t * 3));
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, size, 1 + (t > 0.45 ? 1 : 0), `rgb(${smokeTint.r},${smokeTint.g},${smokeTint.b})`, alpha);
				}
			}

			for (const source of sources) {
				if (source.heat <= 0.06) continue;
				const emberCount = Math.max(2, Math.round(this.cfg.embers * 18 + source.heat * 6));
				for (let i = 0; i < emberCount; i++) {
					const life = 24 + Math.floor(this._hash(36100 + source.x * 19 + i) * 26);
					const age = positiveMod(this.tick + i * 7 + igniteSeed, life);
					const t = age / Math.max(1, life - 1);
					const x = Math.round(source.x + (this._hash(36200 + source.x * 23 + i) * 2 - 1) * (1 + t * 4));
					const y = Math.round(source.y - t * (3 + source.heat * 8));
					const alpha = clamp01((0.08 + this.cfg.embers * 0.34) * (1 - t) * source.heat);
					const color = t < 0.35 ? emberCore : emberColor;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			if (this.timers.ending > 0 || this.timers.lull > 0) {
				const ashCount = Math.max(8, Math.round(this.w * (0.03 + this.cfg.ending_ash * 0.08)));
				for (let i = 0; i < ashCount; i++) {
					const x = Math.round(this._hash(36300 + i) * this.w);
					const drop = positiveMod(this.tick * (0.18 + this._hash(36400 + i) * 0.22) + i * 2.3, Math.max(8, groundRow - 8));
					const y = Math.round(Math.max(4, groundRow - 14 + drop));
					const alpha = clamp01(0.03 + this.cfg.ending_ash * 0.18);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${ashTint.r},${ashTint.g},${ashTint.b})`, alpha);
				}
			}
		}

		_renderSand(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 18 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.45));
				const skyMid = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.12), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.12));
				const skyLow = hslToRGB(this.cfg.hue % 360, clamp01(this.cfg.sat * 0.2), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.2));
				const bg = ctx.createLinearGradient(0, 0, 0, canvasH);
				bg.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				bg.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				bg.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = bg;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const floorRow = Math.max(10, Math.min(this.h - 5, Math.floor(this.h * 0.82)));
			const pipeX = Math.round(this.w * this.cfg.pipe_x);
			const pipeW = Math.max(4, Math.round(this.cfg.pipe_width));
			const outletY = Math.max(4, floorRow - Math.round(this.cfg.pipe_drop));
			let basinCenter = Math.round(this.w * this.cfg.container_x);
			const maxPipeGap = Math.round(this.w * 0.22);
			if (Math.abs(basinCenter - pipeX) > maxPipeGap) {
				basinCenter = pipeX + (basinCenter >= pipeX ? maxPipeGap : -maxPipeGap);
			}
			const basinW = Math.max(18, Math.round(this.cfg.container_width));
			const basinD = Math.max(6, Math.round(this.cfg.container_depth));
			const sceneRightLimit = Math.max(basinW + 8, Math.floor(this.w * 0.72));
			const left = Math.max(4, Math.min(Math.min(this.w - basinW - 4, sceneRightLimit - basinW), basinCenter - Math.floor(basinW / 2)));
			const right = left + basinW - 1;
			const basinFloor = floorRow - 1;
			const basinTop = basinFloor - basinD;
			const fill = clamp01(this.values.fill_level || 0);
			const spill = clamp01(this.values.spill_level || 0);
			const surfaceBias = Math.max(-1, Math.min(1, this.values.surface_bias || 0));
			const flow = this._sandFlowLevel();
			const motion = clamp01(flow * 2.1 + spill * 0.9 + (this.timers.surge > 0 ? 0.2 : 0));
			const sandBase = hslToRGB(this.cfg.hue % 360, clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.5));
			const sandShade = hslToRGB((this.cfg.hue - 4 + 360) % 360, clamp01(this.cfg.sat * 0.44), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
			const sandHi = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.4) % 360, clamp01(this.cfg.sat * 0.34), clamp01(this.cfg.lmax * 0.98));
			const metalColor = hslToRGB((this.cfg.hue + 14) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin * 0.9));
			const basinColor = hslToRGB((this.cfg.hue + 18) % 360, clamp01(this.cfg.sat * 0.14), clamp01(this.cfg.lmin * 1.15));
			const rimColor = hslToRGB((this.cfg.hue + 24) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.42));
			const basinInterior = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin * 0.58));
			const floorColor = hslToRGB((this.cfg.hue - 6 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin * 0.72));

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, floorRow, this.w, this.h - floorRow, `rgb(${floorColor.r},${floorColor.g},${floorColor.b})`, 1);

			const glow = ctx.createRadialGradient(pipeX * sx, outletY * sy, 0, pipeX * sx, outletY * sy, Math.max(18, 12 * sx));
			glow.addColorStop(0, `rgba(255, 214, 140, ${clamp01(0.04 + motion * 0.08)})`);
			glow.addColorStop(1, 'rgba(255, 214, 140, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(Math.floor((pipeX - 8) * sx), Math.floor((outletY - 5) * sy), Math.ceil(16 * sx), Math.ceil(12 * sy));

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - pipeW - 2, outletY - 2, pipeW + 2, 3, `rgb(${metalColor.r},${metalColor.g},${metalColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - 2, outletY - 2, 3, 4, `rgb(${metalColor.r},${metalColor.g},${metalColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - pipeW - 1, outletY - 1, pipeW, 1, `rgb(${sandHi.r},${sandHi.g},${sandHi.b})`, 0.24);

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left, basinTop + 1, basinW, basinD, `rgb(${basinInterior.r},${basinInterior.g},${basinInterior.b})`, 0.35);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 1, basinTop, 2, basinD + 2, `rgb(${basinColor.r},${basinColor.g},${basinColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, right, basinTop, 2, basinD + 2, `rgb(${basinColor.r},${basinColor.g},${basinColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 1, basinFloor, basinW + 2, 2, `rgb(${basinColor.r},${basinColor.g},${basinColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 1, basinTop, 4, 1, `rgb(${rimColor.r},${rimColor.g},${rimColor.b})`, 0.82);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, right - 2, basinTop, 4, 1, `rgb(${rimColor.r},${rimColor.g},${rimColor.b})`, 0.82);

			const surfaceRows = [];
			for (let x = left + 1; x < right; x++) {
				const nx = (x - (left + 1)) / Math.max(1, basinW - 3);
				const peak = 0.5 + surfaceBias * 0.28;
				const mound = clamp01(1 - Math.abs((nx - peak) / 0.52));
				const localFill = fill * (0.58 + mound * (0.42 + this.cfg.settle * 0.24));
				const height = Math.max(0, Math.round(localFill * (basinD - 1)));
				const surfaceY = basinFloor - height;
				surfaceRows.push(surfaceY);
				for (let y = surfaceY; y < basinFloor; y++) {
					const edge = (y - surfaceY) / Math.max(1, basinFloor - surfaceY);
					const color = edge < 0.18 ? sandHi : (edge < 0.5 ? sandBase : sandShade);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, 0.96);
				}
			}

			const impactX = Math.max(left + 2, Math.min(right - 2, pipeX + Math.round(surfaceBias * basinW * 0.18)));
			const impactIdx = Math.max(0, Math.min(surfaceRows.length - 1, impactX - (left + 1)));
			const impactY = surfaceRows[impactIdx] || basinFloor - 1;

			if (spill > 0.01) {
				const dir = surfaceBias < 0 ? -1 : 1;
				const spillLen = Math.max(3, Math.round(spill * (10 + this.cfg.overflow * 22)));
				for (let s = 0; s < spillLen; s++) {
					const height = Math.max(1, Math.round((1 - s / Math.max(1, spillLen - 1)) * (2 + spill * 4)));
					const x = dir > 0 ? right + s : left - s;
					const y = basinFloor - height + 1 + Math.round(s * 0.08);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, height, `rgb(${sandBase.r},${sandBase.g},${sandBase.b})`, 0.92);
					if (height > 1) this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${sandHi.r},${sandHi.g},${sandHi.b})`, 0.4);
				}
			}

			if (flow > 0.01) {
				const streamW = Math.max(1, Math.round(1 + this.cfg.spread * 1.8 + flow * 2));
				for (let y = outletY + 2; y <= impactY; y++) {
					const t = (y - outletY) / Math.max(1, impactY - outletY);
					const drift = Math.sin(this.tick * 0.04 + y * 0.12) * this.cfg.spread * 0.35 * t;
					const centerX = pipeX + drift;
					for (let dx = -Math.floor(streamW / 2); dx <= Math.floor(streamW / 2); dx++) {
						if ((dx + y + this.tick) % 2 !== 0 && Math.abs(dx) > 0) continue;
						const alpha = clamp01(0.24 + flow * 0.95 - Math.abs(dx) * 0.09);
						const color = (y + dx) % 3 === 0 ? sandHi : sandBase;
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(centerX + dx), y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					}
				}

				const splashCount = Math.max(4, Math.round(4 + flow * 14));
				for (let i = 0; i < splashCount; i++) {
					const lift = this._hash(36600 + i) * (1.5 + motion * 4);
					const drift = (this._hash(36700 + i) * 2 - 1) * (1 + this.cfg.spread * 2.4);
					const x = Math.round(impactX + drift);
					const y = Math.round(impactY - lift);
					const alpha = clamp01(0.08 + motion * 0.26);
					const color = i % 3 === 0 ? sandHi : sandBase;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			const dustCount = Math.max(8, Math.round((4 + this.cfg.spread * 10) * Math.max(0.2, motion)));
			for (let i = 0; i < dustCount; i++) {
				const life = 26 + Math.floor(this._hash(36800 + i) * 22);
				const age = positiveMod(this.tick + i * 5, life);
				const t = age / Math.max(1, life - 1);
				const x = Math.round(impactX + (this._hash(36900 + i) * 2 - 1) * (2 + t * 7));
				const y = Math.round(impactY - t * (1 + this.cfg.spread * 2.5));
				const alpha = clamp01((0.05 + motion * 0.12) * (1 - t));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${sandShade.r},${sandShade.g},${sandShade.b})`, alpha);
			}
		}

		_renderWaterPipe(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 24 + 360) % 360, clamp01(this.cfg.sat * 0.28), clamp01(this.cfg.lmin * 0.42));
				const skyMid = hslToRGB((this.cfg.hue - 10 + 360) % 360, clamp01(this.cfg.sat * 0.2), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.08));
				const skyLow = hslToRGB(this.cfg.hue % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				const bg = ctx.createLinearGradient(0, 0, 0, canvasH);
				bg.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				bg.addColorStop(0.65, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				bg.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = bg;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const floorRow = Math.max(10, Math.min(this.h - 5, Math.floor(this.h * 0.82)));
			const pipeX = Math.round(this.w * this.cfg.pipe_x);
			const pipeW = Math.max(4, Math.round(this.cfg.pipe_width));
			const outletY = Math.max(4, floorRow - Math.round(this.cfg.pipe_drop));
			let basinCenter = Math.round(this.w * this.cfg.basin_x);
			const maxPipeGap = Math.round(this.w * 0.22);
			if (Math.abs(basinCenter - pipeX) > maxPipeGap) {
				basinCenter = pipeX + (basinCenter >= pipeX ? maxPipeGap : -maxPipeGap);
			}
			const basinW = Math.max(18, Math.round(this.cfg.basin_width));
			const basinD = Math.max(6, Math.round(this.cfg.basin_depth));
			const sceneRightLimit = Math.max(basinW + 8, Math.floor(this.w * 0.74));
			const left = Math.max(4, Math.min(Math.min(this.w - basinW - 4, sceneRightLimit - basinW), basinCenter - Math.floor(basinW / 2)));
			const right = left + basinW - 1;
			const basinFloor = floorRow - 1;
			const basinTop = basinFloor - basinD;
			const fill = clamp01(this.values.fill_level || 0);
			const spill = clamp01(this.values.spill_level || 0);
			const surfaceBias = Math.max(-1, Math.min(1, this.values.surface_bias || 0));
			const ripplePhase = this.values.ripple_phase || 0;
			const flow = this._waterPipeFlowLevel();
			const motion = clamp01(flow * 2.2 + spill * 1.1 + (this.timers.surge > 0 ? 0.18 : 0));
			const wallColor = hslToRGB((this.cfg.hue + 22) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin * 0.62));
			const floorColor = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin * 0.78));
			const seamColor = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin * 1.12));
			const pipeColor = hslToRGB((this.cfg.hue + 16) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin * 0.96));
			const pipeEdge = hslToRGB((this.cfg.hue + 24) % 360, clamp01(this.cfg.sat * 0.06), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.38));
			const basinColor = hslToRGB((this.cfg.hue + 14) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin * 0.86));
			const basinInterior = hslToRGB((this.cfg.hue + 8) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin * 0.54));
			const rimColor = hslToRGB((this.cfg.hue + 28) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.42));
			const waterDeep = hslToRGB((this.cfg.hue - this.cfg.hue_sp * 0.35 + 360) % 360, clamp01(this.cfg.sat * 0.84), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
			const waterMid = hslToRGB(this.cfg.hue % 360, clamp01(this.cfg.sat * 0.78), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.42));
			const waterHi = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.24) % 360, clamp01(this.cfg.sat * 0.62), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.74));
			const foamColor = hslToRGB((this.cfg.hue + this.cfg.hue_sp * 0.5) % 360, clamp01(this.cfg.sat * 0.18), clamp01(Math.min(1, this.cfg.lmax * 0.99)));

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, 0, this.w, floorRow, `rgb(${wallColor.r},${wallColor.g},${wallColor.b})`, 0.18);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, floorRow, this.w, this.h - floorRow, `rgb(${floorColor.r},${floorColor.g},${floorColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, floorRow, this.w, 1, `rgb(${seamColor.r},${seamColor.g},${seamColor.b})`, 0.42);

			const glow = ctx.createRadialGradient(pipeX * sx, outletY * sy, 0, pipeX * sx, outletY * sy, Math.max(22, 16 * sx));
			glow.addColorStop(0, `rgba(${waterHi.r},${waterHi.g},${waterHi.b}, ${clamp01(0.12 + motion * 0.14)})`);
			glow.addColorStop(1, `rgba(${waterHi.r},${waterHi.g},${waterHi.b}, 0)`);
			ctx.fillStyle = glow;
			ctx.fillRect(Math.floor((pipeX - 10) * sx), Math.floor((outletY - 7) * sy), Math.ceil(20 * sx), Math.ceil(15 * sy));

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - pipeW - 3, outletY - 3, pipeW + 3, 4, `rgb(${pipeColor.r},${pipeColor.g},${pipeColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - 2, outletY - 3, 3, 5, `rgb(${pipeColor.r},${pipeColor.g},${pipeColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - pipeW - 2, outletY - 2, pipeW + 1, 1, `rgb(${pipeEdge.r},${pipeEdge.g},${pipeEdge.b})`, 0.72);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, pipeX - 1, outletY - 2, 1, 4, `rgb(${pipeEdge.r},${pipeEdge.g},${pipeEdge.b})`, 0.72);

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left, basinTop + 1, basinW, basinD, `rgb(${basinInterior.r},${basinInterior.g},${basinInterior.b})`, 0.5);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 1, basinTop, 2, basinD + 2, `rgb(${basinColor.r},${basinColor.g},${basinColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, right, basinTop, 2, basinD + 2, `rgb(${basinColor.r},${basinColor.g},${basinColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 1, basinFloor, basinW + 2, 2, `rgb(${basinColor.r},${basinColor.g},${basinColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 1, basinTop, basinW + 2, 1, `rgb(${rimColor.r},${rimColor.g},${rimColor.b})`, 0.8);

			const surfaceRows = [];
			const peak = 0.5 + surfaceBias * 0.18;
			const rippleAmp = (0.15 + motion * 0.45 + this.cfg.ripple * 0.3) * Math.max(0.2, fill);
			for (let x = left + 1; x < right; x++) {
				const nx = (x - (left + 1)) / Math.max(1, basinW - 3);
				const bowl = clamp01(1 - Math.abs((nx - peak) / 0.58));
				const wave = Math.sin(ripplePhase + nx * Math.PI * (2.8 + this.cfg.ripple * 1.1)) * rippleAmp;
				const localFill = fill * (0.88 + bowl * 0.16);
				const height = Math.max(0, Math.round(localFill * (basinD - 1) + wave));
				const surfaceY = basinFloor - height;
				surfaceRows.push(surfaceY);
				for (let y = surfaceY; y < basinFloor; y++) {
					const depth = (y - surfaceY) / Math.max(1, basinFloor - surfaceY);
					const color = depth < 0.18 ? waterHi : (depth < 0.48 ? waterMid : waterDeep);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, 0.94);
				}
				if (height > 0) {
					const shimmer = Math.max(0, Math.sin(ripplePhase * 0.7 + nx * Math.PI * 4.5));
					const alpha = clamp01(0.12 + this.cfg.foam * 0.16 + shimmer * 0.12);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, surfaceY, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, alpha);
				}
			}

			const impactX = Math.max(left + 2, Math.min(right - 2, pipeX + Math.round(surfaceBias * basinW * 0.18)));
			const impactIdx = Math.max(0, Math.min(surfaceRows.length - 1, impactX - (left + 1)));
			const impactY = surfaceRows[impactIdx] || basinFloor - 1;

			if (spill > 0.01) {
				const dir = surfaceBias < 0 ? -1 : 1;
				const spillLen = Math.max(4, Math.round(spill * (10 + this.cfg.overflow * 22)));
				for (let s = 0; s < spillLen; s++) {
					const taper = 1 - s / Math.max(1, spillLen - 1);
					const height = Math.max(1, Math.round(1 + taper * (1.5 + spill * 3)));
					const x = dir > 0 ? right + s : left - s;
					const y = basinFloor - height + 1 + Math.round(s * 0.06);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, height, `rgb(${waterMid.r},${waterMid.g},${waterMid.b})`, 0.9);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, clamp01(0.16 + taper * this.cfg.foam * 0.3));
				}
				const puddleLen = Math.max(3, Math.round(spill * (6 + this.cfg.overflow * 12)));
				const puddleStart = dir > 0 ? right + spillLen - 1 : left - spillLen + 1;
				for (let i = 0; i < puddleLen; i++) {
					const x = dir > 0 ? puddleStart + i : puddleStart - i;
					const alpha = clamp01(0.16 + spill * 0.22 - i * 0.012);
					if (alpha <= 0.02) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, basinFloor, 1, 1, `rgb(${waterDeep.r},${waterDeep.g},${waterDeep.b})`, alpha);
					if ((i + this.tick) % 3 === 0) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, basinFloor, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, alpha * 0.42);
					}
				}
			}

			if (flow > 0.01) {
				const streamW = Math.max(1, Math.round(1 + this.cfg.stream * 1.2 + flow * 1.6));
				for (let y = outletY + 2; y <= impactY; y++) {
					const t = (y - outletY) / Math.max(1, impactY - outletY);
					const sway = Math.sin(this.tick * 0.05 + y * 0.16) * this.cfg.stream * 0.12;
					const centerX = pipeX + surfaceBias * t * basinW * 0.06 + sway;
					for (let dx = -Math.floor(streamW / 2); dx <= Math.floor(streamW / 2); dx++) {
						const edge = Math.abs(dx) / Math.max(1, streamW / 2);
						const alpha = clamp01(0.18 + flow * 0.9 - edge * 0.24);
						if (alpha <= 0.02) continue;
						const color = edge < 0.35 ? waterHi : waterMid;
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(centerX + dx), y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
					}
				}

				const splashCount = Math.max(5, Math.round(5 + flow * 16 + this.cfg.foam * 10));
				for (let i = 0; i < splashCount; i++) {
					const lift = this._hash(37000 + i) * (1.5 + motion * 4.4);
					const drift = (this._hash(37100 + i) * 2 - 1) * (1 + this.cfg.stream * 1.8);
					const x = Math.round(impactX + drift);
					const y = Math.round(impactY - lift);
					const alpha = clamp01(0.08 + motion * 0.22 + this.cfg.foam * 0.14);
					const color = i % 2 === 0 ? foamColor : waterHi;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, 1, `rgb(${color.r},${color.g},${color.b})`, alpha);
				}
			}

			if (fill > 0.02) {
				for (let x = left + 2; x < right - 1; x++) {
					const idx = x - (left + 1);
					const surfaceY = surfaceRows[Math.max(0, Math.min(surfaceRows.length - 1, idx))];
					if (surfaceY >= basinFloor) continue;
					const nx = (x - left) / Math.max(1, basinW);
					const wave = Math.sin(ripplePhase * 1.1 + nx * Math.PI * (5 + this.cfg.ripple * 2) + (x - impactX) * 0.15);
					const alpha = clamp01((0.05 + this.cfg.foam * 0.1 + motion * 0.08) * Math.max(0, wave));
					if (alpha <= 0.02) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, surfaceY, 1, 1, `rgb(${foamColor.r},${foamColor.g},${foamColor.b})`, alpha);
				}
			}
		}

		_renderTetris(ctx, canvasW, canvasH, opts) {
			opts = opts || {};
			if (opts.transparent) {
				ctx.clearRect(0, 0, canvasW, canvasH);
			} else {
				const skyTop = hslToRGB((this.cfg.hue - 24 + 360) % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin * 0.35));
				const skyMid = hslToRGB((this.cfg.hue - 8 + 360) % 360, clamp01(this.cfg.sat * 0.18), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.06));
				const skyLow = hslToRGB(this.cfg.hue % 360, clamp01(this.cfg.sat * 0.24), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.14));
				const bg = ctx.createLinearGradient(0, 0, 0, canvasH);
				bg.addColorStop(0, `rgb(${skyTop.r},${skyTop.g},${skyTop.b})`);
				bg.addColorStop(0.62, `rgb(${skyMid.r},${skyMid.g},${skyMid.b})`);
				bg.addColorStop(1, `rgb(${skyLow.r},${skyLow.g},${skyLow.b})`);
				ctx.fillStyle = bg;
				ctx.fillRect(0, 0, canvasW, canvasH);
			}

			const sx = canvasW / this.w;
			const sy = canvasH / this.h;
			const ceilSx = Math.ceil(sx);
			const ceilSy = Math.ceil(sy);
			const cell = Math.max(2, Math.round(this.cfg.cell_size));
			const boardW = TETRIS_COLS * cell;
			const boardH = TETRIS_ROWS * cell;
			const left = Math.max(4, Math.min(this.w - boardW - 6, Math.round(this.w * this.cfg.well_x)));
			const top = Math.max(4, Math.min(this.h - boardH - 6, Math.round(this.h * this.cfg.well_y)));
			const right = left + boardW;
			const bottom = top + boardH;
			const fillRatio = clamp01(this.values.fill_ratio || 0);
			const stackHeight = clamp01((this.values.stack_height || 0) / TETRIS_ROWS);
			let sceneLevel = 1;
			if (this.timers.intro > 0) {
				const total = this.values.intro_total || this.cfg.intro_dur;
				const progress = this._phaseProgress(total, this.timers.intro);
				sceneLevel *= 0.35 + 0.65 * progress;
			}
			if (this.timers.ending > 0) {
				const total = this.values.ending_total || (this.cfg.ending_dur + this.cfg.ending_linger);
				const progress = this._phaseProgress(total, this.timers.ending);
				sceneLevel *= 1 - 0.55 * progress;
			}

			const floorColor = hslToRGB((this.cfg.hue + 10) % 360, clamp01(this.cfg.sat * 0.14), clamp01(this.cfg.lmin * 0.72));
			const frameColor = hslToRGB((this.cfg.hue + 14) % 360, clamp01(this.cfg.sat * 0.16), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.26));
			const innerColor = hslToRGB((this.cfg.hue + 4) % 360, clamp01(this.cfg.sat * 0.1), clamp01(this.cfg.lmin * 0.42));
			const lineColor = hslToRGB((this.cfg.hue + 20) % 360, clamp01(this.cfg.sat * 0.08), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
			const capColor = hslToRGB((this.cfg.hue + 34) % 360, clamp01(this.cfg.sat * 0.22), clamp01(this.cfg.lmax * 0.94));

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, 0, bottom - 2, this.w, this.h - bottom + 2, `rgb(${floorColor.r},${floorColor.g},${floorColor.b})`, 1);

			const glow = ctx.createRadialGradient((left + boardW * 0.5) * sx, (top + boardH * 0.58) * sy, 0, (left + boardW * 0.5) * sx, (top + boardH * 0.58) * sy, Math.max(30, boardW * sx * 0.8));
			glow.addColorStop(0, `rgba(110, 196, 255, ${clamp01((this.cfg.glow * 0.65 + fillRatio * 0.14) * sceneLevel)})`);
			glow.addColorStop(1, 'rgba(110, 196, 255, 0)');
			ctx.fillStyle = glow;
			ctx.fillRect(Math.floor((left - 8) * sx), Math.floor((top - 6) * sy), Math.ceil((boardW + 16) * sx), Math.ceil((boardH + 12) * sy));

			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left - 2, top - 2, boardW + 4, boardH + 4, `rgb(${frameColor.r},${frameColor.g},${frameColor.b})`, 0.96);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left, top, boardW, boardH, `rgb(${innerColor.r},${innerColor.g},${innerColor.b})`, 1);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left, top - 1, boardW, 1, `rgb(${capColor.r},${capColor.g},${capColor.b})`, clamp01((0.18 + fillRatio * 0.26) * sceneLevel));

			for (let c = 1; c < TETRIS_COLS; c++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left + c * cell, top, 1, boardH, `rgb(${lineColor.r},${lineColor.g},${lineColor.b})`, 0.14 * sceneLevel);
			}
			for (let r = 1; r < TETRIS_ROWS; r++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left, top + r * cell, boardW, 1, `rgb(${lineColor.r},${lineColor.g},${lineColor.b})`, 0.12 * sceneLevel);
			}

			const hueOffsets = [0, 18, 36, 62, 118, 156, 196];
			const hueScale = Math.max(0.4, this.cfg.hue_sp / 24);
			const blockPalette = (kind, active) => {
				const offset = hueOffsets[Math.max(0, Math.min(hueOffsets.length - 1, Math.round(kind) - 1))] || 0;
				const hue = positiveMod(this.cfg.hue + offset * hueScale, 360);
				const base = hslToRGB(hue, clamp01(this.cfg.sat * (active ? 1.04 : 0.92)), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * (active ? 0.72 : 0.6)));
				const hi = hslToRGB(positiveMod(hue - 6, 360), clamp01(this.cfg.sat * 0.42), clamp01(Math.min(1, this.cfg.lmax * (active ? 1 : 0.96))));
				const shade = hslToRGB(positiveMod(hue + 10, 360), clamp01(this.cfg.sat * 0.72), clamp01(this.cfg.lmin + (this.cfg.lmax - this.cfg.lmin) * 0.18));
				return { base, hi, shade };
			};

			const drawBlock = (col, row, kind, active, alpha) => {
				const x = left + col * cell;
				const y = top + row * cell;
				const palette = blockPalette(kind, active);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, cell, cell, `rgb(${palette.base.r},${palette.base.g},${palette.base.b})`, alpha);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, cell, 1, `rgb(${palette.hi.r},${palette.hi.g},${palette.hi.b})`, alpha * 0.48);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, 1, cell, `rgb(${palette.hi.r},${palette.hi.g},${palette.hi.b})`, alpha * 0.22);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y + cell - 1, cell, 1, `rgb(${palette.shade.r},${palette.shade.g},${palette.shade.b})`, alpha * 0.34);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, x + cell - 1, y, 1, cell, `rgb(${palette.shade.r},${palette.shade.g},${palette.shade.b})`, alpha * 0.2);
			};

			for (let row = 0; row < TETRIS_ROWS; row++) {
				for (let col = 0; col < TETRIS_COLS; col++) {
					const kind = this._tetrisGetCell(row, col);
					if (kind <= 0) continue;
					const alpha = clamp01((0.86 + (1 - row / TETRIS_ROWS) * 0.08) * sceneLevel);
					drawBlock(col, row, kind, false, alpha);
				}
			}

			const piece = this._tetrisCurrentPiece();
			if (piece.alive) {
				let ghostY = piece.y;
				while (!this._tetrisCollision(piece.kind, piece.rot, piece.x, ghostY + 1)) ghostY++;
				if (ghostY > piece.y && this.cfg.ghost > 0) {
					for (const pt of this._tetrisPiecePoints(piece.kind, piece.rot)) {
						drawBlock(piece.x + pt.x, ghostY + pt.y, piece.kind, false, clamp01(this.cfg.ghost * sceneLevel));
					}
				}
				for (const pt of this._tetrisPiecePoints(piece.kind, piece.rot)) {
					drawBlock(piece.x + pt.x, piece.y + pt.y, piece.kind, true, clamp01(0.98 * sceneLevel));
				}
			}

			if ((this.values.ended || 0) > 0.5 || this.timers.ending > 0) {
				const shade = ctx.createLinearGradient(0, top * sy, 0, bottom * sy);
				shade.addColorStop(0, `rgba(8, 10, 14, ${clamp01(0.08 + (1 - sceneLevel) * 0.32)})`);
				shade.addColorStop(1, `rgba(8, 10, 14, ${clamp01(0.22 + (1 - sceneLevel) * 0.28)})`);
				ctx.fillStyle = shade;
				ctx.fillRect(Math.floor(left * sx), Math.floor(top * sy), Math.ceil(boardW * sx), Math.ceil(boardH * sy));
			}

			const dangerAlpha = clamp01((stackHeight - 0.6) * 1.8);
			if (dangerAlpha > 0.02) {
				const danger = hslToRGB(8, 0.78, 0.64);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, left, top + cell, boardW, 1, `rgb(${danger.r},${danger.g},${danger.b})`, dangerAlpha * 0.5);
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

	class Lighthouse extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('lighthouse', w, h, cfg, seed);
		}
	}

	class Rowboat extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('rowboat', w, h, cfg, seed);
		}
	}

	class Underwater extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('underwater', w, h, cfg, seed);
		}
	}

	class Volcano extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('volcano', w, h, cfg, seed);
		}
	}

	class Train extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('train', w, h, cfg, seed);
		}
	}

	class MysteriousMan extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('mysterious-man', w, h, cfg, seed);
		}
	}

	class BurningTrees extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('burning-trees', w, h, cfg, seed);
		}
	}

	class Sand extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('sand', w, h, cfg, seed);
		}
	}

	class WaterPipe extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('water-pipe', w, h, cfg, seed);
		}
	}

	class Tetris extends ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('tetris', w, h, cfg, seed);
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
	const effects = { rain: Rain, dust: Dust, fireflies: Fireflies, waterfall: Waterfall, 'wheat-field': WheatField, beach: Beach, campfire: Campfire, windmill: Windmill, lighthouse: Lighthouse, rowboat: Rowboat, underwater: Underwater, volcano: Volcano, train: Train, 'mysterious-man': MysteriousMan, 'burning-trees': BurningTrees, sand: Sand, 'water-pipe': WaterPipe, tetris: Tetris, aurora: Aurora, snow: Snow, 'autumn-leaves': AutumnLeaves, starfield: Starfield };
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
		lighthouse: [
			{
				key: 'clear-night',
				label: 'clear night',
				config: {
					sweep_speed: 0.07,
					beam_width: 0.18,
					beam_softness: 0.32,
					tower_height: 22,
					tower_width: 6,
					horizon: 0.74,
					haze: 0.08,
					glow: 0.2,
					hue: 216,
					hue_sp: 14,
					sat: 0.3,
					lmin: 0.1,
					lmax: 0.8,
				},
			},
			{
				key: 'steady-sweep',
				label: 'steady sweep',
				config: {
					sweep_speed: 0.08,
					beam_width: 0.22,
					beam_softness: 0.42,
					tower_height: 22,
					tower_width: 6.5,
					horizon: 0.74,
					haze: 0.14,
					glow: 0.22,
					hue: 214,
					hue_sp: 18,
					sat: 0.34,
					lmin: 0.12,
					lmax: 0.84,
					bright_pass_p: 0.0007,
				},
			},
			{
				key: 'foggy-coast',
				label: 'foggy coast',
				config: {
					sweep_speed: 0.06,
					beam_width: 0.28,
					beam_softness: 0.62,
					tower_height: 23,
					tower_width: 7,
					horizon: 0.76,
					haze: 0.24,
					glow: 0.18,
					hue: 210,
					hue_sp: 12,
					sat: 0.24,
					lmin: 0.1,
					lmax: 0.72,
					fog_thicken_p: 0.0012,
					fog_thicken_mult: 2.2,
				},
			},
			{
				key: 'bright-beacon',
				label: 'bright beacon',
				config: {
					sweep_speed: 0.1,
					beam_width: 0.24,
					beam_softness: 0.36,
					tower_height: 21,
					tower_width: 6,
					horizon: 0.72,
					haze: 0.12,
					glow: 0.3,
					hue: 218,
					hue_sp: 20,
					sat: 0.36,
					lmin: 0.12,
					lmax: 0.9,
					bright_pass_p: 0.0014,
					bright_pass_mult: 2.1,
					calm_p: 0.0009,
				},
			},
		],
		rowboat: [
			{
				key: 'still-lake',
				label: 'still lake',
				config: {
					intro_drift: 0.12,
					ending_ripple: 0.12,
					waterline: 0.57,
					drift_speed: 0.05,
					bob_amp: 0.7,
					wave_amp: 0.9,
					wave_freq: 0.12,
					ripple: 0.12,
					reflection: 0.28,
					boat_len: 13,
					boat_height: 3.5,
					hue: 202,
					hue_sp: 10,
					sat: 0.26,
					lmin: 0.16,
					lmax: 0.74,
					calm_p: 0.0011,
				},
			},
			{
				key: 'gentle-drift',
				label: 'gentle drift',
				config: {
					waterline: 0.58,
					drift_speed: 0.08,
					bob_amp: 1.2,
					wave_amp: 1.6,
					wave_freq: 0.16,
					ripple: 0.24,
					reflection: 0.22,
					boat_len: 14,
					boat_height: 3.5,
					hue: 206,
					hue_sp: 16,
					sat: 0.36,
					lmin: 0.16,
					lmax: 0.82,
					drift_p: 0.0009,
				},
			},
			{
				key: 'evening-ripples',
				label: 'evening ripples',
				config: {
					waterline: 0.6,
					drift_speed: 0.1,
					bob_amp: 1.4,
					wave_amp: 1.9,
					wave_freq: 0.18,
					ripple: 0.34,
					reflection: 0.24,
					boat_len: 14.5,
					boat_height: 4,
					hue: 212,
					hue_sp: 18,
					sat: 0.4,
					lmin: 0.18,
					lmax: 0.86,
					wake_p: 0.001,
				},
			},
			{
				key: 'wind-touched-water',
				label: 'wind-touched water',
				config: {
					waterline: 0.56,
					drift_speed: 0.12,
					bob_amp: 1.8,
					wave_amp: 2.5,
					wave_freq: 0.2,
					ripple: 0.42,
					reflection: 0.18,
					boat_len: 15,
					boat_height: 4,
					hue: 198,
					hue_sp: 20,
					sat: 0.46,
					lmin: 0.18,
					lmax: 0.8,
					wake_p: 0.0012,
					wake_mult: 2.1,
					drift_p: 0.0014,
					drift_push: 1.55,
				},
			},
		],
		underwater: [
			{
				key: 'quiet-shallows',
				label: 'quiet shallows',
				config: {
					intro_reveal: 0.18,
					ending_murk: 0.12,
					density: 0.18,
					rise_speed: 0.32,
					drift: 0.04,
					sway: 0.34,
					weed_height: 16,
					weed_count: 9,
					caustics: 0.44,
					depth: 0.28,
					hue: 184,
					hue_sp: 12,
					sat: 0.38,
					lmin: 0.14,
					lmax: 0.86,
					calm_p: 0.0011,
				},
			},
			{
				key: 'bubble-field',
				label: 'bubble field',
				config: {
					density: 0.42,
					rise_speed: 0.54,
					drift: 0.08,
					sway: 0.46,
					weed_height: 18,
					weed_count: 8,
					caustics: 0.26,
					depth: 0.42,
					hue: 190,
					hue_sp: 16,
					sat: 0.42,
					lmin: 0.12,
					lmax: 0.82,
					bubble_burst_p: 0.0012,
				},
			},
			{
				key: 'slow-current',
				label: 'slow current',
				config: {
					density: 0.28,
					rise_speed: 0.4,
					drift: 0.12,
					sway: 0.78,
					weed_height: 22,
					weed_count: 11,
					caustics: 0.3,
					depth: 0.56,
					hue: 192,
					hue_sp: 18,
					sat: 0.42,
					lmin: 0.12,
					lmax: 0.82,
					current_shift_p: 0.0011,
				},
			},
			{
				key: 'deep-water',
				label: 'deep water',
				config: {
					intro_reveal: 0.1,
					ending_murk: 0.16,
					density: 0.16,
					rise_speed: 0.28,
					drift: 0.05,
					sway: 0.26,
					weed_height: 13,
					weed_count: 6,
					caustics: 0.14,
					depth: 0.82,
					hue: 204,
					hue_sp: 10,
					sat: 0.3,
					lmin: 0.08,
					lmax: 0.62,
					calm_p: 0.0014,
					calm_mult: 0.42,
				},
			},
		],
		volcano: [
			{
				key: 'sleeping-cone',
				label: 'sleeping cone',
				config: {
					intro_glow: 0.1,
					ending_embers: 0.12,
					horizon: 0.82,
					cone_height: 24,
					cone_width: 42,
					crater_width: 7,
					glow: 0.18,
					smoke: 0.12,
					ash: 0.08,
					hue: 16,
					hue_sp: 12,
					sat: 0.62,
					lmin: 0.07,
					lmax: 0.78,
				},
			},
			{
				key: 'smoldering-crater',
				label: 'smoldering crater',
				config: {
					horizon: 0.8,
					cone_height: 26,
					cone_width: 44,
					crater_width: 8,
					glow: 0.28,
					smoke: 0.24,
					ash: 0.12,
					hue: 18,
					hue_sp: 18,
					sat: 0.78,
					lmin: 0.08,
					lmax: 0.86,
					smolder_p: 0.0012,
				},
			},
			{
				key: 'active-vent',
				label: 'active vent',
				config: {
					horizon: 0.79,
					cone_height: 27,
					cone_width: 46,
					crater_width: 9,
					glow: 0.34,
					smoke: 0.3,
					ash: 0.22,
					hue: 20,
					hue_sp: 20,
					sat: 0.84,
					lmin: 0.08,
					lmax: 0.9,
					eruption_p: 0.001,
					smolder_p: 0.0008,
				},
			},
			{
				key: 'ember-burst',
				label: 'ember burst',
				config: {
					intro_glow: 0.14,
					ending_embers: 0.1,
					horizon: 0.78,
					cone_height: 25,
					cone_width: 40,
					crater_width: 8,
					glow: 0.4,
					smoke: 0.2,
					ash: 0.3,
					hue: 14,
					hue_sp: 22,
					sat: 0.88,
					lmin: 0.08,
					lmax: 0.92,
					eruption_p: 0.0014,
					flare_p: 0.0013,
					eruption_mult: 2.45,
					flare_mult: 2.2,
				},
			},
		],
		train: [
			{
				key: 'distant-freight',
				label: 'distant freight',
				config: {
					intro_cue: 0.1,
					ending_clear: 0.12,
					horizon: 0.76,
					track_row: 0.83,
					train_height: 4,
					car_count: 8,
					car_gap: 1.2,
					speed: 0.92,
					smoke: 0.24,
					headlight: 0.18,
					hue: 226,
					hue_sp: 14,
					sat: 0.28,
					lmin: 0.07,
					lmax: 0.74,
					pass_p: 0.001,
					quiet_gap_p: 0.0012,
				},
			},
			{
				key: 'night-local',
				label: 'night local',
				config: {
					intro_cue: 0.14,
					ending_clear: 0.09,
					horizon: 0.74,
					track_row: 0.82,
					train_height: 4.5,
					car_count: 6,
					car_gap: 1,
					speed: 1.05,
					smoke: 0.16,
					headlight: 0.26,
					hue: 218,
					hue_sp: 18,
					sat: 0.34,
					lmin: 0.08,
					lmax: 0.84,
					pass_p: 0.0013,
				},
			},
			{
				key: 'steady-passing',
				label: 'steady passing',
				config: {
					intro_cue: 0.12,
					ending_clear: 0.08,
					horizon: 0.73,
					track_row: 0.81,
					train_height: 5,
					car_count: 7,
					car_gap: 0.9,
					speed: 1.18,
					smoke: 0.2,
					headlight: 0.3,
					hue: 214,
					hue_sp: 20,
					sat: 0.38,
					lmin: 0.08,
					lmax: 0.88,
					pass_p: 0.0018,
					pass_mult: 1.25,
				},
			},
			{
				key: 'express-line',
				label: 'express line',
				config: {
					intro_cue: 0.16,
					ending_clear: 0.06,
					horizon: 0.72,
					track_row: 0.8,
					train_height: 4.5,
					car_count: 5,
					car_gap: 0.8,
					speed: 1.35,
					smoke: 0.12,
					headlight: 0.42,
					hue: 208,
					hue_sp: 24,
					sat: 0.42,
					lmin: 0.08,
					lmax: 0.92,
					express_p: 0.0017,
					express_mult: 2.15,
					quiet_gap_p: 0.0008,
				},
			},
		],
		'mysterious-man': [
			{
				key: 'noir-stillness',
				label: 'noir stillness',
				config: {
					intro_reveal: 0.06,
					ending_shadow: 0.12,
					figure_x: 0.39,
					figure_scale: 1,
					lean: 0.06,
					contrast: 0.2,
					ember: 0.18,
					hold_angle: 0.18,
					smoke: 0.1,
					rise_speed: 0.6,
					drift: 0.04,
					hue: 222,
					hue_sp: 12,
					sat: 0.14,
					lmin: 0.03,
					lmax: 0.76,
				},
			},
			{
				key: 'deep-inhale',
				label: 'deep inhale',
				config: {
					intro_reveal: 0.08,
					ending_shadow: 0.08,
					figure_x: 0.38,
					figure_scale: 1.05,
					lean: 0.1,
					contrast: 0.26,
					ember: 0.26,
					hold_angle: 0.24,
					smoke: 0.14,
					rise_speed: 0.68,
					drift: 0.06,
					hue: 216,
					hue_sp: 16,
					sat: 0.18,
					lmin: 0.04,
					lmax: 0.84,
					inhale_p: 0.0016,
					exhale_p: 0.0012,
				},
			},
			{
				key: 'cold-alley',
				label: 'cold alley',
				config: {
					intro_reveal: 0.07,
					ending_shadow: 0.1,
					figure_x: 0.36,
					figure_scale: 0.95,
					lean: 0.04,
					contrast: 0.3,
					ember: 0.2,
					hold_angle: 0.14,
					smoke: 0.2,
					rise_speed: 0.82,
					drift: 0.18,
					hue: 204,
					hue_sp: 20,
					sat: 0.16,
					lmin: 0.03,
					lmax: 0.8,
					exhale_p: 0.0015,
					ash_fall_p: 0.0009,
				},
			},
			{
				key: 'ember-watch',
				label: 'ember watch',
				config: {
					intro_reveal: 0.1,
					ending_shadow: 0.06,
					figure_x: 0.4,
					figure_scale: 1.1,
					lean: 0.12,
					contrast: 0.34,
					ember: 0.32,
					hold_angle: 0.28,
					smoke: 0.18,
					rise_speed: 0.76,
					drift: 0.08,
					hue: 214,
					hue_sp: 22,
					sat: 0.22,
					lmin: 0.04,
					lmax: 0.9,
					inhale_p: 0.0011,
					lighter_flick_p: 0.001,
					lighter_flick_mult: 2.5,
				},
			},
		],
		'burning-trees': [
			{
				key: 'single-ignition',
				label: 'single ignition',
				config: {
					intro_growth: 0.12,
					ending_ash: 0.14,
					horizon: 0.84,
					tree_count: 9,
					tree_height: 12,
					canopy: 0.82,
					spread: 0.9,
					flame: 0.22,
					embers: 0.12,
					smoke: 0.12,
					char: 0.38,
					hue: 118,
					hue_sp: 14,
					sat: 0.46,
					lmin: 0.06,
					lmax: 0.82,
					ignite_p: 0.0011,
				},
			},
			{
				key: 'slow-spread',
				label: 'slow spread',
				config: {
					intro_growth: 0.14,
					ending_ash: 0.12,
					horizon: 0.84,
					tree_count: 10,
					tree_height: 13,
					canopy: 0.78,
					spread: 1.35,
					flame: 0.26,
					embers: 0.16,
					smoke: 0.16,
					char: 0.42,
					hue: 112,
					hue_sp: 20,
					sat: 0.48,
					lmin: 0.06,
					lmax: 0.88,
					ignite_p: 0.0013,
					lull_p: 0.0007,
				},
			},
			{
				key: 'smoldering-line',
				label: 'smoldering line',
				config: {
					intro_growth: 0.1,
					ending_ash: 0.18,
					horizon: 0.85,
					tree_count: 11,
					tree_height: 11.5,
					canopy: 0.68,
					spread: 1.1,
					flame: 0.18,
					embers: 0.24,
					smoke: 0.24,
					char: 0.56,
					hue: 104,
					hue_sp: 18,
					sat: 0.38,
					lmin: 0.05,
					lmax: 0.8,
					ignite_p: 0.001,
					lull_p: 0.0012,
					lull_mult: 0.42,
				},
			},
			{
				key: 'active-burn',
				label: 'active burn',
				config: {
					intro_growth: 0.16,
					ending_ash: 0.1,
					horizon: 0.83,
					tree_count: 12,
					tree_height: 14,
					canopy: 0.84,
					spread: 1.75,
					flame: 0.34,
					embers: 0.24,
					smoke: 0.2,
					char: 0.48,
					hue: 108,
					hue_sp: 24,
					sat: 0.54,
					lmin: 0.06,
					lmax: 0.92,
					ignite_p: 0.0016,
					flare_p: 0.0012,
					flare_mult: 2.1,
				},
			},
		],
		sand: [
			{
				key: 'small-trickle',
				label: 'small trickle',
				config: {
					intro_trickle: 0.08,
					intro_fill: 0.01,
					ending_settle: 0.12,
					pipe_x: 0.24,
					pipe_width: 6,
					pipe_drop: 17,
					container_x: 0.58,
					container_width: 30,
					container_depth: 9,
					flow: 0.14,
					spread: 0.5,
					settle: 0.38,
					overflow: 0.16,
					hue: 36,
					hue_sp: 8,
					sat: 0.46,
					lmin: 0.14,
					lmax: 0.78,
					calm_p: 0.0012,
				},
			},
			{
				key: 'steady-pour',
				label: 'steady pour',
				config: {
					intro_trickle: 0.1,
					intro_fill: 0.02,
					ending_settle: 0.08,
					pipe_x: 0.28,
					pipe_width: 7,
					pipe_drop: 18,
					container_x: 0.58,
					container_width: 34,
					container_depth: 10,
					flow: 0.24,
					spread: 0.9,
					settle: 0.32,
					overflow: 0.28,
					hue: 38,
					hue_sp: 10,
					sat: 0.54,
					lmin: 0.16,
					lmax: 0.86,
					surge_p: 0.001,
				},
			},
			{
				key: 'heavy-fill',
				label: 'heavy fill',
				config: {
					intro_trickle: 0.14,
					intro_fill: 0.03,
					ending_settle: 0.06,
					pipe_x: 0.3,
					pipe_width: 8,
					pipe_drop: 19,
					container_x: 0.6,
					container_width: 32,
					container_depth: 9,
					flow: 0.34,
					spread: 1.2,
					settle: 0.28,
					overflow: 0.36,
					hue: 40,
					hue_sp: 12,
					sat: 0.58,
					lmin: 0.16,
					lmax: 0.9,
					surge_p: 0.0014,
					surge_mult: 2.2,
				},
			},
			{
				key: 'overflow-study',
				label: 'overflow study',
				config: {
					intro_trickle: 0.12,
					intro_fill: 0.05,
					ending_settle: 0.1,
					pipe_x: 0.26,
					pipe_width: 7,
					pipe_drop: 17,
					container_x: 0.54,
					container_width: 24,
					container_depth: 8,
					flow: 0.38,
					spread: 1.1,
					settle: 0.24,
					overflow: 0.6,
					hue: 34,
					hue_sp: 10,
					sat: 0.52,
					lmin: 0.15,
					lmax: 0.84,
					surge_p: 0.0012,
					calm_p: 0.0006,
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
		'water-pipe': [
			{
				key: 'small-trickle',
				label: 'small trickle',
				config: {
					intro_flow: 0.08,
					intro_pool: 0.02,
					ending_ripple: 0.1,
					pipe_x: 0.18,
					pipe_width: 6,
					pipe_drop: 16,
					basin_x: 0.36,
					basin_width: 28,
					basin_depth: 9,
					flow: 0.12,
					stream: 0.55,
					ripple: 0.24,
					overflow: 0.14,
					foam: 0.12,
					hue: 194,
					hue_sp: 14,
					sat: 0.44,
					lmin: 0.12,
					lmax: 0.82,
					dry_up_p: 0.0011,
				},
			},
			{
				key: 'steady-pool',
				label: 'steady pool',
				config: {
					intro_flow: 0.1,
					intro_pool: 0.08,
					ending_ripple: 0.08,
					pipe_x: 0.22,
					pipe_width: 7,
					pipe_drop: 17,
					basin_x: 0.4,
					basin_width: 30,
					basin_depth: 9,
					flow: 0.24,
					stream: 1.05,
					ripple: 0.38,
					overflow: 0.3,
					foam: 0.22,
					hue: 196,
					hue_sp: 18,
					sat: 0.56,
					lmin: 0.12,
					lmax: 0.92,
					surge_p: 0.0008,
				},
			},
			{
				key: 'heavy-spill',
				label: 'heavy spill',
				config: {
					intro_flow: 0.14,
					intro_pool: 0.12,
					ending_ripple: 0.06,
					pipe_x: 0.24,
					pipe_width: 8,
					pipe_drop: 18,
					basin_x: 0.42,
					basin_width: 28,
					basin_depth: 8,
					flow: 0.34,
					stream: 1.25,
					ripple: 0.44,
					overflow: 0.42,
					foam: 0.24,
					hue: 200,
					hue_sp: 20,
					sat: 0.58,
					lmin: 0.12,
					lmax: 0.92,
					surge_p: 0.0012,
					surge_mult: 2.15,
				},
			},
			{
				key: 'edge-runoff',
				label: 'edge runoff',
				config: {
					intro_flow: 0.12,
					intro_pool: 0.05,
					ending_ripple: 0.12,
					pipe_x: 0.28,
					pipe_width: 7,
					pipe_drop: 17,
					basin_x: 0.38,
					basin_width: 22,
					basin_depth: 7,
					flow: 0.28,
					stream: 1.05,
					ripple: 0.36,
					overflow: 0.56,
					foam: 0.22,
					hue: 192,
					hue_sp: 16,
					sat: 0.5,
					lmin: 0.11,
					lmax: 0.88,
					surge_p: 0.001,
					dry_up_p: 0.0005,
				},
			},
		],
		tetris: [
			{
				key: 'museum-pace',
				label: 'museum pace',
				config: {
					intro_dur: 72,
					intro_stack: 0,
					intro_cadence: 0.48,
					ending_dur: 88,
					ending_linger: 34,
					ending_fill: 0.8,
					well_x: 0.17,
					well_y: 0.12,
					cell_size: 3,
					spawn_every: 56,
					fall_every: 10,
					lock_delay: 5,
					glow: 0.14,
					ghost: 0.1,
					hue: 220,
					hue_sp: 18,
					sat: 0.42,
					lmin: 0.08,
					lmax: 0.84,
					lull_p: 0.0012,
					lull_dur: 104,
					lull_mult: 2.35,
				},
			},
			{
				key: 'steady-build',
				label: 'steady build',
				config: {
					intro_dur: 60,
					intro_stack: 1,
					intro_cadence: 0.58,
					ending_dur: 74,
					ending_linger: 28,
					ending_fill: 0.84,
					well_x: 0.18,
					well_y: 0.12,
					cell_size: 3,
					spawn_every: 42,
					fall_every: 8,
					lock_delay: 4,
					glow: 0.18,
					ghost: 0.14,
					hue: 208,
					hue_sp: 24,
					sat: 0.56,
					lmin: 0.1,
					lmax: 0.92,
					new_piece_p: 0.0008,
					new_piece_dur: 16,
					new_piece_cut: 0.22,
					lull_p: 0.0005,
					lull_dur: 76,
					lull_mult: 1.75,
				},
			},
			{
				key: 'dense-stack',
				label: 'dense stack',
				config: {
					intro_dur: 46,
					intro_stack: 4,
					intro_cadence: 0.72,
					ending_dur: 66,
					ending_linger: 24,
					ending_fill: 0.76,
					well_x: 0.18,
					well_y: 0.11,
					cell_size: 3,
					spawn_every: 34,
					fall_every: 7,
					lock_delay: 4,
					glow: 0.2,
					ghost: 0.12,
					hue: 198,
					hue_sp: 28,
					sat: 0.6,
					lmin: 0.1,
					lmax: 0.9,
					new_piece_p: 0.0009,
					new_piece_dur: 15,
					new_piece_cut: 0.2,
					lull_p: 0.0003,
				},
			},
			{
				key: 'late-game',
				label: 'late game',
				config: {
					intro_dur: 34,
					intro_stack: 6,
					intro_cadence: 0.84,
					ending_dur: 58,
					ending_linger: 22,
					ending_fill: 0.9,
					well_x: 0.18,
					well_y: 0.1,
					cell_size: 3,
					spawn_every: 26,
					fall_every: 5,
					lock_delay: 3,
					glow: 0.24,
					ghost: 0.16,
					hue: 192,
					hue_sp: 30,
					sat: 0.66,
					lmin: 0.12,
					lmax: 0.94,
					new_piece_p: 0.0012,
					new_piece_dur: 18,
					new_piece_cut: 0.18,
					lull_p: 0.0002,
					lull_mult: 1.5,
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

	class EffectTransitionController {
		constructor(effect, options) {
			options = options || {};
			this.current = effect || null;
			this.outgoing = null;
			this.startedAt = 0;
			this.durationMs = Math.max(100, Math.round(options.durationMs || 1600));
			this.buffers = [];
		}

		_now() {
			return (global.performance && typeof global.performance.now === 'function')
				? global.performance.now()
				: Date.now();
		}

		_progress() {
			if (!this.outgoing || this.startedAt <= 0) return 1;
			return clamp01((this._now() - this.startedAt) / this.durationMs);
		}

		_buffer(index, width, height) {
			let canvas = this.buffers[index];
			if (!canvas) {
				canvas = global.document && typeof global.document.createElement === 'function'
					? global.document.createElement('canvas')
					: null;
				this.buffers[index] = canvas;
			}
			if (!canvas) return null;
			if (canvas.width !== width) canvas.width = width;
			if (canvas.height !== height) canvas.height = height;
			return canvas;
		}

		currentEffect() {
			return this.current;
		}

		isTransitioning() {
			return !!this.outgoing;
		}

		setEffect(nextEffect) {
			if (!nextEffect) return;
			if (!this.current) {
				this.current = nextEffect;
				this.outgoing = null;
				this.startedAt = 0;
				return;
			}
			this.outgoing = this.current;
			this.current = nextEffect;
			this.startedAt = this._now();
		}

		step() {
			if (this.outgoing) this.outgoing.step();
			if (this.current) this.current.step();
			if (this.outgoing && this._progress() >= 1) {
				this.outgoing = null;
				this.startedAt = 0;
			}
		}

		render(ctx, canvasW, canvasH, opts) {
			if (!this.current) return;
			if (!this.outgoing) {
				this.current.render(ctx, canvasW, canvasH, opts);
				return;
			}

			const fromCanvas = this._buffer(0, canvasW, canvasH);
			const toCanvas = this._buffer(1, canvasW, canvasH);
			if (!fromCanvas || !toCanvas) {
				this.current.render(ctx, canvasW, canvasH, opts);
				return;
			}

			const fromCtx = fromCanvas.getContext('2d');
			const toCtx = toCanvas.getContext('2d');
			fromCtx.clearRect(0, 0, canvasW, canvasH);
			toCtx.clearRect(0, 0, canvasW, canvasH);
			this.outgoing.render(fromCtx, canvasW, canvasH, opts);
			this.current.render(toCtx, canvasW, canvasH, opts);

			const progress = this._progress();
			ctx.clearRect(0, 0, canvasW, canvasH);
			ctx.save();
			ctx.globalAlpha = 1 - progress;
			ctx.drawImage(fromCanvas, 0, 0);
			ctx.restore();
			ctx.save();
			ctx.globalAlpha = progress;
			ctx.drawImage(toCanvas, 0, 0);
			ctx.restore();

			if (progress >= 1) {
				this.outgoing = null;
				this.startedAt = 0;
			}
		}
	}

	function createTransitionController(effect, options) {
		return new EffectTransitionController(effect, options);
	}

	global.AmbienceSim = { Rain, Dust, Fireflies, Waterfall, WheatField, Beach, Campfire, Windmill, Lighthouse, Rowboat, Underwater, Volcano, Train, MysteriousMan, BurningTrees, Sand, WaterPipe, Tetris, Aurora, AutumnLeaves, Snow, Starfield, subscribe, applyDefaults, hslToRGB, effects, presets, createTransitionController };
})(window);
