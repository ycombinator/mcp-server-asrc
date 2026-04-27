package main

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/login.html templates/success.html
var templateFS embed.FS

var loginTmpl = template.Must(template.ParseFS(templateFS, "templates/login.html"))
var successTmpl = template.Must(template.ParseFS(templateFS, "templates/success.html"))

type clubConfig struct {
	Name    string
	Short   string
	Website string
}

type oauthServer struct {
	baseURL string
	store   *tokenStore
	club    clubConfig
}

type loginPageData struct {
	ClubName            string
	ClubShort           string
	ClubWebsite         string
	Error               string
	ResponseType        string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
}

func (o *oauthServer) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"resource":              o.baseURL,
		"authorization_servers": []string{o.baseURL},
		"resource_name":         o.club.Short + " Tennis MCP Server",
	})
}

func (o *oauthServer) handleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                o.baseURL,
		"authorization_endpoint":                o.baseURL + "/oauth/authorize",
		"token_endpoint":                        o.baseURL + "/oauth/token",
		"registration_endpoint":                 o.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"clubspot"},
	})
}

func (o *oauthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	clientID, err := generateRandomString(16)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	o.store.registerClient(&registeredClient{
		clientID:     clientID,
		redirectURIs: req.RedirectURIs,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"client_id":                  clientID,
		"client_name":               req.ClientName,
		"redirect_uris":             req.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":               []string{"authorization_code"},
		"response_types":            []string{"code"},
	})
}

func (o *oauthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		o.showLoginPage(w, r, "")
	case http.MethodPost:
		o.processLogin(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (o *oauthServer) showLoginPage(w http.ResponseWriter, r *http.Request, errorMsg string) {
	get := r.URL.Query().Get
	if r.Method == http.MethodPost {
		r.ParseForm()
		get = r.FormValue
	}

	data := loginPageData{
		ClubName:            o.club.Name,
		ClubShort:           o.club.Short,
		ClubWebsite:         o.club.Website,
		Error:               errorMsg,
		ResponseType:        get("response_type"),
		ClientID:            get("client_id"),
		RedirectURI:         get("redirect_uri"),
		State:               get("state"),
		CodeChallenge:       get("code_challenge"),
		CodeChallengeMethod: get("code_challenge_method"),
		Scope:               get("scope"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := loginTmpl.Execute(w, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

func (o *oauthServer) processLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	clientID := r.FormValue("client_id")

	if email == "" || password == "" {
		o.showLoginPage(w, r, "Email and password are required.")
		return
	}

	if redirectURI == "" {
		o.showLoginPage(w, r, "Invalid authorization request: missing redirect URI.")
		return
	}

	clubspotToken, err := login(email, password)
	if err != nil {
		log.Printf("login failed for %s: %v", email, err)
		o.showLoginPage(w, r, "Invalid email or password. Please try again.")
		return
	}

	code, err := generateRandomString(32)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	o.store.storeAuthCode(code, &authCode{
		session: &userSession{
			clubspotToken: clubspotToken,
			email:         email,
			createdAt:     time.Now(),
		},
		codeChallenge:       codeChallenge,
		codeChallengeMethod: codeChallengeMethod,
		redirectURI:         redirectURI,
		clientID:            clientID,
		expiresAt:           time.Now().Add(5 * time.Minute),
	})

	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	location := redirectURI + sep + "code=" + code
	if state != "" {
		location += "&state=" + state
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	successTmpl.Execute(w, struct {
		ClubName    string
		RedirectURL string
	}{o.club.Name, location})
}

func (o *oauthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, "invalid_request", "Invalid form data", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		writeOAuthError(w, "unsupported_grant_type", "Only authorization_code is supported", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	ac, ok := o.store.consumeAuthCode(code)
	if !ok {
		writeOAuthError(w, "invalid_grant", "Invalid or expired authorization code", http.StatusBadRequest)
		return
	}

	if ac.codeChallenge != "" {
		verifier := r.FormValue("code_verifier")
		if verifier == "" {
			writeOAuthError(w, "invalid_request", "Missing code_verifier", http.StatusBadRequest)
			return
		}
		if !verifyPKCE(verifier, ac.codeChallenge) {
			writeOAuthError(w, "invalid_grant", "PKCE verification failed", http.StatusBadRequest)
			return
		}
	}

	accessToken, err := generateRandomString(32)
	if err != nil {
		writeOAuthError(w, "server_error", "Failed to generate token", http.StatusInternalServerError)
		return
	}

	o.store.storeToken(accessToken, ac.session)
	log.Printf("issued token for %s", ac.session.email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   86400,
	})
}

func (o *oauthServer) authRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		session, ok := o.store.lookupToken(token)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userSessionCtxKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeOAuthError(w http.ResponseWriter, code, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}
