package sim

import (
	"image/color"
	"math"
)

type proceduralRenderState struct {
	kind   string
	w      int
	h      int
	tick   int
	cfg    ProceduralConfig
	timers map[string]int
	values map[string]float64
	seed   uint64
	grid   [][]Pixel
}

func renderProceduralGrid(kind string, w, h, tick int, cfg ProceduralConfig, timers map[string]int, values map[string]float64, seed uint64) [][]Pixel {
	r := proceduralRenderState{
		kind:   kind,
		w:      w,
		h:      h,
		tick:   tick,
		cfg:    cfg,
		timers: timers,
		values: values,
		seed:   seed,
		grid:   newPixelGrid(w, h),
	}
	if w <= 0 || h <= 0 {
		return r.grid
	}
	switch kind {
	case "aurora":
		r.renderAurora()
	case "autumn-leaves":
		r.renderAutumnLeaves()
	case "beach":
		r.renderBeach()
	case "campfire":
		r.renderCampfire()
	case "lighthouse":
		r.renderLighthouse()
	case "mysterious-man":
		r.renderMysteriousMan()
	case "rowboat":
		r.renderRowboat()
	case "snow":
		r.renderSnow()
	case "starfield":
		r.renderStarfield()
	case "train":
		r.renderTrain()
	case "underwater":
		r.renderUnderwater()
	case "volcano":
		r.renderVolcano()
	case "wheat-field":
		r.renderWheatField()
	case "windmill":
		r.renderWindmill()
	default:
		return proceduralGridCopy(kind, w, h, tick, cfg)
	}
	return r.grid
}

func newPixelGrid(w, h int) [][]Pixel {
	if h < 0 {
		h = 0
	}
	if w < 0 {
		w = 0
	}
	grid := make([][]Pixel, h)
	for y := range grid {
		grid[y] = make([]Pixel, w)
	}
	return grid
}

func (r *proceduralRenderState) cfgFloat(key string, fallback float64) float64 {
	if r.cfg == nil {
		return fallback
	}
	if v, ok := r.cfg[key]; ok {
		return v
	}
	return fallback
}

func (r *proceduralRenderState) timer(key string) int {
	if r.timers == nil {
		return 0
	}
	return r.timers[key]
}

func (r *proceduralRenderState) value(key string, fallback float64) float64 {
	if r.values == nil {
		return fallback
	}
	if v, ok := r.values[key]; ok {
		return v
	}
	return fallback
}

func (r *proceduralRenderState) hash(i int) float64 {
	x := math.Sin((float64(r.seed%1000003)*0.000001 + float64(i)*12.9898) * 43758.5453)
	return x - math.Floor(x)
}

func (r *proceduralRenderState) fillRect(x0, y0, w, h int, c color.RGBA) {
	r.fillRectAlpha(x0, y0, w, h, c, 1)
}

func (r *proceduralRenderState) fillRectAlpha(x0, y0, w, h int, c color.RGBA, alpha float64) {
	for y := max(0, y0); y < min(r.h, y0+h); y++ {
		for x := max(0, x0); x < min(r.w, x0+w); x++ {
			r.blend(x, y, c, alpha)
		}
	}
}

func (r *proceduralRenderState) paint(x, y int, c color.RGBA) {
	r.paintAlpha(x, y, c, 1)
}

func (r *proceduralRenderState) paintAlpha(x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= r.h || x < 0 || x >= r.w {
		return
	}
	r.blend(x, y, c, alpha)
}

func (r *proceduralRenderState) blend(x, y int, c color.RGBA, alpha float64) {
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	if alpha >= 1 || !r.grid[y][x].Filled {
		r.grid[y][x] = Pixel{Filled: true, C: color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}}
		return
	}
	prev := r.grid[y][x].C
	r.grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(prev.R)*(1-alpha) + float64(c.R)*alpha + 0.5),
		G: uint8(float64(prev.G)*(1-alpha) + float64(c.G)*alpha + 0.5),
		B: uint8(float64(prev.B)*(1-alpha) + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func (r *proceduralRenderState) hsl(hue, sat, light float64) color.RGBA {
	return hslToRGB(math.Mod(hue+360, 360), clamp01(sat), clamp01(light))
}

func positiveModFloat(value, mod float64) float64 {
	if mod == 0 {
		return 0
	}
	return math.Mod(math.Mod(value, mod)+mod, mod)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func mixColor(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t + 0.5),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t + 0.5),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t + 0.5),
		A: 255,
	}
}

func (r *proceduralRenderState) verticalGradient(top, bottom color.RGBA) {
	for y := 0; y < r.h; y++ {
		t := 0.0
		if r.h > 1 {
			t = float64(y) / float64(r.h-1)
		}
		r.fillRect(0, y, r.w, 1, mixColor(top, bottom, t))
	}
}

func proceduralPhaseProgress(total, left int) float64 {
	if left <= 1 || total <= 1 {
		return 1
	}
	elapsed := total - left
	if elapsed <= 0 {
		return 0
	}
	return clamp01(float64(elapsed) / float64(max(1, total-1)))
}

func (r *proceduralRenderState) snowDensity() float64 {
	level := r.cfgFloat("density", 0.36)
	if r.timer("gust") > 0 {
		level *= 1.28
	}
	if r.timer("calm") > 0 {
		level *= r.cfgFloat("calm_mult", 0.45)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 60))))
		level *= r.cfgFloat("intro_density", 0.1) + (1-r.cfgFloat("intro_density", 0.1))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 80)+r.cfgFloat("ending_linger", 30))))
		level *= 1 - (1-r.cfgFloat("ending_density", 0.18))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.02, level)
}

func (r *proceduralRenderState) autumnDensity() float64 {
	level := r.cfgFloat("density", 0.34)
	if r.timer("gust") > 0 {
		level *= 1.22
	}
	if r.timer("lull") > 0 {
		level *= r.cfgFloat("lull_mult", 0.45)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 50))))
		level *= r.cfgFloat("intro_density", 0.12) + (1-r.cfgFloat("intro_density", 0.12))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 70)+r.cfgFloat("ending_linger", 30))))
		level *= 1 - (1-r.cfgFloat("ending_density", 0.08))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.015, level)
}

func (r *proceduralRenderState) tideLevelBeach() float64 {
	level := 1.0
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 54))))
		level *= r.cfgFloat("intro_tide", 0.18) + (1-r.cfgFloat("intro_tide", 0.18))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 74)+r.cfgFloat("ending_linger", 26))))
		level *= 1 - (1-r.cfgFloat("ending_wet", 0.16))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.05, level)
}

func (r *proceduralRenderState) intensityLevelAurora() float64 {
	level := r.cfgFloat("intensity", 0.56)
	if r.timer("brighten") > 0 {
		level *= r.value("brighten_gain", r.cfgFloat("brighten_mult", 1.45))
	}
	if r.timer("fade") > 0 {
		level *= r.cfgFloat("fade_mult", 0.6)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 70))))
		level *= r.cfgFloat("intro_glow", 0.18) + (1-r.cfgFloat("intro_glow", 0.18))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 80)+r.cfgFloat("ending_linger", 20))))
		level *= 1 - (1-r.cfgFloat("ending_glow", 0.05))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.02, level)
}

func (r *proceduralRenderState) motionLevelWheatField() float64 {
	level := r.cfgFloat("sway", 0.68)
	if r.timer("gust") > 0 {
		level *= 1 + math.Abs(r.value("gust_push", r.cfgFloat("gust_mult", 1.85)))*0.35
	}
	if r.timer("calm") > 0 {
		level *= r.cfgFloat("calm_mult", 0.4)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 60))))
		level *= r.cfgFloat("intro_breeze", 0.16) + (1-r.cfgFloat("intro_breeze", 0.16))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 70)+r.cfgFloat("ending_linger", 20))))
		level *= 1 - (1-r.cfgFloat("ending_sway", 0.08))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.05, level)
}

func (r *proceduralRenderState) flameLevelCampfire() float64 {
	level := 1.0
	if r.timer("crackle") > 0 {
		level *= r.value("crackle_gain", r.cfgFloat("crackle_mult", 1.85))
	}
	if r.timer("lull") > 0 {
		level *= r.cfgFloat("lull_mult", 0.55)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 45))))
		level *= r.cfgFloat("intro_glow", 0.14) + (1-r.cfgFloat("intro_glow", 0.14))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 60)+r.cfgFloat("ending_linger", 24))))
		level *= 1 - (1-r.cfgFloat("ending_glow", 0.08))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.05, level)
}

func (r *proceduralRenderState) pressureLevelVolcano() float64 {
	level := 1.0
	if r.timer("eruption") > 0 {
		level *= r.value("eruption_gain", r.cfgFloat("eruption_mult", 2.4))
	}
	if r.timer("smolder") > 0 {
		level *= r.cfgFloat("smolder_mult", 0.55)
	}
	if r.timer("flare") > 0 {
		level *= 1 + (r.value("flare_gain", r.cfgFloat("flare_mult", 1.85))-1)*0.5
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 55))))
		level *= r.cfgFloat("intro_glow", 0.16) + (1-r.cfgFloat("intro_glow", 0.16))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 70)+r.cfgFloat("ending_linger", 22))))
		level *= 1 - (1-r.cfgFloat("ending_glow", 0.10))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.05, level)
}

func (r *proceduralRenderState) rotationLevelWindmill() float64 {
	level := 1.0
	if r.timer("gust") > 0 {
		level *= r.value("gust_gain", r.cfgFloat("gust_mult", 1.9))
	}
	if r.timer("lull") > 0 {
		level *= r.cfgFloat("lull_mult", 0.45)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 45))))
		level *= r.cfgFloat("intro_turn", 0.12) + (1-r.cfgFloat("intro_turn", 0.12))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 60)+r.cfgFloat("ending_linger", 20))))
		level *= 1 - (1-r.cfgFloat("ending_turn", 0.05))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.03, level)
}

func (r *proceduralRenderState) beamLevelLighthouse() float64 {
	level := 1.0
	if r.timer("bright-pass") > 0 {
		level *= r.value("bright_gain", r.cfgFloat("bright_pass_mult", 1.75))
	}
	if r.timer("calm") > 0 {
		level *= r.cfgFloat("calm_mult", 0.55)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 50))))
		level *= r.cfgFloat("intro_beam", 0.16) + (1-r.cfgFloat("intro_beam", 0.16))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 65)+r.cfgFloat("ending_linger", 18))))
		level *= 1 - (1-r.cfgFloat("ending_beam", 0.08))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.05, level)
}

func (r *proceduralRenderState) rippleLevelRowboat() float64 {
	level := 1.0
	if r.timer("wake") > 0 {
		level *= r.value("wake_gain", r.cfgFloat("wake_mult", 1.85))
	}
	if r.timer("calm") > 0 {
		level *= r.cfgFloat("calm_mult", 0.5)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 50))))
		level *= r.cfgFloat("intro_drift", 0.18) + (1-r.cfgFloat("intro_drift", 0.18))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 65)+r.cfgFloat("ending_linger", 18))))
		level *= 1 - (1-r.cfgFloat("ending_ripple", 0.08))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	if r.timer("drift") > 0 {
		level *= 1 + math.Abs(r.value("drift_push", r.cfgFloat("drift_push", 1.3)))*0.22
	}
	return math.Max(0.04, level)
}

func (r *proceduralRenderState) sceneLevelUnderwater() float64 {
	level := 1.0
	if r.timer("bubble-burst") > 0 {
		level *= r.value("bubble_gain", r.cfgFloat("bubble_burst_mult", 1.9))
	}
	if r.timer("calm") > 0 {
		level *= r.cfgFloat("calm_mult", 0.55)
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 55))))
		level *= r.cfgFloat("intro_reveal", 0.14) + (1-r.cfgFloat("intro_reveal", 0.14))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 70)+r.cfgFloat("ending_linger", 22))))
		level *= 1 - (1-r.cfgFloat("ending_murk", 0.08))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	if r.timer("current-shift") > 0 {
		level *= 1 + math.Abs(r.value("current_push", r.cfgFloat("current_shift_push", 1.2)))*0.18
	}
	return math.Max(0.04, level)
}

