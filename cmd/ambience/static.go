package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"time"
)

var defaultStaticMTime = time.Unix(0, 0)

// staticETag derives a strong, content-addressed ETag from the asset bytes.
// http.ServeContent prefers If-None-Match over If-Modified-Since, so a
// content-derived validator makes conditional GETs revalidate correctly
// across deploys even though defaultStaticMTime never changes. Without it,
// browsers keep serving a stale /ambience.wasm after the content changes.
func staticETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func init() {
	// Go's built-in MIME table has no .ttf entry, and the distroless runtime
	// image ships no /etc/mime.types, so serveStaticFile would otherwise rely
	// on content sniffing for the chrome wordmark font. Register it explicitly
	// so /fonts/Archivo.ttf always reports a stable font/ttf Content-Type.
	_ = mime.AddExtensionType(".ttf", "font/ttf")
}

type staticAssets struct {
	embedded fs.FS
}

func newStaticAssets(embedded fs.FS) staticAssets {
	return staticAssets{
		embedded: embedded,
	}
}

func (s staticAssets) readFile(name string) ([]byte, error) {
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
		// no-cache lets browsers cache the bytes but forces a conditional
		// revalidation on every load; paired with the content ETag the server
		// answers 304 while content is unchanged and ships fresh bytes the
		// instant a deploy changes them.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", staticETag(data))
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
