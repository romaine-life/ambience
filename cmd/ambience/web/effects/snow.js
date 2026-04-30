'use strict';
(function (api) {
	class Snow extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('snow', w, h, cfg, seed);
		}
	}

	api.presets['snow'] = [
		{
			key: 'quiet-flurries',
			label: 'quiet flurries',
			config: {
				density: 0.2,
				speed: 0.38,
				drift: 0.04,
				sway: 0.35,
				layers: 2,
				size: 1,
				hue: 208,
				hue_sp: 8,
				sat: 0.12,
				lmin: 0.76,
				lmax: 0.96,
				calm_p: 0.0012,
			},
		},
		{
			key: 'pine-evening',
			label: 'pine evening',
			config: {
				density: 0.3,
				speed: 0.5,
				drift: 0.08,
				sway: 0.4,
				layers: 3,
				size: 1,
				hue: 214,
				hue_sp: 12,
				sat: 0.16,
				lmin: 0.74,
				lmax: 0.98,
				gust_p: 0.0008,
			},
		},
		{
			key: 'crosswind',
			label: 'crosswind',
			config: {
				density: 0.34,
				speed: 0.56,
				drift: 0.16,
				sway: 0.58,
				layers: 3,
				size: 1.2,
				hue: 206,
				hue_sp: 10,
				sat: 0.14,
				lmin: 0.72,
				lmax: 0.98,
				gust_p: 0.0015,
				gust_mult: 2.25,
				gust_dur: 68,
			},
		},
		{
			key: 'whiteout-edge',
			label: 'whiteout edge',
			config: {
				intro_density: 0.22,
				ending_density: 0.14,
				density: 0.52,
				speed: 0.7,
				drift: 0.12,
				sway: 0.74,
				layers: 4,
				size: 1.5,
				hue: 212,
				hue_sp: 16,
				sat: 0.18,
				lmin: 0.76,
				lmax: 1,
				gust_p: 0.0018,
				gust_mult: 2.8,
				gust_dur: 76,
				calm_p: 0.0003,
			},
		},
	];
	api.effects['snow'] = Snow;
})(window.AmbienceSim);
