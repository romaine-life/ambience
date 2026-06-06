package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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

// The "Exposed" chrome is summoned/dismissed with Esc. The dev workbench shows
// its chrome on load (summoned: true). The live monitor is world-first
// (dismissed by default) but honors a #monitor / ?monitor / ?chrome=open deep
// link so the operator schematic is shareable, deep-linkable, and capturable
// headlessly without a keypress. Both pages mount the shared chrome
// (window.AmbienceChrome) instead of the old per-page control panel.
func TestChromeSummonDefaults(t *testing.T) {
	for _, page := range []string{"index.html", "dev.html"} {
		bodyBytes, err := os.ReadFile(filepath.Join("web", page))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(bodyBytes), "window.AmbienceChrome.mount(") {
			t.Fatalf("%s must mount the shared Exposed chrome", page)
		}
	}

	dev, err := os.ReadFile(filepath.Join("web", "dev.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dev), "summoned: true") {
		t.Fatal("dev.html must mount the chrome summoned on load")
	}

	index, err := os.ReadFile(filepath.Join("web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexBody := string(index)
	// The mount flag is derived (world-first default, deep-link opens it), not a
	// literal true/false, so assert the derivation and the deep-link wiring.
	for _, want := range []string{
		"const monitorDeepLink",
		"'#monitor'",
		"summoned: monitorDeepLink",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("index.html live monitor must be world-first with a deep-link open; missing %q", want)
		}
	}
}

func TestLiveControlsExposeNextEffect(t *testing.T) {
	bodyBytes, err := os.ReadFile(filepath.Join("web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	// The next-effect control is now rendered by the shared chrome (showNext +
	// the onNext handler) and still advances the broadcast via POST /next-effect.
	for _, want := range []string{
		`showNext: true`,
		`onNext: () => advanceSharedEffect()`,
		`function advanceSharedEffect()`,
		`fetch('/next-effect'`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("live controls next-effect UI missing %q", want)
		}
	}
}

// The "Exposed" chrome adds three assets (chrome.js, chrome.css, the wordmark
// font). The router has no static catch-all — unregistered paths fall through
// to the index page — so each needs an explicit route. This guards against the
// regression where /chrome.js (or .css) silently serves the index HTML.
func TestRegisterStaticRoutesServesChromeAssets(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("INDEX-PAGE")},
		"dev.html":          &fstest.MapFile{Data: []byte("DEV-PAGE")},
		"styleguide.html":   &fstest.MapFile{Data: []byte("STYLEGUIDE")},
		"sim.js":            &fstest.MapFile{Data: []byte("// sim")},
		"chrome.js":         &fstest.MapFile{Data: []byte("// chrome js")},
		"chrome.css":        &fstest.MapFile{Data: []byte("/* chrome css */")},
		"fonts/Archivo.ttf": &fstest.MapFile{Data: []byte{0x00, 0x01, 0x00, 0x00}},
	}, "")

	mux := http.NewServeMux()
	registerStaticRoutes(mux, static,
		func(string) (bool, error) { return true, nil },
		func() string { return "test" })

	for _, tc := range []struct {
		path, wantBody, wantCT string
	}{
		{"/chrome.js", "// chrome js", "javascript"},
		{"/chrome.css", "/* chrome css */", "text/css"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", tc.path, rec.Code, http.StatusOK)
		}
		if got := rec.Body.String(); got != tc.wantBody {
			t.Fatalf("GET %s body = %q, want %q (fell through to index page?)", tc.path, got, tc.wantBody)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, tc.wantCT) {
			t.Fatalf("GET %s Content-Type = %q, want it to contain %q", tc.path, ct, tc.wantCT)
		}
	}

	// The wordmark font must serve from /fonts/Archivo.ttf (the @font-face URL
	// resolved relative to /chrome.css) with a font Content-Type.
	req := httptest.NewRequest(http.MethodGet, "/fonts/Archivo.ttf", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /fonts/Archivo.ttf status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "font") && !strings.Contains(ct, "ttf") {
		t.Fatalf("GET /fonts/Archivo.ttf Content-Type = %q, want a font type", ct)
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

func TestChromeBottomLogLayoutStaysResponsive(t *testing.T) {
	jsBytes, err := os.ReadFile(filepath.Join("web", "chrome.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, want := range []string{
		`class: 'd5__trkwrap'`,
		`class: 'logline__msg'`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("chrome.js bottom feed layout missing %q", want)
		}
	}
	if strings.Contains(js, `class: 'd5__trk', style: 'flex:1;min-width:0'`) {
		t.Fatal("track wrapper must not reuse d5__trk; nested track flex rules make the feed unstable")
	}

	cssBytes, err := os.ReadFile(filepath.Join("web", "chrome.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, want := range []string{
		`.d5__trkwrap { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 4px; }`,
		`.d5__log { flex: 0 0 clamp(220px, 25vw, 340px); min-width: 0; display: flex; flex-direction: column; gap: 3px; }`,
		`.logline__msg { flex: 1; min-width: 0; overflow-wrap: anywhere; }`,
		`@media (max-width: 760px)`,
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("chrome.css responsive log layout missing %q", want)
		}
	}
}
