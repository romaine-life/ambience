package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserEffectsComeFromWASMRuntime(t *testing.T) {
	runtime, err := os.ReadFile(filepath.Join("web", "wasm_runtime.js"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(runtime)
	if !strings.Contains(body, "window.ambienceWasm.supportedEffects()") {
		t.Fatal("wasm_runtime.js must register effects from the Go/WASM runtime")
	}
	if !strings.Contains(body, "renderPixelGridEffect(this, ctx, canvasW, canvasH, opts)") {
		t.Fatal("wasm_runtime.js render must delegate to shared pixel-grid renderer")
	}

	files, err := os.ReadDir(filepath.Join("web", "effects"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 0 {
		t.Fatalf("legacy browser effect files should not be bundled; found %d files in web/effects", len(files))
	}
}
