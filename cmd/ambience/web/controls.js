(function (global) {
	'use strict';

	const SLOT_LABELS = {
		'spawn': 'spawn config',
		'lever': 'continuous levers',
		'event': 'discrete events',
		'event-mod': 'event modifiers',
		'end': 'end conditions',
	};
	const SLOT_ORDER = ['spawn', 'lever', 'event', 'event-mod', 'end'];
	const CUSTOM_PRESET = '__custom__';
	const CHEV = '\u25be';

	function effectLabel(name) {
		return String(name || '').replace(/-/g, ' ');
	}

	function effectPresets(name) {
		const presets = global.AmbienceSim && global.AmbienceSim.presets && global.AmbienceSim.presets[name];
		return Array.isArray(presets) ? presets : [];
	}

	function presetValue(preset) {
		return preset.key || preset.name;
	}

	function presetConfig(preset) {
		return (preset && (preset.config || preset.values)) || {};
	}

	function findPreset(effect, value) {
		return effectPresets(effect).find((preset) => presetValue(preset) === value) || null;
	}

	function knobMatches(knob, left, right) {
		if (!knob) return false;
		const a = Number(left);
		const b = Number(right);
		if (!Number.isFinite(a) || !Number.isFinite(b)) return false;
		if (knob.type === 'int') return Math.round(a) === Math.round(b);
		const step = Number(knob.step) || 0.001;
		return Math.abs(a - b) <= Math.max(1e-9, step / 2);
	}

	function formatValue(knob, value) {
		const v = Number(value);
		if (!Number.isFinite(v)) return '';
		if (knob.type === 'int') return String(Math.round(v));
		const step = Number(knob.step) || 0;
		const decimals = step < 0.1 ? 2 : (step < 1 ? 1 : 0);
		return v.toFixed(decimals) + ((knob.key === 'hue' || knob.key === 'hue_sp') ? '\u00b0' : '');
	}

	function create(options) {
		const state = {
			effect: options.effect || 'rain',
			locked: !!options.locked,
			schema: null,
			knobs: [],
		};

		const root = options.root;
		const effectSelect = options.effectSelect || null;
		const presetSelect = options.presetSelect || null;
		const toggleAll = options.toggleAll || null;
		const storagePrefix = options.storagePrefix || 'ambience.controls.section.';
		const canSwitchEffect = !!options.canSwitchEffect;

		function currentValues() {
			const values = {};
			for (const knob of state.knobs) {
				const input = document.getElementById(knob.key);
				if (input) values[knob.key] = +input.value;
			}
			return values;
		}

		function updateReadouts() {
			for (const knob of state.knobs) {
				const input = document.getElementById(knob.key);
				const readout = document.getElementById(knob.key + '_v');
				if (input && readout) readout.textContent = formatValue(knob, input.value);
			}
		}

		function resolvedPresetConfig(schema, preset) {
			const resolved = {};
			for (const knob of (schema && schema.knobs) || []) resolved[knob.key] = knob.default;
			return Object.assign(resolved, presetConfig(preset));
		}

		function matchingPresetValue() {
			if (!state.schema || !state.knobs.length) return CUSTOM_PRESET;
			const values = currentValues();
			for (const preset of effectPresets(state.effect)) {
				const resolved = resolvedPresetConfig(state.schema, preset);
				let matches = true;
				for (const knob of state.knobs) {
					if (!knobMatches(knob, values[knob.key], resolved[knob.key])) {
						matches = false;
						break;
					}
				}
				if (matches) return presetValue(preset);
			}
			return CUSTOM_PRESET;
		}

		function syncEffectSwitcher() {
			if (!effectSelect) return;
			const optionsList = (typeof options.effectOptions === 'function')
				? options.effectOptions(state.effect)
				: [state.effect];
			effectSelect.innerHTML = '';
			for (const name of optionsList) {
				const opt = document.createElement('option');
				opt.value = name;
				opt.textContent = effectLabel(name);
				opt.selected = name === state.effect;
				effectSelect.appendChild(opt);
			}
			if (!optionsList.includes(state.effect)) {
				const opt = document.createElement('option');
				opt.value = state.effect;
				opt.textContent = effectLabel(state.effect);
				opt.selected = true;
				effectSelect.appendChild(opt);
			}
			effectSelect.disabled = !canSwitchEffect;
		}

		function syncPresetSwitcher(selectedValue) {
			if (!presetSelect) return;
			const presets = effectPresets(state.effect);
			presetSelect.innerHTML = '';
			const custom = document.createElement('option');
			custom.value = CUSTOM_PRESET;
			custom.textContent = 'custom';
			presetSelect.appendChild(custom);
			for (const preset of presets) {
				const opt = document.createElement('option');
				opt.value = presetValue(preset);
				opt.textContent = preset.label || preset.name || opt.value;
				presetSelect.appendChild(opt);
			}
			const desired = selectedValue || matchingPresetValue();
			presetSelect.value = findPreset(state.effect, desired) ? desired : CUSTOM_PRESET;
			presetSelect.disabled = state.locked || !state.schema || presets.length === 0;
			presetSelect.title = presets.length ? 'apply effect preset' : 'no presets for this effect yet';
		}

		function setInteractiveDisabled(disabled) {
			for (const input of root.querySelectorAll('input[type="range"]')) input.disabled = disabled;
			for (const button of root.querySelectorAll('button.fire')) button.disabled = disabled;
			syncEffectSwitcher();
			syncPresetSwitcher();
		}

		function applyValues(values) {
			for (const knob of state.knobs) {
				const input = document.getElementById(knob.key);
				if (!input) continue;
				const next = values && values[knob.key] != null ? values[knob.key] : knob.default;
				input.value = next;
			}
			updateReadouts();
			syncPresetSwitcher();
		}

		function buildRow(knob) {
			const row = document.createElement('div');
			row.className = 'control-row';

			const label = document.createElement('label');
			label.htmlFor = knob.key;
			label.textContent = knob.label;

			const input = document.createElement('input');
			input.type = 'range';
			input.id = knob.key;
			input.min = knob.min;
			input.max = knob.max;
			input.step = knob.step;
			input.value = knob.default;

			const value = document.createElement('span');
			value.className = 'val';
			value.id = knob.key + '_v';

			row.append(label, input, value);

			if (knob.trigger) {
				const fire = document.createElement('button');
				fire.type = 'button';
				fire.className = 'fire';
				fire.textContent = 'fire';
				fire.title = 'trigger this event now';
				fire.addEventListener('click', () => {
					if (state.locked || typeof options.onTrigger !== 'function') return;
					options.onTrigger(knob.trigger);
				});
				row.appendChild(fire);
			}

			if (knob.description) {
				const help = document.createElement('span');
				help.className = 'help';
				help.textContent = '?';
				help.title = knob.description;
				row.appendChild(help);
			}

			input.addEventListener('input', () => {
				updateReadouts();
				if (presetSelect && !presetSelect.disabled) presetSelect.value = CUSTOM_PRESET;
				if (state.locked || typeof options.onConfigChange !== 'function') return;
				options.onConfigChange(currentValues(), knob);
			});

			return row;
		}

		function buildSection(id, titleText, count, defaultOpen, bodyBuilder) {
			const section = document.createElement('div');
			section.className = 'section';
			section.dataset.id = id;

			const storedClosed = localStorage.getItem(storagePrefix + id) === 'closed';
			const initiallyClosed = storedClosed || (!defaultOpen && localStorage.getItem(storagePrefix + id) === null);
			if (initiallyClosed) section.classList.add('closed');

			const head = document.createElement('div');
			head.className = 'section-head';
			head.innerHTML = '<span class="chev">' + CHEV + '</span><span class="name">' + titleText + '</span>' +
				(count != null ? '<span class="count">' + count + '</span>' : '');
			head.addEventListener('click', () => {
				section.classList.toggle('closed');
				localStorage.setItem(storagePrefix + id, section.classList.contains('closed') ? 'closed' : 'open');
			});
			section.appendChild(head);

			const body = document.createElement('div');
			body.className = 'section-body';
			bodyBuilder(body);
			section.appendChild(body);
			return section;
		}

		function render(schema) {
			state.schema = schema;
			state.knobs = Array.isArray(schema && schema.knobs) ? schema.knobs.slice() : [];
			root.innerHTML = '';

			const bySlot = {};
			for (const knob of state.knobs) {
				if (!bySlot[knob.slot]) bySlot[knob.slot] = {};
				const group = knob.group || '';
				if (!bySlot[knob.slot][group]) bySlot[knob.slot][group] = [];
				bySlot[knob.slot][group].push(knob);
			}

			for (const slot of SLOT_ORDER) {
				const slotKnobs = bySlot[slot] ? Object.values(bySlot[slot]).flat() : [];
				const section = buildSection(
					'slot.' + slot,
					SLOT_LABELS[slot] || slot,
					slotKnobs.length > 0 ? slotKnobs.length : 'none',
					slot === 'spawn',
					(body) => {
						if (!bySlot[slot]) {
							const empty = document.createElement('div');
							empty.className = 'empty-slot';
							empty.textContent = 'none yet';
							body.appendChild(empty);
							return;
						}
						const groups = bySlot[slot];
						const groupNames = Object.keys(groups);
						const showSubheaders = !(groupNames.length === 1 && groupNames[0] === '');
						for (const group of groupNames) {
							if (showSubheaders && group) {
								const heading = document.createElement('h3');
								heading.textContent = group;
								body.appendChild(heading);
							}
							for (const knob of groups[group]) body.appendChild(buildRow(knob));
						}
					}
				);
				root.appendChild(section);
			}

			updateReadouts();
			syncEffectSwitcher();
			syncPresetSwitcher();
			setInteractiveDisabled(state.locked);
		}

		if (effectSelect) {
			effectSelect.addEventListener('change', () => {
				const next = effectSelect.value;
				if (!canSwitchEffect || !next || next === state.effect || typeof options.onEffectChange !== 'function') return;
				options.onEffectChange(next);
			});
		}

		if (presetSelect) {
			presetSelect.addEventListener('change', () => {
				if (state.locked || presetSelect.value === CUSTOM_PRESET || !state.schema) return;
				const preset = findPreset(state.effect, presetSelect.value);
				if (!preset) return;
				applyValues(resolvedPresetConfig(state.schema, preset));
				if (typeof options.onConfigChange === 'function') options.onConfigChange(currentValues(), null);
			});
		}

		if (toggleAll) {
			toggleAll.addEventListener('click', () => {
				const sections = root.querySelectorAll('.section');
				const anyOpen = Array.from(sections).some((section) => !section.classList.contains('closed'));
				for (const section of sections) {
					section.classList.toggle('closed', anyOpen);
					localStorage.setItem(storagePrefix + section.dataset.id, anyOpen ? 'closed' : 'open');
				}
			});
		}

		syncEffectSwitcher();
		syncPresetSwitcher();

		return {
			currentValues,
			getKnobs() {
				return state.knobs.slice();
			},
			getSchema() {
				return state.schema;
			},
			isLocked() {
				return state.locked;
			},
			render,
			setEffect(effect) {
				state.effect = effect;
				syncEffectSwitcher();
				syncPresetSwitcher();
			},
			setLocked(locked) {
				state.locked = !!locked;
				setInteractiveDisabled(state.locked);
			},
			setValues: applyValues,
			updateReadouts,
		};
	}

	global.AmbienceControls = { create };
})(window);