func (r *proceduralRenderState) emberLevelMysteriousMan() float64 {
	level := 1.0
	if r.timer("inhale") > 0 {
		level *= r.value("inhale_gain", r.cfgFloat("inhale_mult", 1.85))
	}
	if r.timer("lighter-flick") > 0 {
		level *= r.value("flick_gain", r.cfgFloat("lighter_flick_mult", 2.4))
	}
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 70))))
		level *= r.cfgFloat("intro_glow", 0.10) + (1-r.cfgFloat("intro_glow", 0.10))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 85)+r.cfgFloat("ending_linger", 24))))
		level *= 1 - (1-r.cfgFloat("ending_glow", 0.06))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0, level)
}

func (r *proceduralRenderState) revealLevelMysteriousMan() float64 {
	level := 1.0
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 70))))
		level *= math.Pow(proceduralPhaseProgress(total, r.timer("intro")), 1.6)
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 85)+r.cfgFloat("ending_linger", 24))))
		level *= 1 - proceduralPhaseProgress(total, r.timer("ending"))*0.92
	}
	return clamp01(level)
}

func (r *proceduralRenderState) renderSnow() {
	hue := r.cfgFloat("hue", 205)
	sat := r.cfgFloat("sat", 0.12)
	r.verticalGradient(color.RGBA{9, 17, 29, 255}, color.RGBA{23, 38, 58, 255})
	ground := int(float64(r.h) * 0.80)
	moonX := int(float64(r.w) * (0.16 + r.hash(401)*0.18))
	moonY := int(float64(r.h) * (0.14 + r.hash(402)*0.08))
	moonR := math.Max(4, float64(min(r.w, r.h))*0.065)
	for y := 0; y < ground; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x-moonX), float64(y-moonY))
			if d <= moonR*2.6 {
				r.paintAlpha(x, y, color.RGBA{225, 234, 255, 255}, 0.18*(1-d/(moonR*2.6)))
			}
		}
	}
	for y := ground; y < r.h; y++ {
		t := float64(y-ground) / math.Max(1, float64(r.h-ground))
		r.fillRect(0, y, r.w, 1, r.hsl(hue-8, sat*0.55, 0.10+0.22*t))
	}
	for i := 0; i < 13; i++ {
		center := int((float64(i)+0.5)*float64(r.w)/13 + (r.hash(500+i)-0.5)*6)
		crownH := 7 + int(r.hash(560+i)*10)
		maxHalf := 2 + int(r.hash(590+i)*4)
		c := r.hsl(hue-26, sat*0.6, 0.18+r.hash(620+i)*0.08)
		for row := 0; row < crownH; row++ {
			width := max(1, maxHalf-row/2)
			y := ground - crownH + row
			r.fillRect(center-width, y, width*2+1, 1, c)
		}
		trunkH := 1 + int(math.Floor(r.hash(530+i)*2))
		for row := 0; row < trunkH; row++ {
			r.paint(center, ground+row-trunkH, c)
		}
	}
	layers := max(1, int(math.Round(r.cfgFloat("layers", 3))))
	density := r.snowDensity()
	for layer := 0; layer < layers; layer++ {
		layerRatio := 1.0
		if layers > 1 {
			layerRatio = float64(layer) / float64(layers-1)
		}
		layerCount := max(8, int(math.Round(float64(r.w)*density*(0.35+layerRatio*0.8))))
		baseSpeed := r.cfgFloat("speed", 0.45) * (0.4 + layerRatio*0.85)
		drift := r.cfgFloat("drift", 0.1)*(0.35+layerRatio*0.65) + r.value("gust_push", 0)*0.035*(0.5+layerRatio*0.65)
		size := max(1, int(math.Round(r.cfgFloat("size", 1)+layerRatio)))
		for i := 0; i < layerCount; i++ {
			idx := layer*1000 + i
			baseX := r.hash(1000+idx) * float64(r.w)
			baseY := r.hash(2000+idx) * math.Max(1, float64(ground-2))
			sway := (r.hash(3000+idx)*2 - 1) * r.cfgFloat("sway", 0.5) * (1.4 + layerRatio*2.4)
			fall := baseY + float64(r.tick)*baseSpeed*(0.75+r.hash(4000+idx)*0.5)
			row := positiveModFloat(fall, math.Max(1, float64(ground-2)))
			col := positiveModFloat(baseX+float64(r.tick)*drift+math.Sin(float64(r.tick)*0.035+float64(idx)*0.19)*sway, float64(r.w))
			localHue := hue + (r.hash(5000+idx)*2-1)*r.cfgFloat("hue_sp", 16)
			light := clamp01(r.cfgFloat("lmin", 0.34) + (r.cfgFloat("lmax", 0.75)-r.cfgFloat("lmin", 0.34))*(0.35+0.55*(0.3+layerRatio*0.7)))
			alpha := clamp01(0.35 + 0.55*(0.25+layerRatio*0.75))
			r.fillRectAlpha(int(math.Round(col)), int(math.Round(row)), size, size, r.hsl(localHue, sat, light), alpha)
		}
	}
}

func (r *proceduralRenderState) renderStarfield() {
	hue := r.cfgFloat("hue", 218)
	sat := r.cfgFloat("sat", 0.18)
	r.verticalGradient(color.RGBA{5, 9, 18, 255}, color.RGBA{11, 17, 40, 255})
	density := r.cfgFloat("density", 0.22)
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 50))))
		density *= r.cfgFloat("intro_density", 0.08) + (1-r.cfgFloat("intro_density", 0.08))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 60)+r.cfgFloat("ending_linger", 16))))
		density *= 1 - (1-r.cfgFloat("ending_density", 0.03))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	layers := max(1, int(math.Round(r.cfgFloat("layers", 3))))
	burst := 1.0
	if r.timer("twinkle-burst") > 0 {
		burst = r.cfgFloat("twinkle_burst_mult", 1.7)
	}
	for layer := 0; layer < layers; layer++ {
		layerRatio := 1.0
		if layers > 1 {
			layerRatio = float64(layer) / float64(layers-1)
		}
		layerCount := max(10, int(math.Round(float64(r.w)*density*(0.4+layerRatio*1.2))))
		speed := r.cfgFloat("speed", 0.12) * (0.18 + layerRatio*0.82)
		drift := r.cfgFloat("drift", 0.04) * (0.25 + layerRatio*0.9)
		size := max(1, int(math.Round(r.cfgFloat("size", 1)+layerRatio)))
		for i := 0; i < layerCount; i++ {
			idx := layer*1400 + i
			baseX := r.hash(15000+idx) * float64(r.w)
			baseY := r.hash(16000+idx) * float64(r.h)
			col := positiveModFloat(baseX+float64(r.tick)*drift*speed*2, float64(r.w))
			localHue := hue + (r.hash(17000+idx)*2-1)*r.cfgFloat("hue_sp", 18)
			twinkle := 0.4 + 0.6*math.Pow(0.5+0.5*math.Sin(float64(r.tick)*(0.02+r.hash(18000+idx)*0.03)+float64(idx)), 2)
			light := clamp01((r.cfgFloat("lmin", 0.55) + (r.cfgFloat("lmax", 0.95)-r.cfgFloat("lmin", 0.55))*(0.3+layerRatio*0.7)) * twinkle * burst)
			alpha := clamp01(0.35 + 0.25*layerRatio + 0.25*twinkle)
			r.fillRectAlpha(int(math.Round(col)), int(math.Round(baseY)), size, size, r.hsl(localHue, sat, light), alpha)
		}
	}
	if r.timer("shooting-star") > 0 {
		total := math.Max(1, r.value("shooting-star_total", r.cfgFloat("shooting_star_dur", 26)))
		progress := 1 - float64(r.timer("shooting-star"))/total
		row := r.value("shooting_row", float64(r.h)*0.25)
		dir := r.value("shooting_dir", 1)
		start := r.value("shooting_start", float64(r.w)*0.2)
		head := positiveModFloat(start+dir*progress*float64(r.w)*0.6, float64(r.w))
		for i := 0; i < 7; i++ {
			fade := 1 - float64(i)/7
			x := int(math.Round(head - dir*float64(i)*1.5))
			y := int(math.Round(row + float64(i)*0.6))
			light := clamp01(r.cfgFloat("lmax", 0.95) * r.cfgFloat("shooting_star_mult", 1.8) * fade * 0.55)
			r.fillRectAlpha(x, y, 1, 1, r.hsl(hue-8, sat*0.9, light), fade)
		}
	}
}

func (r *proceduralRenderState) renderBeach() {
	hue := r.cfgFloat("hue", 198)
	r.verticalGradient(color.RGBA{244, 177, 122, 255}, color.RGBA{139, 180, 196, 255})
	horizon := max(8, int(math.Floor(float64(r.h)*0.34)))
	tideLevel := r.tideLevelBeach()
	tideBias := r.value("tide_bias", 0)
	foamGain := r.value("foam_gain", 1)
	tidePhase := float64(r.tick) * r.cfgFloat("speed", 0.1) * 0.08
	baseShore := float64(r.h)*r.cfgFloat("shoreline", 0.58) + math.Sin(tidePhase)*r.cfgFloat("tide_amp", 6)*tideLevel*0.34 + tideBias*1.6
	for y := horizon; y < r.h; y++ {
		depth := float64(y-horizon) / math.Max(1, float64(r.h-horizon))
		var c color.RGBA
		if depth < 0.46 {
			c = mixColor(r.hsl(hue-12, r.cfgFloat("sat", 0.5)*0.72, r.cfgFloat("lmax", 0.82)*0.74), r.hsl(hue, r.cfgFloat("sat", 0.5), r.cfgFloat("lmin", 0.28)+(r.cfgFloat("lmax", 0.82)-r.cfgFloat("lmin", 0.28))*0.44), depth/0.46)
		} else {
			c = mixColor(r.hsl(hue, r.cfgFloat("sat", 0.5), r.cfgFloat("lmin", 0.28)+(r.cfgFloat("lmax", 0.82)-r.cfgFloat("lmin", 0.28))*0.44), r.hsl(hue+6, r.cfgFloat("sat", 0.5)*1.06, r.cfgFloat("lmin", 0.28)*0.8), (depth-0.46)/0.54)
		}
		r.fillRect(0, y, r.w, 1, c)
	}
	shoreRows := make([]float64, r.w)
	for x := 0; x < r.w; x++ {
		nx := float64(x) / math.Max(1, float64(r.w-1))
		slopeOffset := (nx - 0.5) * r.cfgFloat("slope", 0.16) * float64(r.h) * 0.34
		wave := math.Sin(float64(x)*r.cfgFloat("wave_freq", 0.18) + tidePhase*2.2)
		backwash := math.Sin(float64(x)*r.cfgFloat("wave_freq", 0.18)*0.46 - tidePhase*1.6 + 0.8)
		chop := math.Sin(float64(x)*r.cfgFloat("wave_freq", 0.18)*1.85 + tidePhase*3.1 + r.hash(24100+x)*3)
		shore := baseShore + slopeOffset + wave*r.cfgFloat("wave_amp", 2.4)*(0.14+tideLevel*0.1) + backwash*r.cfgFloat("wave_amp", 2.4)*0.08 + chop*r.cfgFloat("wave_amp", 2.4)*0.03
		shoreRows[x] = math.Max(float64(horizon+3), math.Min(float64(r.h-4), shore))
	}
	for pass := 0; pass < 2; pass++ {
		for x := 1; x < r.w-1; x++ {
			shoreRows[x] = (shoreRows[x-1] + shoreRows[x]*2 + shoreRows[x+1]) / 4
		}
	}
	for x := 0; x < r.w; x++ {
		shore := shoreRows[x]
		for y := int(math.Round(shore)); y < r.h; y++ {
			depth := float64(y-int(shore)) / math.Max(1, float64(r.h-int(shore)))
			c := mixColor(r.hsl(39, 0.54, 0.8), r.hsl(33, 0.42, 0.54), depth)
			r.paint(x, y, c)
		}
		surfRow := int(math.Round(shore))
		wetBand := max(2, int(math.Round(2+tideLevel*3+math.Max(0, foamGain-1)*1.4)))
		foamBand := max(1, int(math.Round(1+r.cfgFloat("foam", 0.36)*2.8+math.Max(0, foamGain-1)*1.4)))
		wetColor := r.hsl(34, 0.34, 0.34+clamp01(0.22+tideLevel*0.36)*0.12)
		for row := surfRow; row < min(r.h, surfRow+wetBand); row++ {
			fade := 1 - float64(row-int(shore))/math.Max(1, float64(wetBand))
			r.paintAlpha(x, row, wetColor, clamp01(0.14+fade*0.32))
		}
		foamColor := r.hsl(hue-8, r.cfgFloat("sat", 0.5)*0.18, r.cfgFloat("lmax", 0.82)*1.02)
		for i := 0; i < foamBand; i++ {
			pulse := 0.55 + 0.45*math.Sin(float64(r.tick)*0.05+float64(x)*0.18+float64(i)*0.9)
			alpha := clamp01((0.12 + r.cfgFloat("foam", 0.36)*0.42) * foamGain * (0.5 + 0.5*pulse))
			r.paintAlpha(x, surfRow-i, foamColor, alpha)
		}
		if (x+r.tick)%2 == 0 {
			depth := 0.18 + r.hash(24400+x)*0.56
			row := max(horizon+1, int(math.Floor(float64(horizon)+(shore-float64(horizon))*depth)))
			width := 1 + int(math.Floor(r.hash(24500+x)*3))
			blink := 0.35 + 0.65*math.Pow(0.5+0.5*math.Sin(float64(r.tick)*0.03+float64(x)*0.12), 2)
			shimmerColor := r.hsl(hue-12, r.cfgFloat("sat", 0.5)*0.5, r.cfgFloat("lmax", 0.82)*0.96)
			r.fillRectAlpha(x, row, width, 1, shimmerColor, clamp01((0.08+r.cfgFloat("shimmer", 0.22)*0.34)*blink))
		}
	}
}

