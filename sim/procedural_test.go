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

func TestProceduralSnowSnapshotRestore(t *testing.T) {
	p := NewProcedural("snow", 160, 80, 42, nil)
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

	restored := NewProcedural("snow", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Tick != snap.Tick {
		t.Fatalf("restored tick = %d, want %d", again.Tick, snap.Tick)
	}
	if again.Timers["intro"] != snap.Timers["intro"] {
		t.Fatalf("restored intro timer = %d, want %d", again.Timers["intro"], snap.Timers["intro"])
	}
}

func TestProceduralAutumnLeavesSnapshotRestore(t *testing.T) {
	p := NewProcedural("autumn-leaves", 160, 80, 99, nil)
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

	restored := NewProcedural("autumn-leaves", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["swirl"] != snap.Timers["swirl"] {
		t.Fatalf("restored swirl timer = %d, want %d", again.Timers["swirl"], snap.Timers["swirl"])
	}
}

func TestProceduralStarfieldSnapshotRestore(t *testing.T) {
	p := NewProcedural("starfield", 160, 80, 12, nil)
	if !p.TriggerEvent("shooting-star") {
		t.Fatal("expected shooting-star trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["shooting-star"] <= 0 {
		t.Fatal("expected shooting-star timer in snapshot")
	}

	restored := NewProcedural("starfield", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["shooting-star"] != snap.Timers["shooting-star"] {
		t.Fatalf("restored shooting-star timer = %d, want %d", again.Timers["shooting-star"], snap.Timers["shooting-star"])
	}
}

func TestProceduralAuroraSnapshotRestore(t *testing.T) {
	p := NewProcedural("aurora", 160, 80, 77, nil)
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

	restored := NewProcedural("aurora", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["shift"] != snap.Timers["shift"] {
		t.Fatalf("restored shift timer = %d, want %d", again.Timers["shift"], snap.Timers["shift"])
	}
	if again.Values["shift_push"] != snap.Values["shift_push"] {
		t.Fatalf("restored shift push = %f, want %f", again.Values["shift_push"], snap.Values["shift_push"])
	}
}

func TestProceduralWheatFieldSnapshotRestore(t *testing.T) {
	p := NewProcedural("wheat-field", 160, 80, 88, nil)
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

	restored := NewProcedural("wheat-field", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["gust"] != snap.Timers["gust"] {
		t.Fatalf("restored gust timer = %d, want %d", again.Timers["gust"], snap.Timers["gust"])
	}
	if again.Values["gust_push"] != snap.Values["gust_push"] {
		t.Fatalf("restored gust push = %f, want %f", again.Values["gust_push"], snap.Values["gust_push"])
	}
}

func TestProceduralBeachSnapshotRestore(t *testing.T) {
	p := NewProcedural("beach", 160, 80, 91, nil)
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

	restored := NewProcedural("beach", 160, 80, 7, nil)
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
