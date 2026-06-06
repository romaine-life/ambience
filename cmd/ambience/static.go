package main

import (
	"bytes"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var defaultStaticMTime = time.Unix(0, 0)

func init() {
	// Go's built-in MIME table has no .ttf entry, and the distroless runtime
	// image ships no /etc/mime.types, so serveStaticFile would otherwise rely
	// on content sniffing for the chrome wordmark font. Register it explicitly
	// so /fonts/Archivo.ttf always reports a stable font/ttf Content-Type.
	_ = mime.AddExtensionType(".ttf", "font/ttf")
}

type staticAssets struct {
	embedded    fs.FS
	overrideDir string
}

func newStaticAssets(embedded fs.FS, overrideDir string) staticAssets {
	return staticAssets{
		embedded:    embedded,
		overrideDir: overrideDir,
	}
}

func (s staticAssets) readFile(name string) ([]byte, error) {
	if s.overrideDir != "" {
		path := filepath.Join(s.overrideDir, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return fs.ReadFile(s.embedded, name)
}

func serveStaticFile(static staticAssets, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		data, err := static.readFile(name)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		if ctype := mime.TypeByExtension(filepath.Ext(name)); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		http.ServeContent(w, req, name, defaultStaticMTime, bytes.NewReader(data))
	}
}

func serveExactStaticFile(static staticAssets, routePath, name string) http.HandlerFunc {
	handler := serveStaticFile(static, name)
	return func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != routePath {
			http.NotFound(w, req)
			return
		}
		handler(w, req)
	}
}