func (r *proceduralRenderState) renderUnderwater() {
	hue := r.cfgFloat("hue", 192)
	sat := r.cfgFloat("sat", 0.42)
	lmin := r.cfgFloat("lmin", 0.12)
	lmax := r.cfgFloat("lmax", 0.82)
	top := r.hsl(hue-8, sat*0.58, lmin+(lmax-lmin)*0.54)
	mid := r.hsl(hue, sat*0.82, lmin+(lmax-lmin)*0.28)
	deep := r.hsl(hue+10, sat*0.72, lmin*(0.72-r.cfgFloat("depth", 0.56)*0.18))
	for y := 0; y < r.h; y++ {
		t := float64(y) / math.Max(1, float64(r.h-1))
		if t < 0.46 {
			r.fillRect(0, y, r.w, 1, mixColor(top, mid, t/0.46))
		} else {
			r.fillRect(0, y, r.w, 1, mixColor(mid, deep, (t-0.46)/0.54))
		}
	}
	level := r.sceneLevelUnderwater()
	current := 0.0
	if r.timer("current-shift") > 0 {
		current = r.value("current_push", r.cfgFloat("current_shift_push", 1.2))
	}
	floorBase := max(int(math.Floor(float64(r.h)*0.78)), min(r.h-4, int(math.Floor(float64(r.h)*(0.84+r.cfgFloat("depth", 0.56)*0.08)))))
	phase := float64(r.tick) * r.cfgFloat("rise_speed", 0.42) * 0.04
	for i := 0; i < 4; i++ {
		source := int(float64(r.w) * (0.08 + float64(i)*0.24 + r.hash(30100+i)*0.08))
		spread := int(float64(r.w) * (0.08 + r.hash(30200+i)*0.08))
		bend := int((current*3 + math.Sin(float64(r.tick)*0.02+float64(i))*3) * r.cfgFloat("caustics", 0.3))
		for y := 0; y < int(float64(r.h)*0.70); y++ {
			t := float64(y) / math.Max(1, float64(r.h)*0.70)
			left := float64(source) - float64(spread)*0.2 + (float64(spread)+float64(bend)-float64(spread)*0.45)*t
			right := float64(source) + float64(spread)*0.22 + (float64(spread)+float64(bend)-float64(spread)*0.22)*t
			r.fillRectAlpha(int(math.Round(left)), y, max(1, int(math.Round(right-left))), 1, color.RGBA{210, 248, 242, 255}, clamp01(0.025+r.cfgFloat("caustics", 0.3)*0.09*level))
		}
	}
	caustic := r.hsl(hue-10, sat*0.24, lmax*0.96)
	for band := 0; band < 5; band++ {
		baseY := int(float64(r.h) * (0.16 + float64(band)*0.09))
		for x := 0; x < r.w; x++ {
			if (x+band)%2 != 0 {
				continue
			}
			wave := math.Sin(float64(x)*0.16+phase*(1.2+float64(band)*0.18)+float64(band)) + math.Sin(float64(x)*0.07-phase*1.4+float64(band)*1.7)
			row := baseY + int(math.Round(wave*r.cfgFloat("caustics", 0.3)*level*1.3))
			r.paintAlpha(x, row, caustic, clamp01((0.03+r.cfgFloat("caustics", 0.3)*0.18*level)*(0.82-float64(band)*0.12)))
		}
	}
	seabed := r.hsl(hue+36, sat*0.22, lmin*0.85)
	for x := 0; x < r.w; x++ {
		row := floorBase - int(math.Abs(math.Sin(float64(x)*0.08+r.hash(30500+x)*2.4))*2+r.hash(30600+x)*2)
		r.fillRect(x, row, 1, r.h-row, seabed)
	}
	weedCount := max(1, int(math.Round(r.cfgFloat("weed_count", 11))))
	for i := 0; i < weedCount; i++ {
		baseX := int((float64(i)+0.35)*float64(r.w)/float64(weedCount) + (r.hash(30700+i)-0.5)*5)
		rootY := floorBase - 1 - int(r.hash(30800+i)*3)
		fronds := 2 + int(r.hash(30900+i)*2)
		for f := 0; f < fronds; f++ {
			height := max(7, int(math.Round(r.cfgFloat("weed_height", 20)*(0.58+r.hash(31000+i*5+f)*0.5))))
			offset := (float64(f) - float64(fronds-1)/2) * 1.2
			localPhase := float64(r.tick)*0.035*(0.8+r.hash(31100+i*5+f)*0.4) + float64(i)*0.7 + float64(f)*0.4
			for seg := 0; seg < height; seg++ {
				p := float64(seg) / math.Max(1, float64(height-1))
				sway := math.Sin(localPhase+p*2.6)*r.cfgFloat("sway", 0.54)*level*(1.1+math.Abs(current)*0.55) + current*p*1.4
				x := int(math.Round(float64(baseX) + offset + sway*p*1.2))
				c := r.hsl(hue-36, sat*0.6, lmin+(lmax-lmin)*0.28)
				if float64(seg) >= float64(height)*0.28 {
					c = r.hsl(hue-18, sat*0.48, lmin+(lmax-lmin)*0.4)
				}
				r.fillRectAlpha(x, rootY-seg, 1, 1, c, clamp01(0.3+p*0.42))
			}
		}
	}
	particulate := max(18, int(math.Round(float64(r.w)*0.14)))
	for i := 0; i < particulate; i++ {
		x := int(r.hash(30300+i) * float64(r.w))
		y := int(r.hash(30400+i) * float64(max(1, floorBase-6)))
		blink := 0.35 + 0.65*math.Pow(0.5+0.5*math.Sin(float64(r.tick)*0.018+float64(i)*0.6), 2)
		r.paintAlpha(x, y, r.hsl(hue+8, sat*0.16, lmax*0.8), clamp01((0.04+r.cfgFloat("depth", 0.56)*0.08)*blink))
	}
	burstGain := 1.0
	if r.timer("bubble-burst") > 0 {
		burstGain = r.value("bubble_gain", r.cfgFloat("bubble_burst_mult", 1.9))
	}
	count := max(12, int(math.Round(float64(r.w)*math.Max(0.04, r.cfgFloat("density", 0.28)*level)*(0.44+math.Max(0, burstGain-1)*0.18))))
	for i := 0; i < count; i++ {
		baseX := r.hash(31200+i) * float64(r.w)
		baseY := r.hash(31300+i) * math.Max(6, float64(floorBase-6))
		row := 1 + positiveModFloat(baseY-float64(r.tick)*r.cfgFloat("rise_speed", 0.42)*(0.55+r.hash(31400+i)*0.7), math.Max(1, float64(floorBase-5)))
		drift := r.cfgFloat("drift", 0.1)*(0.4+r.hash(31500+i)*0.9) + current*0.05*(0.45+r.hash(31600+i)*0.55)
		wobble := math.Sin(float64(r.tick)*0.03+float64(i)*0.72) * r.cfgFloat("sway", 0.54) * (0.6 + r.hash(31700+i)*0.5)
		col := positiveModFloat(baseX+float64(r.tick)*drift+wobble, float64(r.w))
		size := 1
		if r.hash(31800+i) > 0.82 {
			size = 2
		}
		alpha := clamp01((0.22 + r.hash(31900+i)*0.28) * (0.8 + math.Max(0, burstGain-1)*0.14))
		r.fillRectAlpha(int(math.Round(col)), int(math.Round(row)), size, size, r.hsl(hue-4, sat*0.18, lmax*0.98), alpha)
		r.paintAlpha(int(math.Round(col)), int(math.Round(row)), color.RGBA{235, 248, 246, 255}, alpha*0.62)
	}
}

func (r *proceduralRenderState) renderAurora() {
	hue := r.cfgFloat("hue", 150)
	r.verticalGradient(color.RGBA{2, 6, 15, 255}, color.RGBA{10, 18, 32, 255})
	ground := int(float64(r.h) * 0.82)
	intensity := r.intensityLevelAurora()
	shiftPush := r.value("shift_push", 0)
	shiftSeed := r.value("shift_seed", 0)
	for y := ground - 8; y < r.h; y++ {
		fade := float64(y-(ground-8)) / math.Max(1, float64(r.h-(ground-8)))
		r.fillRectAlpha(0, y, r.w, 1, color.RGBA{48, 168, 140, 255}, clamp01((0.04+intensity*0.16)*fade))
	}
	r.fillRect(0, ground-1, r.w, r.h-ground+1, r.hsl(212, 0.2, 0.04))
	r.starSpeckles(0.0022, r.hsl(210, 0.18, 0.82), ground-10)
	for x := 0; x < r.w; x++ {
		ridge := ground - 5 - int(r.hash(19500+x%8)*6) - int((0.5+0.5*math.Sin(float64(x)*0.04+shiftSeed))*4)
		r.fillRect(x, ridge, 1, r.h-ridge, r.hsl(210, 0.24, 0.055))
	}
	for i := 0; i < 12; i++ {
		center := int((float64(i)+0.5)*float64(r.w)/12 + (r.hash(19900+i)-0.5)*5)
		baseY := ground - 1 - int(r.hash(20300+i)*4)
		crownH := 5 + int(r.hash(20100+i)*5)
		half := 1 + int(r.hash(20200+i)*2)
		c := r.hsl(210+r.hash(20400+i)*10, 0.22, 0.045+r.hash(20500+i)*0.02)
		for row := 0; row < crownH; row++ {
			width := max(1, half-row/2)
			r.fillRect(center-width, baseY-crownH+row, width*2+1, 1, c)
		}
		r.fillRect(center, baseY-1, 1, 2, c)
	}
	bands := max(2, int(math.Round(r.cfgFloat("bands", 4))))
	for band := 0; band < bands; band++ {
		ratio := 0.5
		if bands > 1 {
			ratio = float64(band) / float64(bands-1)
		}
		phase := float64(r.tick)*r.cfgFloat("speed", 0.11)*(0.18+ratio*0.12) + float64(band)*1.6 + shiftSeed
		amp := r.cfgFloat("wave_amp", 6) * (0.8 + ratio*0.35)
		freq := r.cfgFloat("wave_freq", 0.16) * (0.82 + ratio*0.26)
		thickness := r.cfgFloat("thickness", 9) * (0.8 + ratio*0.28)
		curtain := r.cfgFloat("curtain_len", 15) * (0.8 + ratio*0.22)
		baseY := float64(r.h)*(0.16+ratio*0.08) + math.Sin(float64(r.tick)*0.01+float64(band)*0.8)*1.1
		hueBase := hue + (ratio-0.5)*r.cfgFloat("hue_sp", 26)*1.15 + shiftPush*5
		for x := 0; x < r.w; x++ {
			nx := float64(x) / math.Max(1, float64(r.w-1))
			arch := math.Sin(nx*math.Pi*(1.08+ratio*0.24) + float64(band)*0.75)
			wave := math.Sin(float64(x)*freq + phase + float64(r.tick)*r.cfgFloat("drift", 0.08)*0.04)
			sub := math.Sin(float64(x)*freq*0.47 - phase*0.62 + float64(band)*2.1)
			center := baseY + arch*amp*0.72 + wave*amp*0.52 + sub*amp*0.22 + shiftPush*math.Sin(float64(x)*0.07+phase)*1.05
			for y := max(0, int(math.Floor(center-thickness*1.15))); y <= min(ground-2, int(math.Ceil(center+curtain))); y++ {
				dy := float64(y) - center
				core := math.Exp(-(dy * dy) / math.Max(1, thickness*thickness*1.4))
				tail := 0.0
				if float64(y) >= center {
					tail = math.Exp(-(float64(y) - center) / math.Max(1, curtain))
				}
				shimmer := 0.76 + 0.24*math.Sin(float64(r.tick)*0.03+float64(x)*0.1+float64(band)*1.7)
				strength := (core*0.9 + tail*0.7) * intensity * shimmer * (0.58 + 0.42*math.Max(0.2, arch))
				if strength < 0.025 {
					continue
				}
				localHue := hueBase + math.Sin(float64(y)*0.18+float64(x)*0.06+phase)*r.cfgFloat("hue_sp", 26)*0.32 + float64(band)*6
				s := clamp01(r.cfgFloat("sat", 0.72) * (0.84 + core*0.28))
				light := clamp01(r.cfgFloat("lmin", 0.20) + (r.cfgFloat("lmax", 0.74)-r.cfgFloat("lmin", 0.20))*math.Min(1, 0.24+strength))
				alpha := clamp01(strength * (0.34 + core*0.46))
				r.paintAlpha(x, y, r.hsl(localHue, s, light), alpha)
			}
		}
	}
}

