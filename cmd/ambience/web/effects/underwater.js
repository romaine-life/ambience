'use strict';
(function (api) {
	class Underwater extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('underwater', w, h, cfg, seed);
		}
	}

	api.presets['underwater'] = [
		{
			key: 'quiet-shallows',
			label: 'quiet shallows',
			config: {
				intro_reveal: 0.18,
				ending_murk: 0.12,
				density: 0.18,
				rise_speed: 0.32,
				drift: 0.04,
				sway: 0.34,
				weed_height: 16,
				weed_count: 9,
				caustics: 0.44,
				depth: 0.28,
				hue: 184,
				hue_sp: 12,
				sat: 0.38,
				lmin: 0.14,
				lmax: 0.86,
				calm_p: 0.0011,
			},
		},
		{
			key: 'bubble-field',
			label: 'bubble field',
			config: {
				density: 0.42,
				rise_speed: 0.54,
				drift: 0.08,
				sway: 0.46,
				weed_height: 18,
				weed_count: 8,
				caustics: 0.26,
				depth: 0.42,
				hue: 190,
				hue_sp: 16,
				sat: 0.42,
				lmin: 0.12,
				lmax: 0.82,
				bubble_burst_p: 0.0012,
			},
		},
		{
			key: 'slow-current',
			label: 'slow current',
			config: {
				density: 0.28,
				rise_speed: 0.4,
				drift: 0.12,
				sway: 0.78,
				weed_height: 22,
				weed_count: 11,
				caustics: 0.3,
				depth: 0.56,
				hue: 192,
				hue_sp: 18,
				sat: 0.42,
				lmin: 0.12,
				lmax: 0.82,
				current_shift_p: 0.0011,
			},
		},
		{
			key: 'deep-water',
			label: 'deep water',
			config: {
				intro_reveal: 0.1,
				ending_murk: 0.16,
				density: 0.16,
				rise_speed: 0.28,
				drift: 0.05,
				sway: 0.26,
				weed_height: 13,
				weed_count: 6,
				caustics: 0.14,
				depth: 0.82,
				hue: 204,
				hue_sp: 10,
				sat: 0.3,
				lmin: 0.08,
				lmax: 0.62,
				calm_p: 0.0014,
				calm_mult: 0.42,
			},
		},
	];
	api.effects['underwater'] = Underwater;
})(window.AmbienceSim);
