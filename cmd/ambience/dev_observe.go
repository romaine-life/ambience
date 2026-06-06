package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/romaine-life/ambience/sim"
)

const (
	defaultObserveMaxTicks = 3600
	maxObserveTicks        = 20000
	maxObserveHoldTicks    = 3600
	devObservationImageW   = 1200
	devObservationImageH   = 630
)

type devObservation struct {
	ID        string
	CreatedAt time.Time
	Tick      int
	Frame     [][]sim.Pixel
}

type devObserveRequest struct {
	Effect      string
	Session     string
	Trigger     string
	WaitEvent   string
	StatePath   string
	StateEquals string
	MaxTicks    int
	HoldTicks   int
}

type devObserveResponse struct {
	Type          string          `json:"type"`
	Effect        string          `json:"effect"`
	Session       string          `json:"session"`
	Seed          int64           `json:"seed"`
	Config        json.RawMessage `json:"config"`
	State         json.RawMessage `json:"state"`
	StartTick     int             `json:"startTick"`
	ObservedTick  int             `json:"observedTick"`
	HeldUntilTick int             `json:"heldUntilTick"`
	ElapsedTicks  int             `json:"elapsedTicks"`
	MaxTicks      int             `json:"maxTicks"`
	HoldTicks     int             `json:"holdTicks"`
	Trigger       string          `json:"trigger,omitempty"`
	WaitEvent     string          `json:"waitEvent,omitempty"`
	StatePath     string          `json:"statePath,omitempty"`
	StateEquals   string          `json:"stateEquals,omitempty"`
	Applied       bool            `json:"applied"`
	Observed      bool            `json:"observed"`
	MatchedEvents []appliedEvent  `json:"matchedEvents,omitempty"`
	AppliedEvents []appliedEvent  `json:"appliedEvents"`
	ObservationID string          `json:"observationId"`
	FrameURL      string          `json:"frameUrl"`
}