func (r *proceduralRenderState) renderAutumnLeaves() {
	hue := r.cfgFloat("hue", 32)
	r.verticalGradient(color.RGBA{32, 23, 15, 255}, color.RGBA{124, 96, 66, 255})
	ground := int(float64(r.h) * 0.82)
	for y := ground; y < r.h; y++ {
		ratio := float64(y-ground) / math.Max(1, float64(r.h-ground))
		r.fillRect(0, y, r.w, 1, r.hsl(38, 0.35, 0.1+ratio*0.16))
	}
	for x := 0; x < r.w; x++ {
		canopyDepth := 4 + int(math.Floor(r.hash(6100+x)*6))
		shade := r.hsl(24+r.hash(6200+x)*18, 0.45, 0.14+r.hash(6300+x)*0.08)
		r.fillRectAlpha(x, 0, 1, canopyDepth, shade, 0.75)
	}
	density := r.autumnDensity()
	layers := max(1, int(math.Round(r.cfgFloat("layers", 3))))
	for layer := 0; layer < layers; layer++ {
		layerRatio := 1.0
		if layers > 1 {
			layerRatio = float64(layer) / float64(layers-1)
		}
		layerCount := max(6, int(math.Round(float64(r.w)*density*(0.28+layerRatio*0.62))))
		baseSpeed := r.cfgFloat("speed", 0.38) * (0.34 + layerRatio*0.72)
		drift := r.cfgFloat("drift", 0.2)*(0.45+layerRatio*0.65) + r.value("gust_push", 0)*0.04*(0.5+layerRatio*0.7)
		size := max(1, int(math.Round(r.cfgFloat("size", 1.1)+layerRatio*0.8)))
		for i := 0; i < layerCount; i++ {
			idx := layer*1000 + i
			baseX := r.hash(7000+idx) * float64(r.w)
			baseY := r.hash(8000+idx) * math.Max(1, float64(ground-2))
			flutter := (r.hash(9000+idx)*2 - 1) * r.cfgFloat("sway", 0.65) * (2.4 + layerRatio*2.8)
			row := positiveModFloat(baseY+float64(r.tick)*baseSpeed*(0.7+r.hash(10000+idx)*0.55), math.Max(1, float64(ground-2)))
			col := positiveModFloat(baseX+float64(r.tick)*drift+math.Sin(float64(r.tick)*0.04+float64(idx)*0.23)*flutter, float64(r.w))
			if r.timer("swirl") > 0 {
				sr := r.value("swirl_row", float64(r.h)*0.55)
				sc := r.value("swirl_col", float64(r.w)*0.5)
				angle := math.Atan2(row-sr, col-sc) + r.value("swirl_spin", 0)*0.015
				radius := math.Hypot(col-sc, row-sr)
				col = positiveModFloat(sc+math.Cos(angle)*radius, float64(r.w))
				row = math.Max(0, math.Min(float64(ground-2), sr+math.Sin(angle)*radius*0.94))
			}
			localHue := hue + (r.hash(11000+idx)*2-1)*r.cfgFloat("hue_sp", 38)
			light := clamp01(r.cfgFloat("lmin", 0.38) + (r.cfgFloat("lmax", 0.72)-r.cfgFloat("lmin", 0.38))*(0.3+r.hash(12000+idx)*0.7))
			alpha := clamp01(0.4 + layerRatio*0.45)
			r.fillRectAlpha(int(math.Round(col)), int(math.Round(row)), size, 1, r.hsl(localHue, r.cfgFloat("sat", 0.72), light), alpha)
			if (idx+r.tick)%3 == 0 {
				offset := 0
				if r.hash(13000+idx) >= 0.5 {
					offset = 1
				}
				accent := r.hsl(localHue+12, r.cfgFloat("sat", 0.72)*0.85, light*1.08)
				r.fillRectAlpha(int(math.Round(col))+offset, int(math.Round(row)), 1, 1, accent, alpha*0.8)
			}
		}
	}
}

func (r *proceduralRenderState) renderWheatField() {
	hue := r.cfgFloat("hue", 48)
	for y := 0; y < r.h; y++ {
		t := float64(y) / math.Max(1, float64(r.h-1))
		var c color.RGBA
		if t < 0.56 {
			c = mixColor(color.RGBA{22, 33, 63, 255}, color.RGBA{85, 103, 133, 255}, t/0.56)
		} else {
			c = mixColor(color.RGBA{85, 103, 133, 255}, color.RGBA{210, 173, 102, 255}, (t-0.56)/0.44)
		}
		r.fillRect(0, y, r.w, 1, c)
	}
	fieldBase := int(math.Floor(float64(r.h) * r.cfgFloat("field_top", 0.62)))
	sunX := int(float64(r.w) * (0.16 + r.hash(21000)*0.18))
	sunY := int(float64(r.h) * (0.2 + r.hash(21001)*0.06))
	for y := 0; y < fieldBase; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x-sunX), float64(y-sunY))
			r.paintAlpha(x, y, color.RGBA{255, 228, 153, 255}, clamp01(0.16*(1-d/(float64(min(r.w, r.h))*0.18))))
		}
	}
	for x := 0; x < r.w; x++ {
		hill := int(float64(r.h)*0.56) - int(r.hash(21100+x%8)*5) - int((0.5+0.5*math.Sin(float64(x)*0.045+r.hash(21200+x%8)*3))*5)
		r.fillRect(x, hill, 1, r.h-hill, r.hsl(38, 0.18, 0.30))
	}
	for y := fieldBase; y < r.h; y++ {
		t := float64(y-fieldBase) / math.Max(1, float64(r.h-fieldBase))
		c := mixColor(color.RGBA{178, 141, 69, 255}, color.RGBA{142, 110, 50, 255}, t)
		r.fillRect(0, y, r.w, 1, c)
	}
	motion := r.motionLevelWheatField()
	density := math.Max(0.08, r.cfgFloat("density", 0.48))
	layers := max(1, int(math.Round(r.cfgFloat("layers", 3))))
	gustPush := r.value("gust_push", 0)
	for layer := 0; layer < layers; layer++ {
		ratio := 1.0
		if layers > 1 {
			ratio = float64(layer) / float64(layers-1)
		}
		amp := motion * (2.4 + ratio*4.8)
		speed := r.cfgFloat("speed", 0.12) * (0.4 + ratio*0.65)
		drift := r.cfgFloat("drift", 0.16)*(0.25+ratio*0.75) + gustPush*0.04*(0.5+ratio*0.5)
		topBase := float64(fieldBase) - r.cfgFloat("stalk_h", 18)*(0.28+ratio*0.46)
		tipChance := clamp01(density * (0.4 + ratio*0.22))
		for x := 0; x < r.w; {
			width := max(1, min(4, int(math.Round(1+r.hash(21700+layer*4000+x)*(1.2+ratio*2.4)))))
			sampleX := min(r.w-1, x+width/2)
			idx := layer*2000 + sampleX
			wave := math.Sin(float64(sampleX)*r.cfgFloat("wave_freq", 0.18)*(0.85+ratio*0.25) + float64(r.tick)*speed + float64(layer)*1.7)
			sub := math.Sin(float64(sampleX)*r.cfgFloat("wave_freq", 0.18)*0.42 - float64(r.tick)*speed*0.62 + float64(layer)*2.3)
			lean := wave*amp + sub*amp*0.32 + math.Sin(float64(r.tick)*0.012+float64(sampleX)*0.05)*drift*3.2
			top := max(0, min(r.h-2, int(math.Round(topBase+lean+r.hash(21300+idx)*2))))
			depth := max(6, int(math.Round(r.cfgFloat("stalk_h", 18)*(0.48+ratio*0.42))))
			localHue := hue + (r.hash(21400+idx)*2-1)*r.cfgFloat("hue_sp", 18)
			light := clamp01(r.cfgFloat("lmin", 0.30) + (r.cfgFloat("lmax", 0.76)-r.cfgFloat("lmin", 0.30))*(0.18+ratio*0.6))
			alpha := clamp01(0.34 + ratio*0.24)
			c := r.hsl(localHue, r.cfgFloat("sat", 0.64), light)
			r.fillRectAlpha(x, top, width, depth, c, alpha)
			r.fillRectAlpha(x, top, width, max(1, int(math.Round(float64(depth)*0.28))), r.hsl(localHue, r.cfgFloat("sat", 0.64)*0.72, light*0.76), alpha*0.55)
			if r.hash(21500+idx) < tipChance {
				tipHeight := 1 + int(r.hash(21600+idx)*(1+ratio*3))
				tipX := max(0, min(r.w-1, int(math.Round(float64(x)+float64(width)*0.5+lean*0.18))))
				r.fillRectAlpha(tipX, top-tipHeight+1, 1, tipHeight, r.hsl(localHue+4, r.cfgFloat("sat", 0.64)*0.82, light*1.08), alpha*0.9)
			}
			x += width
		}
	}
}

