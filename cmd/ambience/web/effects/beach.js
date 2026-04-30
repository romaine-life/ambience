'use strict';
(function (api) {
	class Beach extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('beach', w, h, cfg, seed);
		}
	}

	api.presets['beach'] = [
		{
			key: 'still-shore',
			label: 'still shore',
			config: {
				shoreline: 0.56,
				tide_amp: 3.2,
				wave_amp: 1.3,
				wave_freq: 0.14,
				speed: 0.05,
				slope: 0.08,
				foam: 0.24,
				shimmer: 0.18,
				hue: 196,
				hue_sp: 10,
				sat: 0.42,
				lmin: 0.26,
				lmax: 0.78,
			},
		},
		{
			key: 'gentle-tide',
			label: 'gentle tide',
			config: {
				shoreline: 0.58,
				tide_amp: 6,
				wave_amp: 2.4,
				wave_freq: 0.18,
				speed: 0.1,
				slope: 0.16,
				foam: 0.36,
				shimmer: 0.22,
				hue: 198,
				hue_sp: 16,
				sat: 0.5,
				lmin: 0.28,
				lmax: 0.82,
				high_tide_p: 0.0008,
				low_tide_p: 0.0006,
			},
		},
		{
			key: 'foamy-edge',
			label: 'foamy edge',
			config: {
				shoreline: 0.6,
				tide_amp: 7.4,
				wave_amp: 3.1,
				wave_freq: 0.21,
				speed: 0.12,
				slope: 0.2,
				foam: 0.5,
				shimmer: 0.18,
				hue: 194,
				hue_sp: 18,
				sat: 0.54,
				lmin: 0.3,
				lmax: 0.84,
				high_tide_p: 0.0012,
				foam_burst_p: 0.0013,
				foam_burst_mult: 2.2,
			},
		},
		{
			key: 'wide-beach',
			label: 'wide beach',
			config: {
				shoreline: 0.52,
				tide_amp: 4.8,
				wave_amp: 1.8,
				wave_freq: 0.12,
				speed: 0.08,
				slope: -0.1,
				foam: 0.3,
				shimmer: 0.28,
				hue: 202,
				hue_sp: 14,
				sat: 0.44,
				lmin: 0.24,
				lmax: 0.78,
				low_tide_p: 0.0011,
			},
		},
	];
	api.effects['beach'] = Beach;
})(window.AmbienceSim);
