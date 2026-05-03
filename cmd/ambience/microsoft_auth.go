package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type microsoftControlAuthConfig struct {
	Tenant   string
	ClientID string
}

type microsoftControlAuth struct {
	tenant   string
	clientID string
	jwksURL  string
	client   *http.Client
	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	fetched  time.Time
}

func newMicrosoftControlAuth(cfg microsoftControlAuthConfig) *microsoftControlAuth {
	tenant := strings.TrimSpace(cfg.Tenant)
	clientID := strings.TrimSpace(cfg.ClientID)
	if tenant == "" || clientID == "" {
		return nil
	}
	return &microsoftControlAuth{
		tenant:   tenant,
		clientID: clientID,
		jwksURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/discovery/v2.0/keys", tenant),
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (m *microsoftControlAuth) verifyIDToken(ctx context.Context, raw, nonce string) error {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return errors.New("token must be jwt")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	var claims struct {
		Aud   any    `json:"aud"`
		Iss   string `json:"iss"`
		Tid   string `json:"tid"`
		Exp   int64  `json:"exp"`
		Nbf   int64  `json:"nbf"`
		Nonce string `json:"nonce"`
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
	if !jwtAudienceMatches(claims.Aud, m.clientID) {
		return errors.New("token audience invalid")
	}
	if !m.issuerAllowed(claims.Iss, claims.Tid) {
		return errors.New("token issuer invalid")
	}
	if nonce == "" || claims.Nonce != nonce {
		return errors.New("token nonce invalid")
	}
	key, err := m.key(ctx, header.Kid)
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

func (m *microsoftControlAuth) issuerAllowed(issuer, tenantID string) bool {
	tenant := strings.ToLower(m.tenant)
	if tenant == "common" || tenant == "organizations" {
		return tenantID != "" && strings.HasPrefix(issuer, "https://login.microsoftonline.com/"+tenantID+"/")
	}
	return issuer == "https://login.microsoftonline.com/"+m.tenant+"/v2.0"
}

func (m *microsoftControlAuth) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	m.mu.Lock()
	if key := m.keys[kid]; key != nil && time.Since(m.fetched) < time.Hour {
		m.mu.Unlock()
		return key, nil
	}
	m.mu.Unlock()
	if err := m.refreshKeys(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.keys[kid]
	if key == nil {
		return nil, errors.New("microsoft key not found")
	}
	return key, nil
}

func (m *microsoftControlAuth) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("microsoft jwks returned %s", resp.Status)
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
		return errors.New("microsoft jwks had no rsa keys")
	}
	m.mu.Lock()
	m.keys = keys
	m.fetched = time.Now()
	m.mu.Unlock()
	return nil
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
