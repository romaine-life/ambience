'use strict';
(function (api) {
	class Lighthouse extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('lighthouse', w, h, cfg, seed);
		}
	}

	api.presets['lighthouse'] = [
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
	];
	api.effects['lighthouse'] = Lighthouse;
})(window.AmbienceSim);
