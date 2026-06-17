package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticAssetsReadFileFallsBackToEmbedded(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"dev.html": &fstest.MapFile{Data: []byte("embedded-dev")},
	})

	got, err := static.readFile("dev.html")
	if err != nil {
		t.Fatalf("readFile returned error: %v", err)
	}
	if string(got) != "embedded-dev" {
		t.Fatalf("readFile = %q, want embedded content", string(got))
	}
}

func TestServeDevPageWithEffectLookupUsesCustomLookup(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"dev.html": &fstest.MapFile{Data: []byte("dev-page")},
	})

	var lookedUp string
	handler := serveDevPageWithEffectLookup(static, func(effect string) (bool, error) {
		lookedUp = effect
		return effect == "volcano", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/dev/volcano", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if lookedUp != "volcano" {
		t.Fatalf("looked up effect = %q, want %q", lookedUp, "volcano")
	}
	if body := rec.Body.String(); body != "dev-page" {
		t.Fatalf("body = %q, want %q", body, "dev-page")
	}
}

func TestServeDevPageInjectsSocialEmbeds(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"dev.html": &fstest.MapFile{Data: []byte("<html><head><!-- __AMBIENCE_SOCIAL_META__ --></head><body></body></html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/dev/beach", nil)
	req.Host = "ambience.dev.romaine.life"
	rec := httptest.NewRecorder()

	serveDevPage(static).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<meta property="og:title" content="ambience dev - beach">`,
		`<meta property="og:url" content="https://ambience.dev.romaine.life/dev/beach">`,
		`<meta property="og:image" content="https://ambience.dev.romaine.life/og-image.png?effect=beach&amp;page=dev">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dev page missing %q in:\n%s", want, body)
		}
	}
}

func TestServeDevPageWithEffectLookupReturnsUnavailableOnLookupError(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"dev.html": &fstest.MapFile{Data: []byte("dev-page")},
	})

	handler := serveDevPageWithEffectLookup(static, func(string) (bool, error) {
		return false, os.ErrDeadlineExceeded
	})

	req := httptest.NewRequest(http.MethodGet, "/dev/volcano", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestServeExactStaticFileServesOnlyConfiguredRoute(t *testing.T) {
	static := newStaticAssets(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("home")},
	})

	handler := serveExactStaticFile(static, "/", "index.html")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "home" {
		t.Fatalf("root body = %q, want %q", body, "home")
	}

	req = httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/index.html status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	req = httptest.NewRequest(http.MethodGet, "/not-a-page", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

var _ fs.FS = fstest.MapFS{}