func (r *proceduralRenderState) renderCampfire() {
	r.verticalGradient(color.RGBA{8, 17, 26, 255}, color.RGBA{22, 17, 12, 255})
	ground := int(float64(r.h) * 0.84)
	cx := r.w / 2
	flameLevel := r.flameLevelCampfire()
	crackleGain := r.value("crackle_gain", 1)
	halfW := max(2, int(math.Round(r.cfgFloat("flame_width", 10)*0.5)))
	flameH := max(4, int(math.Round(r.cfgFloat("flame_height", 14)*(0.52+flameLevel*0.38))))
	speed := float64(r.tick) * r.cfgFloat("flame_speed", 0.12) * 1.7
	for y := ground; y < r.h; y++ {
		ratio := float64(y-ground) / math.Max(1, float64(r.h-ground))
		r.fillRect(0, y, r.w, 1, r.hsl(18, 0.24, 0.08+ratio*0.12))
	}
	glowStrength := clamp01(r.cfgFloat("glow", 0.54) * (0.6 + flameLevel*0.45))
	for y := 0; y < ground; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x-cx), float64(y-ground))
			r.paintAlpha(x, y, color.RGBA{255, 120, 44, 255}, clamp01(glowStrength*0.35*(1-d/(float64(min(r.w, r.h))*0.32))))
		}
	}
	logHalf := halfW + 2
	for dx := -logHalf; dx <= logHalf; dx++ {
		r.paint(cx+dx, ground+1+int(math.Round(float64(dx)*0.12)), r.hsl(20, 0.46, 0.22))
		r.paintAlpha(cx+dx, ground+1-int(math.Round(float64(dx)*0.1)), r.hsl(20, 0.46, 0.22), 0.82)
	}
	for dx := -halfW; dx <= halfW; dx++ {
		coalHeat := 0.45 + 0.55*math.Pow(0.5+0.5*math.Sin(speed*1.8+float64(dx)*0.8), 2)
		r.paintAlpha(cx+dx, ground, r.hsl(r.cfgFloat("hue", 24)-4, r.cfgFloat("sat", 0.82)*0.88, r.cfgFloat("lmin", 0.32)+(r.cfgFloat("lmax", 0.94)-r.cfgFloat("lmin", 0.32))*(0.22+coalHeat*0.45)), 0.28+coalHeat*0.42)
	}
	for x := -halfW; x <= halfW; x++ {
		nx := math.Abs(float64(x)) / math.Max(1, float64(halfW))
		widthShape := math.Max(0, 1-math.Pow(nx, 1.32))
		if widthShape <= 0.04 {
			continue
		}
		pulse := 0.8 + 0.2*math.Sin(speed*1.3+float64(x)*0.7+r.hash(26000+x+halfW)*5)
		columnH := max(2, int(math.Round(float64(flameH)*widthShape*pulse)))
		for y := 0; y < columnH; y++ {
			lift := float64(y) / math.Max(1, float64(columnH))
			taper := 1 - lift
			sway := math.Sin(speed*2.1+float64(x)*0.35+float64(y)*0.24+r.hash(26100+y*31+x+400)*6) * r.cfgFloat("flicker", 0.72) * taper * 0.72
			col := int(math.Round(float64(cx+x) + sway))
			row := ground - 1 - y
			localHue := r.cfgFloat("hue", 24) - lift*r.cfgFloat("hue_sp", 18)*0.34 + (r.hash(26200+x*17+y+700)*2-1)*r.cfgFloat("hue_sp", 18)*0.08
			light := clamp01(r.cfgFloat("lmin", 0.32) + (r.cfgFloat("lmax", 0.94)-r.cfgFloat("lmin", 0.32))*(0.18+taper*0.8))
			alpha := clamp01((0.12 + taper*0.56) * (0.36 + widthShape*0.64) * (0.72 + flameLevel*0.18))
			r.paintAlpha(col, row, r.hsl(localHue, r.cfgFloat("sat", 0.82)*(0.88+taper*0.16), light), alpha)
			if widthShape > 0.35 && lift < 0.6 && (x+y)%2 == 0 {
				r.paintAlpha(col, row, r.hsl(r.cfgFloat("hue", 24)+8, r.cfgFloat("sat", 0.82)*0.74, r.cfgFloat("lmax", 0.94)*(0.72+taper*0.24)), alpha*0.52)
			}
		}
	}
	emberCount := max(4, int(math.Round(r.cfgFloat("flame_width", 10)*(0.8+r.cfgFloat("ember_rate", 0.26)*3.8))))
	for i := 0; i < emberCount; i++ {
		maxRise := max(10, int(math.Round(r.cfgFloat("flame_height", 14)*2.1+r.cfgFloat("ember_speed", 0.62)*12)))
		cycle := maxRise + 8 + int(r.hash(27000+i)*12)
		progress := positiveModFloat(float64(r.tick)*r.cfgFloat("ember_speed", 0.62)*(0.7+r.hash(27100+i)*0.7)+r.hash(27200+i)*float64(cycle), float64(cycle))
		if progress > float64(maxRise) {
			continue
		}
		fade := 1 - progress/math.Max(1, float64(maxRise))
		drift := (r.hash(27300+i)*2-1)*(1.2+progress*0.08) + math.Sin(speed+float64(i)*0.7)*0.6
		row := ground - 2 - int(math.Round(progress))
		size := 1
		if fade > 0.72 && r.timer("crackle") > 0 && r.hash(27400+i) > 0.5 {
			size = 2
		}
		r.fillRectAlpha(cx+int(math.Round(drift)), row, size, 1, r.hsl(r.cfgFloat("hue", 24)-6+r.hash(27500+i)*10, r.cfgFloat("sat", 0.82)*0.8, r.cfgFloat("lmin", 0.32)+(r.cfgFloat("lmax", 0.94)-r.cfgFloat("lmin", 0.32))*(0.42+fade*0.5)), clamp01((0.16+fade*0.68)*(0.78+math.Max(0, crackleGain-1)*0.16)))
	}
}

func (r *proceduralRenderState) renderLighthouse() {
	hue := r.cfgFloat("hue", 214)
	sat := r.cfgFloat("sat", 0.34)
	lmin := r.cfgFloat("lmin", 0.12)
	lmax := r.cfgFloat("lmax", 0.84)
	top := r.hsl(hue+358, sat*0.5, lmin*0.92)
	mid := r.hsl(hue, sat, lmin+(lmax-lmin)*0.18)
	low := r.hsl(hue-r.cfgFloat("hue_sp", 18)*0.6, sat*0.82, lmin+(lmax-lmin)*0.46)
	for y := 0; y < r.h; y++ {
		t := float64(y) / math.Max(1, float64(r.h-1))
		if t < 0.62 {
			r.fillRect(0, y, r.w, 1, mixColor(top, mid, t/0.62))
		} else {
			r.fillRect(0, y, r.w, 1, mixColor(mid, low, (t-0.62)/0.38))
		}
	}
	horizon := max(8, min(r.h-10, int(math.Floor(float64(r.h)*r.cfgFloat("horizon", 0.74)))))
	towerX := int(float64(r.w) * 0.18)
	towerH := max(10, int(math.Round(r.cfgFloat("tower_height", 22))))
	towerW := max(3, int(math.Round(r.cfgFloat("tower_width", 6.5))))
	beamLevel := r.beamLevelLighthouse()
	fogLevel := clamp01(r.cfgFloat("haze", 0.14))
	if r.timer("fog-thicken") > 0 {
		fogLevel = clamp01(fogLevel * r.value("fog_gain", r.cfgFloat("fog_thicken_mult", 1.85)))
	}
	seaTop := r.hsl(hue, sat*0.55, lmin+(lmax-lmin)*0.12)
	seaLow := r.hsl(hue+8, sat*0.42, lmin*0.8)
	for y := horizon; y < r.h; y++ {
		r.fillRect(0, y, r.w, 1, mixColor(seaTop, seaLow, float64(y-horizon)/math.Max(1, float64(r.h-horizon))))
	}
	coastRows := make([]int, r.w)
	for x := 0; x < r.w; x++ {
		bluff := math.Exp(-math.Pow((float64(x-towerX))/18, 2)) * 7.5
		swell := math.Sin(float64(x)*0.032+0.5)*1.3 + math.Sin(float64(x)*0.013+2.1)*1.1
		coastRows[x] = int(math.Round(float64(horizon) + swell - bluff))
		r.fillRect(x, coastRows[x], 1, r.h-coastRows[x], r.hsl(hue+214, sat*0.12, 0.08))
	}
	for x := 0; x < r.w; x += 2 {
		if (x+r.tick)%5 == 0 {
			r.fillRectAlpha(x, horizon+1+int(math.Round(math.Sin(float64(x)*0.07+float64(r.tick)*0.02))), 2, 1, r.hsl(hue, sat*0.35, lmin+(lmax-lmin)*0.3), 0.18)
		}
	}
	towerBase := coastRows[max(0, min(r.w-1, towerX))]
	lampY := towerBase - towerH + 2
	towerColor := r.hsl(hue+212, sat*0.08, 0.1)
	for y := lampY; y <= towerBase; y++ {
		ratio := float64(y-lampY) / math.Max(1, float64(towerBase-lampY))
		half := max(1, int(math.Round((float64(towerW)*(0.32+ratio*0.68))*0.5)))
		r.fillRect(towerX-half, y, half*2+1, 1, towerColor)
	}
	r.fillRect(towerX-max(2, int(math.Round(float64(towerW)*0.6))), lampY-1, max(4, int(math.Round(float64(towerW)*1.2))), 1, towerColor)
	lampGlow := r.hsl(48, 0.68, clamp01(0.42+r.cfgFloat("glow", 0.22)*0.34))
	r.fillRectAlpha(towerX, lampY, 1, 1, lampGlow, clamp01(0.3+r.cfgFloat("glow", 0.22)*0.7))
	beamAngle := -0.26 + math.Sin(float64(r.tick)*r.cfgFloat("sweep_speed", 0.08)*0.06)*0.78
	beamWidth := r.cfgFloat("beam_width", 0.22) * (1 + fogLevel*0.9)
	beamSoft := clamp01(r.cfgFloat("beam_softness", 0.42) * (1 + fogLevel*0.35))
	beamBase := r.hsl(hue-18, sat*0.18, lmax*0.98)
	beamCore := r.hsl(44, 0.42, lmax*1.02)
	for y := 0; y <= horizon+5; y++ {
		for x := towerX + 1; x < r.w; x++ {
			dx := float64(x - towerX)
			dy := float64(y - lampY)
			dist := math.Hypot(dx, dy)
			if dist < 2 {
				continue
			}
			diff := math.Abs(math.Atan2(math.Sin(math.Atan2(dy, dx)-beamAngle), math.Cos(math.Atan2(dy, dx)-beamAngle)))
			if diff > beamWidth {
				continue
			}
			cone := 1 - diff/beamWidth
			strength := math.Pow(cone, math.Max(0.6, 1.8-beamSoft)) * math.Pow(clamp01(1-dist/(float64(r.w)*0.92)), 0.72) * beamLevel
			if strength < 0.02 {
				continue
			}
			r.paintAlpha(x, y, beamBase, strength*(0.42+fogLevel*0.16))
			if diff < beamWidth*0.34 && strength > 0.08 {
				r.paintAlpha(x, y, beamCore, strength*0.34)
			}
		}
	}
	for y := horizon - 8; y < horizon+10; y++ {
		r.fillRectAlpha(0, y, r.w, 1, color.RGBA{201, 214, 226, 255}, clamp01((0.02+fogLevel*0.08)*(1-float64(y-(horizon-8))/18)))
	}
}