func serveDevSessionObserve(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	s, effect, session, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params, err := parseDevObserveRequest(req, effect, session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.observe(req.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestTimeout)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func serveDevSessionFrame(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	s, _, _, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.URL.Query().Get("observation"))
	if id == "" {
		http.Error(w, "observation param required", http.StatusBadRequest)
		return
	}
	frame, ok := s.observationFrame(id)
	if !ok {
		http.Error(w, "unknown observation: "+id, http.StatusNotFound)
		return
	}
	img := renderPixelGridImage(frame, devObservationImageW, devObservationImageH)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "encode frame", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}

func parseDevObserveRequest(req *http.Request, effect, session string) (devObserveRequest, error) {
	q := req.URL.Query()
	out := devObserveRequest{
		Effect:      effect,
		Session:     session,
		Trigger:     strings.TrimSpace(q.Get("trigger")),
		WaitEvent:   strings.TrimSpace(q.Get("wait_event")),
		StatePath:   strings.TrimSpace(q.Get("state_path")),
		StateEquals: strings.TrimSpace(q.Get("state_equals")),
		MaxTicks:    parseBoundedInt(q.Get("max_ticks"), defaultObserveMaxTicks, 1, maxObserveTicks),
		HoldTicks:   parseBoundedInt(q.Get("hold_ticks"), 0, 0, maxObserveHoldTicks),
	}
	if out.WaitEvent == "" && out.StatePath == "" && out.Trigger == "" {
		return out, fmt.Errorf("one of trigger, wait_event, or state_path is required")
	}
	if out.StatePath != "" && out.StateEquals == "" {
		out.StateEquals = "true"
	}
	return out, nil
}

func (s *devSession) observe(ctx context.Context, params devObserveRequest) (devObserveResponse, error) {
	start := s.snapshot()
	startTick := start.Tick
	if params.Trigger != "" && !s.triggerEvent(params.Trigger) {
		return devObserveResponse{}, fmt.Errorf("unknown event: %s", params.Trigger)
	}

	var (
		observedTick int
		heldTicks    int
		last         devSnapshotData
		appliedSeen  = params.Trigger == ""
		waitSeen     = params.WaitEvent == ""
		matched      []appliedEvent
	)
	for step := 0; step <= params.MaxTicks; step++ {
		if err := ctx.Err(); err != nil {
			return devObserveResponse{}, err
		}
		if step > 0 || params.Trigger != "" {
			s.stepAndBroadcast()
		}
		last = s.snapshot()
		if !appliedSeen {
			if event, ok := findAppliedEvent(last.AppliedEvents, params.Trigger, startTick); ok {
				appliedSeen = true
				matched = append(matched, event)
			}
		}
		if !waitSeen {
			if event, ok := findAppliedEvent(last.AppliedEvents, params.WaitEvent, startTick); ok {
				waitSeen = true
				matched = append(matched, event)
			}
		}
		stateOK, err := observeStatePredicateMet(last, params)
		if err != nil {
			return devObserveResponse{}, err
		}
		if appliedSeen && waitSeen && stateOK {
			if observedTick == 0 {
				observedTick = last.Tick
			}
			if heldTicks >= params.HoldTicks {
				return s.storeObservation(params, startTick, observedTick, appliedSeen, matched, last), nil
			}
			heldTicks++
			continue
		}
		observedTick = 0
		heldTicks = 0
	}
	return devObserveResponse{}, fmt.Errorf("observe timed out after %d ticks", params.MaxTicks)
}

func observeStatePredicateMet(snap devSnapshotData, params devObserveRequest) (bool, error) {
	if params.StatePath != "" {
		ok, err := statePredicateMet(snap.State, params.StatePath, params.StateEquals)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func findAppliedEvent(events []appliedEvent, event string, minTick int) (appliedEvent, bool) {
	for _, e := range events {
		if e.Event == event && e.Tick >= minTick {
			return e, true
		}
	}
	return appliedEvent{}, false
}

func hasAppliedEvent(events []appliedEvent, event string, minTick int) bool {
	for _, e := range events {
		if e.Event == event && e.Tick >= minTick {
			return true
		}
	}
	return false
}

func statePredicateMet(raw json.RawMessage, path, wantRaw string) (bool, error) {
	got, ok, err := lookupJSONPath(raw, path)
	if err != nil || !ok {
		return false, err
	}
	want, err := parseJSONComparable(wantRaw)
	if err != nil {
		return false, err
	}
	return jsonComparableEqual(got, want), nil
}

func lookupJSONPath(raw json.RawMessage, path string) (any, bool, error) {
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, false, err
	}
	current := node
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, false, nil
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		current, ok = obj[part]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}

func parseJSONComparable(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out, nil
	}
	return raw, nil
}

func jsonComparableEqual(left, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func (s *devSession) storeObservation(params devObserveRequest, startTick, observedTick int, applied bool, matched []appliedEvent, snap devSnapshotData) devObserveResponse {
	frame := s.effect.Frame()
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), observedTick)
	s.mu.Lock()
	s.observed = append(s.observed, devObservation{
		ID:        id,
		CreatedAt: time.Now(),
		Tick:      snap.Tick,
		Frame:     frame,
	})
	if len(s.observed) > devObservationCap {
		s.observed = s.observed[len(s.observed)-devObservationCap:]
	}
	s.mu.Unlock()
	frameURL := fmt.Sprintf("/dev/frame?session=%s&effect=%s&observation=%s", params.Session, params.Effect, id)
	return devObserveResponse{
		Type:          snap.Type,
		Effect:        params.Effect,
		Session:       params.Session,
		Seed:          snap.Seed,
		Config:        snap.Config,
		State:         snap.State,
		StartTick:     startTick,
		ObservedTick:  observedTick,
		HeldUntilTick: snap.Tick,
		ElapsedTicks:  snap.Tick - startTick,
		MaxTicks:      params.MaxTicks,
		HoldTicks:     params.HoldTicks,
		Trigger:       params.Trigger,
		WaitEvent:     params.WaitEvent,
		StatePath:     params.StatePath,
		StateEquals:   params.StateEquals,
		Applied:       applied,
		Observed:      true,
		MatchedEvents: matched,
		AppliedEvents: snap.AppliedEvents,
		ObservationID: id,
		FrameURL:      frameURL,
	}
}

func (s *devSession) observationFrame(id string) ([][]sim.Pixel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.observed) - 1; i >= 0; i-- {
		if s.observed[i].ID == id {
			return clonePixelGrid(s.observed[i].Frame), true
		}
	}
	return nil, false
}

func clonePixelGrid(in [][]sim.Pixel) [][]sim.Pixel {
	out := make([][]sim.Pixel, len(in))
	for i := range in {
		out[i] = append([]sim.Pixel(nil), in[i]...)
	}
	return out
}
