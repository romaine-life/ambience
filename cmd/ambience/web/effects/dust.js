'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

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
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
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

	api.presets['dust'] = [
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
	];
	api.effects['dust'] = Dust;
})(window.AmbienceSim);
