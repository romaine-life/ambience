'use strict';
(function (api) {
	class Aurora extends api._ProceduralScene {
		constructor(w, h, cfg, seed) {
			super('aurora', w, h, cfg, seed);
		}
	}

	api.presets['aurora'] = [
		{
			key: 'green-veil',
			label: 'green veil',
			config: {
				intensity: 0.54,
				speed: 0.1,
				drift: 0.06,
				bands: 3,
				thickness: 9,
				wave_amp: 5.5,
				wave_freq: 0.15,
				curtain_len: 14,
				hue: 134,
				hue_sp: 18,
				sat: 0.7,
				lmin: 0.2,
				lmax: 0.72,
				shift_p: 0.0007,
			},
		},
		{
			key: 'cold-ribbons',
			label: 'cold ribbons',
			config: {
				intensity: 0.48,
				speed: 0.12,
				drift: 0.1,
				bands: 4,
				thickness: 7.5,
				wave_amp: 6.5,
				wave_freq: 0.18,
				curtain_len: 13,
				hue: 164,
				hue_sp: 34,
				sat: 0.66,
				lmin: 0.18,
				lmax: 0.76,
				shift_p: 0.0011,
				fade_p: 0.0005,
			},
		},
		{
			key: 'quiet-sky',
			label: 'quiet sky',
			config: {
				intensity: 0.34,
				speed: 0.07,
				drift: 0.03,
				bands: 2,
				thickness: 8.5,
				wave_amp: 4.5,
				wave_freq: 0.12,
				curtain_len: 11,
				hue: 142,
				hue_sp: 14,
				sat: 0.58,
				lmin: 0.16,
				lmax: 0.64,
				fade_p: 0.0008,
			},
		},
		{
			key: 'bright-aurora',
			label: 'bright aurora',
			config: {
				intensity: 0.72,
				speed: 0.14,
				drift: 0.12,
				bands: 4,
				thickness: 10,
				wave_amp: 7.2,
				wave_freq: 0.19,
				curtain_len: 18,
				hue: 136,
				hue_sp: 30,
				sat: 0.78,
				lmin: 0.22,
				lmax: 0.82,
				brighten_p: 0.0012,
				brighten_mult: 1.7,
				shift_p: 0.001,
			},
		},
	];
	api.effects['aurora'] = Aurora;
})(window.AmbienceSim);
