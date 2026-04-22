package main

import (
	"testing"
	"time"
)

func TestDevPageEffectFromPath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "/dev", want: "rain", wantOK: true},
		{path: "/dev/", want: "rain", wantOK: true},
		{path: "/dev/aurora", want: "aurora", wantOK: true},
		{path: "/dev/beach", want: "beach", wantOK: true},
		{path: "/dev/campfire", want: "campfire", wantOK: true},
		{path: "/dev/dust", want: "dust", wantOK: true},
		{path: "/dev/autumn-leaves", want: "autumn-leaves", wantOK: true},
		{path: "/dev/fireflies", want: "fireflies", wantOK: true},
		{path: "/dev/lighthouse", want: "lighthouse", wantOK: true},
		{path: "/dev/rowboat", want: "rowboat", wantOK: true},
		{path: "/dev/snow", want: "snow", wantOK: true},
		{path: "/dev/starfield", want: "starfield", wantOK: true},
		{path: "/dev/underwater", want: "underwater", wantOK: true},
		{path: "/dev/train", want: "train", wantOK: true},
		{path: "/dev/volcano", want: "volcano", wantOK: true},
		{path: "/dev/mysterious-man", want: "mysterious-man", wantOK: true},
		{path: "/dev/burning-trees", want: "burning-trees", wantOK: true},
		{path: "/dev/sand", want: "sand", wantOK: true},
		{path: "/dev/waterfall", want: "waterfall", wantOK: true},
		{path: "/dev/windmill", want: "windmill", wantOK: true},
		{path: "/dev/wheat-field", want: "wheat-field", wantOK: true},
		{path: "/dev/unknown", wantOK: false},
		{path: "/dev/fireflies/extra", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := devPageEffectFromPath(tc.path)
		if ok != tc.wantOK {
			t.Fatalf("devPageEffectFromPath(%q) ok=%v, want %v", tc.path, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("devPageEffectFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestEffectFromSchemaPath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "/effects/rain/schema", want: "rain", wantOK: true},
		{path: "/effects/aurora/schema", want: "aurora", wantOK: true},
		{path: "/effects/beach/schema", want: "beach", wantOK: true},
		{path: "/effects/campfire/schema", want: "campfire", wantOK: true},
		{path: "/effects/autumn-leaves/schema", want: "autumn-leaves", wantOK: true},
		{path: "/effects/dust/schema", want: "dust", wantOK: true},
		{path: "/effects/fireflies/schema", want: "fireflies", wantOK: true},
		{path: "/effects/lighthouse/schema", want: "lighthouse", wantOK: true},
		{path: "/effects/rowboat/schema", want: "rowboat", wantOK: true},
		{path: "/effects/snow/schema", want: "snow", wantOK: true},
		{path: "/effects/starfield/schema", want: "starfield", wantOK: true},
		{path: "/effects/underwater/schema", want: "underwater", wantOK: true},
		{path: "/effects/train/schema", want: "train", wantOK: true},
		{path: "/effects/volcano/schema", want: "volcano", wantOK: true},
		{path: "/effects/mysterious-man/schema", want: "mysterious-man", wantOK: true},
		{path: "/effects/burning-trees/schema", want: "burning-trees", wantOK: true},
		{path: "/effects/sand/schema", want: "sand", wantOK: true},
		{path: "/effects/waterfall/schema", want: "waterfall", wantOK: true},
		{path: "/effects/windmill/schema", want: "windmill", wantOK: true},
		{path: "/effects/wheat-field/schema", want: "wheat-field", wantOK: true},
		{path: "/effects/unknown/schema", wantOK: false},
		{path: "/effects/fireflies/not-schema", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := effectFromSchemaPath(tc.path)
		if ok != tc.wantOK {
			t.Fatalf("effectFromSchemaPath(%q) ok=%v, want %v", tc.path, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("effectFromSchemaPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNewDevSessionFirefliesSnapshotType(t *testing.T) {
	session, err := newDevSession("fireflies")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "fireflies" {
		t.Fatalf("snapshot type = %q, want fireflies", snap.Type)
	}
}

func TestNewDevSessionDustSnapshotType(t *testing.T) {
	session, err := newDevSession("dust")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "dust" {
		t.Fatalf("snapshot type = %q, want dust", snap.Type)
	}
}

func TestNewDevSessionWaterfallSnapshotType(t *testing.T) {
	session, err := newDevSession("waterfall")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "waterfall" {
		t.Fatalf("snapshot type = %q, want waterfall", snap.Type)
	}
}

func TestNewDevSessionSnowSnapshotType(t *testing.T) {
	session, err := newDevSession("snow")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "snow" {
		t.Fatalf("snapshot type = %q, want snow", snap.Type)
	}
}

func TestNewDevSessionAuroraSnapshotType(t *testing.T) {
	session, err := newDevSession("aurora")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "aurora" {
		t.Fatalf("snapshot type = %q, want aurora", snap.Type)
	}
}

func TestNewDevSessionBeachSnapshotType(t *testing.T) {
	session, err := newDevSession("beach")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "beach" {
		t.Fatalf("snapshot type = %q, want beach", snap.Type)
	}
}

func TestNewDevSessionCampfireSnapshotType(t *testing.T) {
	session, err := newDevSession("campfire")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "campfire" {
		t.Fatalf("snapshot type = %q, want campfire", snap.Type)
	}
}

func TestNewDevSessionWindmillSnapshotType(t *testing.T) {
	session, err := newDevSession("windmill")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "windmill" {
		t.Fatalf("snapshot type = %q, want windmill", snap.Type)
	}
}

func TestNewDevSessionLighthouseSnapshotType(t *testing.T) {
	session, err := newDevSession("lighthouse")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "lighthouse" {
		t.Fatalf("snapshot type = %q, want lighthouse", snap.Type)
	}
}

func TestNewDevSessionRowboatSnapshotType(t *testing.T) {
	session, err := newDevSession("rowboat")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "rowboat" {
		t.Fatalf("snapshot type = %q, want rowboat", snap.Type)
	}
}

func TestNewDevSessionUnderwaterSnapshotType(t *testing.T) {
	session, err := newDevSession("underwater")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "underwater" {
		t.Fatalf("snapshot type = %q, want underwater", snap.Type)
	}
}

func TestNewDevSessionVolcanoSnapshotType(t *testing.T) {
	session, err := newDevSession("volcano")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "volcano" {
		t.Fatalf("snapshot type = %q, want volcano", snap.Type)
	}
}

func TestNewDevSessionTrainSnapshotType(t *testing.T) {
	session, err := newDevSession("train")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "train" {
		t.Fatalf("snapshot type = %q, want train", snap.Type)
	}
}

func TestNewDevSessionMysteriousManSnapshotType(t *testing.T) {
	session, err := newDevSession("mysterious-man")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "mysterious-man" {
		t.Fatalf("snapshot type = %q, want mysterious-man", snap.Type)
	}
}

func TestNewDevSessionBurningTreesSnapshotType(t *testing.T) {
	session, err := newDevSession("burning-trees")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "burning-trees" {
		t.Fatalf("snapshot type = %q, want burning-trees", snap.Type)
	}
}

func TestNewDevSessionSandSnapshotType(t *testing.T) {
	session, err := newDevSession("sand")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "sand" {
		t.Fatalf("snapshot type = %q, want sand", snap.Type)
	}
}

func TestNewDevSessionWheatFieldSnapshotType(t *testing.T) {
	session, err := newDevSession("wheat-field")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "wheat-field" {
		t.Fatalf("snapshot type = %q, want wheat-field", snap.Type)
	}
}

func TestNewDevSessionAutumnLeavesSnapshotType(t *testing.T) {
	session, err := newDevSession("autumn-leaves")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "autumn-leaves" {
		t.Fatalf("snapshot type = %q, want autumn-leaves", snap.Type)
	}
}

func TestNewDevSessionStarfieldSnapshotType(t *testing.T) {
	session, err := newDevSession("starfield")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "starfield" {
		t.Fatalf("snapshot type = %q, want starfield", snap.Type)
	}
}

func TestDevSessionRandomizeConfigChangesSnapshotConfig(t *testing.T) {
	session, err := newDevSession("dust")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}

	before := session.snapshot()
	time.Sleep(time.Nanosecond)
	if _, err := session.randomizeConfig(99); err != nil {
		t.Fatalf("randomizeConfig: %v", err)
	}
	after := session.snapshot()

	if configsEqualJSON(before.Config, after.Config) {
		t.Fatal("expected randomized config to differ from previous session config")
	}
}
