package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/nelsong6/ambience/sim"
)

func TestLoadAppConfigFromEnvRequiresAuthorityURLForEdge(t *testing.T) {
	t.Setenv("AMBIENCE_ROLE", "edge")
	t.Setenv("AMBIENCE_AUTHORITY_URL", "")

	_, err := loadAppConfigFromEnv()
	if err == nil {
		t.Fatal("expected missing authority URL error")
	}
}

func TestEntropyForwarderFlushesPendingPayloads(t *testing.T) {
	var (
		mu       sync.Mutex
		requests [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		mu.Lock()
		requests = append(requests, append([]byte(nil), body...))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	f := &entropyForwarder{
		ctx:        context.Background(),
		client:     server.Client(),
		entropyURL: server.URL,
	}

	f.enqueue([]byte("alpha"))
	f.enqueue([]byte("beta"))
	f.flush()

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("forwarded %d requests, want 2", len(requests))
	}
	if got := string(requests[0]); got != "alpha" {
		t.Fatalf("first payload = %q, want %q", got, "alpha")
	}
	if got := string(requests[1]); got != "beta" {
		t.Fatalf("second payload = %q, want %q", got, "beta")
	}
	if len(f.pending) != 0 {
		t.Fatalf("pending queue length = %d, want 0", len(f.pending))
	}
}

func TestAuthorityMirrorSnapshotFramesRefreshCacheWithoutBroadcast(t *testing.T) {
	m := &authorityMirror{
		ctx:       context.Background(),
		client:    &http.Client{},
		listeners: make(map[chan Command]struct{}),
	}
	ch := m.addListener()
	defer m.removeListener(ch)

	snap := snapshotData{Type: "rain", Tick: 42, EntropyBytes: 7}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	cmdPayload, err := json.Marshal(Command{Kind: "snapshot", Tick: snap.Tick, Data: data})
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	if err := m.handleEventPayload(cmdPayload); err != nil {
		t.Fatalf("handleEventPayload: %v", err)
	}

	got, ok := m.snapshot()
	if !ok {
		t.Fatal("mirror did not become ready")
	}
	if got.Tick != snap.Tick || got.EntropyBytes != snap.EntropyBytes {
		t.Fatalf("cached snapshot = %+v, want %+v", got, snap)
	}

	select {
	case cmd := <-ch:
		t.Fatalf("unexpected downstream broadcast: %+v", cmd)
	default:
	}
}

func TestAuthorityMirrorAppliesMetricAndConfigCommands(t *testing.T) {
	m := &authorityMirror{
		ctx:       context.Background(),
		client:    &http.Client{},
		listeners: make(map[chan Command]struct{}),
	}
	m.setSnapshot(snapshotData{
		Type:           "rain",
		Tick:           10,
		Config:         sim.Config{Wind: 1},
		CurrentScene:   Scene{Name: "before"},
		NextScene:      Scene{Name: "later"},
		EntropyBytes:   1,
		SceneRemaining: 100,
	})

	cfgData, err := json.Marshal(sim.Config{Wind: 3})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	m.applyCommand(Command{Kind: "config", Tick: 11, Data: cfgData})

	metricData, err := json.Marshal(map[string]any{
		"entropyBytes":   int64(9),
		"sceneRemaining": 77,
		"currentName":    "after",
		"nextName":       "next-up",
	})
	if err != nil {
		t.Fatalf("marshal metric: %v", err)
	}
	m.applyCommand(Command{Kind: "metric", Tick: 12, Data: metricData})

	got, ok := m.snapshot()
	if !ok {
		t.Fatal("mirror did not retain snapshot")
	}
	if got.Tick != 12 {
		t.Fatalf("tick = %d, want 12", got.Tick)
	}
	if got.Config.Wind != 3 {
		t.Fatalf("wind = %v, want 3", got.Config.Wind)
	}
	if got.EntropyBytes != 9 {
		t.Fatalf("entropy = %d, want 9", got.EntropyBytes)
	}
	if got.SceneRemaining != 77 {
		t.Fatalf("sceneRemaining = %d, want 77", got.SceneRemaining)
	}
	if got.CurrentScene.Name != "after" || got.NextScene.Name != "next-up" {
		t.Fatalf("scene names = %q/%q", got.CurrentScene.Name, got.NextScene.Name)
	}
}

func TestNewAuthorityProxyStartsReadyAfterSnapshotFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/snapshot":
			_ = json.NewEncoder(w).Encode(snapshotData{Type: "rain", Tick: 5})
		case "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": keep-alive\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &authorityProxy{
		proxy:   httputil.NewSingleHostReverseProxy(baseURL),
		entropy: newEntropyForwarder(ctx, baseURL),
		mirror:  newAuthorityMirror(ctx, baseURL),
	}
	for i := 0; i < 20; i++ {
		if p.ready() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("proxy never became ready after snapshot fetch")
}
