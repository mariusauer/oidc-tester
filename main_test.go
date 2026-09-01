package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func TestDecodeJWT(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice","admin":true}`))

	decoded := decodeJWT(header + "." + payload + ".signature")
	for _, want := range []string{`"alg": "RS256"`, `"sub": "alice"`, `"admin": true`} {
		if !strings.Contains(decoded, want) {
			t.Fatalf("decodeJWT() = %q, want it to contain %q", decoded, want)
		}
	}

	if got := decodeJWT("not-a-jwt"); got != "invalid JWT" {
		t.Fatalf("decodeJWT() = %q, want %q", got, "invalid JWT")
	}
}

func TestRandomString(t *testing.T) {
	first := randomString(32)
	second := randomString(32)
	if first == second {
		t.Fatal("randomString returned the same value twice")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("randomString returned invalid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("randomString decoded to %d bytes, want 32", len(decoded))
	}
}

func TestHomeHandler(t *testing.T) {
	setTestTemplates(t)
	cfg = Config{
		IssuerURL:   "https://issuer.example",
		ClientID:    "test-client",
		RedirectURL: "http://localhost:3000/callback",
		ListenAddr:  ":3000",
		Scopes:      []string{"openid", "profile"},
	}

	recorder := httptest.NewRecorder()
	homeHandler(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want HTML", got)
	}
	if !strings.Contains(recorder.Body.String(), cfg.IssuerURL) {
		t.Fatalf("response does not contain issuer URL %q", cfg.IssuerURL)
	}
}

func TestLoginHandler(t *testing.T) {
	oauthConfig = &oauth2.Config{
		ClientID:    "test-client",
		RedirectURL: "http://localhost:3000/callback",
		Scopes:      []string{"openid", "profile"},
		Endpoint: oauth2.Endpoint{
			AuthURL: "https://issuer.example/authorize",
		},
	}

	recorder := httptest.NewRecorder()
	loginHandler(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	query := location.Query()
	if query.Get("state") == "" || query.Get("state") != currentState {
		t.Fatalf("redirect state = %q, current state = %q", query.Get("state"), currentState)
	}
	if query.Get("client_id") != "test-client" || query.Get("scope") != "openid profile" {
		t.Fatalf("unexpected authorization query: %s", location.RawQuery)
	}
}

func TestCallbackHandlerRejectsProviderError(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/callback?error=access_denied&error_description=nope", nil)
	callbackHandler(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "access_denied: nope") {
		t.Fatalf("unexpected response: %q", recorder.Body.String())
	}
}

func TestCallbackHandlerRejectsInvalidState(t *testing.T) {
	currentState = "expected"
	recorder := httptest.NewRecorder()
	callbackHandler(recorder, httptest.NewRequest(http.MethodGet, "/callback?state=wrong&code=code", nil))

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid state") {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestOIDCFlow(t *testing.T) {
	setTestTemplates(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(t, w, map[string]any{
				"issuer":                 server.URL,
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"jwks_uri":               server.URL + "/keys",
				"end_session_endpoint":   server.URL + "/logout",
			})
		case "/keys":
			writeJSON(t, w, map[string]any{"keys": []any{jwk(&key.PublicKey)}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token request: %v", err)
			}
			if got := r.Form.Get("code"); got != "valid-code" {
				t.Errorf("token code = %q, want valid-code", got)
			}
			token := signedIDToken(t, key, server.URL, "test-client")
			writeJSON(t, w, map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"id_token":     token,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, server.Client())
	provider, err = oidc.NewProvider(ctx, server.URL)
	if err != nil {
		t.Fatalf("discover provider: %v", err)
	}
	verifier = provider.Verifier(&oidc.Config{ClientID: "test-client"})
	oauthConfig = &oauth2.Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost:3000/callback",
		Scopes:       []string{oidc.ScopeOpenID},
		Endpoint:     provider.Endpoint(),
	}
	currentState = "valid-state"

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/callback?state=valid-state&code=valid-code", nil)
	callbackHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	for _, want := range []string{"alice@example.com", "Alice Example"} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("callback response does not contain %q", want)
		}
	}
	if strings.Contains(recorder.Body.String(), "access-token") {
		t.Fatal("callback response contains the access token")
	}

	logoutRecorder := httptest.NewRecorder()
	logoutHandler(logoutRecorder, httptest.NewRequest(http.MethodGet, "/logout", nil))
	if logoutRecorder.Code != http.StatusFound || logoutRecorder.Header().Get("Location") != server.URL+"/logout" {
		t.Fatalf("logout redirect = %q (status %d)", logoutRecorder.Header().Get("Location"), logoutRecorder.Code)
	}
}

func setTestTemplates(t *testing.T) {
	t.Helper()
	var err error
	tmpl, err = template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode JSON response: %v", err)
	}
}

func jwk(key *rsa.PublicKey) map[string]any {
	exponent := big.NewInt(int64(key.E)).Bytes()
	return map[string]any{
		"kty": "RSA",
		"kid": "test-key",
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func signedIDToken(t *testing.T, key *rsa.PrivateKey, issuer, audience string) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal JWT header: %v", err)
	}
	claims, err := json.Marshal(map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "alice",
		"email": "alice@example.com",
		"name":  "Alice Example",
		"iat":   time.Now().Add(-time.Minute).Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}

	unsigned := fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString(header),
		base64.RawURLEncoding.EncodeToString(claims),
	)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}
