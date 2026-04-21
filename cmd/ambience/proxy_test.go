package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
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