func (r *proceduralRenderState) renderRowboat() {
	hue := r.cfgFloat("hue", 206)
	sat := r.cfgFloat("sat", 0.36)
	lmin := r.cfgFloat("lmin", 0.16)
	lmax := r.cfgFloat("lmax", 0.82)
	r.verticalGradient(r.hsl(hue-12, sat*0.45, lmin+0.02), r.hsl(hue+r.cfgFloat("hue_sp", 16)*0.18, sat*0.72, lmin+(lmax-lmin)*0.62))
	waterline := max(12, min(r.h-10, int(math.Floor(float64(r.h)*r.cfgFloat("waterline", 0.58)))))
	motion := r.rippleLevelRowboat()
	driftPush := r.value("drift_push", 0)
	phase := float64(r.tick) * r.cfgFloat("drift_speed", 0.08) * 0.08
	glowX := int(float64(r.w) * 0.74)
	glowY := int(float64(r.h) * 0.24)
	for y := 0; y < waterline; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x-glowX), float64(y-glowY))
			r.paintAlpha(x, y, color.RGBA{255, 223, 174, 255}, clamp01(0.16*(1-d/(float64(min(r.w, r.h))*0.20))))
		}
	}
	ridgeBase := waterline - max(5, int(math.Round(float64(r.h)*0.12)))
	shorelineY := max(ridgeBase+2, waterline-max(2, int(math.Round(float64(r.h)*0.04))))
	for x := 0; x < r.w; x++ {
		ridge := ridgeBase - int(math.Abs(math.Sin(float64(x)*0.055+r.hash(25100+x%8)*2.4))*3+r.hash(25200+x%8)*3)
		r.fillRect(x, ridge, 1, shorelineY-ridge, r.hsl(hue+54, sat*0.24, lmin*0.7))
	}
	for i := 0; i < 11; i++ {
		col := int((float64(i)+0.4)*float64(r.w)/11 + (r.hash(25300+i)-0.5)*6)
		top := ridgeBase - 1 - int(r.hash(25400+i)*4)
		r.fillRectAlpha(col, top, 1, 2+int(r.hash(25500+i)*3), r.hsl(hue+72, sat*0.2, lmin*0.52), 0.92)
	}
	for y := waterline; y < r.h; y++ {
		depth := float64(y-waterline) / math.Max(1, float64(r.h-waterline))
		r.fillRect(0, y, r.w, 1, r.hsl(hue+depth*r.cfgFloat("hue_sp", 16)*0.22, sat*(0.8-depth*0.22), lmin+(lmax-lmin)*(0.36-depth*0.18)))
	}
	r.fillRectAlpha(0, shorelineY-3, r.w, waterline-shorelineY+11, color.RGBA{255, 240, 220, 255}, 0.08)
	surfaceColor := r.hsl(hue-6, sat*0.34, lmax*0.92)
	for x := 0; x < r.w; x++ {
		wave := math.Sin(float64(x)*r.cfgFloat("wave_freq", 0.16)+phase)*r.cfgFloat("wave_amp", 1.6) + math.Sin(float64(x)*r.cfgFloat("wave_freq", 0.16)*0.42-phase*1.7)*r.cfgFloat("wave_amp", 1.6)*0.36
		row := waterline + int(math.Round(wave*motion*0.22))
		twinkle := 0.45 + 0.55*math.Pow(0.5+0.5*math.Sin(float64(r.tick)*0.03+float64(x)*0.11), 2)
		r.paintAlpha(x, row, surfaceColor, clamp01((0.05+r.cfgFloat("reflection", 0.22)*0.16)*twinkle))
	}
	boatLen := max(7, int(math.Round(r.cfgFloat("boat_len", 14))))
	boatHeight := max(2, int(math.Round(r.cfgFloat("boat_height", 3.5))))
	boatX := max(int(math.Floor(float64(boatLen)*0.6)), min(r.w-int(math.Ceil(float64(boatLen)*0.6)), int(math.Round(float64(r.w)*0.34+math.Sin(phase*1.6+0.8)*(2.4+motion*1.6)+driftPush*1.8))))
	bob := math.Sin(phase*2.4+0.6)*r.cfgFloat("bob_amp", 1.2)*motion*0.52 + math.Sin(phase*0.95+1.3)*0.35
	hullBaseY := waterline - int(math.Round(bob*0.55))
	tilt := math.Sin(phase*1.9+0.4)*motion*0.9 + driftPush*0.2
	type hullRow struct{ start, width, y int }
	rows := []hullRow{}
	for row := 0; row < boatHeight; row++ {
		t := 0.5
		if boatHeight > 1 {
			t = float64(row) / float64(boatHeight-1)
		}
		arch := 1 - math.Abs(t*2-1)
		width := max(4, int(math.Round(float64(boatLen)*(0.54+arch*0.42))))
		y := hullBaseY - (boatHeight - 1 - row)
		start := int(math.Round(float64(boatX) - float64(width)/2 + (t-0.5)*tilt*1.8))
		rows = append(rows, hullRow{start, width, y})
		r.fillRectAlpha(start, y, width, 1, r.hsl(24, 0.34, 0.22), 0.78+t*0.2)
	}
	if len(rows) > 0 {
		r.fillRectAlpha(rows[0].start+1, rows[0].y, rows[0].width-2, 1, r.hsl(31, 0.26, 0.36), 0.72)
	}
	r.fillRectAlpha(boatX-boatLen/2, waterline, boatLen, 1, r.hsl(hue+10, sat*0.28, lmin*0.9), 0.26)
	refl := r.hsl(hue+8, sat*0.28, lmax*0.58)
	for i, row := range rows {
		distance := hullBaseY - row.y + 1
		alpha := clamp01(r.cfgFloat("reflection", 0.22) * (0.3 + motion*0.22) * (0.4 - float64(distance)*0.045))
		r.fillRectAlpha(row.start+int(math.Round(math.Sin(float64(r.tick)*0.08+float64(i)*0.8+float64(row.start)*0.03)*(0.5+motion*0.45))), hullBaseY+distance, max(2, row.width-1-distance/2), 1, refl, alpha)
	}
	rippleColor := r.hsl(hue-10, sat*0.32, lmax*0.98)
	wakeGain := 1.0
	if r.timer("wake") > 0 {
		wakeGain = r.value("wake_gain", r.cfgFloat("wake_mult", 1.85))
	}
	for band := 0; band < 4+int(math.Round(r.cfgFloat("ripple", 0.24)*7)); band++ {
		centerX := int(math.Round(float64(boatX) + float64(boatLen)*0.18 + float64(band)*(1.2+wakeGain*0.28)))
		half := max(3, int(math.Round(float64(boatLen)*(0.24+float64(band)*0.14+wakeGain*0.03))))
		centerY := waterline + int(math.Round(0.8+float64(band)*0.75+math.Abs(math.Sin(phase*4.2+float64(band)*0.9))*1.1))
		for dx := -half; dx <= half; dx++ {
			if (dx+band+r.tick)%2 != 0 {
				continue
			}
			edge := 1 - math.Abs(float64(dx))/math.Max(1, float64(half))
			r.paintAlpha(centerX+dx, centerY+int(math.Round(math.Sin(float64(dx)*0.22+float64(r.tick)*0.08+float64(band)*0.7)*0.7)), rippleColor, clamp01((0.08+r.cfgFloat("ripple", 0.24)*0.24*motion)*math.Pow(edge, 0.45)))
		}
	}
}

func (r *proceduralRenderState) renderTrain() {
	hue := r.cfgFloat("hue", 220)
	sat := r.cfgFloat("sat", 0.42)
	lmin := r.cfgFloat("lmin", 0.10)
	lmax := r.cfgFloat("lmax", 0.78)
	r.verticalGradient(r.hsl(hue, sat*0.52, lmin+0.02), r.hsl(hue+r.cfgFloat("hue_sp", 18), sat*0.26, lmin+0.06))
	horizon := int(math.Floor(float64(r.h) * r.cfgFloat("horizon", 0.70)))
	trackY := int(math.Floor(float64(r.h) * r.cfgFloat("track_y", 0.78)))
	for y := horizon; y < r.h; y++ {
		depth := float64(y-horizon) / math.Max(1, float64(r.h-horizon))
		r.fillRect(0, y, r.w, 1, r.hsl(hue+r.cfgFloat("hue_sp", 18)*0.8, sat*0.18, lmin*(0.65+depth*0.35)))
	}
	rail := r.hsl(32, 0.3, 0.35)
	r.fillRect(0, trackY, r.w, 1, rail)
	r.fillRect(0, trackY+2, r.w, 1, r.hsl(24, 0.22, 0.18))
	for x := 0; x < r.w; x += 4 {
		r.fillRect(x, trackY+1, 2, 2, r.hsl(26, 0.2, 0.14))
	}
	lifecycle := r.trainLifecycleLevel()
	if r.timer("ending") > 0 {
		r.fillRectAlpha(r.w/2-4, trackY-1, 8, 1, r.hsl(36, 0.4, lmax*0.85), lifecycle*r.cfgFloat("ending_glow", 0.1)*0.33)
	}
	kind := ""
	left := 0
	total := 0
	dir := 1.0
	if r.timer("express") > 0 {
		kind = "express"
		left = r.timer("express")
		total = int(math.Round(r.value("express_total", r.cfgFloat("express_dur", 110))))
		dir = r.value("express_dir", 1)
	} else if r.timer("pass") > 0 {
		kind = "pass"
		left = r.timer("pass")
		total = int(math.Round(r.value("pass_total", r.cfgFloat("pass_dur", 160))))
		dir = r.value("pass_dir", 1)
	}
	if kind == "" {
		return
	}
	elapsed := total - left
	cueLead := max(0, int(math.Round(r.cfgFloat("cue_lead", 14))))
	tailLinger := max(0, int(math.Round(r.cfgFloat("tail_linger", 12))))
	movement := max(1, total-cueLead-tailLinger)
	isExpress := kind == "express"
	intensity := lifecycle
	if isExpress {
		intensity *= r.cfgFloat("express_speed_mult", 1.7)
	}
	if elapsed < cueLead {
		progress := clamp01(float64(elapsed) / math.Max(1, float64(max(1, cueLead))))
		r.renderTrainCue(trackY, dir, progress, intensity, isExpress)
		return
	}
	if elapsed >= cueLead+movement {
		progress := clamp01(float64(elapsed-cueLead-movement) / math.Max(1, float64(max(1, tailLinger))))
		r.renderTrainTail(trackY, dir, progress, intensity, isExpress)
		return
	}
	mvProgress := float64(elapsed-cueLead) / math.Max(1, float64(movement))
	cars := max(0, int(math.Round(r.cfgFloat("cars", 3))))
	locoLen := max(3, int(math.Round(r.cfgFloat("loco_len", 7))))
	carLen := max(2, int(math.Round(r.cfgFloat("car_len", 6))))
	totalLen := locoLen + cars*(carLen+1)
	span := r.w + totalLen + 4
	travel := float64(-totalLen-2) + float64(span)*mvProgress
	headX := travel
	if dir < 0 {
		headX = float64(r.w-1) - travel
	}
	r.renderTrainBody(trackY, int(math.Round(headX)), dir, cars, locoLen, carLen, intensity, isExpress)
}

func (r *proceduralRenderState) trainLifecycleLevel() float64 {
	level := 1.0
	if r.timer("intro") > 0 {
		total := int(math.Round(r.value("intro_total", r.cfgFloat("intro_dur", 60))))
		level *= r.cfgFloat("intro_glow", 0.4) + (1-r.cfgFloat("intro_glow", 0.4))*proceduralPhaseProgress(total, r.timer("intro"))
	}
	if r.timer("ending") > 0 {
		total := int(math.Round(r.value("ending_total", r.cfgFloat("ending_dur", 70)+r.cfgFloat("ending_linger", 24))))
		level *= 1 - (1-r.cfgFloat("ending_glow", 0.1))*proceduralPhaseProgress(total, r.timer("ending"))
	}
	return math.Max(0.04, level)
}

func (r *proceduralRenderState) renderTrainCue(trackY int, dir, progress, intensity float64, express bool) {
	alpha := clamp01((r.cfgFloat("intro_glow", 0.4) + (1-r.cfgFloat("intro_glow", 0.4))*progress) * r.cfgFloat("light_glow", 0.45) * intensity)
	if express {
		alpha *= 1.25
	}
	edgeX := -2.0
	if dir < 0 {
		edgeX = float64(r.w + 2)
	}
	cx := edgeX + 6 + progress*4
	if dir < 0 {
		cx = edgeX - 6 - progress*4
	}
	for y := -2; y <= 2; y++ {
		for x := -6; x <= 6; x++ {
			if absInt(x)+absInt(y) <= 7 {
				r.paintAlpha(int(math.Round(cx+float64(x))), trackY-1+y, r.hsl(42, 0.75, 0.72), alpha*0.35)
			}
		}
	}
}

