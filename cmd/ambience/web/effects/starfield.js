'use strict';
(function (api) {
	class Starfield extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('starfield', w, h, cfg, seed);
		}
	}

	api.presets['starfield'] = [
		{
			key: 'still-night',
			label: 'still night',
			config: {
				density: 0.16,
				speed: 0.08,
				drift: 0.02,
				layers: 2,
				size: 1,
				hue: 214,
				hue_sp: 12,
				sat: 0.16,
				lmin: 0.5,
				lmax: 0.9,
			},
		},
		{
			key: 'soft-parallax',
			label: 'soft parallax',
			config: {
				density: 0.22,
				speed: 0.12,
				drift: 0.04,
				layers: 3,
				size: 1,
				hue: 218,
				hue_sp: 18,
				sat: 0.18,
				lmin: 0.55,
				lmax: 0.95,
				twinkle_burst_p: 0.0006,
			},
		},
		{
			key: 'meteor-watch',
			label: 'meteor watch',
			config: {
				density: 0.24,
				speed: 0.14,
				drift: 0.06,
				layers: 3,
				size: 1.2,
				hue: 214,
				hue_sp: 22,
				sat: 0.2,
				lmin: 0.56,
				lmax: 0.96,
				shooting_star_p: 0.0012,
				shooting_star_mult: 2.4,
			},
		},
		{
			key: 'cold-deep-space',
			label: 'cold deep space',
			config: {
				density: 0.2,
				speed: 0.09,
				drift: 0.03,
				layers: 4,
				size: 1,
				hue: 226,
				hue_sp: 26,
				sat: 0.22,
				lmin: 0.52,
				lmax: 0.94,
				twinkle_burst_p: 0.0009,
				twinkle_burst_mult: 1.9,
			},
		},
	];
	api.effects['starfield'] = Starfield;
})(window.AmbienceSim);
