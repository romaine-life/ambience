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
		{path: "/dev/dust", want: "dust", wantOK: true},
		{path: "/dev/autumn-leaves", want: "autumn-leaves", wantOK: true},
		{path: "/dev/fireflies", want: "fireflies", wantOK: true},
		{path: "/dev/snow", want: "snow", wantOK: true},
		{path: "/dev/starfield", want: "starfield", wantOK: true},
		{path: "/dev/waterfall", want: "waterfall", wantOK: true},
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
		{path: "/effects/autumn-leaves/schema", want: "autumn-leaves", wantOK: true},
		{path: "/effects/dust/schema", want: "dust", wantOK: true},
		{path: "/effects/fireflies/schema", want: "fireflies", wantOK: true},
		{path: "/effects/snow/schema", want: "snow", wantOK: true},
		{path: "/effects/starfield/schema", want: "starfield", wantOK: true},
		{path: "/effects/waterfall/schema", want: "waterfall", wantOK: true},
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
