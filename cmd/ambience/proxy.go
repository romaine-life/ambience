package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

const (
	edgeEntropyBufferLimit = 256 * 1024
	edgeEntropyFlushEvery  = 2 * time.Second
	edgeForwardTimeout     = 1 * time.Second
)

type authorityProxy struct {
	proxy   *httputil.ReverseProxy
	entropy *entropyForwarder
}

func newAuthorityProxy(ctx context.Context, rawURL string) (*authorityProxy, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse AMBIENCE_AUTHORITY_URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("AMBIENCE_AUTHORITY_URL must include scheme and host")
	}

	proxy := httputil.NewSingleHostReverseProxy(baseURL)
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Context().Err() != nil {
			return
		}
		log.Printf("authority proxy %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "authority unavailable", http.StatusBadGateway)
	}

	return &authorityProxy{
		proxy:   proxy,
		entropy: newEntropyForwarder(ctx, baseURL),
	}, nil
}

func registerEdgeRoutes(mux *http.ServeMux, proxy *authorityProxy) {
	mux.HandleFunc("/snapshot", cors(proxy.serveHTTP))
	mux.HandleFunc("/events", cors(proxy.serveHTTP))
	mux.HandleFunc("/entropy", cors(proxy.serveEntropy))
	mux.HandleFunc("/dev/snapshot", proxy.serveHTTP)
	mux.HandleFunc("/dev/events", proxy.serveHTTP)
	mux.HandleFunc("/dev/config", proxy.serveHTTP)
	mux.HandleFunc("/dev/trigger/", proxy.serveHTTP)
}

func (p *authorityProxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

func (p *authorityProxy) serveEntropy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	const maxBytes = 4096
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read entropy body", http.StatusBadRequest)
		return
	}
	if len(payload) > 0 {
		if err := p.entropy.forward(payload); err != nil {
			p.entropy.enqueue(payload)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type entropyForwarder struct {
	ctx        context.Context
	client     *http.Client
	entropyURL string

	mu      sync.Mutex
	pending [][]byte
	bytes   int
}

func newEntropyForwarder(ctx context.Context, baseURL *url.URL) *entropyForwarder {
	entropyURL := baseURL.ResolveReference(&url.URL{Path: "/entropy"}).String()
	f := &entropyForwarder{
		ctx:        ctx,
		client:     &http.Client{},
		entropyURL: entropyURL,
	}
	go f.run()
	return f
}

func (f *entropyForwarder) run() {
	t := time.NewTicker(edgeEntropyFlushEvery)
	defer t.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-t.C:
			f.flush()
		}
	}
}

func (f *entropyForwarder) forward(payload []byte) error {
	ctx, cancel := context.WithTimeout(f.ctx, edgeForwardTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.entropyURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("authority /entropy returned %s", resp.Status)
	}
	return nil
}

func (f *entropyForwarder) enqueue(payload []byte) {
	if len(payload) == 0 {
		return
	}
	chunk := append([]byte(nil), payload...)
	if len(chunk) > edgeEntropyBufferLimit {
		chunk = chunk[len(chunk)-edgeEntropyBufferLimit:]
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for len(f.pending) > 0 && f.bytes+len(chunk) > edgeEntropyBufferLimit {
		dropped := f.pending[0]
		f.pending = f.pending[1:]
		f.bytes -= len(dropped)
	}
	if f.bytes+len(chunk) > edgeEntropyBufferLimit {
		log.Printf("edge entropy buffer full, dropping %d bytes", len(chunk))
		return
	}
	f.pending = append(f.pending, chunk)
	f.bytes += len(chunk)
}

func (f *entropyForwarder) flush() {
	for {
		payload := f.peek()
		if len(payload) == 0 {
			return
		}
		if err := f.forward(payload); err != nil {
			return
		}
		f.pop()
	}
}

func (f *entropyForwarder) peek() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return nil
	}
	return append([]byte(nil), f.pending[0]...)
}

func (f *entropyForwarder) pop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return
	}
	f.bytes -= len(f.pending[0])
	f.pending = f.pending[1:]
}
