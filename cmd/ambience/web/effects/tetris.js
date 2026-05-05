'use strict';
(function (api) {
	const { makeRNG, hslToRGB } = api._helpers;

	// Tetris — slow ambient tetromino effect. Mirrors sim/tetris.go: same
	// piece shapes, same lifecycle, and the same mulberry32 sequence RNG so
	// authority and browser agree on piece kinds/columns/rotations.
	const TETRIS_PIECES = {
		1: { hue: 0,    rotations: [
			[[0,0],[0,1],[0,2],[0,3]],
			[[0,0],[1,0],[2,0],[3,0]],
		]},
		2: { hue: -120, rotations: [
			[[0,0],[0,1],[1,0],[1,1]],
		]},
		3: { hue: 105,  rotations: [
			[[0,0],[0,1],[0,2],[1,1]],
			[[0,0],[1,0],[1,1],[2,0]],
			[[0,1],[1,0],[1,1],[1,2]],
			[[0,1],[1,0],[1,1],[2,1]],
		]},
		4: { hue: -60,  rotations: [
			[[0,1],[0,2],[1,0],[1,1]],
			[[0,0],[1,0],[1,1],[2,1]],
		]},
		5: { hue: 180,  rotations: [
			[[0,0],[0,1],[1,1],[1,2]],
			[[0,1],[1,0],[1,1],[2,0]],
		]},
		6: { hue: 45,   rotations: [
			[[0,0],[1,0],[1,1],[1,2]],
			[[0,0],[0,1],[1,0],[2,0]],
			[[0,0],[0,1],[0,2],[1,2]],
			[[0,1],[1,1],[2,0],[2,1]],
		]},
		7: { hue: -150, rotations: [
			[[0,2],[1,0],[1,1],[1,2]],
			[[0,0],[1,0],[2,0],[2,1]],
			[[0,0],[0,1],[0,2],[1,0]],
			[[0,0],[0,1],[1,1],[2,1]],
		]},
	};

	const TETRIS_DEFAULTS = {
		intro_dur: 60, intro_h: 0, intro_first: 8,
		ending_dur: 80, ending_linger: 60,
		board_w: 10, board_h: 20,
		fall_every: 14, spawn_pause: 18, lock_delay: 6,
		hue: 200, hue_sp: 0, sat: 0.55, lmin: 0.4, lmax: 0.66, ghost: 0,
		lull_p: 0, lull_dur: 80,
		fill_thresh: 0.85,
	};

	function applyTetrisDefaults(cfg) {
		const c = Object.assign({}, TETRIS_DEFAULTS, cfg || {});
		if (c.intro_dur <= 0) c.intro_dur = TETRIS_DEFAULTS.intro_dur;
		if (c.intro_h < 0) c.intro_h = 0;
		if (c.intro_h > 0.85) c.intro_h = 0.85;
		if (c.intro_first < 0) c.intro_first = 0;
		if (c.ending_dur <= 0) c.ending_dur = TETRIS_DEFAULTS.ending_dur;
		if (c.ending_linger < 0) c.ending_linger = 0;
		if (c.board_w < 6) c.board_w = 6;
		if (c.board_w > 24) c.board_w = 24;
		if (c.board_h < 10) c.board_h = 10;
		if (c.board_h > 32) c.board_h = 32;
		if (c.fall_every <= 0) c.fall_every = TETRIS_DEFAULTS.fall_every;
		if (c.spawn_pause <= 0) c.spawn_pause = TETRIS_DEFAULTS.spawn_pause;
		if (c.lock_delay <= 0) c.lock_delay = TETRIS_DEFAULTS.lock_delay;
		if (c.intro_first <= 0) c.intro_first = TETRIS_DEFAULTS.intro_first;
		if (c.sat <= 0) c.sat = TETRIS_DEFAULTS.sat;
		if (c.lmin <= 0) c.lmin = TETRIS_DEFAULTS.lmin;
		if (c.lmax <= 0) c.lmax = TETRIS_DEFAULTS.lmax;
		if (c.lmax < c.lmin) [c.lmin, c.lmax] = [c.lmax, c.lmin];
		if (c.ghost < 0) c.ghost = 0;
		if (c.ghost > 0.6) c.ghost = 0.6;
		if (c.lull_p < 0) c.lull_p = 0;
		if (c.lull_dur <= 0) c.lull_dur = TETRIS_DEFAULTS.lull_dur;
		if (c.fill_thresh <= 0) c.fill_thresh = TETRIS_DEFAULTS.fill_thresh;
		if (c.fill_thresh > 1) c.fill_thresh = 1;
		return c;
	}

	function tetrisSeqUint32(stateRef) {
		// Math.imul produces 32-bit signed multiplication, matching Go's
		// uint32 multiply when interpreted bitwise. >>> 0 keeps everything
		// in the unsigned-32 range.
		stateRef.s = (stateRef.s + 0x6D2B79F5) >>> 0;
		let z = stateRef.s;
		z = Math.imul(z ^ (z >>> 15), z | 1);
		z = (z ^ (z + Math.imul(z ^ (z >>> 7), z | 61))) >>> 0;
		return (z ^ (z >>> 14)) >>> 0;
	}

	function tetrisShapeExtent(kind, rot) {
		const piece = TETRIS_PIECES[kind];
		if (!piece || rot < 0 || rot >= piece.rotations.length) return [0, 0];
		let w = 0, h = 0;
		for (const [r, c] of piece.rotations[rot]) {
			if (c + 1 > w) w = c + 1;
			if (r + 1 > h) h = r + 1;
		}
		return [w, h];
	}

	function tetrisPieceHueBase(cfg, kind) {
		const piece = TETRIS_PIECES[kind];
		if (!piece) return cfg.hue;
		let h = cfg.hue + piece.hue;
		while (h < 0) h += 360;
		while (h >= 360) h -= 360;
		return h;
	}

	class Tetris {
		constructor(w, h, cfg, seed) {
			this.kind = 'tetris';
			this.w = w;
			this.h = h;
			this.grid = new Uint8ClampedArray(w * h * 3);
			this.cfg = applyTetrisDefaults(cfg);
			this.seed = Number(seed || Date.now());
			this.rng = makeRNG(this.seed);
			this.boardW = this.cfg.board_w;
			this.boardH = this.cfg.board_h;
			this.cells = new Uint8Array(this.boardW * this.boardH);
			this.hues = new Float32Array(this.boardW * this.boardH);
			this.tick = 0;
			this.fallTimer = 0;
			this.spawnPause = this.cfg.intro_first;
			this.lullPause = 0;
			this.pieceIndex = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.seqState = { s: (this.seed >>> 0) || 0x9e3779b9 };
			this.active = null;
			this.nextKind = 1 + (tetrisSeqUint32(this.seqState) % 7);
		}

		setConfig(cfg) {
			const next = applyTetrisDefaults(Object.assign({}, this.cfg, cfg));
			if (next.board_w !== this.boardW || next.board_h !== this.boardH) {
				this.boardW = next.board_w;
				this.boardH = next.board_h;
				this.cells = new Uint8Array(this.boardW * this.boardH);
				this.hues = new Float32Array(this.boardW * this.boardH);
				this.active = null;
				this.fallTimer = 0;
				this.spawnPause = next.intro_first;
			}
			this.cfg = next;
		}

		restoreSnapshot(snap) {
			const state = snap.state || snap;
			this.setConfig(snap.config || {});
			this.tick = state.tick || 0;
			if (state.boardW > 0 && state.boardH > 0) {
				this.boardW = state.boardW;
				this.boardH = state.boardH;
			}
			const total = this.boardW * this.boardH;
			this.cells = new Uint8Array(total);
			this.hues = new Float32Array(total);
			if (state.cells) {
				if (typeof state.cells === 'string') {
					// Go encoding/json marshals []byte as a base64 string.
					const bin = atob(state.cells);
					const n = Math.min(total, bin.length);
					for (let i = 0; i < n; i++) this.cells[i] = bin.charCodeAt(i) & 0xff;
				} else if (state.cells.length === total) {
					for (let i = 0; i < total; i++) this.cells[i] = state.cells[i] | 0;
				}
			}
			if (Array.isArray(state.hues) && state.hues.length === total) {
				for (let i = 0; i < total; i++) this.hues[i] = +state.hues[i];
			}
			this.active = state.active ? Object.assign({}, state.active) : null;
			this.nextKind = state.nextKind || (1 + (tetrisSeqUint32(this.seqState) % 7));
			this.fallTimer = state.fallTimer || 0;
			this.spawnPause = state.spawnPause || 0;
			this.lullPause = state.lullPause || 0;
			this.pieceIndex = state.pieceIndex || 0;
			this.introTicks = state.introTicks || 0;
			this.introTotal = state.introTotal || 0;
			this.endingTicks = state.endingTicks || 0;
			this.endingTotal = state.endingTotal || 0;
			this.endingFade = state.endingFade || 0;
			if (typeof snap.seed === 'number') {
				this.seed = snap.seed;
				this.rng = makeRNG(snap.seed);
			}
			if (typeof state.rngState === 'number' && state.rngState !== 0) {
				this.seqState.s = state.rngState >>> 0;
			}
			if (snap.gridW > 0 && snap.gridH > 0) {
				this.w = snap.gridW;
				this.h = snap.gridH;
			}
		}

		triggerEvent(name) {
			switch (name) {
				case 'intro':
					this._startIntro();
					return true;
				case 'ending':
					this._startEnding();
					return true;
				case 'new-piece':
					this.spawnPause = 0;
					this.lullPause = 0;
					this._spawnNext();
					return true;
				case 'lull':
					this.lullPause = this.cfg.lull_dur;
					return true;
			}
			return false;
		}

		step() {
			this.tick++;
			api._helpers.paintProceduralGrid(this);
			if (this.endingTicks > 0) {
				this.endingTicks--;
				if (this.endingTicks === 0) this._startIntro();
				return;
			}
			if (this.introTicks > 0) this.introTicks--;
			if (!this.active) {
				if (this.lullPause > 0) { this.lullPause--; return; }
				if (this.spawnPause > 0) { this.spawnPause--; return; }
				if (!this._spawnNext()) this._startEnding();
				return;
			}
			if (this.active.locking) {
				this.active.lockTick++;
				if (this.active.lockTick < this.cfg.lock_delay) return;
				this._lockActive();
				this.fallTimer = 0;
				this.spawnPause = this.cfg.spawn_pause;
				if (this.cfg.lull_p > 0 && this.rng() < this.cfg.lull_p) {
					this.lullPause = this.cfg.lull_dur;
				}
				if (this._fillRatio() >= this.cfg.fill_thresh) this._startEnding();
				return;
			}
			this.fallTimer++;
			if (this.fallTimer < this.cfg.fall_every) return;
			this.fallTimer = 0;
			if (this._canPlace(this.active.kind, this.active.rot, this.active.row + 1, this.active.col)) {
				this.active.row++;
				return;
			}
			this.active.locking = true;
			this.active.lockTick = 0;
		}

		render(ctx, canvasW, canvasH, opts) {
			api._helpers.renderPixelGridEffect(this, ctx, canvasW, canvasH, opts);
		}

		_fillCell(ctx, x, y, size, hue, alpha) {
			const baseLight = (this.cfg.lmin + this.cfg.lmax) * 0.5;
			const inner = hslToRGB(hue, this.cfg.sat, baseLight);
			const edge = hslToRGB(hue, this.cfg.sat, Math.max(0.08, this.cfg.lmin * 0.6));
			ctx.globalAlpha = alpha == null ? 1 : alpha;
			ctx.fillStyle = `rgb(${edge.r},${edge.g},${edge.b})`;
			ctx.fillRect(x, y, size, size);
			const inset = Math.max(1, Math.floor(size * 0.18));
			ctx.fillStyle = `rgb(${inner.r},${inner.g},${inner.b})`;
			ctx.fillRect(x + inset, y + inset, Math.max(1, size - inset * 2), Math.max(1, size - inset * 2));
			ctx.globalAlpha = 1;
		}

		_canPlace(kind, rot, row, col) {
			const piece = TETRIS_PIECES[kind];
			if (!piece || rot < 0 || rot >= piece.rotations.length) return false;
			for (const [dr, dc] of piece.rotations[rot]) {
				const r = row + dr;
				const c = col + dc;
				if (r < 0) continue;
				if (r >= this.boardH || c < 0 || c >= this.boardW) return false;
				if (this.cells[r * this.boardW + c] !== 0) return false;
			}
			return true;
		}

		_dropRow(kind, rot, row, col) {
			let r = row;
			while (this._canPlace(kind, rot, r + 1, col)) r++;
			return r;
		}

		_spawnNext() {
			let kind = this.nextKind;
			if (!kind) kind = 1 + (tetrisSeqUint32(this.seqState) % 7);
			this.nextKind = 1 + (tetrisSeqUint32(this.seqState) % 7);
			this.pieceIndex++;
			const piece = TETRIS_PIECES[kind];
			let rot = 0;
			if (piece && piece.rotations.length > 1) {
				rot = tetrisSeqUint32(this.seqState) % piece.rotations.length;
			}
			const [bw] = tetrisShapeExtent(kind, rot);
			const maxCol = Math.max(0, this.boardW - bw);
			let col = 0;
			if (maxCol > 0) col = tetrisSeqUint32(this.seqState) % (maxCol + 1);
			if (!this._canPlace(kind, rot, 0, col)) {
				this.active = null;
				return false;
			}
			this.active = {
				kind: kind,
				rot: rot,
				row: 0,
				col: col,
				hue: tetrisPieceHueBase(this.cfg, kind),
				locking: false,
				lockTick: 0,
			};
			this.fallTimer = 0;
			return true;
		}

		_lockActive() {
			if (!this.active) return;
			const piece = TETRIS_PIECES[this.active.kind];
			if (!piece) { this.active = null; return; }
			for (const [dr, dc] of piece.rotations[this.active.rot]) {
				const r = this.active.row + dr;
				const c = this.active.col + dc;
				if (r < 0 || r >= this.boardH || c < 0 || c >= this.boardW) continue;
				const idx = r * this.boardW + c;
				this.cells[idx] = this.active.kind;
				this.hues[idx] = this.active.hue;
			}
			this.active = null;
		}

		_fillRatio() {
			if (!this.cells.length) return 0;
			let filled = 0;
			for (let i = 0; i < this.cells.length; i++) if (this.cells[i] !== 0) filled++;
			return filled / this.cells.length;
		}

		_startIntro() {
			// Clear the board locally; the authoritative debris layout (when
			// intro_h > 0) arrives via the next server snapshot. We don't
			// reproduce it here because the Go and JS RNGs intentionally
			// differ (Splitmix64 vs Mulberry32), so a local fill would drift
			// from the server's state.
			this.cells = new Uint8Array(this.boardW * this.boardH);
			this.hues = new Float32Array(this.boardW * this.boardH);
			this.active = null;
			this.spawnPause = this.cfg.intro_first;
			this.lullPause = 0;
			this.fallTimer = 0;
			this.introTotal = this.cfg.intro_dur > 0 ? this.cfg.intro_dur : TETRIS_DEFAULTS.intro_dur;
			this.introTicks = this.introTotal;
			this.endingTicks = 0;
			this.endingTotal = 0;
			this.endingFade = 0;
			this.nextKind = 1 + (tetrisSeqUint32(this.seqState) % 7);
		}

		_startEnding() {
			this.active = null;
			this.spawnPause = 0;
			this.lullPause = 0;
			this.fallTimer = 0;
			this.introTicks = 0;
			this.introTotal = 0;
			this.endingFade = this.cfg.ending_dur > 0 ? this.cfg.ending_dur : TETRIS_DEFAULTS.ending_dur;
			const linger = Math.max(0, this.cfg.ending_linger);
			this.endingTotal = Math.max(1, this.endingFade + linger);
			this.endingTicks = this.endingTotal;
		}

	}

	api.presets['tetris'] = [
		{
			key: 'museum-pace',
			label: 'museum pace',
			config: {
				intro_dur: 80,
				intro_h: 0,
				intro_first: 12,
				ending_dur: 100,
				ending_linger: 90,
				board_w: 10,
				board_h: 20,
				fall_every: 22,
				spawn_pause: 36,
				lock_delay: 10,
				hue: 200,
				hue_sp: 4,
				sat: 0.42,
				lmin: 0.36,
				lmax: 0.62,
				ghost: 0.05,
				lull_p: 0.012,
				lull_dur: 140,
				fill_thresh: 0.92,
			},
		},
		{
			key: 'steady-build',
			label: 'steady build',
			config: {
				intro_dur: 60,
				intro_h: 0.05,
				intro_first: 8,
				ending_dur: 80,
				ending_linger: 60,
				board_w: 10,
				board_h: 20,
				fall_every: 14,
				spawn_pause: 18,
				lock_delay: 6,
				hue: 200,
				hue_sp: 0,
				sat: 0.55,
				lmin: 0.4,
				lmax: 0.66,
				ghost: 0,
				lull_p: 0.004,
				lull_dur: 80,
				fill_thresh: 0.85,
			},
		},
		{
			key: 'dense-stack',
			label: 'dense stack',
			config: {
				intro_dur: 50,
				intro_h: 0.25,
				intro_first: 4,
				ending_dur: 60,
				ending_linger: 40,
				board_w: 10,
				board_h: 22,
				fall_every: 10,
				spawn_pause: 8,
				lock_delay: 4,
				hue: 18,
				hue_sp: 8,
				sat: 0.7,
				lmin: 0.42,
				lmax: 0.74,
				ghost: 0.08,
				lull_p: 0.0,
				lull_dur: 60,
				fill_thresh: 0.94,
			},
		},
		{
			key: 'late-game',
			label: 'late game',
			config: {
				intro_dur: 40,
				intro_h: 0.55,
				intro_first: 2,
				ending_dur: 50,
				ending_linger: 30,
				board_w: 10,
				board_h: 20,
				fall_every: 8,
				spawn_pause: 4,
				lock_delay: 3,
				hue: 0,
				hue_sp: 12,
				sat: 0.78,
				lmin: 0.44,
				lmax: 0.78,
				ghost: 0.12,
				lull_p: 0.0,
				lull_dur: 60,
				fill_thresh: 0.98,
			},
		},
	];
	api.effects['tetris'] = Tetris;
})(window.AmbienceSim);
