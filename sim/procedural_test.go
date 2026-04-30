package sim

import "testing"

func TestSnowSchema(t *testing.T) {
	schema := SnowSchema()
	if schema.Name != "snow" {
		t.Fatalf("schema name = %q, want snow", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected snow schema knobs")
	}
}

func TestAutumnLeavesSchema(t *testing.T) {
	schema := AutumnLeavesSchema()
	if schema.Name != "autumn-leaves" {
		t.Fatalf("schema name = %q, want autumn-leaves", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected autumn leaves schema knobs")
	}
}

func TestStarfieldSchema(t *testing.T) {
	schema := StarfieldSchema()
	if schema.Name != "starfield" {
		t.Fatalf("schema name = %q, want starfield", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected starfield schema knobs")
	}
}

func TestAuroraSchema(t *testing.T) {
	schema := AuroraSchema()
	if schema.Name != "aurora" {
		t.Fatalf("schema name = %q, want aurora", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected aurora schema knobs")
	}
}

func TestWheatFieldSchema(t *testing.T) {
	schema := WheatFieldSchema()
	if schema.Name != "wheat-field" {
		t.Fatalf("schema name = %q, want wheat-field", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected wheat-field schema knobs")
	}
}

func TestBeachSchema(t *testing.T) {
	schema := BeachSchema()
	if schema.Name != "beach" {
		t.Fatalf("schema name = %q, want beach", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected beach schema knobs")
	}
}

func TestCampfireSchema(t *testing.T) {
	schema := CampfireSchema()
	if schema.Name != "campfire" {
		t.Fatalf("schema name = %q, want campfire", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected campfire schema knobs")
	}
}

func TestWindmillSchema(t *testing.T) {
	schema := WindmillSchema()
	if schema.Name != "windmill" {
		t.Fatalf("schema name = %q, want windmill", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected windmill schema knobs")
	}
}

func TestLighthouseSchema(t *testing.T) {
	schema := LighthouseSchema()
	if schema.Name != "lighthouse" {
		t.Fatalf("schema name = %q, want lighthouse", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected lighthouse schema knobs")
	}
}

func TestRowboatSchema(t *testing.T) {
	schema := RowboatSchema()
	if schema.Name != "rowboat" {
		t.Fatalf("schema name = %q, want rowboat", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected rowboat schema knobs")
	}
}

func TestUnderwaterSchema(t *testing.T) {
	schema := UnderwaterSchema()
	if schema.Name != "underwater" {
		t.Fatalf("schema name = %q, want underwater", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected underwater schema knobs")
	}
}

func TestSnowSnapshotRestore(t *testing.T) {
	p := NewSnow(160, 80, 42, nil)
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	if !p.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Tick != 1 {
		t.Fatalf("tick = %d, want 1", snap.Tick)
	}
	if snap.Timers["intro"] <= 0 {
		t.Fatal("expected intro timer in snapshot")
	}

	restored := NewSnow(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Tick != snap.Tick {
		t.Fatalf("restored tick = %d, want %d", again.Tick, snap.Tick)
	}
	if again.Timers["intro"] != snap.Timers["intro"] {
		t.Fatalf("restored intro timer = %d, want %d", again.Timers["intro"], snap.Timers["intro"])
	}
}

func TestAutumnLeavesSnapshotRestore(t *testing.T) {
	p := NewAutumnLeaves(160, 80, 99, nil)
	if !p.TriggerEvent("swirl") {
		t.Fatal("expected swirl trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Tick != 1 {
		t.Fatalf("tick = %d, want 1", snap.Tick)
	}
	if snap.Timers["swirl"] <= 0 {
		t.Fatal("expected swirl timer in snapshot")
	}

	restored := NewAutumnLeaves(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["swirl"] != snap.Timers["swirl"] {
		t.Fatalf("restored swirl timer = %d, want %d", again.Timers["swirl"], snap.Timers["swirl"])
	}
}

func TestStarfieldSnapshotRestore(t *testing.T) {
	p := NewStarfield(160, 80, 12, nil)
	if !p.TriggerEvent("shooting-star") {
		t.Fatal("expected shooting-star trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["shooting-star"] <= 0 {
		t.Fatal("expected shooting-star timer in snapshot")
	}

	restored := NewStarfield(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["shooting-star"] != snap.Timers["shooting-star"] {
		t.Fatalf("restored shooting-star timer = %d, want %d", again.Timers["shooting-star"], snap.Timers["shooting-star"])
	}
}

func TestAuroraSnapshotRestore(t *testing.T) {
	p := NewAurora(160, 80, 77, nil)
	if !p.TriggerEvent("shift") {
		t.Fatal("expected shift trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["shift"] <= 0 {
		t.Fatal("expected shift timer in snapshot")
	}
	if snap.Values["shift_push"] == 0 {
		t.Fatal("expected shift push value in snapshot")
	}

	restored := NewAurora(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["shift"] != snap.Timers["shift"] {
		t.Fatalf("restored shift timer = %d, want %d", again.Timers["shift"], snap.Timers["shift"])
	}
	if again.Values["shift_push"] != snap.Values["shift_push"] {
		t.Fatalf("restored shift push = %f, want %f", again.Values["shift_push"], snap.Values["shift_push"])
	}
}

func TestWheatFieldSnapshotRestore(t *testing.T) {
	p := NewWheatField(160, 80, 88, nil)
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["gust"] <= 0 {
		t.Fatal("expected gust timer in snapshot")
	}
	if snap.Values["gust_push"] == 0 {
		t.Fatal("expected gust push value in snapshot")
	}

	restored := NewWheatField(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["gust"] != snap.Timers["gust"] {
		t.Fatalf("restored gust timer = %d, want %d", again.Timers["gust"], snap.Timers["gust"])
	}
	if again.Values["gust_push"] != snap.Values["gust_push"] {
		t.Fatalf("restored gust push = %f, want %f", again.Values["gust_push"], snap.Values["gust_push"])
	}
}

func TestBeachSnapshotRestore(t *testing.T) {
	p := NewBeach(160, 80, 91, nil)
	if !p.TriggerEvent("foam-burst") {
		t.Fatal("expected foam-burst trigger to succeed")
	}
	if !p.TriggerEvent("high-tide") {
		t.Fatal("expected high-tide trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["foam-burst"] <= 0 {
		t.Fatal("expected foam-burst timer in snapshot")
	}
	if snap.Timers["high-tide"] <= 0 {
		t.Fatal("expected high-tide timer in snapshot")
	}

	restored := NewBeach(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["foam-burst"] != snap.Timers["foam-burst"] {
		t.Fatalf("restored foam-burst timer = %d, want %d", again.Timers["foam-burst"], snap.Timers["foam-burst"])
	}
	if again.Values["tide_bias"] != snap.Values["tide_bias"] {
		t.Fatalf("restored tide bias = %f, want %f", again.Values["tide_bias"], snap.Values["tide_bias"])
	}
}

func TestProceduralCampfireSnapshotRestore(t *testing.T) {
	p := NewProcedural("campfire", 160, 80, 33, nil)
	if !p.TriggerEvent("crackle") {
		t.Fatal("expected crackle trigger to succeed")
	}
	if !p.TriggerEvent("lull") {
		t.Fatal("expected lull trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["crackle"] <= 0 {
		t.Fatal("expected crackle timer in snapshot")
	}
	if snap.Timers["lull"] <= 0 {
		t.Fatal("expected lull timer in snapshot")
	}
	if snap.Values["crackle_gain"] <= 1 {
		t.Fatal("expected crackle gain value in snapshot")
	}

	restored := NewProcedural("campfire", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["crackle"] != snap.Timers["crackle"] {
		t.Fatalf("restored crackle timer = %d, want %d", again.Timers["crackle"], snap.Timers["crackle"])
	}
	if again.Timers["lull"] != snap.Timers["lull"] {
		t.Fatalf("restored lull timer = %d, want %d", again.Timers["lull"], snap.Timers["lull"])
	}
	if again.Values["crackle_gain"] != snap.Values["crackle_gain"] {
		t.Fatalf("restored crackle gain = %f, want %f", again.Values["crackle_gain"], snap.Values["crackle_gain"])
	}
}

func TestProceduralWindmillSnapshotRestore(t *testing.T) {
	p := NewProcedural("windmill", 160, 80, 44, nil)
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	if !p.TriggerEvent("lull") {
		t.Fatal("expected lull trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["gust"] <= 0 {
		t.Fatal("expected gust timer in snapshot")
	}
	if snap.Timers["lull"] <= 0 {
		t.Fatal("expected lull timer in snapshot")
	}
	if snap.Values["gust_gain"] <= 1 {
		t.Fatal("expected gust gain value in snapshot")
	}

	restored := NewProcedural("windmill", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["gust"] != snap.Timers["gust"] {
		t.Fatalf("restored gust timer = %d, want %d", again.Timers["gust"], snap.Timers["gust"])
	}
	if again.Timers["lull"] != snap.Timers["lull"] {
		t.Fatalf("restored lull timer = %d, want %d", again.Timers["lull"], snap.Timers["lull"])
	}
	if again.Values["gust_gain"] != snap.Values["gust_gain"] {
		t.Fatalf("restored gust gain = %f, want %f", again.Values["gust_gain"], snap.Values["gust_gain"])
	}
}

func TestProceduralLighthouseSnapshotRestore(t *testing.T) {
	p := NewProcedural("lighthouse", 160, 80, 55, nil)
	if !p.TriggerEvent("bright-pass") {
		t.Fatal("expected bright-pass trigger to succeed")
	}
	if !p.TriggerEvent("fog-thicken") {
		t.Fatal("expected fog-thicken trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["bright-pass"] <= 0 {
		t.Fatal("expected bright-pass timer in snapshot")
	}
	if snap.Timers["fog-thicken"] <= 0 {
		t.Fatal("expected fog-thicken timer in snapshot")
	}
	if snap.Values["bright_gain"] <= 1 {
		t.Fatal("expected bright gain value in snapshot")
	}
	if snap.Values["fog_gain"] <= 1 {
		t.Fatal("expected fog gain value in snapshot")
	}

	restored := NewProcedural("lighthouse", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["bright-pass"] != snap.Timers["bright-pass"] {
		t.Fatalf("restored bright-pass timer = %d, want %d", again.Timers["bright-pass"], snap.Timers["bright-pass"])
	}
	if again.Timers["fog-thicken"] != snap.Timers["fog-thicken"] {
		t.Fatalf("restored fog-thicken timer = %d, want %d", again.Timers["fog-thicken"], snap.Timers["fog-thicken"])
	}
	if again.Values["bright_gain"] != snap.Values["bright_gain"] {
		t.Fatalf("restored bright gain = %f, want %f", again.Values["bright_gain"], snap.Values["bright_gain"])
	}
	if again.Values["fog_gain"] != snap.Values["fog_gain"] {
		t.Fatalf("restored fog gain = %f, want %f", again.Values["fog_gain"], snap.Values["fog_gain"])
	}
}

func TestProceduralRowboatSnapshotRestore(t *testing.T) {
	p := NewProcedural("rowboat", 160, 80, 66, nil)
	if !p.TriggerEvent("wake") {
		t.Fatal("expected wake trigger to succeed")
	}
	if !p.TriggerEvent("drift") {
		t.Fatal("expected drift trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["wake"] <= 0 {
		t.Fatal("expected wake timer in snapshot")
	}
	if snap.Timers["drift"] <= 0 {
		t.Fatal("expected drift timer in snapshot")
	}
	if snap.Values["wake_gain"] <= 1 {
		t.Fatal("expected wake gain value in snapshot")
	}
	if snap.Values["drift_push"] == 0 {
		t.Fatal("expected drift push value in snapshot")
	}

	restored := NewProcedural("rowboat", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["wake"] != snap.Timers["wake"] {
		t.Fatalf("restored wake timer = %d, want %d", again.Timers["wake"], snap.Timers["wake"])
	}
	if again.Timers["drift"] != snap.Timers["drift"] {
		t.Fatalf("restored drift timer = %d, want %d", again.Timers["drift"], snap.Timers["drift"])
	}
	if again.Values["wake_gain"] != snap.Values["wake_gain"] {
		t.Fatalf("restored wake gain = %f, want %f", again.Values["wake_gain"], snap.Values["wake_gain"])
	}
	if again.Values["drift_push"] != snap.Values["drift_push"] {
		t.Fatalf("restored drift push = %f, want %f", again.Values["drift_push"], snap.Values["drift_push"])
	}
}

func TestProceduralUnderwaterSnapshotRestore(t *testing.T) {
	p := NewProcedural("underwater", 160, 80, 77, nil)
	if !p.TriggerEvent("bubble-burst") {
		t.Fatal("expected bubble-burst trigger to succeed")
	}
	if !p.TriggerEvent("current-shift") {
		t.Fatal("expected current-shift trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["bubble-burst"] <= 0 {
		t.Fatal("expected bubble-burst timer in snapshot")
	}
	if snap.Timers["current-shift"] <= 0 {
		t.Fatal("expected current-shift timer in snapshot")
	}
	if snap.Values["bubble_gain"] <= 1 {
		t.Fatal("expected bubble gain value in snapshot")
	}
	if snap.Values["current_push"] == 0 {
		t.Fatal("expected current push value in snapshot")
	}

	restored := NewProcedural("underwater", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["bubble-burst"] != snap.Timers["bubble-burst"] {
		t.Fatalf("restored bubble-burst timer = %d, want %d", again.Timers["bubble-burst"], snap.Timers["bubble-burst"])
	}
	if again.Timers["current-shift"] != snap.Timers["current-shift"] {
		t.Fatalf("restored current-shift timer = %d, want %d", again.Timers["current-shift"], snap.Timers["current-shift"])
	}
	if again.Values["bubble_gain"] != snap.Values["bubble_gain"] {
		t.Fatalf("restored bubble gain = %f, want %f", again.Values["bubble_gain"], snap.Values["bubble_gain"])
	}
	if again.Values["current_push"] != snap.Values["current_push"] {
		t.Fatalf("restored current push = %f, want %f", again.Values["current_push"], snap.Values["current_push"])
	}
}
