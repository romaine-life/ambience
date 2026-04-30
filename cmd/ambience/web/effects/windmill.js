'use strict';
(function (api) {
	class Windmill extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('windmill', w, h, cfg, seed);
		}
	}

	api.presets['windmill'] = [
		{
			key: 'still-dusk',
			label: 'still dusk',
			config: {
				intro_turn: 0.08,
				ending_turn: 0.04,
				turn_speed: 0.04,
				blade_len: 12,
				blade_width: 1.6,
				tower_height: 19,
				tower_width: 5.5,
				horizon: 0.74,
				glow: 0.22,
				hue: 26,
				hue_sp: 14,
				sat: 0.38,
				lmin: 0.16,
				lmax: 0.78,
			},
		},
		{
			key: 'steady-turning',
			label: 'steady turning',
			config: {
				turn_speed: 0.08,
				blade_len: 14,
				blade_width: 1.8,
				tower_height: 20,
				tower_width: 6,
				horizon: 0.72,
				glow: 0.18,
				hue: 28,
				hue_sp: 18,
				sat: 0.42,
				lmin: 0.18,
				lmax: 0.82,
				gust_p: 0.0006,
			},
		},
		{
			key: 'windy-hill',
			label: 'windy hill',
			config: {
				turn_speed: 0.12,
				blade_len: 15,
				blade_width: 2.1,
				tower_height: 21,
				tower_width: 6.5,
				horizon: 0.7,
				glow: 0.14,
				hue: 24,
				hue_sp: 20,
				sat: 0.4,
				lmin: 0.16,
				lmax: 0.8,
				gust_p: 0.0014,
				gust_mult: 2.2,
				gust_dur: 62,
			},
		},
		{
			key: 'silhouette-mill',
			label: 'silhouette mill',
			config: {
				turn_speed: 0.06,
				blade_len: 16,
				blade_width: 1.5,
				tower_height: 23,
				tower_width: 5,
				horizon: 0.76,
				glow: 0.1,
				hue: 222,
				hue_sp: 12,
				sat: 0.22,
				lmin: 0.12,
				lmax: 0.68,
				lull_p: 0.0012,
				lull_mult: 0.38,
			},
		},
	];
	api.effects['windmill'] = Windmill;
})(window.AmbienceSim);