func (r *proceduralRenderState) renderTrainTail(trackY int, dir, progress, intensity float64, express bool) {
	fade := (1 - progress) * intensity
	if express {
		fade *= 1.2
	} else {
		fade *= 0.9
	}
	exitX := r.w - 4
	if dir < 0 {
		exitX = 3
	}
	dust := r.hsl(r.cfgFloat("hue", 220)+r.cfgFloat("hue_sp", 18), r.cfgFloat("sat", 0.42)*0.32, r.cfgFloat("lmax", 0.78)*0.7)
	for i := 0; i < 5; i++ {
		x := float64(exitX) - dir*(float64(i)*1.6+progress*5)
		y := float64(trackY-1) - float64(i)*1.2 + math.Sin(float64(r.tick)*0.18+float64(i)*1.1)*1.1
		r.paintAlpha(int(math.Round(x)), int(math.Round(y)), dust, clamp01((0.18+r.cfgFloat("smoke", 0.32)*0.22)*fade*(1-float64(i)/6)))
	}
}

func (r *proceduralRenderState) renderTrainBody(trackY, headX int, dir float64, cars, locoLen, carLen int, intensity float64, express bool) {
	trainH := max(2, int(math.Round(r.cfgFloat("train_height", 5))))
	baseY := trackY - 1
	topY := baseY - trainH + 1
	hull := r.hsl(r.cfgFloat("hue", 220)+200, r.cfgFloat("sat", 0.42)*0.24, r.cfgFloat("lmin", 0.10)+0.04)
	cab := r.hsl(r.cfgFloat("hue", 220)+210, r.cfgFloat("sat", 0.42)*0.18, r.cfgFloat("lmin", 0.10)*0.85)
	trimHue := 28.0
	if express {
		trimHue = 14
	}
	trim := r.hsl(trimHue, 0.38, r.cfgFloat("lmax", 0.78)*0.6)
	window := r.hsl(48, 0.7, r.cfgFloat("lmax", 0.78)*0.95)
	wheel := r.hsl(0, 0, 0.06)
	locoBack := headX - int(dir)*locoLen + int(dir)
	locoLeft := min(headX, locoBack)
	locoRight := max(headX, locoBack)
	r.fillRectAlpha(locoLeft, topY, locoLen, trainH, hull, 0.96)
	cabLen := max(2, int(math.Round(float64(locoLen)*0.45)))
	cabX := locoLeft
	if dir < 0 {
		cabX = locoRight - cabLen + 1
	}
	r.fillRectAlpha(cabX, topY, cabLen, trainH, cab, 0.95)
	if trainH >= 3 {
		winX := cabX + 1
		if dir > 0 {
			winX = cabX + max(0, cabLen-2)
		}
		r.paintAlpha(winX, topY+1, window, 0.9*intensity)
	}
	r.fillRectAlpha(locoLeft, baseY-1, locoLen, 1, trim, 0.7)
	stackX := locoRight - 1
	if dir < 0 {
		stackX = locoLeft + 1
	}
	r.paintAlpha(stackX, topY-1, cab, 0.95)
	r.paintAlpha(headX, baseY, trim, 0.85)
	for i := 0; i < locoLen; i += 2 {
		r.paintAlpha(locoLeft+i, baseY+1, wheel, 0.85)
	}
	headlightY := topY + max(0, int(math.Floor(float64(trainH)*0.55)))
	light := r.hsl(54, 0.78, 0.72)
	r.paintAlpha(headX, headlightY, light, 0.9*intensity)
	haloAlpha := clamp01(r.cfgFloat("light_glow", 0.45) * intensity)
	if express {
		haloAlpha *= 1.35
	}
	for y := -2; y <= 2; y++ {
		for x := -5; x <= 5; x++ {
			if absInt(x)+absInt(y) <= 6 {
				r.paintAlpha(headX+x, headlightY+y, light, haloAlpha*0.25)
			}
		}
	}
	for i := 0; i < 6; i++ {
		x := float64(stackX) - dir*(float64(i)*1.4+0.5)
		y := float64(topY-1) - float64(i)*0.9 + math.Sin(float64(r.tick)*0.19+float64(i)*0.6)*0.7
		r.paintAlpha(int(math.Round(x)), int(math.Round(y)), r.hsl(0, 0, 0.74), clamp01(r.cfgFloat("smoke", 0.32)*intensity*(1-float64(i)/7)*0.55))
	}
	carH := max(2, trainH-1)
	for i := 0; i < cars; i++ {
		carAnchor := locoLeft - 1 - i*(carLen+1)
		if dir < 0 {
			carAnchor = locoRight + 1 + i*(carLen+1)
		}
		carLeft := carAnchor - carLen + 1
		if dir < 0 {
			carLeft = carAnchor
		}
		carTop := baseY - carH + 1
		carColor := r.hsl(r.cfgFloat("hue", 220)+196+float64(i%2)*-8, r.cfgFloat("sat", 0.42)*0.22, r.cfgFloat("lmin", 0.10)+0.06+float64(i%2)*0.02)
		r.fillRectAlpha(carLeft, carTop, carLen, carH, carColor, 0.95)
		r.fillRectAlpha(carLeft, baseY-1, carLen, 1, trim, 0.55)
		for wx := 1; wx < carLen-1; wx += 2 {
			r.paintAlpha(carLeft+wx, carTop, window, 0.7*intensity)
		}
		for wx := 0; wx < carLen; wx += 2 {
			r.paintAlpha(carLeft+wx, baseY+1, wheel, 0.85)
		}
	}
}

func (r *proceduralRenderState) renderVolcano() {
	r.verticalGradient(color.RGBA{18, 10, 18, 255}, color.RGBA{44, 20, 16, 255})
	baseY := max(8, min(r.h-4, int(math.Floor(float64(r.h)*r.cfgFloat("horizon", 0.86)))))
	cx := r.w / 2
	pressure := r.pressureLevelVolcano()
	halfW := max(4, int(math.Round(r.cfgFloat("cone_width", 46)*0.5)))
	coneH := max(6, int(math.Round(r.cfgFloat("cone_height", 28))))
	craterHalf := max(2, int(math.Round(r.cfgFloat("crater_width", 8)*0.5)))
	peakRow := baseY - coneH
	for y := baseY - 4; y < baseY+8; y++ {
		r.fillRectAlpha(0, y, r.w, 1, r.hsl(r.cfgFloat("hue", 18), r.cfgFloat("sat", 0.78)*0.6, r.cfgFloat("lmin", 0.18)+(r.cfgFloat("lmax", 0.92)-r.cfgFloat("lmin", 0.18))*0.18), clamp01(0.18+r.cfgFloat("glow", 0.55)*0.18+pressure*0.08))
	}
	for y := baseY; y < r.h; y++ {
		ratio := float64(y-baseY) / math.Max(1, float64(r.h-baseY))
		r.fillRect(0, y, r.w, 1, r.hsl(r.cfgFloat("hue", 18)+352, r.cfgFloat("sat", 0.78)*0.16, r.cfgFloat("lmin", 0.18)*(0.4+ratio*0.45)))
	}
	coneColor := r.hsl(r.cfgFloat("hue", 18)+350, r.cfgFloat("sat", 0.78)*0.18, r.cfgFloat("lmin", 0.18)*0.55)
	coneEdge := r.hsl(r.cfgFloat("hue", 18)+348, r.cfgFloat("sat", 0.78)*0.24, r.cfgFloat("lmin", 0.18)*0.78)
	for dx := -halfW; dx <= halfW; dx++ {
		nx := math.Abs(float64(dx)) / math.Max(1, float64(halfW))
		slope := math.Pow(1-nx, 1.3)
		topY := baseY - int(math.Round(float64(coneH)*slope+(r.hash(31000+dx)*2-1)*r.cfgFloat("slope_jitter", 1.6)))
		if int(math.Abs(float64(dx))) < craterHalf {
			topY = peakRow + max(1, int(math.Round(2+float64(craterHalf)*0.4*(1-math.Abs(float64(dx))/math.Max(1, float64(craterHalf))))))
		}
		col := cx + dx
		for y := max(0, topY); y <= baseY; y++ {
			c := coneColor
			if y == topY || y == topY+1 {
				c = coneEdge
			}
			r.paint(col, y, c)
		}
	}
	glowStrength := clamp01(r.cfgFloat("glow", 0.55) * pressure)
	for y := peakRow - 12; y < peakRow+14; y++ {
		for x := cx - 22; x <= cx+22; x++ {
			d := math.Hypot(float64(x-cx), float64(y-peakRow))
			r.paintAlpha(x, y, r.hsl(r.cfgFloat("hue", 18)+4, r.cfgFloat("sat", 0.78)*0.95, r.cfgFloat("lmax", 0.92)*(0.7+glowStrength*0.25)), clamp01((0.35+glowStrength*0.45)*(1-d/24)))
		}
	}
	for dx := -craterHalf; dx <= craterHalf; dx++ {
		t := 1 - math.Abs(float64(dx))/math.Max(1, float64(craterHalf))
		row := peakRow + max(1, int(math.Round(2+float64(craterHalf)*0.4*t)))
		r.paintAlpha(cx+dx, row, r.hsl(r.cfgFloat("hue", 18)+r.hash(31300+dx)*r.cfgFloat("hue_sp", 16)*0.4, r.cfgFloat("sat", 0.78), r.cfgFloat("lmin", 0.18)+(r.cfgFloat("lmax", 0.92)-r.cfgFloat("lmin", 0.18))*(0.4+t*0.5+glowStrength*0.2)), 0.35+t*0.5+glowStrength*0.25)
	}
	smokeStrength := clamp01(r.cfgFloat("smoke", 0.32))
	if r.timer("smolder") > 0 {
		smokeStrength *= r.cfgFloat("smolder_mult", 0.55)
	}
	for i := 0; i < max(4, int(math.Round(8+smokeStrength*14))); i++ {
		maxRise := max(8, int(math.Round(r.cfgFloat("smoke_height", 18))))
		progress := positiveModFloat(float64(r.tick)*0.12*(0.7+r.hash(31700+i)*0.6)+r.hash(31800+i)*float64(maxRise+10), float64(maxRise+10))
		if progress > float64(maxRise) {
			continue
		}
		fade := 1 - progress/math.Max(1, float64(maxRise))
		col := cx + int(math.Round(math.Sin(float64(r.tick)*0.04+float64(i)*0.7)*(1.4+progress*0.12)+(r.hash(31900+i)*2-1)*1.6))
		row := peakRow - 1 - int(math.Round(progress))
		r.paintAlpha(col, row, r.hsl(r.cfgFloat("hue", 18)+12+r.hash(32000+i)*r.cfgFloat("hue_sp", 16)*0.4, r.cfgFloat("sat", 0.78)*0.32, r.cfgFloat("lmin", 0.18)+(r.cfgFloat("lmax", 0.92)-r.cfgFloat("lmin", 0.18))*(0.36+fade*0.34)), 0.18+fade*smokeStrength*0.7)
	}
	eruption := r.timer("eruption") > 0
	if eruption {
		total := int(math.Round(r.cfgFloat("eruption_dur", 80)))
		env := math.Sin(math.Pi * proceduralPhaseProgress(max(total, r.timer("eruption")), r.timer("eruption")))
		archHeight := max(6, int(math.Round(r.cfgFloat("eruption_height", 28)*(0.65+env*0.5)*r.value("eruption_gain", 1)*0.55)))
		count := max(8, int(math.Round(r.cfgFloat("crater_width", 8)*1.6+float64(archHeight)*0.8)))
		seed := int(r.value("eruption_seed", 0))
		for i := 0; i < count; i++ {
			cycle := max(14, int(math.Round(float64(archHeight)*1.2+14)))
			t := positiveModFloat(float64(r.tick)*0.4*(0.7+r.hash(32200+i+seed)*0.6)+r.hash(32300+i+seed)*float64(cycle), float64(cycle)) / float64(cycle)
			angle := (r.hash(32400+i+seed)*2 - 1) * math.Pi * 0.42
			v0 := float64(archHeight) * (0.7 + r.hash(32500+i+seed)*0.6)
			col := cx + int(math.Round(math.Sin(angle)*(1.2+float64(craterHalf)*0.6)+(r.hash(32600+i+seed)*2-1)*v0*0.18*t))
			row := peakRow + 1 + int(math.Round(-v0*math.Sin(math.Pi*t)))
			fade := 1 - math.Pow(t, 1.6)
			size := 1
			if fade > 0.7 {
				size = 2
			}
			r.fillRectAlpha(col, row, size, 1, r.hsl(r.cfgFloat("hue", 18)+(r.hash(32700+i+seed)*2-1)*r.cfgFloat("hue_sp", 16)*0.5, r.cfgFloat("sat", 0.78), r.cfgFloat("lmin", 0.18)+(r.cfgFloat("lmax", 0.92)-r.cfgFloat("lmin", 0.18))*(0.55+fade*0.45)), 0.45+fade*0.5)
		}
	}
	for y := peakRow + 2; y < baseY; y++ {
		w := max(1, int(2+math.Sin(float64(y)*0.2+float64(r.tick)*0.05)*1.5))
		r.fillRectAlpha(cx-w/2, y, w, 1, r.hsl(r.cfgFloat("hue", 18), r.cfgFloat("sat", 0.78), 0.45), 0.45)
	}
}

