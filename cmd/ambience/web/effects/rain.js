'use strict';
(function (api) {
	const { makeRNG, jitterInt, hslToRGB } = api._helpers;

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

	api.effects['rain'] = Rain;
})(window.AmbienceSim);
