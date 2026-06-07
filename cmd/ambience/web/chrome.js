'use strict';
/*
 * ambience chrome — "Exposed" control/monitor surface, shared by dev.html and
 * index.html. Builds the hairline schematic that Esc summons over the full-bleed
 * pixel world: crop marks, identity, telemetry, the 5-slot control surface, and
 * the bottom track + event log. Styling lives in /chrome.css.
 *
 * Pure presentation: the page owns all data/IO and hands this module values +
 * callbacks. window.AmbienceChrome.mount(opts) returns a controller.
 */
window.AmbienceChrome = (function () {
	const SLOT_LABELS = {
		'spawn': 'spawn config', 'lever': 'continuous levers', 'event': 'discrete events',
		'event-mod': 'event modifiers', 'end': 'end conditions',
	};
	const SLOT_ORDER = ['spawn', 'lever', 'event', 'event-mod', 'end'];
	const CUSTOM_PRESET = '__custom__';

	// ── tiny DOM helper ────────────────────────────────────────────────────
	function h(tag, attrs, ...kids) {
		const el = document.createElement(tag);
		if (attrs) for (const k in attrs) {
			const v = attrs[k];
			if (v == null || v === false) continue;
			if (k === 'class') el.className = v;
			else if (k === 'html') el.innerHTML = v;
			else if (k === 'on' && typeof v === 'object') for (const ev in v) el.addEventListener(ev, v[ev]);
			else if (k in el && k !== 'list') { try { el[k] = v; } catch (_) { el.setAttribute(k, v); } }
			else el.setAttribute(k, v);
		}
		for (const kid of kids.flat()) {
			if (kid == null || kid === false) continue;
			el.appendChild((typeof kid === 'string' || typeof kid === 'number') ? document.createTextNode(String(kid)) : kid);
		}
		return el;
	}

	const ICONS = {
		dice: '<rect x="3" y="3" width="10" height="10" rx="2"/><circle cx="6" cy="6" r="0.7" fill="currentColor" stroke="none"/><circle cx="10" cy="10" r="0.7" fill="currentColor" stroke="none"/><circle cx="10" cy="6" r="0.7" fill="currentColor" stroke="none"/><circle cx="6" cy="10" r="0.7" fill="currentColor" stroke="none"/>',
		chevD: '<polyline points="3,6 8,11 13,6"/>',
		check: '<polyline points="3,8.5 6.5,12 13,4"/>',
		next: '<polyline points="4,3 9,8 4,13"/><line x1="12" y1="3" x2="12" y2="13"/>',
		lock: '<rect x="3.5" y="7" width="9" height="6" rx="1"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/>',
		unlock: '<rect x="3.5" y="7" width="9" height="6" rx="1"/><path d="M5.5 7V5a2.5 2.5 0 0 1 4.8-1"/>',
		eye: '<path d="M1.5 8s2.5-4.5 6.5-4.5S14.5 8 14.5 8s-2.5 4.5-6.5 4.5S1.5 8 1.5 8z"/><circle cx="8" cy="8" r="1.9"/>',
		signout: '<path d="M9.5 3.5H4a1 1 0 0 0-1 1v7a1 1 0 0 0 1 1h5.5"/><polyline points="11,5.5 13.5,8 11,10.5"/><line x1="13.5" y1="8" x2="7" y2="8"/>',
	};
	function icon(name, size) {
		size = size || 14;
		return '<svg width="' + size + '" height="' + size + '" viewBox="0 0 16 16" fill="none" stroke="currentColor" ' +
			'stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' + (ICONS[name] || '') + '</svg>';
	}

	// crunchy bitmap wordmark — render small in Archivo, upscale nearest-neighbour
	function pixelText(text, opts) {
		opts = opts || {};
		const size = opts.size || 13, scale = opts.scale || 2, weight = opts.weight || 700,
			color = opts.color || '#ffffff', letter = opts.letter || 0;
		const cv = h('canvas', { class: 'pixeltext' });
		function draw() {
			const m0 = cv.getContext('2d');
			const pad = 2;
			m0.font = weight + ' ' + size + 'px Archivo, sans-serif';
			const w = Math.ceil(m0.measureText(text).width) + pad * 2 + (text.length - 1) * letter;
			const hh = Math.ceil(size * 1.32) + pad * 2;
			cv.width = w; cv.height = hh;
			const c = cv.getContext('2d');
			c.clearRect(0, 0, w, hh);
			c.font = weight + ' ' + size + 'px Archivo, sans-serif';
			c.textBaseline = 'middle';
			c.fillStyle = color;
			let x = pad;
			for (const ch of text) { c.fillText(ch, x, hh / 2); x += c.measureText(ch).width + letter; }
			cv.style.width = (w * scale) + 'px';
			cv.style.height = (hh * scale) + 'px';
		}
		draw();
		if (document.fonts && document.fonts.ready) document.fonts.ready.then(draw);
		return cv;
	}

	function fmtKnob(knob, v) {
		if (knob.type === 'bool') return Number(v) ? 'on' : 'off';
		v = Number(v);
		if (!Number.isFinite(v)) return '';
		if (knob.type === 'int') return String(Math.round(v));
		const d = (knob.step < 0.1) ? 2 : (knob.step < 1 ? 1 : 0);
		return v.toFixed(d) + ((knob.key === 'hue' || knob.key === 'hue_sp') ? '°' : '');
	}
	function effectLabel(name) { return String(name || '').replace(/-/g, ' '); }
	function presetsFor(name) {
		const p = window.AmbienceSim && window.AmbienceSim.presets && window.AmbienceSim.presets[name];
		return Array.isArray(p) ? p : [];
	}
	function presetVal(p) { return p.key || p.name; }
	function presetCfg(p) { return (p && (p.config || p.values)) || {}; }

	// ── controller ─────────────────────────────────────────────────────────
	function mount(opts) {
		const C = {
			host: opts.host, canvas: opts.canvas, hint: opts.hint || null,
			mode: opts.mode, gridW: opts.gridW || 320, gridH: opts.gridH || 180,
			transition: opts.transition || 'cinematic',
			handlers: opts.handlers || {},
			summoned: opts.summoned !== false,
			effect: opts.effect || 'rain',
			locked: !!opts.locked,
			schema: null, knobs: [],
			refs: {}, presetOpen: false,
		};

		const refs = C.refs;
		const root = h('div', { class: 'd5 d5--' + C.transition });
		refs.root = root;

		root.appendChild(h('div', { class: 'd5__grid' }));
		root.appendChild(h('div', { class: 'd5__scan' }));
		root.appendChild(h('div', { class: 'd5__flash' }));
		root.appendChild(h('div', { class: 'd5__crop d5__crop--tl' }));
		root.appendChild(h('div', { class: 'd5__crop d5__crop--tr' }));
		root.appendChild(h('div', { class: 'd5__crop d5__crop--bl' }));
		root.appendChild(h('div', { class: 'd5__crop d5__crop--br' }));
		root.appendChild(h('div', { class: 'd5__framelbl' }, C.gridW + ' × ' + C.gridH + ' cells · rendering local replica'));

		// identity
		const sceneWrap = h('div', { class: 'd5__scene' });
		refs.sceneWrap = sceneWrap;
		const lineEl = h('p', { class: 'd5__line' }, opts.line || '');
		refs.lineEl = lineEl;
		const idrow = h('div', { class: 'd5__idrow' },
			pixelText('ambience', { size: 13, scale: 2, weight: 800, color: '#f4f4f4' }),
			h('span', { class: 'chip chip--' + (C.mode === 'live' ? 'live' : 'dev') },
				h('span', { class: 'dot dot--' + (C.mode === 'live' ? 'live' : 'dev') }),
				C.mode === 'live' ? 'live' : 'dev'));
		if (opts.routeJump) {
			idrow.appendChild(h('button', { class: 'modeswitch', title: opts.routeJump.title || '',
				on: { click: opts.routeJump.onClick } }, opts.routeJump.label));
		}
		const idEl = h('div', { class: 'd5__id' }, idrow, sceneWrap, lineEl);
		refs.idEl = idEl;
		root.appendChild(idEl);

		// telemetry
		root.appendChild(buildTele(C, opts.tele || {}));

		// control surface
		const ctlBody = h('div', { class: 'd5__ctlbody' });
		refs.ctlBody = ctlBody;
		const head = h('div', { class: 'd5__ctlhead' },
			h('span', { class: 'd5__ctltitle' }, (opts.ctl && opts.ctl.title) || 'control surface'),
			h('span', { class: 'd5__ctlmode' }, (opts.ctl && opts.ctl.modeLabel) || ''));
		if (opts.ctl && opts.ctl.showRandomize) {
			head.appendChild(h('button', { class: 'iconbtn', id: 'd5RandomizeBtn', title: 'randomize current effect',
				html: icon('dice', 13), on: { click: () => C.handlers.onRandomize && C.handlers.onRandomize() } }));
		}
		const ctl = h('div', { class: 'd5__ctl' + (C.locked ? ' is-locked' : '') }, head);
		refs.ctl = ctl;
		if (opts.showAuth) {
			const authWrap = h('div', { class: 'd5__authwrap' });
			refs.authWrap = authWrap;
			ctl.appendChild(authWrap);
		}
		const note = h('div', { class: 'd5__ctlnote' }, (opts.ctl && opts.ctl.note) || '');
		refs.note = note;
		ctl.appendChild(note);
		ctl.appendChild(ctlBody);
		root.appendChild(ctl);

		// bottom feed — broadcast track only. The event log used to share this
		// bar and floated over the bottom of the pixel world; it now lives in the
		// right rail (below the authority clock) where there was dead space.
		const trk = h('div', { class: 'd5__trk', id: 'd5Trk' });
		refs.trk = trk;
		const trkWrap = h('div', { class: 'd5__trkwrap' },
			h('span', { class: 'd5__feedlbl' }, opts.feedLabel || 'effects'), trk);
		if (opts.showNext) {
			const nextBtn = h('button', { class: 'd5__nextbtn', disabled: true, title: 'advance the broadcast',
				html: icon('next', 11) + '<span>next effect</span>',
				on: { click: () => C.handlers.onNext && C.handlers.onNext() } });
			refs.nextBtn = nextBtn;
			trkWrap.appendChild(nextBtn);
		}
		const feed = h('div', { class: 'd5__feed' }, trkWrap);
		refs.feed = feed;
		root.appendChild(feed);

		// event log — docked in the right rail beneath the authority clock so it
		// reads as authority telemetry and stops covering the world. Collapsible:
		// the caret (or header) hides the body for an unobstructed view.
		const logBody = h('div', { class: 'd5__logbody', id: 'd5Log' });
		refs.logBody = logBody;
		const logToggle = h('button', { class: 'd5__logtoggle', 'aria-expanded': 'true',
			title: 'collapse the event log', html: icon('chevD', 11) });
		const logClear = h('button', { class: 'd5__logclear', title: 'clear the event log',
			on: { click: (e) => { e.stopPropagation(); logBody.innerHTML = ''; scheduleChromeLayout(C); } } }, 'clear');
		const logHead = h('div', { class: 'd5__teleh d5__teleh--state d5__loghead' },
			h('span', { class: 'd5__logheadlbl' }, logToggle, h('span', null, 'event log')),
			logClear);
		const logWrap = h('div', { class: 'd5__log' }, logHead, logBody);
		refs.log = logWrap;
		function toggleLog() {
			C.logCollapsed = !C.logCollapsed;
			logWrap.classList.toggle('is-collapsed', C.logCollapsed);
			logToggle.setAttribute('aria-expanded', String(!C.logCollapsed));
			logToggle.title = (C.logCollapsed ? 'expand' : 'collapse') + ' the event log';
			scheduleChromeLayout(C);
		}
		logToggle.addEventListener('click', (e) => { e.stopPropagation(); toggleLog(); });
		logHead.addEventListener('click', toggleLog);
		(refs.teleEl || root).appendChild(logWrap);

		C.host.appendChild(root);
		window.addEventListener('resize', () => scheduleChromeLayout(C), { passive: true });
		if (document.fonts && document.fonts.ready) document.fonts.ready.then(() => scheduleChromeLayout(C));

		// outside-click closes preset popover
		window.addEventListener('mousedown', (e) => {
			if (C.presetOpen && refs.preset && !refs.preset.wrap.contains(e.target)) closePreset(C);
		});
		// Esc summon/dismiss
		window.addEventListener('keydown', (e) => {
			const t = e.target.tagName;
			if (t === 'INPUT' || t === 'SELECT' || t === 'TEXTAREA') return;
			if (e.key === 'Escape') { e.preventDefault(); api.setSummoned(!C.summoned); }
		});

		// ── controller API ───────────────────────────────────────────────────
		const api = {
			refs,
			setLine(text) { refs.lineEl.textContent = text; scheduleChromeLayout(C); },
			setScene(name) {
				refs.sceneWrap.innerHTML = '';
				refs.sceneWrap.appendChild(pixelText(effectLabel(name), { size: 18, scale: 2, weight: 800, color: '#ffffff' }));
				scheduleChromeLayout(C);
			},
			setTele(k, v) { if (refs.teleRows && refs.teleRows[k]) { refs.teleRows[k].textContent = v; scheduleChromeLayout(C); } },
			setTeleTone(k, tone) {
				if (!refs.teleRows || !refs.teleRows[k]) return;
				const el = refs.teleRows[k];
				el.className = 'd5__tv' + (tone ? ' is-' + tone : '');
			},
			setTeleBadge(tone, label) {
				if (!refs.teleBadge) return;
				refs.teleBadge.className = 'd5__telebadge d5__telebadge--' + tone;
				refs.teleBadgeLabel.textContent = label;
			},
			setTeleFooter(tone, html) {
				if (!refs.teleFooter) return;
				refs.teleFooter.className = 'd5__telesync d5__telesync--' + tone;
				refs.teleFooterText.innerHTML = html;
			},
			renderControls(schema) { renderControls(C, schema); },
			setControlsLocked(locked) {
				C.locked = !!locked;
				refs.ctl.classList.toggle('is-locked', C.locked);
				for (const inp of refs.ctlBody.querySelectorAll('input')) inp.disabled = C.locked;
				for (const b of refs.ctlBody.querySelectorAll('button.fire')) b.disabled = C.locked;
				if (refs.preset) refs.preset.btn.disabled = C.locked;
			},
			isLocked() { return C.locked; },
			currentValues() { return currentValues(C); },
			getKnobs() { return C.knobs.slice(); },
			getSchema() { return C.schema; },
			setValues(values) { applyValues(C, values); },
			renderTrack(items) { renderTrack(C, items); },
			setNextEnabled(on) { if (refs.nextBtn) refs.nextBtn.disabled = !on; },
			appendLog(entry) { appendLog(C, entry); },
			clearLog() { refs.logBody.innerHTML = ''; scheduleChromeLayout(C); },
			flashPanel() { flashPanel(C); },
			setAuth(state, info) { setAuth(C, state, info || {}); },
			setSummoned(on) { setSummoned(C, on); },
			toggle() { setSummoned(C, !C.summoned); },
			isOpen() { return C.summoned; },
		};
		C.api = api;
		setupHint(C);
		setSummoned(C, C.summoned);
		scheduleChromeLayout(C);
		return api;
	}

	// Keep the left control surface below the variable-height identity block.
	// The scene name is rendered as scaled bitmap text, and the intro line can
	// wrap differently once the Archivo font resolves.
	const MIN_CTL_TOP = 198;
	const CTL_GAP = 16;
	const FEED_GAP = 18;
	const MIN_CTL_HEIGHT = 96;
	function scheduleChromeLayout(C) {
		if (C._layoutFrame) cancelAnimationFrame(C._layoutFrame);
		C._layoutFrame = requestAnimationFrame(() => layoutChrome(C));
	}
	function layoutChrome(C) {
		C._layoutFrame = null;
		const ctl = C.refs.ctl, id = C.refs.idEl;
		if (!ctl || !id || !C.summoned) return;
		const tele = C.refs.teleEl;
		const narrow = window.innerWidth <= 760;
		const rootBox = C.refs.root ? C.refs.root.getBoundingClientRect() : { top: 0 };

		// Bound the right-rail event log FIRST so the authority column has a
		// known height before we measure it. On narrow the rail stacks above the
		// control surface, so an unbounded log would shove (or overlap) it; the
		// body scrolls within the cap instead.
		const logBody = C.refs.logBody;
		if (logBody) {
			if (C.logCollapsed) {
				logBody.style.maxHeight = '';
			} else {
				const bodyTop = logBody.getBoundingClientRect().top;
				const cap = narrow ? 120 : 260;
				logBody.style.maxHeight = Math.max(40, Math.min(cap, window.innerHeight - bodyTop - 24)) + 'px';
			}
		}

		const idBox = id.getBoundingClientRect();
		const feedBox = C.refs.feed ? C.refs.feed.getBoundingClientRect() : { height: 80 };
		let stackBottom = idBox.bottom;
		if (tele) {
			if (narrow) {
				const teleTop = Math.ceil(idBox.bottom - rootBox.top + CTL_GAP);
				tele.style.top = teleTop + 'px';
				tele.style.left = '';
				tele.style.right = '';
				tele.style.width = '';
				stackBottom = tele.getBoundingClientRect().bottom;
			} else {
				tele.style.top = '';
				tele.style.left = '';
				tele.style.right = '';
				tele.style.width = '';
			}
		}
		const top = Math.max(MIN_CTL_TOP, Math.ceil(stackBottom - rootBox.top + CTL_GAP));
		const bottomReserve = Math.ceil(feedBox.height + FEED_GAP);
		ctl.style.top = top + 'px';
		ctl.style.maxHeight = Math.max(MIN_CTL_HEIGHT, window.innerHeight - top - bottomReserve) + 'px';
	}

	// ── telemetry ────────────────────────────────────────────────────────
	function buildTele(C, spec) {
		const refs = C.refs;
		refs.teleRows = {};
		const head = h('div', { class: 'd5__teleh d5__teleh--state' }, h('span', null, spec.header || 'telemetry'));
		if (spec.badge) {
			const lbl = h('span', { class: 'd5__telebadgedot' });
			const badgeLabel = document.createTextNode(spec.badge.label || '');
			const badge = h('span', { class: 'd5__telebadge d5__telebadge--' + (spec.badge.tone || 'muted') }, lbl);
			badge.appendChild(badgeLabel);
			refs.teleBadge = badge; refs.teleBadgeLabel = badgeLabel;
			head.appendChild(badge);
		}
		const rowsEls = (spec.rows || []).map((r) => {
			const val = h('span', { class: 'd5__tv' }, r.v);
			refs.teleRows[r.k] = val;
			return h('div', { class: 'd5__trow' }, h('span', { class: 'd5__tk' }, r.k), val);
		});
		const footerText = h('span', null);
		footerText.innerHTML = (spec.footer && spec.footer.html) || '';
		const footer = h('div', { class: 'd5__telesync d5__telesync--' + ((spec.footer && spec.footer.tone) || 'muted') },
			h('span', { class: 'd5__syncdot' }), footerText);
		refs.teleFooter = footer; refs.teleFooterText = footerText;
		const el = h('div', { class: 'd5__tele' }, head, ...rowsEls, footer);
		refs.teleEl = el;
		return el;
	}

	// ── control surface ────────────────────────────────────────────────────
	function currentValues(C) {
		const v = {};
		for (const k of C.knobs) {
			const i = document.getElementById(k.key);
			if (!i) continue;
			v[k.key] = k.type === 'bool' ? (i.checked ? 1 : 0) : +i.value;
		}
		return v;
	}
	function updateReadouts(C) {
		for (const k of C.knobs) {
			const r = document.getElementById(k.key + '_v');
			const i = document.getElementById(k.key);
			if (r && i) r.textContent = fmtKnob(k, i.value);
		}
	}
	function applyValues(C, values) {
		for (const k of C.knobs) {
			const i = document.getElementById(k.key);
			if (!i) continue;
			const next = values && values[k.key] != null ? values[k.key] : k.default;
			if (k.type === 'bool') i.checked = !!Number(next);
			else i.value = next;
		}
		updateReadouts(C);
		refreshPreset(C);
	}
	function resolvedPresetConfig(schema, preset) {
		const r = {};
		for (const k of (schema && schema.knobs) || []) r[k.key] = k.default;
		return Object.assign(r, presetCfg(preset));
	}
	function knobMatches(knob, a, b) {
		a = Number(a); b = Number(b);
		if (!Number.isFinite(a) || !Number.isFinite(b)) return false;
		if (knob.type === 'int') return Math.round(a) === Math.round(b);
		const step = Number(knob.step) || 0.001;
		return Math.abs(a - b) <= Math.max(1e-9, step / 2);
	}
	function matchingPreset(C) {
		if (!C.schema || !C.knobs.length) return CUSTOM_PRESET;
		const vals = currentValues(C);
		for (const p of presetsFor(C.effect)) {
			const res = resolvedPresetConfig(C.schema, p);
			if (C.knobs.every((k) => knobMatches(k, vals[k.key], res[k.key]))) return presetVal(p);
		}
		return CUSTOM_PRESET;
	}

	function buildRow(C, knob) {
		if (knob.trigger) {
			return h('div', { class: 'trigrow' },
				h('span', { class: 'trigrow__name' }, knob.label),
				knob.description ? h('span', { class: 'knob__help', title: knob.description, tabIndex: 0, 'aria-label': knob.description }, '?') : null,
				h('button', { class: 'fire', disabled: C.locked, title: 'trigger this event now',
					on: { click: () => { if (!C.locked && C.handlers.onFire) C.handlers.onFire(knob.trigger); } } }, 'fire'));
		}
		if (knob.type === 'bool') {
			const input = h('input', {
				type: 'checkbox', class: 'knob__check', id: knob.key,
				checked: !!Number(knob.default), disabled: C.locked,
				on: { input: () => { updateReadouts(C); refreshPreset(C); if (!C.locked && C.handlers.onKnob) C.handlers.onKnob(knob, currentValues(C)); } },
			});
			return h('div', { class: 'knob knob--bool' },
				h('label', { class: 'knob__label', title: knob.label }, input, h('span', null, knob.label)),
				knob.description ? h('span', { class: 'knob__help', title: knob.description, tabIndex: 0, 'aria-label': knob.description }, '?') : null,
				h('span', { class: 'knob__val', id: knob.key + '_v' }, ''));
		}
		const input = h('input', {
			type: 'range', class: 'knob__range', id: knob.key,
			min: knob.min, max: knob.max, step: knob.step, value: knob.default, disabled: C.locked,
			on: { input: () => { updateReadouts(C); refreshPreset(C); if (!C.locked && C.handlers.onKnob) C.handlers.onKnob(knob, currentValues(C)); } },
		});
		return h('div', { class: 'knob' },
			h('label', { class: 'knob__label', title: knob.label }, knob.label),
			knob.description ? h('span', { class: 'knob__help', title: knob.description, tabIndex: 0, 'aria-label': knob.description }, '?') : null,
			input,
			h('span', { class: 'knob__val', id: knob.key + '_v' }, ''));
	}

	function renderControls(C, schema) {
		C.schema = schema;
		C.knobs = Array.isArray(schema && schema.knobs) ? schema.knobs.slice() : [];
		const body = C.refs.ctlBody;
		body.innerHTML = '';
		C.refs.preset = null;

		const preset = buildPreset(C);
		if (preset) body.appendChild(preset);

		const bySlot = {};
		for (const k of C.knobs) { (bySlot[k.slot] ||= {}); (bySlot[k.slot][k.group || ''] ||= []).push(k); }
		SLOT_ORDER.forEach((slot, i) => {
			const groups = bySlot[slot];
			const count = groups ? Object.values(groups).reduce((n, g) => n + g.length, 0) : 0;
			const slotEl = h('div', { class: 'd5__slot' },
				h('div', { class: 'd5__slothead' },
					h('span', { class: 'd5__slotidx' }, String(i + 1).padStart(2, '0')),
					h('span', { class: 'd5__slotname' }, SLOT_LABELS[slot] || slot),
					h('span', { class: 'd5__slotn' }, count || '—')));
			if (!groups) {
				slotEl.appendChild(h('div', { class: 'd5__slotempty' }, 'none yet'));
			} else {
				const names = Object.keys(groups);
				const showSub = !(names.length === 1 && names[0] === '');
				for (const g of names) {
					if (showSub && g) slotEl.appendChild(h('h3', { class: 'd5__group' }, g));
					for (const k of groups[g]) slotEl.appendChild(buildRow(C, k));
				}
			}
			body.appendChild(slotEl);
		});
		updateReadouts(C);
		refreshPreset(C);
		flashPanel(C);
	}

	// preset popover
	function buildPreset(C) {
		if (!presetsFor(C.effect).length) return null;
		const btnName = h('span', { class: 'preset__name' }, '');
		const btn = h('button', { class: 'preset__btn', disabled: C.locked,
			on: { click: (e) => { e.stopPropagation(); togglePreset(C); } } },
			btnName, h('span', { class: 'preset__chev', html: icon('chevD', 11) }));
		const menu = h('div', { class: 'preset__menu', style: 'display:none' });
		const wrap = h('div', { class: 'preset' + (C.locked ? ' preset--locked' : '') }, h('span', { class: 'preset__lbl' }, 'preset'), btn, menu);
		C.refs.preset = { wrap, btn, btnName, menu };
		refreshPreset(C);
		return wrap;
	}
	function togglePreset(C) {
		C.presetOpen = !C.presetOpen;
		const p = C.refs.preset; if (!p) return;
		p.btn.classList.toggle('is-open', C.presetOpen);
		p.menu.style.display = C.presetOpen ? '' : 'none';
		if (C.presetOpen) renderPresetMenu(C);
	}
	function closePreset(C) {
		C.presetOpen = false;
		if (C.refs.preset) { C.refs.preset.menu.style.display = 'none'; C.refs.preset.btn.classList.remove('is-open'); }
	}
	function renderPresetMenu(C) {
		const p = C.refs.preset; if (!p) return;
		const active = matchingPreset(C);
		p.menu.innerHTML = '';
		if (active === CUSTOM_PRESET) {
			p.menu.appendChild(h('div', { class: 'preset__item is-custom-row' },
				h('span', { class: 'preset__iname' }, 'custom'), h('span', { class: 'preset__inote' }, 'edited by hand')));
		}
		for (const pr of presetsFor(C.effect)) {
			const val = presetVal(pr);
			p.menu.appendChild(h('button', { class: 'preset__item' + (val === active ? ' is-active' : ''),
				on: { click: () => { if (C.handlers.onPreset) C.handlers.onPreset(val, resolvedPresetConfig(C.schema, pr)); closePreset(C); } } },
				h('span', { class: 'preset__check', html: val === active ? icon('check', 11) : '' }),
				h('span', { class: 'preset__iname' }, pr.label || pr.name || val),
				h('span', { class: 'preset__inote' }, pr.note || '')));
		}
	}
	function refreshPreset(C) {
		const p = C.refs.preset; if (!p) return;
		const active = matchingPreset(C);
		const cur = presetsFor(C.effect).find((x) => presetVal(x) === active);
		p.btnName.textContent = cur ? (cur.label || cur.name || active) : 'custom —';
		p.btn.classList.toggle('is-custom', !cur);
		if (C.presetOpen) renderPresetMenu(C);
	}

	// ── track + log ──────────────────────────────────────────────────────
	function renderTrack(C, items) {
		const trk = C.refs.trk;
		trk.innerHTML = '';
		for (const it of items) {
			const seg = h('button', {
				class: 'd5__seg' + (it.now ? ' is-now' : '') + (it.onClick ? ' is-click' : ''),
				title: it.title || '', disabled: it.disabled,
				on: it.onClick ? { click: it.onClick } : null,
			}, it.label, it.dur != null ? h('span', { class: 'd5__segdur tnum' }, it.dur) : null);
			trk.appendChild(seg);
		}
	}
	function appendLog(C, entry) {
		const box = C.refs.logBody;
		const stick = box.scrollHeight - box.clientHeight - box.scrollTop <= 12;
		const kind = entry.kind || 'trigger';
		box.appendChild(h('div', { class: 'logline logline--' + kind },
			h('span', { class: 'logline__t' }, entry.t != null ? entry.t : ''),
			h('span', { class: 'logline__msg' }, entry.text || '')));
		while (box.children.length > 60) box.removeChild(box.firstChild);
		if (stick) box.scrollTop = box.scrollHeight;
		// The rail's height changes as lines stream in; re-flow so the narrow
		// layout (tele stacked above the control surface) never overlaps. rAF-
		// batched, so a burst of entries coalesces into one layout pass.
		scheduleChromeLayout(C);
	}
	function flashPanel(C) {
		const el = C.refs.ctl; if (!el) return;
		el.classList.remove('d5__ctl--flash'); void el.offsetWidth; el.classList.add('d5__ctl--flash');
		setTimeout(() => el.classList.remove('d5__ctl--flash'), 700);
	}

	// ── auth bar (live) ──────────────────────────────────────────────────
	// state: 'readonly' (signed out) | 'viewonly' (signed in, locked) | 'armed'
	function setAuth(C, state, info) {
		const wrap = C.refs.authWrap; if (!wrap) return;
		const label = state === 'readonly' ? 'read only' : state === 'viewonly' ? 'view only' : 'live mutations armed';
		const ic = state === 'readonly' ? 'lock' : state === 'viewonly' ? 'eye' : 'unlock';
		const bar = h('div', { class: 'd5__auth d5__auth--' + state },
			h('span', { class: 'd5__authdot' }),
			h('span', { class: 'd5__authstate', html: icon(ic, 12) + '<span>' + label + '</span>' }));
		if (info.identity) bar.appendChild(h('span', { class: 'd5__authid', title: 'signed in' }, info.identity));
		const actions = h('div', { class: 'd5__authactions' });
		if (state === 'readonly') {
			actions.appendChild(h('button', { class: 'authbtn authbtn--primary', on: { click: () => C.handlers.onSignIn && C.handlers.onSignIn() } }, info.signInLabel || 'sign in'));
		} else {
			actions.appendChild(h('button', { class: 'authbtn' + (state === 'armed' ? ' is-armed' : ''),
				html: icon(state === 'armed' ? 'unlock' : 'lock', 11) + '<span>' + (state === 'armed' ? 'disarm' : 'arm') + '</span>',
				on: { click: () => C.handlers.onArm && C.handlers.onArm() } }));
			actions.appendChild(h('button', { class: 'authbtn authbtn--icon', title: 'sign out', html: icon('signout', 13),
				on: { click: () => C.handlers.onSignOut && C.handlers.onSignOut() } }));
		}
		bar.appendChild(actions);
		wrap.innerHTML = '';
		wrap.appendChild(bar);
	}

	// ── summon hint — minimal "esc →", mouse-revealed, auto-hiding ────────
	// Builds the dismissed-state affordance and wires it: it stays invisible
	// until the pointer moves, fades back out a couple seconds after the
	// pointer goes idle, and summons the chrome on click / Enter / Space.
	const HINT_IDLE_MS = 2000;
	function setupHint(C) {
		const hint = C.hint;
		if (!hint) return;
		hint.innerHTML = '<span class="summon__kbd">esc</span><span class="summon__arrow">→</span>';
		hint.setAttribute('role', 'button');
		hint.setAttribute('tabindex', '0');
		hint.setAttribute('aria-label', 'reveal the control monitor');
		hint.title = 'reveal the control monitor';
		hint.addEventListener('click', () => C.api.setSummoned(true));
		hint.addEventListener('keydown', (e) => {
			if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); C.api.setSummoned(true); }
		});
		// Reveal on any pointer movement; re-arm the idle timer each move so it
		// lingers while the mouse is active and fades once it settles.
		window.addEventListener('mousemove', () => revealHint(C), { passive: true });
	}
	function revealHint(C) {
		const hint = C.hint;
		if (!hint || C.summoned) return;
		if (!hint.classList.contains('is-visible')) hint.classList.add('is-visible');
		if (C._hintT) clearTimeout(C._hintT);
		C._hintT = setTimeout(() => hint.classList.remove('is-visible'), HINT_IDLE_MS);
	}

	// ── summon / dismiss + cinematic dip ─────────────────────────────────
	function setSummoned(C, on) {
		C.summoned = on;
		C.host.style.display = on ? '' : 'none';
		if (C.hint) {
			if (on) {
				if (C._hintT) { clearTimeout(C._hintT); C._hintT = null; }
				C.hint.classList.remove('is-visible');
				C.hint.style.display = 'none';
			} else {
				// Present but transparent (opacity 0) until mouse movement reveals it.
				C.hint.style.display = '';
			}
		}
		if (on) {
			const d5 = C.refs.root;
			d5.classList.remove('d5--' + C.transition); void d5.offsetWidth; d5.classList.add('d5--' + C.transition);
			scheduleChromeLayout(C);
			runWorldDip(C);
		}
	}
	function runWorldDip(C) {
		if (C.transition !== 'cinematic' || !C.canvas) return;
		C.canvas.classList.add('sim--dip');
		if (C._dipT) clearTimeout(C._dipT);
		C._dipT = setTimeout(() => C.canvas.classList.remove('sim--dip'), 820);
	}

	return { mount, pixelText, icon, h, effectLabel };
})();
