'use strict';
(function (api) {
	const { makeRNG, jitterInt, hslToRGB } = api._helpers;

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

	api.effects['fireflies'] = Fireflies;
})(window.AmbienceSim);
