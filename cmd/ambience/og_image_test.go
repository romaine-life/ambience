package main

import (
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nelsong6/ambience/sim"
)

func TestServeOGImageReturnsPNG(t *testing.T) {
	handler := serveOGImage(func() [][]sim.Pixel {
		return [][]sim.Pixel{
			{
				{Filled: true, C: color.RGBA{R: 255, G: 100, B: 100, A: 255}},
				{Filled: false},
			},
		}
	})
	req := httptest.NewRequest(http.MethodGet, "/og-image.png", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	img, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != ogImageWidth || bounds.Dy() != ogImageHeight {
		t.Fatalf("png size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), ogImageWidth, ogImageHeight)
	}
}

func TestServeIndexPageInjectsVersionedOGImage(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<meta property="og:image" content="__AMBIENCE_OG_IMAGE__">`)},
	}, "")
	handler := serveIndexPage(static, func() string { return "rain-scene-t1" })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if strings.Contains(body, "__AMBIENCE_OG_IMAGE__") {
		t.Fatalf("body still contains og placeholder: %q", body)
	}
	if !strings.Contains(body, `https://ambience.romaine.life/og-image.png?v=rain-scene-t1`) {
		t.Fatalf("body missing versioned og image URL: %q", body)
	}
}
