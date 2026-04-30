'use strict';
(function (api) {
	class Rowboat extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('rowboat', w, h, cfg, seed);
		}
	}

	api.presets['rowboat'] = [
		{
			key: 'still-lake',
			label: 'still lake',
			config: {
				intro_drift: 0.12,
				ending_ripple: 0.12,
				waterline: 0.57,
				drift_speed: 0.05,
				bob_amp: 0.7,
				wave_amp: 0.9,
				wave_freq: 0.12,
				ripple: 0.12,
				reflection: 0.28,
				boat_len: 13,
				boat_height: 3.5,
				hue: 202,
				hue_sp: 10,
				sat: 0.26,
				lmin: 0.16,
				lmax: 0.74,
				calm_p: 0.0011,
			},
		},
		{
			key: 'gentle-drift',
			label: 'gentle drift',
			config: {
				waterline: 0.58,
				drift_speed: 0.08,
				bob_amp: 1.2,
				wave_amp: 1.6,
				wave_freq: 0.16,
				ripple: 0.24,
				reflection: 0.22,
				boat_len: 14,
				boat_height: 3.5,
				hue: 206,
				hue_sp: 16,
				sat: 0.36,
				lmin: 0.16,
				lmax: 0.82,
				drift_p: 0.0009,
			},
		},
		{
			key: 'evening-ripples',
			label: 'evening ripples',
			config: {
				waterline: 0.6,
				drift_speed: 0.1,
				bob_amp: 1.4,
				wave_amp: 1.9,
				wave_freq: 0.18,
				ripple: 0.34,
				reflection: 0.24,
				boat_len: 14.5,
				boat_height: 4,
				hue: 212,
				hue_sp: 18,
				sat: 0.4,
				lmin: 0.18,
				lmax: 0.86,
				wake_p: 0.001,
			},
		},
		{
			key: 'wind-touched-water',
			label: 'wind-touched water',
			config: {
				waterline: 0.56,
				drift_speed: 0.12,
				bob_amp: 1.8,
				wave_amp: 2.5,
				wave_freq: 0.2,
				ripple: 0.42,
				reflection: 0.18,
				boat_len: 15,
				boat_height: 4,
				hue: 198,
				hue_sp: 20,
				sat: 0.46,
				lmin: 0.18,
				lmax: 0.8,
				wake_p: 0.0012,
				wake_mult: 2.1,
				drift_p: 0.0014,
				drift_push: 1.55,
			},
		},
	];
	api.effects['rowboat'] = Rowboat;
})(window.AmbienceSim);