func (r *proceduralRenderState) renderWindmill() {
	hue := r.cfgFloat("hue", 28)
	sat := r.cfgFloat("sat", 0.42)
	lmin := r.cfgFloat("lmin", 0.18)
	lmax := r.cfgFloat("lmax", 0.82)
	top := r.hsl(hue+210, sat*0.5, lmin*0.95)
	mid := r.hsl(hue+248, sat*0.42, lmin+(lmax-lmin)*0.32)
	low := r.hsl(hue, sat*0.82, lmin+(lmax-lmin)*0.78)
	for y := 0; y < r.h; y++ {
		t := float64(y) / math.Max(1, float64(r.h-1))
		if t < 0.58 {
			r.fillRect(0, y, r.w, 1, mixColor(top, mid, t/0.58))
		} else {
			r.fillRect(0, y, r.w, 1, mixColor(mid, low, (t-0.58)/0.42))
		}
	}
	horizon := max(8, min(r.h-8, int(math.Floor(float64(r.h)*r.cfgFloat("horizon", 0.72)))))
	cx := int(float64(r.w) * 0.58)
	for x := 0; x < r.w; x++ {
		broad := math.Sin(float64(x)*0.045+0.4)*1.2 + math.Sin(float64(x)*0.012+1.3)*2.1
		mound := math.Exp(-math.Pow(float64(x-cx)/18, 2))*6.2 + math.Exp(-math.Pow((float64(x)-float64(r.w)*0.18)/24, 2))*2.1
		topY := int(math.Round(float64(horizon) + broad - mound))
		r.fillRect(x, topY, 1, r.h-topY, r.hsl(hue+205, sat*0.16, 0.08))
	}
	ground := int(math.Round(float64(horizon) - math.Exp(0)*6.2))
	towerH := max(10, int(math.Round(r.cfgFloat("tower_height", 20))))
	towerW := max(3, int(math.Round(r.cfgFloat("tower_width", 6))))
	hubY := ground - towerH + 2
	millColor := r.hsl(hue+208, sat*0.08, 0.10)
	for y := hubY; y <= ground; y++ {
		ratio := float64(y-hubY) / math.Max(1, float64(ground-hubY))
		half := max(1, int(math.Round((float64(towerW)*(0.38+ratio*0.62))*0.5)))
		r.fillRect(cx-half, y, half*2+1, 1, millColor)
	}
	r.fillRect(cx-max(2, int(math.Round(float64(towerW)*0.42))), hubY-2, max(4, int(math.Round(float64(towerW)*0.84))), 1, millColor)
	r.fillRectAlpha(cx, int(math.Round(float64(hubY)+float64(towerH)*0.46)), 1, 2, r.hsl(42, 0.72, 0.38+r.cfgFloat("glow", 0.18)*0.5), 0.18+r.cfgFloat("glow", 0.18)*0.58)
	angle := float64(r.tick)*r.cfgFloat("turn_speed", 0.08)*r.rotationLevelWindmill() + math.Pi*0.08
	length := max(5, int(math.Round(r.cfgFloat("blade_len", 14))))
	bladeWidth := r.cfgFloat("blade_width", 1.8)
	bladeColor := r.hsl(hue+210, sat*0.08, 0.13)
	for b := 0; b < 4; b++ {
		a := angle + float64(b)*math.Pi/2
		px := -math.Sin(a)
		py := math.Cos(a) * 0.88
		for d := 0; d < length; d++ {
			fade := 1 - float64(d)/math.Max(1, float64(length))
			bx := float64(cx) + math.Cos(a)*float64(d)
			by := float64(hubY) + math.Sin(a)*float64(d)*0.88
			half := max(0, int(math.Round(bladeWidth*fade*0.55)))
			for spread := -half; spread <= half; spread++ {
				r.paint(int(math.Round(bx+px*float64(spread)*0.7)), int(math.Round(by+py*float64(spread)*0.7)), bladeColor)
			}
		}
	}
	r.fillRect(cx-1, hubY-1, 3, 3, millColor)
}

func (r *proceduralRenderState) renderMysteriousMan() {
	hue := r.cfgFloat("hue", 22)
	r.verticalGradient(r.hsl(hue+220, r.cfgFloat("sat", 0.72)*0.18, r.cfgFloat("lmin", 0.06)+0.02), r.hsl(hue+6, r.cfgFloat("sat", 0.72)*0.32, r.cfgFloat("lmin", 0.06)+0.06))
	cx := int(float64(r.w) * r.cfgFloat("figure_x", 0.5))
	figH := max(12, int(math.Round(r.cfgFloat("figure_height", 30))))
	figW := max(4, int(math.Round(r.cfgFloat("figure_width", 11))))
	halfBody := max(2, int(math.Round(float64(figW)*0.5)))
	ground := min(r.h-1, int(math.Floor(float64(r.h)*0.94)))
	headRadius := max(2, int(math.Round(float64(figW)*0.32)))
	headTop := max(2, ground-figH)
	headCenterY := headTop + headRadius
	shoulderRow := headCenterY + headRadius + 1
	reveal := r.revealLevelMysteriousMan()
	ember := r.emberLevelMysteriousMan()
	emberX := int(float64(r.w) * r.cfgFloat("ember_x", 0.56))
	emberY := int(float64(r.h) * r.cfgFloat("ember_y", 0.62))
	emberPulse := clamp01(r.cfgFloat("ember_brightness", 0.86) * (1 + math.Sin(float64(r.tick)*0.05)*r.cfgFloat("ember_pulse", 0.34)*0.45) * ember)
	for y := emberY - 12; y <= emberY+12; y++ {
		for x := emberX - 18; x <= emberX+18; x++ {
			d := math.Hypot(float64(x-emberX), float64(y-emberY))
			r.paintAlpha(x, y, r.hsl(hue+4, r.cfgFloat("sat", 0.72)*0.95, r.cfgFloat("lmax", 0.86)*0.9), clamp01((0.32+emberPulse*0.4)*(1-d/18)))
		}
	}
	silAlpha := clamp01(r.cfgFloat("silhouette", 0.92) * reveal)
	sil := r.hsl(hue+220, r.cfgFloat("sat", 0.72)*0.1, r.cfgFloat("lmin", 0.06)*0.4)
	for y := shoulderRow; y <= ground; y++ {
		t := float64(y-shoulderRow) / math.Max(1, float64(ground-shoulderRow))
		half := max(2, int(math.Round(float64(halfBody)*(0.78+t*0.34))))
		r.fillRectAlpha(cx-half, y, half*2+1, 1, sil, silAlpha)
	}
	for dx := -(halfBody + 2); dx <= halfBody+2; dx++ {
		nx := math.Abs(float64(dx)) / math.Max(1, float64(halfBody+2))
		top := shoulderRow - int(math.Round(r.cfgFloat("shoulder", 1)*1.4*math.Pow(1-nx, 1.6)))
		r.fillRectAlpha(cx+dx, top, 1, shoulderRow+2-top, sil, silAlpha)
	}
	for dy := -headRadius; dy <= headRadius; dy++ {
		span := int(math.Round(math.Sqrt(math.Max(0, float64(headRadius*headRadius-dy*dy)))))
		r.fillRectAlpha(cx-span, headCenterY+dy, span*2+1, 1, sil, silAlpha)
	}
	brimHalf := headRadius + max(1, int(math.Round(r.cfgFloat("hat", 1)*2)))
	brimY := max(0, headCenterY-headRadius)
	r.fillRectAlpha(cx-brimHalf, brimY, brimHalf*2+1, 1, sil, silAlpha)
	for dy := 1; dy <= max(1, int(math.Round(r.cfgFloat("hat", 1)*2))); dy++ {
		crownHalf := max(1, headRadius-int(math.Round(float64(dy)*0.4)))
		r.fillRectAlpha(cx-crownHalf, brimY-dy, crownHalf*2+1, 1, sil, silAlpha)
	}
	rimDir := 1
	if emberX < cx {
		rimDir = -1
	}
	for y := shoulderRow; y <= ground; y++ {
		dist := math.Hypot(float64(cx+halfBody*rimDir-emberX), float64(y-emberY))
		r.paintAlpha(cx+halfBody*rimDir, y, r.hsl(hue, r.cfgFloat("sat", 0.72)*0.6, r.cfgFloat("lmin", 0.06)+(r.cfgFloat("lmax", 0.86)-r.cfgFloat("lmin", 0.06))*0.32), clamp01(math.Exp(-dist/math.Max(4, float64(figH)*0.6))*(0.18+emberPulse*0.32)))
	}
	mouthX := cx + int(math.Round(float64(headRadius)*0.6))*rimDir
	mouthY := headCenterY + max(1, int(math.Round(float64(headRadius)*0.5)))
	steps := max(1, int(math.Abs(float64(emberX-mouthX))))
	for i := 1; i < steps; i++ {
		t := float64(i) / math.Max(1, float64(steps))
		x := mouthX + rimDir*i
		y := int(math.Round(float64(mouthY) + float64(emberY-mouthY)*t))
		r.paintAlpha(x, y, r.hsl(0, 0, 0.55), 0.22*silAlpha)
	}
	r.paintAlpha(emberX, emberY, r.hsl(hue+6, r.cfgFloat("sat", 0.72)*0.95, r.cfgFloat("lmax", 0.86)*(0.86+emberPulse*0.14)), 0.6+emberPulse*0.4)
	r.fillRectAlpha(emberX-1, emberY, 3, 1, r.hsl(hue+350, r.cfgFloat("sat", 0.72), r.cfgFloat("lmin", 0.06)+(r.cfgFloat("lmax", 0.86)-r.cfgFloat("lmin", 0.06))*0.7), 0.32+emberPulse*0.32)
	smokeCount := max(2, int(math.Round(r.cfgFloat("smoke_density", 0.42)*22*reveal)))
	for i := 0; i < smokeCount; i++ {
		maxRise := max(8, int(math.Round(float64(r.h)*0.42+r.cfgFloat("smoke_rise", 0.46)*14)))
		progress := positiveModFloat(float64(r.tick)*r.cfgFloat("smoke_rise", 0.46)*(0.5+r.hash(28100+i)*0.9)+r.hash(28200+i)*float64(maxRise+12), float64(maxRise+12))
		if progress > float64(maxRise) {
			continue
		}
		fade := 1 - progress/math.Max(1, float64(maxRise))
		col := emberX + int(math.Round((r.hash(28300+i)*2-1)*0.6+r.cfgFloat("smoke_drift", 0.18)*(0.3+progress*0.06)+math.Sin(float64(r.tick)*0.03+float64(i)*0.7)*0.4))
		row := emberY - 1 - int(math.Round(progress))
		size := 1
		if fade > 0.6 {
			size = 2
		}
		r.fillRectAlpha(col, row, size, size, r.hsl(hue+220, 0.06, 0.62+r.cfgFloat("lmin", 0.06)*0.4), clamp01((0.08+fade*0.42)*(0.6+r.cfgFloat("smoke_softness", 0.62)*0.5)*reveal))
	}
}

func (r *proceduralRenderState) starSpeckles(density float64, c color.RGBA, maxY int) {
	count := int(float64(r.w*max(1, maxY)) * density)
	for i := 0; i < count; i++ {
		x := int(r.hash(12000+i) * float64(r.w))
		y := int(r.hash(14000+i) * float64(max(1, maxY)))
		r.paint(x, y, c)
	}
}
