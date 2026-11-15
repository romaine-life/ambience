package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
)

// Run without persistence, listeners or credentials in the read-only CI image.
func selfTest() error {
	web, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	for _, name := range []string{"ambience.wasm", "ambience-rain.wasm"} {
		data, err := fs.ReadFile(web, name)
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(data, []byte{0, 'a', 's', 'm', 1, 0, 0, 0}) {
			return fmt.Errorf("%s: invalid WebAssembly header", name)
		}
	}
	mux := http.NewServeMux()
	registerCommonRoutes(mux, func() bool { return true })
	registerStaticRoutes(mux, newStaticAssets(web), func(effect string) (bool, error) {
		_, ok := lookupEffectDefinition(effect)
		return ok, nil
	}, func() string { return "self-test" })
	for _, route := range []string{"/", "/dev/rain", "/healthz", "/readyz", "/client.js", "/wasm_runtime.js", "/wasm_exec.js", "/ambience.wasm", "/ambience-rain.wasm"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if (route == "/healthz" || route == "/readyz") && response.Code == http.StatusNoContent {
			continue
		}
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			return fmt.Errorf("GET %s: status %d, bytes %d", route, response.Code, response.Body.Len())
		}
	}
	for effect := range effectRegistry {
		runtime, err := newEffectRuntime(effect, 160, 80, 1, nil)
		if err != nil {
			return err
		}
		runtime.Step()
		snapshot, err := runtime.Snapshot()
		if err != nil {
			return err
		}
		if err := runtime.Restore(snapshot); err != nil {
			return err
		}
		if runtime.CurrentTick() != snapshot.Tick || len(runtime.Frame()) == 0 {
			return fmt.Errorf("%s: snapshot/frame self-test failed", effect)
		}
	}
	auth := newControlAuthenticator("", oidcControlAuthConfig{
		Issuer: "https://auth.romaine.life", ClientID: "ambience",
	})
	response := httptest.NewRecorder()
	if auth.require(response, httptest.NewRequest(http.MethodPost, "/next-effect", nil)) || response.Code != http.StatusUnauthorized {
		return fmt.Errorf("unauthenticated controls were not rejected")
	}
	response = httptest.NewRecorder()
	auth.serve(response, httptest.NewRequest(http.MethodPost, "/control-auth", bytes.NewBufferString(`{}`)))
	if response.Code != http.StatusUnauthorized || len(response.Result().Cookies()) != 0 {
		return fmt.Errorf("invalid OIDC callback established a session")
	}
	return nil
}
