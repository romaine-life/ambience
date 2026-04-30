'use strict';
(function (api) {
	class Campfire extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('campfire', w, h, cfg, seed);
		}
	}

	api.presets['campfire'] = [
		{
			key: 'small-fire',
			label: 'small fire',
			config: {
				flame_height: 9,
				flame_width: 7,
				flame_speed: 0.1,
				flicker: 0.56,
				ember_rate: 0.18,
				ember_speed: 0.52,
				glow: 0.4,
				hue: 22,
				hue_sp: 12,
				sat: 0.76,
				lmin: 0.28,
				lmax: 0.88,
			},
		},
		{
			key: 'steady-campfire',
			label: 'steady campfire',
			config: {
				flame_height: 14,
				flame_width: 10,
				flame_speed: 0.12,
				flicker: 0.72,
				ember_rate: 0.26,
				ember_speed: 0.62,
				glow: 0.54,
				hue: 24,
				hue_sp: 18,
				sat: 0.82,
				lmin: 0.32,
				lmax: 0.94,
				crackle_p: 0.0008,
			},
		},
		{
			key: 'crackling-fire',
			label: 'crackling fire',
			config: {
				flame_height: 16,
				flame_width: 11,
				flame_speed: 0.15,
				flicker: 0.92,
				ember_rate: 0.34,
				ember_speed: 0.78,
				glow: 0.62,
				hue: 21,
				hue_sp: 22,
				sat: 0.88,
				lmin: 0.34,
				lmax: 0.96,
				crackle_p: 0.0015,
				crackle_mult: 2.15,
				crackle_dur: 48,
			},
		},
		{
			key: 'late-embers',
			label: 'late embers',
			config: {
				intro_glow: 0.1,
				ending_glow: 0.14,
				flame_height: 8,
				flame_width: 8,
				flame_speed: 0.08,
				flicker: 0.42,
				ember_rate: 0.3,
				ember_speed: 0.48,
				glow: 0.34,
				hue: 18,
				hue_sp: 14,
				sat: 0.68,
				lmin: 0.24,
				lmax: 0.8,
				lull_p: 0.0014,
				lull_mult: 0.42,
			},
		},
	];
	api.effects['campfire'] = Campfire;
})(window.AmbienceSim);
