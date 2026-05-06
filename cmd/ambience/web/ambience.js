// Ambience — browser canvas renderer for the ambience SSE stream.
//
// Usage:
//   new Ambience({ canvas: <HTMLCanvasElement>, stream: '/stream' });
//
// The constructor connects to the server's SSE stream, parses each frame
// ({ w, h, pixels: [{x, y, r, g, b}, ...] }) and renders it onto the canvas,
// scaling to fill the canvas's current pixel size. Handles window resize.
//
// Zero deps. Zero coupling to any app the canvas sits in.

class Ambience {
	constructor({ canvas, stream }) {
		this.canvas = canvas;
		this.ctx = canvas.getContext('2d');
		if (this.canvas.style) this.canvas.style.imageRendering = this.canvas.style.imageRendering || 'pixelated';
		this.ctx.imageSmoothingEnabled = false;
		this.frame = null;

		this._resize = () => this.resize();
		this.resize();
		window.addEventListener('resize', this._resize);

		this.es = new EventSource(stream);
		this.es.onmessage = (e) => {
			try {
				this.frame = JSON.parse(e.data);
				this.draw();
			} catch (err) {
				console.error('ambience parse error', err);
			}
		};
		this.es.onerror = (err) => {
			// EventSource auto-reconnects; log for visibility only.
			console.warn('ambience SSE error (will retry)', err);
		};
	}

	resize() {
		const dpr = window.devicePixelRatio || 1;
		this.canvas.width = window.innerWidth * dpr;
		this.canvas.height = window.innerHeight * dpr;
		this.draw();
	}

	draw() {
		if (!this.frame) return;
		const { w, h, pixels } = this.frame;
		const cw = this.canvas.width;
		const ch = this.canvas.height;
		const sx = cw / w;
		const sy = ch / h;

		this.ctx.fillStyle = '#0a0a0a';
		this.ctx.fillRect(0, 0, cw, ch);

		for (const p of (pixels || [])) {
			this.ctx.fillStyle = `rgb(${p.r},${p.g},${p.b})`;
			this.ctx.fillRect(Math.floor(p.x * sx), Math.floor(p.y * sy), Math.ceil(sx), Math.ceil(sy));
		}
	}

	close() {
		if (this.es) this.es.close();
		window.removeEventListener('resize', this._resize);
	}
}

window.Ambience = Ambience;
