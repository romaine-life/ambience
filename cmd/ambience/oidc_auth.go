package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// oidcControlAuth authenticates control-surface admins against an OpenID
// Connect provider (auth.romaine.life) using the authorization-code + PKCE
// flow. The browser only ever holds the short-lived `code`; this backend does
// the code→token exchange (BFF pattern), so the provider's token endpoint
// needs no cross-origin CORS and the id_token never reaches page JS.
type oidcControlAuthConfig struct {
	Issuer   string
	ClientID string
}

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type oidcPendingLogin struct {
	verifier    string
	redirectURI string
	expires     time.Time
}

type oidcControlAuth struct {
	issuer   string
	clientID string
	client   *http.Client

	mu      sync.Mutex
	disco   *oidcDiscovery
	discoAt time.Time
	keys    map[string]*rsa.PublicKey
	keysAt  time.Time
	pending map[string]oidcPendingLogin
}

func newOIDCControlAuth(cfg oidcControlAuthConfig) *oidcControlAuth {
	issuer := strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	clientID := strings.TrimSpace(cfg.ClientID)
	if issuer == "" || clientID == "" {
		return nil
	}
	return &oidcControlAuth{
		issuer:   issuer,
		clientID: clientID,
		client:   &http.Client{Timeout: 8 * time.Second},
		pending:  make(map[string]oidcPendingLogin),
	}
}

func (o *oidcControlAuth) discovery(ctx context.Context) (*oidcDiscovery, error) {
	o.mu.Lock()
	if o.disco != nil && time.Since(o.discoAt) < time.Hour {
		d := o.disco
		o.mu.Unlock()
		return d, nil
	}
	o.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var d oidcDiscovery
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	if d.Issuer != o.issuer || d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return nil, errors.New("oidc discovery doc incomplete or issuer mismatch")
	}
	o.mu.Lock()
	o.disco = &d
	o.discoAt = time.Now()
	o.mu.Unlock()
	return &d, nil
}

// startLogin records a pending PKCE login keyed by an opaque state and returns
// the provider authorize URL the browser should be redirected to.
func (o *oidcControlAuth) startLogin(ctx context.Context, redirectURI string) (string, error) {
	d, err := o.discovery(ctx)
	if err != nil {
		return "", err
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		return "", err
	}
	state, err := randomURLToken(24)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	o.mu.Lock()
	o.gcPendingLocked()
	o.pending[state] = oidcPendingLogin{verifier: verifier, redirectURI: redirectURI, expires: time.Now().Add(10 * time.Minute)}
	o.mu.Unlock()

	q := url.Values{}
	q.Set("client_id", o.clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return d.AuthorizationEndpoint + "?" + q.Encode(), nil
}

// completeLogin redeems the authorization code for an id_token and verifies it.
func (o *oidcControlAuth) completeLogin(ctx context.Context, code, state string) error {
	if code == "" || state == "" {
		return errors.New("missing code or state")
	}
	o.mu.Lock()
	p, ok := o.pending[state]
	if ok {
		delete(o.pending, state)
	}
	o.mu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return errors.New("unknown or expired login state")
	}
	d, err := o.discovery(ctx)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURI)
	form.Set("client_id", o.clientID)
	form.Set("code_verifier", p.verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc token endpoint returned %s", resp.Status)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return err
	}
	if tok.IDToken == "" {
		return errors.New("token response missing id_token")
	}
	return o.verifyIDToken(ctx, tok.IDToken)
}

func (o *oidcControlAuth) verifyIDToken(ctx context.Context, raw string) error {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return errors.New("token must be jwt")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	var claims struct {
		Aud any    `json:"aud"`
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
		Nbf int64  `json:"nbf"`
	}
	if err := decodeJWTJSON(parts[0], &header); err != nil {
		return err
	}
	if header.Alg != "RS256" || header.Kid == "" {
		return errors.New("unsupported token header")
	}
	if err := decodeJWTJSON(parts[1], &claims); err != nil {
		return err
	}
	now := time.Now().Unix()
	if claims.Exp <= now || (claims.Nbf != 0 && claims.Nbf > now+60) {
		return errors.New("token time invalid")
	}
	if !jwtAudienceMatches(claims.Aud, o.clientID) {
		return errors.New("token audience invalid")
	}
	if claims.Iss != o.issuer {
		return errors.New("token issuer invalid")
	}
	key, err := o.key(ctx, header.Kid)
	if err != nil {
		return err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig)
}

func (o *oidcControlAuth) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	o.mu.Lock()
	if key := o.keys[kid]; key != nil && time.Since(o.keysAt) < time.Hour {
		o.mu.Unlock()
		return key, nil
	}
	o.mu.Unlock()
	if err := o.refreshKeys(ctx); err != nil {
		return nil, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	key := o.keys[kid]
	if key == nil {
		return nil, errors.New("oidc signing key not found")
	}
	return key, nil
}

func (o *oidcControlAuth) refreshKeys(ctx context.Context) error {
	d, err := o.discovery(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.JWKSURI, nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc jwks returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		if jwk.Kty != "RSA" || jwk.Kid == "" {
			continue
		}
		key, err := rsaPublicKeyFromJWK(jwk.N, jwk.E)
		if err != nil {
			continue
		}
		keys[jwk.Kid] = key
	}
	if len(keys) == 0 {
		return errors.New("oidc jwks had no rsa keys")
	}
	o.mu.Lock()
	o.keys = keys
	o.keysAt = time.Now()
	o.mu.Unlock()
	return nil
}

// gcPendingLocked drops expired pending logins. Caller holds o.mu.
func (o *oidcControlAuth) gcPendingLocked() {
	now := time.Now()
	for state, p := range o.pending {
		if now.After(p.expires) {
			delete(o.pending, state)
		}
	}
}

func randomURLToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func decodeJWTJSON(segment string, target any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func jwtAudienceMatches(aud any, clientID string) bool {
	switch value := aud.(type) {
	case string:
		return value == clientID
	case []any:
		for _, item := range value {
			if s, ok := item.(string); ok && s == clientID {
				return true
			}
		}
	}
	return false
}

func rsaPublicKeyFromJWK(nRaw, eRaw string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nRaw)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eRaw)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		return nil, errors.New("invalid rsa exponent")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}
