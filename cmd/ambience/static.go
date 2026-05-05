package main

import (
	"bytes"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"
)

var defaultStaticMTime = time.Unix(0, 0)

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

// listEffectFiles returns legacy per-effect JS files if a development override
// tree still has them. The production browser runtime registers effects from
// /wasm_runtime.js instead; this compatibility path keeps old override dirs
// from breaking while the JS effects are removed from the embedded bundle.
func (s staticAssets) listEffectFiles() ([]string, error) {
	seen := map[string]struct{}{}
	collect := func(name string) {
		if path.Ext(name) == ".js" {
			seen[name] = struct{}{}
		}
	}
	if s.overrideDir != "" {
		dir := filepath.Join(s.overrideDir, "effects")
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					collect(e.Name())
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	entries, err := fs.ReadDir(s.embedded, "effects")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			collect(e.Name())
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// serveSimBundle answers GET /sim.js with the AmbienceSim namespace and shared
// browser helpers. If a development override tree still contains legacy
// web/effects/*.js files, they are appended for compatibility; production
// effects are registered by /wasm_runtime.js from the Go/WASM runtime.
func serveSimBundle(static staticAssets) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		core, err := static.readFile("sim.js")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		var buf bytes.Buffer
		buf.Write(core)
		if len(core) > 0 && core[len(core)-1] != '\n' {
			buf.WriteByte('\n')
		}
		files, err := static.listEffectFiles()
		if err != nil {
			http.Error(w, "list effects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, f := range files {
			data, err := static.readFile(path.Join("effects", f))
			if err != nil {
				http.Error(w, "read effect "+f+": "+err.Error(), http.StatusInternalServerError)
				return
			}
			buf.WriteString("// ===== effects/" + f + " =====\n")
			buf.Write(data)
			if len(data) > 0 && data[len(data)-1] != '\n' {
				buf.WriteByte('\n')
			}
		}
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeContent(w, req, "sim.js", defaultStaticMTime, bytes.NewReader(buf.Bytes()))
	}
}
