'use strict';
(function (api) {
	const { makeRNG, jitterInt, clamp01, hslToRGB } = api._helpers;

	const DEFAULTS = {
		intro_dur: 60,
		intro_glow: 0.4,
		ending_dur: 70,
		ending_linger: 24,
		ending_glow: 0.1,
		horizon: 0.7,
		track_y: 0.78,
		loco_len: 7,
		car_len: 6,
		cars: 3,
		train_height: 5,
		light_glow: 0.45,
		smoke: 0.32,
		cue_lead: 14,
		tail_linger: 12,
		hue: 220,
		hue_sp: 18,
		sat: 0.42,
		lmin: 0.1,
		lmax: 0.78,
		pass_p: 0,
		express_p: 0,
		quiet_p: 0,
		pass_dur: 160,
		express_dur: 110,
		express_speed_mult: 1.7,
		quiet_dur: 240,
		quiet_mult: 0.15,
	};

	function applyDefaults(cfg) {
		const c = Object.assign({}, DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = DEFAULTS.intro_dur;
		c.intro_glow = clamp01(c.intro_glow);
		if (c.ending_dur <= 0) c.ending_dur = DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		c.ending_glow = clamp01(c.ending_glow);
		if (c.horizon <= 0) c.horizon = DEFAULTS.horizon;
		if (c.track_y <= 0) c.track_y = DEFAULTS.track_y;
		if (c.track_y < c.horizon) c.track_y = c.horizon + 0.04;
		if (c.loco_len <= 0) c.loco_len = DEFAULTS.loco_len;
		if (c.car_len <= 0) c.car_len = DEFAULTS.car_len;
		if (c.cars < 0) c.cars = 0;
		if (c.train_height <= 0) c.train_height = DEFAULTS.train_height;
		if (c.light_glow <= 0) c.light_glow = DEFAULTS.light_glow;
		if (c.smoke < 0) c.smoke = 0;
		if (c.cue_lead < 0) c.cue_lead = 0;
		if (c.tail_linger < 0) c.tail_linger = 0;
		if (c.hue_sp < 0) c.hue_sp = 0;
		if (c.sat <= 0) c.sat = DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.pass_dur <= 0) c.pass_dur = DEFAULTS.pass_dur;
		if (c.express_dur <= 0) c.express_dur = DEFAULTS.express_dur;
		if (c.express_speed_mult <= 0) c.express_speed_mult = DEFAULTS.express_speed_mult;
		if (c.quiet_dur <= 0) c.quiet_dur = DEFAULTS.quiet_dur;
		if (c.quiet_mult < 0) c.quiet_mult = 0;
		return c;
	}

	class Train {
		constructor(w, h, cfg, seed) {
			this.w = w;
			this.h = h;
			this.grid = new Uint8ClampedArray(w * h * 3);
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

		_fillCell(ctx, sx, sy, ceilSx, ceilSy, x, y, w, h, color, alpha) {
			ctx.fillStyle = color;
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillRect(Math.floor(x * sx), Math.floor(y * sy), Math.max(1, Math.ceil(w * sx || ceilSx)), Math.max(1, Math.ceil(h * sy || ceilSy)));
			ctx.globalAlpha = 1;
		}

		triggerEvent(name) {
			const rng = this._eventRng(name.length + 71);
			switch (name) {
				case 'pass':
					this.timers.pass = jitterInt(rng, this.cfg.pass_dur, 0.18);
					this.timers.express = 0;
					this.values.pass_total = this.timers.pass;
					this.values.pass_dir = rng() < 0.5 ? -1 : 1;
					delete this.values.express_total;
					delete this.values.express_dir;
					return true;
				case 'express':
					this.timers.express = jitterInt(rng, this.cfg.express_dur, 0.18);
					this.timers.pass = 0;
					this.values.express_total = this.timers.express;
					this.values.express_dir = rng() < 0.5 ? -1 : 1;
					delete this.values.pass_total;
					delete this.values.pass_dir;
					return true;
				case 'quiet-gap':
					this.timers['quiet-gap'] = jitterInt(rng, this.cfg.quiet_dur, 0.25);
					return true;
				case 'intro':
					this.timers.pass = 0;
					this.timers.express = 0;
					this.timers['quiet-gap'] = 0;
					this.timers.ending = 0;
					delete this.values.pass_total;
					delete this.values.pass_dir;
					delete this.values.express_total;
					delete this.values.express_dir;
					this.timers.intro = Math.max(1, Math.round(this.cfg.intro_dur));
					this.values.intro_total = this.timers.intro;
					return true;
				case 'ending':
					this.timers.intro = 0;
					this.timers.pass = 0;
					this.timers.express = 0;
					this.timers['quiet-gap'] = 0;
					delete this.values.pass_total;
					delete this.values.pass_dir;
					delete this.values.express_total;
					delete this.values.express_dir;
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
			if (!this.timers.pass || this.timers.pass <= 0) {
				delete this.values.pass_total;
				delete this.values.pass_dir;
			}
			if (!this.timers.express || this.timers.express <= 0) {
				delete this.values.express_total;
				delete this.values.express_dir;
			}
			api._helpers.paintProceduralGrid(this);
		}

		// Returns { kind, dir, total, left, lifecycle } when a train is in
		// flight, or null when the frame is empty. lifecycle is the elapsed
		// fraction across the entire pass timer (0 = just triggered, 1 = about
		// to clear).
		_activePass() {
			if (this.timers.express > 0) {
				const total = this.values.express_total || this.cfg.express_dur;
				return { kind: 'express', dir: this.values.express_dir || 1, total, left: this.timers.express };
			}
			if (this.timers.pass > 0) {
				const total = this.values.pass_total || this.cfg.pass_dur;
				return { kind: 'pass', dir: this.values.pass_dir || 1, total, left: this.timers.pass };
			}
			return null;
		}

		// Compute how much the lifecycle phases (intro/ending) attenuate the
		// scene. Returned value is a 0..1 multiplier: 1 = full presence.
		_lifecycleLevel() {
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
			return Math.max(0.04, level);
		}

		_trainGeometry() {
			const cars = Math.max(0, Math.round(this.cfg.cars));
			const locoLen = Math.max(3, Math.round(this.cfg.loco_len));
			const carLen = Math.max(2, Math.round(this.cfg.car_len));
			const gap = 1;
			const total = locoLen + cars * (carLen + gap);
			return { cars, locoLen, carLen, gap, total };
		}

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}

		_renderPass(ctx, sx, sy, ceilSx, ceilSy, trackY, pass, lifecycle) {
			const cfg = this.cfg;
			const elapsed = pass.total - pass.left;
			const cueLead = Math.max(0, Math.round(cfg.cue_lead));
			const tailLinger = Math.max(0, Math.round(cfg.tail_linger));
			const movement = Math.max(1, pass.total - cueLead - tailLinger);
			const geom = this._trainGeometry();
			const dir = pass.dir >= 0 ? 1 : -1;
			const isExpress = pass.kind === 'express';
			const intensity = lifecycle * (isExpress ? cfg.express_speed_mult : 1);
			// Span the train so its leading edge starts just off-screen on the
			// entry side and exits just past the far edge.
			let mvProgress = -1;
			if (elapsed >= cueLead && elapsed < cueLead + movement) {
				mvProgress = (elapsed - cueLead) / movement;
			}

			// Headlight cue: distant glow at the entry edge before the loco
			// arrives, plus a halo riding the engine while it's in frame.
			const cueProgress = clamp01(elapsed / Math.max(1, cueLead || 1));
			if (elapsed < cueLead) {
				this._renderCueGlow(ctx, sx, sy, trackY, dir, cueProgress, intensity, isExpress);
				return;
			}
			if (mvProgress < 0) {
				// Tail linger phase — train has fully exited; draw residual
				// dust / steam puff drifting where it left.
				const tailProgress = clamp01((elapsed - cueLead - movement) / Math.max(1, tailLinger || 1));
				this._renderTailLinger(ctx, sx, sy, ceilSx, ceilSy, trackY, dir, tailProgress, intensity, isExpress);
				return;
			}

			const span = this.w + geom.total + 4;
			const travel = -geom.total - 2 + span * mvProgress;
			const headX = dir > 0 ? travel : (this.w - 1 - travel);
			this._renderTrain(ctx, sx, sy, ceilSx, ceilSy, trackY, headX, dir, geom, intensity, isExpress);
		}

		_renderCueGlow(ctx, sx, sy, trackY, dir, progress, intensity, isExpress) {
			const cfg = this.cfg;
			const baseAlpha = clamp01((cfg.intro_glow + (1 - cfg.intro_glow) * progress) * cfg.light_glow * intensity * (isExpress ? 1.25 : 1));
			const edgeX = dir > 0 ? -2 : this.w + 2;
			const cx = dir > 0 ? edgeX + 6 + progress * 4 : edgeX - 6 - progress * 4;
			const light = hslToRGB(42, 0.75, 0.72);
			for (let y = -2; y <= 2; y++) {
				for (let x = -6; x <= 6; x++) {
					if (Math.abs(x) + Math.abs(y) > 7) continue;
					this._fillCell(ctx, sx, sy, 1, 1, Math.round(cx + x), trackY - 1 + y, 1, 1, `rgb(${light.r},${light.g},${light.b})`, baseAlpha * 0.35);
				}
			}
		}

		_renderTailLinger(ctx, sx, sy, ceilSx, ceilSy, trackY, dir, progress, intensity, isExpress) {
			const cfg = this.cfg;
			const fade = (1 - progress) * intensity * (isExpress ? 1.2 : 0.9);
			if (fade <= 0.04) return;
			const exitX = dir > 0 ? this.w - 4 : 3;
			const dustColor = hslToRGB((cfg.hue + cfg.hue_sp + 360) % 360, clamp01(cfg.sat * 0.32), clamp01(cfg.lmax * 0.7));
			for (let i = 0; i < 5; i++) {
				const drift = Math.sin(this.tick * 0.18 + i * 1.1) * 1.1;
				const x = exitX - dir * (i * 1.6 + progress * 5);
				const y = trackY - 1 - i * 1.2 + drift;
				const alpha = clamp01((0.18 + cfg.smoke * 0.22) * fade * (1 - i / 6));
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(x), Math.round(y), 1, 1, `rgb(${dustColor.r},${dustColor.g},${dustColor.b})`, alpha);
			}
		}

		_renderEndingGlow(ctx, sx, sy, ceilSx, ceilSy, trackY, lifecycle) {
			const cfg = this.cfg;
			const alpha = clamp01(lifecycle * cfg.ending_glow * 0.6);
			if (alpha <= 0.02) return;
			const haloColor = hslToRGB(36, 0.4, clamp01(cfg.lmax * 0.85));
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.floor(this.w * 0.5) - 4, trackY - 1, 8, 1, `rgb(${haloColor.r},${haloColor.g},${haloColor.b})`, alpha * 0.55);
		}

		_renderTrain(ctx, sx, sy, ceilSx, ceilSy, trackY, headX, dir, geom, intensity, isExpress) {
			const cfg = this.cfg;
			const trainHeight = Math.max(2, Math.round(cfg.train_height));
			const baseY = trackY - 1;
			const topY = baseY - trainHeight + 1;
			const hullColor = hslToRGB((cfg.hue + 200) % 360, clamp01(cfg.sat * 0.24), clamp01(cfg.lmin + 0.04));
			const cabColor = hslToRGB((cfg.hue + 210) % 360, clamp01(cfg.sat * 0.18), clamp01(cfg.lmin * 0.85));
			const trimColor = hslToRGB(isExpress ? 14 : 28, 0.38, clamp01(cfg.lmax * 0.6));
			const windowColor = hslToRGB(48, 0.7, clamp01(cfg.lmax * 0.95));
			const wheelColor = hslToRGB(0, 0, 0.06);

			// Locomotive: leading edge at headX, extending backward (away from
			// dir of travel) for locoLen cells.
			const locoBackEnd = dir > 0 ? headX - geom.locoLen + 1 : headX + geom.locoLen - 1;
			const locoLeftX = Math.min(headX, locoBackEnd);
			const locoRightX = Math.max(headX, locoBackEnd);

			// Body fill.
			for (let row = 0; row < trainHeight; row++) {
				const y = topY + row;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, locoLeftX, y, geom.locoLen, 1, `rgb(${hullColor.r},${hullColor.g},${hullColor.b})`, 0.96);
			}
			// Cab silhouette — taller back portion.
			const cabLen = Math.max(2, Math.round(geom.locoLen * 0.45));
			const cabX = dir > 0 ? locoLeftX : locoRightX - cabLen + 1;
			for (let row = 0; row < trainHeight; row++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, cabX, topY + row, cabLen, 1, `rgb(${cabColor.r},${cabColor.g},${cabColor.b})`, 0.95);
			}
			// Cab window.
			if (trainHeight >= 3) {
				const winY = topY + 1;
				const winX = cabX + (dir > 0 ? Math.max(0, cabLen - 2) : 1);
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, winX, winY, 1, 1, `rgb(${windowColor.r},${windowColor.g},${windowColor.b})`, 0.9 * intensity);
			}
			// Stripe along the body.
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, locoLeftX, baseY - 1, geom.locoLen, 1, `rgb(${trimColor.r},${trimColor.g},${trimColor.b})`, 0.7);
			// Smokestack near front.
			const stackX = dir > 0 ? locoRightX - 1 : locoLeftX + 1;
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, stackX, topY - 1, 1, 1, `rgb(${cabColor.r},${cabColor.g},${cabColor.b})`, 0.95);
			// Cowcatcher — slim wedge at the leading nose.
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, headX, baseY, 1, 1, `rgb(${trimColor.r},${trimColor.g},${trimColor.b})`, 0.85);
			// Wheels.
			for (let i = 0; i < geom.locoLen; i += 2) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, locoLeftX + i, baseY + 1, 1, 1, `rgb(${wheelColor.r},${wheelColor.g},${wheelColor.b})`, 0.85);
			}

			// Headlight + halo.
			const headlightX = headX;
			const headlightY = topY + Math.max(0, Math.floor(trainHeight * 0.55));
			const lightColor = hslToRGB(54, 0.78, 0.72);
			this._fillCell(ctx, sx, sy, ceilSx, ceilSy, headlightX, headlightY, 1, 1, `rgb(${lightColor.r},${lightColor.g},${lightColor.b})`, clamp01(0.9 * intensity));
			const haloAlpha = clamp01(cfg.light_glow * intensity * (isExpress ? 1.35 : 1));
			for (let y = -2; y <= 2; y++) {
				for (let x = -5; x <= 5; x++) {
					if (Math.abs(x) + Math.abs(y) > 6) continue;
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, headlightX + x, headlightY + y, 1, 1, `rgb(${lightColor.r},${lightColor.g},${lightColor.b})`, haloAlpha * 0.25);
				}
			}

			// Smoke / steam plume drifting back from the stack.
			const smokeStrength = clamp01(cfg.smoke * intensity * (isExpress ? 1.25 : 1));
			if (smokeStrength > 0.02) {
				const smokeColor = hslToRGB(0, 0, 0.74);
				for (let i = 0; i < 6; i++) {
					const drift = Math.sin(this.tick * 0.19 + i * 0.6) * 0.7;
					const sx2 = stackX - dir * (i * 1.4 + 0.5);
					const sy2 = topY - 1 - i * 0.9 + drift;
					const alpha = clamp01(smokeStrength * (1 - i / 7) * 0.55);
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, Math.round(sx2), Math.round(sy2), 1, 1, `rgb(${smokeColor.r},${smokeColor.g},${smokeColor.b})`, alpha);
				}
			}

			// Trailing cars.
			const carHeight = Math.max(2, trainHeight - 1);
			for (let i = 0; i < geom.cars; i++) {
				const offset = geom.locoLen + geom.gap + i * (geom.carLen + geom.gap);
				const carRightAnchor = dir > 0 ? locoLeftX - geom.gap - i * (geom.carLen + geom.gap) : locoRightX + geom.gap + i * (geom.carLen + geom.gap);
				const carLeftX = dir > 0 ? carRightAnchor - geom.carLen + 1 : carRightAnchor;
				const carTopY = baseY - carHeight + 1;
				const carColor = i % 2 === 0 ?
					hslToRGB((cfg.hue + 196) % 360, clamp01(cfg.sat * 0.2), clamp01(cfg.lmin + 0.06)) :
					hslToRGB((cfg.hue + 188) % 360, clamp01(cfg.sat * 0.22), clamp01(cfg.lmin + 0.08));
				for (let row = 0; row < carHeight; row++) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, carLeftX, carTopY + row, geom.carLen, 1, `rgb(${carColor.r},${carColor.g},${carColor.b})`, 0.95);
				}
				// Stripe.
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, carLeftX, baseY - 1, geom.carLen, 1, `rgb(${trimColor.r},${trimColor.g},${trimColor.b})`, 0.55);
				// Windows: every other cell on the upper row.
				if (carHeight >= 2) {
					for (let wx = 1; wx < geom.carLen - 1; wx += 2) {
						this._fillCell(ctx, sx, sy, ceilSx, ceilSy, carLeftX + wx, carTopY + Math.max(0, Math.floor(carHeight * 0.25)), 1, 1, `rgb(${windowColor.r},${windowColor.g},${windowColor.b})`, 0.7 * intensity);
					}
				}
				// Wheels.
				for (let wx = 0; wx < geom.carLen; wx += 2) {
					this._fillCell(ctx, sx, sy, ceilSx, ceilSy, carLeftX + wx, baseY + 1, 1, 1, `rgb(${wheelColor.r},${wheelColor.g},${wheelColor.b})`, 0.85);
				}
				// Coupling bar.
				const couplingX = dir > 0 ? carLeftX + geom.carLen : carLeftX - 1;
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, couplingX, baseY, 1, 1, `rgb(${wheelColor.r},${wheelColor.g},${wheelColor.b})`, 0.7);
				void offset;
			}

			for (let x = -4; x <= 4; x++) {
				this._fillCell(ctx, sx, sy, ceilSx, ceilSy, headlightX + x, baseY + 2, 1, 1, `rgb(${lightColor.r},${lightColor.g},${lightColor.b})`, haloAlpha * 0.16);
			}
		}
	}

	api.presets['train'] = [
		{
			key: 'distant-freight',
			label: 'distant freight',
			config: {
				horizon: 0.7,
				track_y: 0.8,
				loco_len: 8,
				car_len: 6,
				cars: 4,
				train_height: 5,
				light_glow: 0.42,
				smoke: 0.5,
				cue_lead: 18,
				tail_linger: 18,
				hue: 220,
				hue_sp: 16,
				sat: 0.32,
				lmin: 0.12,
				lmax: 0.68,
				pass_p: 0.0008,
				express_p: 0.0,
				quiet_p: 0.0014,
				pass_dur: 220,
				quiet_dur: 360,
				quiet_mult: 0.1,
			},
		},
		{
			key: 'night-local',
			label: 'night local',
			config: {
				horizon: 0.66,
				track_y: 0.78,
				loco_len: 6,
				car_len: 5,
				cars: 2,
				train_height: 4.5,
				light_glow: 0.7,
				smoke: 0.18,
				cue_lead: 22,
				tail_linger: 14,
				hue: 230,
				hue_sp: 22,
				sat: 0.46,
				lmin: 0.08,
				lmax: 0.74,
				pass_p: 0.0011,
				express_p: 0.0,
				quiet_p: 0.0011,
				pass_dur: 180,
				quiet_dur: 280,
				quiet_mult: 0.18,
			},
		},
		{
			key: 'steady-passing',
			label: 'steady passing',
			config: {
				horizon: 0.7,
				track_y: 0.78,
				loco_len: 7,
				car_len: 6,
				cars: 3,
				train_height: 5,
				light_glow: 0.45,
				smoke: 0.3,
				cue_lead: 14,
				tail_linger: 12,
				hue: 218,
				hue_sp: 18,
				sat: 0.42,
				lmin: 0.1,
				lmax: 0.78,
				pass_p: 0.0018,
				express_p: 0.0,
				quiet_p: 0.0006,
				pass_dur: 160,
				quiet_dur: 200,
				quiet_mult: 0.2,
			},
		},
		{
			key: 'express-line',
			label: 'express line',
			config: {
				horizon: 0.68,
				track_y: 0.76,
				loco_len: 7,
				car_len: 7,
				cars: 4,
				train_height: 5,
				light_glow: 0.7,
				smoke: 0.18,
				cue_lead: 10,
				tail_linger: 10,
				hue: 212,
				hue_sp: 22,
				sat: 0.5,
				lmin: 0.1,
				lmax: 0.84,
				pass_p: 0.0006,
				express_p: 0.0014,
				quiet_p: 0.0007,
				pass_dur: 150,
				express_dur: 100,
				express_speed_mult: 1.85,
				quiet_dur: 220,
				quiet_mult: 0.25,
			},
		},
	];
	api.effects['train'] = Train;
})(window.AmbienceSim);
