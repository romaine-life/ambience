'use strict';
(function (api) {
	class WheatField extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('wheat-field', w, h, cfg, seed);
		}
	}

	api.presets['wheat-field'] = [
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
	];
	api.effects['wheat-field'] = WheatField;
})(window.AmbienceSim);
