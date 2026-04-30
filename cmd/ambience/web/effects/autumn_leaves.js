'use strict';
(function (api) {
	class AutumnLeaves extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('autumn-leaves', w, h, cfg, seed);
		}
	}

	api.presets['autumn-leaves'] = [
		{
			key: 'few-leaves',
			label: 'few leaves',
			config: {
				density: 0.14,
				speed: 0.36,
				drift: 0.12,
				sway: 0.7,
				layers: 1,
				size: 1,
				hue: 24,
				hue_sp: 18,
				sat: 0.58,
				lmin: 0.36,
				lmax: 0.7,
				lull_p: 0.0014,
			},
		},
		{
			key: 'gentle-fall',
			label: 'gentle fall',
			config: {
				density: 0.24,
				speed: 0.44,
				drift: 0.18,
				sway: 0.86,
				layers: 2,
				size: 1.2,
				hue: 28,
				hue_sp: 24,
				sat: 0.62,
				lmin: 0.38,
				lmax: 0.78,
				gust_p: 0.0008,
			},
		},
		{
			key: 'windy-autumn',
			label: 'windy autumn',
			config: {
				density: 0.3,
				speed: 0.5,
				drift: 0.26,
				sway: 1.05,
				layers: 2,
				size: 1.4,
				hue: 22,
				hue_sp: 28,
				sat: 0.68,
				lmin: 0.36,
				lmax: 0.8,
				gust_p: 0.0016,
				gust_mult: 2.35,
			},
		},
		{
			key: 'swirl-study',
			label: 'swirl study',
			config: {
				density: 0.28,
				speed: 0.42,
				drift: 0.12,
				sway: 1.15,
				layers: 2,
				size: 1.4,
				hue: 30,
				hue_sp: 34,
				sat: 0.7,
				lmin: 0.4,
				lmax: 0.84,
				swirl_p: 0.0015,
				swirl_dur: 68,
				swirl_pull: 1.55,
			},
		},
	];
	api.effects['autumn-leaves'] = AutumnLeaves;
})(window.AmbienceSim);
