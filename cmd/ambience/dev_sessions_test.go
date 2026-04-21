package main

import "testing"

func TestDevPageEffectFromPath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "/dev", want: "rain", wantOK: true},
		{path: "/dev/", want: "rain", wantOK: true},
		{path: "/dev/fireflies", want: "fireflies", wantOK: true},
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
		{path: "/effects/fireflies/schema", want: "fireflies", wantOK: true},
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
