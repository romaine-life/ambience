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
	microsoft    *microsoftControlAuth
	mu           sync.Mutex
	sessions     map[string]time.Time
}

func newControlAuthenticator(password string, microsoftCfg microsoftControlAuthConfig) *controlAuthenticator {
	password = strings.TrimSpace(password)
	microsoftAuth := newMicrosoftControlAuth(microsoftCfg)
	a := &controlAuthenticator{
		required:  password != "" || microsoftAuth != nil,
		microsoft: microsoftAuth,
		sessions:  make(map[string]time.Time),
	}
	if password != "" {
		a.passwordHash = sha256.Sum256([]byte(password))
	}
	return a
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
	if a.microsoft != nil {
		status["provider"] = "microsoft"
		status["microsoftTenant"] = a.microsoft.tenant
		status["microsoftClientId"] = a.microsoft.clientID
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (a *controlAuthenticator) login(w http.ResponseWriter, req *http.Request) {
	if !a.required {
		a.writeStatus(w, req)
		return
	}
	var body struct {
		Password string `json:"password"`
		IDToken  string `json:"idToken"`
		Nonce    string `json:"nonce"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "bad auth payload", http.StatusBadRequest)
		return
	}
	if a.microsoft != nil {
		if err := a.microsoft.verifyIDToken(req.Context(), body.IDToken, body.Nonce); err != nil {
			http.Error(w, "bad microsoft token", http.StatusUnauthorized)
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
