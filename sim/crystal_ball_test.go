package sim

import (
	"reflect"
	"testing"
)

func TestCrystalBallSchemaContainsIssueKnobsAndTriggers(t *testing.T) {
	schema := CrystalBallSchema()
	if schema.Name != "crystal-ball" {
		t.Fatalf("schema name = %q, want crystal-ball", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("crystal-ball ending should hold a terminal dim state")
	}
	wantKnobs := map[string]bool{
		"swirl":        true,
		"mistRate":     true,
		"visionChance": true,
		"glowPulse":    true,
		"hue":          true,
	}
	wantTriggers := map[string]bool{
		CrystalBallEventIntro:      true,
		CrystalBallEventEnding:     true,
		CrystalBallEventVisionForm: true,
		CrystalBallEventGlowPulse:  true,
		CrystalBallEventSwirl:      true,
	}
	for _, knob := range schema.Knobs {
		delete(wantKnobs, knob.Key)
		delete(wantTriggers, knob.Trigger)
	}
	if len(wantKnobs) != 0 {
		t.Fatalf("schema missing knobs: %v", wantKnobs)
	}
	if len(wantTriggers) != 0 {
		t.Fatalf("schema missing triggers: %v", wantTriggers)
	}
}

func TestCrystalBallFrameReadsAsOrbAndHueChangesFrame(t *testing.T) {
	violet := NewCrystalBall(64, 40, 11, CrystalBallConfig{Hue: 276})
	if filled := countCrystalBallFilled(violet.Frame()); filled < 64*40 {
		t.Fatalf("frame has %d filled pixels, want a full ambient backing scene", filled)
	}
	if brightness := crystalBallFrameBrightness(violet.Frame()); brightness < 26000 {
		t.Fatalf("default frame brightness = %d, want visible orb and stand", brightness)
	}
	emerald := NewCrystalBall(64, 40, 11, CrystalBallConfig{Hue: 142})
	if reflect.DeepEqual(violet.Frame(), emerald.Frame()) {
		t.Fatal("hue knob did not change the rendered frame")
	}
}

func TestCrystalBallVisionAndGlowAreTransient(t *testing.T) {
	ball := NewCrystalBall(64, 40, 21, CrystalBallConfig{VisionChance: 0.0001, GlowPulse: 1.1})
	baseline := crystalBallFrameBrightness(ball.Frame())
	if !ball.TriggerEvent(CrystalBallEventVisionForm) {
		t.Fatal("vision-form trigger rejected")
	}
	if !ball.TriggerEvent(CrystalBallEventGlowPulse) {
		t.Fatal("glow-pulse trigger rejected")
	}
	for i := 0; i < 45; i++ {
		ball.Step()
	}
	active := crystalBallFrameBrightness(ball.Frame())
	if active <= baseline {
		t.Fatalf("vision/glow did not brighten frame: baseline=%d active=%d", baseline, active)
	}
	for i := 0; i < 420; i++ {
		ball.Step()
	}
	if got := ball.Snapshot().Lifecycle; got != LifecycleRunning {
		t.Fatalf("transient events changed lifecycle to %q, want running", got)
	}
}

func TestCrystalBallSnapshotRestoreRoundTrip(t *testing.T) {
	src := NewCrystalBall(72, 42, 42, CrystalBallConfig{
		Swirl:        1.15,
		MistRate:     0.92,
		VisionChance: 0.0004,
		GlowPulse:    0.95,
		Hue:          226,
	})
	for i := 0; i < 18; i++ {
		src.Step()
	}
	src.TriggerEvent(CrystalBallEventVisionForm)
	src.TriggerEvent(CrystalBallEventSwirl)
	for i := 0; i < 11; i++ {
		src.Step()
	}

	dst := NewCrystalBall(72, 42, 7, CrystalBallConfig{})
	dst.SetConfig(src.EffectiveConfig())
	dst.RestoreSnapshot(src.Snapshot())
	if !reflect.DeepEqual(src.Snapshot(), dst.Snapshot()) {
		t.Fatalf("snapshot mismatch after restore\nsrc=%+v\ndst=%+v", src.Snapshot(), dst.Snapshot())
	}
	if !reflect.DeepEqual(src.Frame(), dst.Frame()) {
		t.Fatal("frame mismatch after restore")
	}
	for i := 0; i < 30; i++ {
		src.Step()
		dst.Step()
	}
	if !reflect.DeepEqual(src.Snapshot(), dst.Snapshot()) {
		t.Fatal("snapshot mismatch after replayed steps")
	}
	if !reflect.DeepEqual(src.Frame(), dst.Frame()) {
		t.Fatal("frame mismatch after replayed steps")
	}
}

func TestCrystalBallPersistedRoundTripAndTerminalEnding(t *testing.T) {
	ball := NewCrystalBall(64, 40, 88, CrystalBallConfig{MistRate: 1.1})
	ball.TriggerEvent(CrystalBallEventGlowPulse)
	for i := 0; i < 20; i++ {
		ball.Step()
	}
	persisted := ball.SnapshotPersistedState()

	restored := NewCrystalBall(64, 40, 1, CrystalBallConfig{MistRate: 1.1})
	restored.RestorePersistedState(persisted)
	if !reflect.DeepEqual(persisted, restored.SnapshotPersistedState()) {
		t.Fatal("persisted state mismatch after restore")
	}
	before := crystalBallFrameBrightness(restored.Frame())
	if !restored.TriggerEvent(CrystalBallEventEnding) {
		t.Fatal("ending trigger rejected")
	}
	if got := restored.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("lifecycle = %q, want ending", got)
	}
	for i := 0; i < 420 && restored.Snapshot().Lifecycle != LifecycleEnded; i++ {
		restored.Step()
	}
	if got := restored.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", got)
	}
	after := crystalBallFrameBrightness(restored.Frame())
	if after >= before/2 {
		t.Fatalf("terminal frame did not dim enough: before=%d after=%d", before, after)
	}
	for i := 0; i < 30; i++ {
		restored.Step()
	}
	if got := restored.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	held := crystalBallFrameBrightness(restored.Frame())
	if held != after {
		t.Fatalf("terminal frame changed while held: after=%d held=%d", after, held)
	}
	if !restored.TriggerEvent(CrystalBallEventIntro) {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := restored.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func TestCrystalBallDevTriggers(t *testing.T) {
	ball := NewCrystalBall(48, 32, 99, CrystalBallConfig{})
	for _, event := range []string{
		CrystalBallEventIntro,
		CrystalBallEventVisionForm,
		CrystalBallEventGlowPulse,
		CrystalBallEventSwirl,
		CrystalBallEventClear,
		CrystalBallEventEnding,
	} {
		if !ball.TriggerEvent(event) {
			t.Fatalf("%s trigger rejected", event)
		}
	}
	if ball.TriggerEvent("nope") {
		t.Fatal("unknown trigger accepted")
	}
}

func countCrystalBallFilled(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled {
				count++
			}
		}
	}
	return count
}

func crystalBallFrameBrightness(grid [][]Pixel) int {
	total := 0
	for _, row := range grid {
		for _, p := range row {
			total += int(p.C.R) + int(p.C.G) + int(p.C.B)
		}
	}
	return total
}
