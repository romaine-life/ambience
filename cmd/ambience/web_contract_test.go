package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectFilesStayPixelGridRendered(t *testing.T) {
	forbidden := []string{
		"createLinearGradient",
		"createRadialGradient",
		"beginPath(",
		"lineTo(",
		"arc(",
		"stroke(",
		"ctx.fill(",
	}
	files, err := os.ReadDir(filepath.Join("web", "effects"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".js" {
			continue
		}
		path := filepath.Join("web", "effects", file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		if !strings.Contains(body, "this.grid = new Uint8ClampedArray") {
			t.Fatalf("%s must allocate an owned pixel grid", path)
		}
		if strings.Contains(body, "_usesProceduralGrid") {
			t.Fatalf("%s must not rely on renderer-side procedural fallback flags", path)
		}
		if !strings.Contains(body, ".grid.fill(0)") && !strings.Contains(body, "paintProceduralGrid(this)") {
			t.Fatalf("%s step must update its owned pixel grid", path)
		}
		if !strings.Contains(body, "renderPixelGridEffect(this, ctx, canvasW, canvasH, opts)") {
			t.Fatalf("%s render must delegate to shared pixel-grid renderer", path)
		}
		for _, token := range forbidden {
			if strings.Contains(body, token) {
				t.Fatalf("%s uses non-pixel canvas API %q", path, token)
			}
		}
	}
}
