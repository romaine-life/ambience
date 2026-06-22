package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const controlAuthCookie = "ambience_control"

type controlAuthenticator struct {
	passwordHash [sha256.Size]byte
	required     bool
	oidc         *oidcControlAuth
	mu           sync.Mutex
	sessions     map[string]time.Time
}

func newControlAuthenticator(password string, oidcCfg oidcControlAuthConfig) *controlAuthenticator {
	password = strings.TrimSpace(password)
	oidcAuth := newOIDCControlAuth(oidcCfg)
	a := &controlAuthenticator{
		required: password != "" || oidcAuth != nil,
		oidc:     oidcAuth,
		sessions: make(map[string]time.Time),
	}
	if password != "" {
		a.passwordHash = sha256.Sum256([]byte(password))
	}
	return a
}

// controlCallbackURL derives the OIDC redirect_uri from the inbound request so
// it always matches the public origin (and the redirectUrls registered for the
// "ambience" client in auth.romaine.life) without trusting client input.
func controlCallbackURL(req *http.Request) string {
	host := req.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = req.Host
	}
	proto := req.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if req.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + host + "/auth/callback"
}

func (a *controlAuthenticator) authenticated(req *http.Request) bool {
	if a == nil || !a.required {
		return true
	}
	cookie, err := req.Cookie(controlAuthCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	expires, ok := a.sessions[cookie.Value]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(a.sessions, cookie.Value)
		return false
	}
	return true
}

func (a *controlAuthenticator) require(w http.ResponseWriter, req *http.Request) bool {
	if a.authenticated(req) {
		return true
	}
	http.Error(w, "control auth required", http.StatusUnauthorized)
	return false
}

func (a *controlAuthenticator) serve(w http.ResponseWriter, req *http.Request) {
	if a == nil {
		http.Error(w, "control auth unavailable", http.StatusServiceUnavailable)
		return
	}
	switch req.Method {
	case http.MethodGet:
		a.writeStatus(w, req)
	case http.MethodPost:
		a.login(w, req)
	case http.MethodDelete:
		a.logout(w, req)
	default:
		http.Error(w, "GET, POST, or DELETE required", http.StatusMethodNotAllowed)
	}
}

func (a *controlAuthenticator) writeStatus(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := map[string]any{
		"required":      a.required,
		"authenticated": a.authenticated(req),
	}
	if a.oidc != nil {
		status["provider"] = "oidc"
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (a *controlAuthenticator) login(w http.ResponseWriter, req *http.Request) {
	if !a.required {
		a.writeStatus(w, req)
		return
	}
	var body struct {
		Action   string `json:"action"`
		Password string `json:"password"`
		Code     string `json:"code"`
		State    string `json:"state"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "bad auth payload", http.StatusBadRequest)
		return
	}
	// OIDC first leg: hand the browser the provider authorize URL. No session
	// is established until the code comes back to completeLogin.
	if a.oidc != nil && body.Action == "start" {
		authURL, err := a.oidc.startLogin(req.Context(), controlCallbackURL(req))
		if err != nil {
			http.Error(w, "oidc start failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"authorizeUrl": authURL})
		return
	}
	if a.oidc != nil {
		if err := a.oidc.completeLogin(req.Context(), body.Code, body.State); err != nil {
			http.Error(w, "oidc sign-in failed", http.StatusUnauthorized)
			return
		}
	} else {
		got := sha256.Sum256([]byte(body.Password))
		if subtle.ConstantTimeCompare(got[:], a.passwordHash[:]) != 1 {
			http.Error(w, "bad password", http.StatusUnauthorized)
			return
		}
	}
	token, err := randomToken()
	if err != nil {
		http.Error(w, "auth token unavailable", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(24 * time.Hour)
	a.mu.Lock()
	a.sessions[token] = expires
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     controlAuthCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https",
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{
		"required":      true,
		"authenticated": true,
	})
}

func (a *controlAuthenticator) logout(w http.ResponseWriter, req *http.Request) {
	if cookie, err := req.Cookie(controlAuthCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     controlAuthCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https",
	})
	a.writeStatus(w, req)
}

func randomToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
