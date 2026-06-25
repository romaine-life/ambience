package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

const (
	ogImageWidth  = 1200
	ogImageHeight = 630
)

type frameProvider func() [][]sim.Pixel
type socialVersionProvider func() string

func serveOGImage(frame frameProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/og-image.png" {
			http.NotFound(w, req)
			return
		}
		grid := frame()
		if effect := strings.TrimSpace(req.URL.Query().Get("effect")); effect != "" {
			if preview, ok := effectPreviewFrame(effect); ok {
				grid = preview
			}
		}
		img := renderPixelGridImage(grid, ogImageWidth, ogImageHeight)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			http.Error(w, "encode og image", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Length", fmt.Sprint(buf.Len()))
		_, _ = w.Write(buf.Bytes())
	}
}

func effectPreviewFrame(effect string) ([][]sim.Pixel, bool) {
	if _, ok := schemaForEffect(effect); !ok {
		return nil, false
	}
	rng := rngutil.New(0x0a6b1e5eed)
	scene := generateEffectScene(effect, rng, 0, 1200)
	rt, err := newEffectRuntime(effect, gridW, gridH, 0x0a6b1e5eed, scene.Config)
	if err != nil {
		return nil, false
	}
	if effect == "train" {
		rt.Trigger("pass")
	}
	for i := 0; i < 80; i++ {
		rt.Step()
	}
	return rt.Frame(), true
}

func serveIndexPage(static staticAssets, version socialVersionProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" && req.URL.Path != "/auth/callback" {
			http.NotFound(w, req)
			return
		}
		data, err := static.readFile("index.html")
		if err != nil {
			http.Error(w, "index page not found", http.StatusNotFound)
			return
		}
		body := string(data)
		imageURL := "https://ambience.romaine.life/og-image.png"
		if version != nil {
			if v := strings.TrimSpace(version()); v != "" {
				imageURL += "?v=" + v
			}
		}
		body = strings.Replace(body, "__AMBIENCE_OG_IMAGE__", imageURL, -1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}

func renderPixelGridImage(grid [][]sim.Pixel, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fillImage(img, color.RGBA{10, 10, 10, 255})
	if len(grid) == 0 || len(grid[0]) == 0 {
		return img
	}
	srcH := len(grid)
	srcW := 0
	for _, row := range grid {
		if len(row) > srcW {
			srcW = len(row)
		}
	}
	if srcW == 0 || srcH == 0 {
		return img
	}
	// Nearest-neighbor stretch to fill the whole frame, sampled per output
	// pixel. The old integer cell-scale left big black margins once the shared
	// grid grew past ~400 wide (e.g. 640×360 only scaled ×1 into a 1200×630
	// card); driving from the output makes the preview resolution-independent.
	for y := 0; y < height; y++ {
		sy := y * srcH / height
		if sy >= len(grid) {
			sy = len(grid) - 1
		}
		row := grid[sy]
		for x := 0; x < width; x++ {
			sx := x * srcW / width
			if sx >= len(row) {
				continue
			}
			c := color.RGBA{0, 0, 0, 255}
			if p := row[sx]; p.Filled {
				c = color.RGBA{p.C.R, p.C.G, p.C.B, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func fillImage(img *image.RGBA, c color.RGBA) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func sharedFrame() [][]sim.Pixel {
	if shared == nil {
		return nil
	}
	shared.mu.Lock()
	effect := shared.effect
	shared.mu.Unlock()
	if effect == nil {
		return nil
	}
	return effect.Frame()
}

func sharedSocialImageVersion() string {
	if shared == nil {
		return ""
	}
	snap := shared.snapshot()
	return socialImageVersion(snap)
}

func socialImageVersion(snap snapshotData) string {
	parts := []string{snap.Type}
	if snap.CurrentScene.Name != "" {
		parts = append(parts, snap.CurrentScene.Name)
	}
	if snap.Tick > 0 {
		parts = append(parts, fmt.Sprintf("t%d", snap.Tick/300))
	}
	return sanitizeSocialVersion(strings.Join(parts, "-"))
}

func sanitizeSocialVersion(raw string) string {
	raw = strings.ToLower(raw)
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
