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

func TestBrowserRenderingStaysPixelGrid(t *testing.T) {
	sim, err := os.ReadFile(filepath.Join("web", "sim.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sim), "ctx.imageSmoothingEnabled = false") {
		t.Fatal("shared pixel renderer must disable canvas smoothing")
	}

	client, err := os.ReadFile(filepath.Join("web", "client.js"))
	if err != nil {
		t.Fatal(err)
	}
	clientBody := string(client)
	if !strings.Contains(clientBody, "style.imageRendering") || !strings.Contains(clientBody, "'pixelated'") {
		t.Fatal("embeddable client must force pixelated canvas scaling")
	}
	if !strings.Contains(clientBody, "ctx.imageSmoothingEnabled = false") {
		t.Fatal("embeddable client must disable canvas smoothing")
	}
}

func TestDevPanelsDefaultCollapsed(t *testing.T) {
	for _, name := range []string{"index.html", "dev.html"} {
		bodyBytes, err := os.ReadFile(filepath.Join("web", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(bodyBytes), "setPanelCollapsed(true)") && !strings.Contains(string(bodyBytes), "setCollapsed(true)") {
			t.Fatalf("%s must collapse the control panel on load", name)
		}
	}
}

func TestDevEffectSwitchIgnoresStaleStreams(t *testing.T) {
	bodyBytes, err := os.ReadFile(filepath.Join("web", "dev.html"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"let streamSeq = 0",
		"const streamID = ++streamSeq",
		"if (streamID !== streamSeq) return",
		"if (snapType !== expectedEffect) return",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dev effect switching must guard stale SSE streams; missing %q", want)
		}
	}
}
