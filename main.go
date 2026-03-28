package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Config struct {
	IssuerURL    string   `json:"issuer_url"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	ListenAddr   string   `json:"listen_addr"`
	Scopes       []string `json:"scopes"`
}

var (
	cfg          Config
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauthConfig  *oauth2.Config
	currentState string
)

func main() {
	loadConfig()

	ctx := insecureContext(context.Background())

	var err error
	provider, err = oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		log.Fatalf("failed to discover provider: %v", err)
	}

	verifier = provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	oauthConfig = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   provider.Endpoint().AuthURL,
			TokenURL:  provider.Endpoint().TokenURL,
			AuthStyle: oauth2.AuthStyleInHeader, // AD FS friendly
		},
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/logout", logoutHandler)

	log.Printf("App running at http://localhost%s\n", cfg.ListenAddr)
	log.Fatal(http.ListenAndServe(cfg.ListenAddr, nil))
}

// ===============================
// CONFIG LOADING
// ===============================
func loadConfig() {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("failed to open config.json: %v", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		log.Fatalf("failed to parse config.json: %v", err)
	}
}

// ===============================
// HANDLERS
// ===============================

func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<h2>OIDC Test App</h2><a href="/login">Login with OIDC</a>`))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	currentState = randomString(32)
	authURL := oauthConfig.AuthCodeURL(currentState)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := insecureContext(r.Context())

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		http.Error(w, fmt.Sprintf("%s: %s", errParam, desc), http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != currentState {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")

	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("token exchange failed: %v", err), http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing id_token", http.StatusInternalServerError)
		return
	}

	log.Println("==== RAW JWT ====")
	log.Println(rawIDToken)

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("verify failed: %v", err), http.StatusInternalServerError)
		return
	}

	var claims map[string]interface{}
	idToken.Claims(&claims)

	pretty, _ := json.MarshalIndent(claims, "", "  ")

	log.Println("==== CLAIMS ====")
	log.Println(string(pretty))

	decoded := decodeJWT(rawIDToken)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(fmt.Sprintf(`
<h2>Login successful</h2>
<form action="/logout" method="get">
  <button type="submit">Logout</button>
</form>
<h3>Claims</h3>
<pre>%s</pre>
<h3>Decoded JWT</h3>
<pre>%s</pre>
`, pretty, decoded)))
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	var metadata struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&metadata); err == nil && metadata.EndSessionEndpoint != "" {
		http.Redirect(w, r, metadata.EndSessionEndpoint, http.StatusFound)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// ===============================
// HELPERS
// ===============================

func insecureContext(ctx context.Context) context.Context {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: tr})
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeJWT(token string) string {
	parts := split(token)
	if len(parts) < 2 {
		return "invalid JWT"
	}

	h, _ := base64.RawURLEncoding.DecodeString(parts[0])
	p, _ := base64.RawURLEncoding.DecodeString(parts[1])

	var out map[string]interface{}
	json.Unmarshal([]byte(fmt.Sprintf(`{"header":%s,"payload":%s}`, h, p)), &out)

	pretty, _ := json.MarshalIndent(out, "", "  ")
	return string(pretty)
}

func split(s string) []string {
	var out []string
	start := 0
	for i := range s {
		if s[i] == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
