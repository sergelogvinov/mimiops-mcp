/*
Copyright 2026 Serge Logvinov.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sergelogvinov/mimiops-mcp/internal/oidc"
)

const (
	// wellKnownAuthorizationServer is the RFC 8416 well-known path under
	// which OAuth 2.0 Authorization Server Metadata is served.
	wellKnownAuthorizationServer = "/.well-known/oauth-authorization-server"

	proxyClientTTL  = 24 * time.Hour
	proxySessionTTL = 10 * time.Minute
	proxyGrantTTL   = 2 * time.Minute

	maxProxyClients = 10000
)

// defaultProxyScopes are used when no scopes are configured. "openid"
// guarantees an id_token, "email" is needed when --oidc-email-domains is
// configured.
var defaultProxyScopes = []string{"openid", "profile", "email"}

// oauthProxy implements the MCP authorization flow as an OAuth 2.0
// authorization-server proxy (enabled by --oidc-callback-url). It advertises
// itself as the authorization server, performs dynamic client registration
// (RFC 7591), and terminates the authorization-code flow at a fixed callback
// URL registered with the upstream provider. The code exchange happens
// server-side; the upstream token is handed to the MCP client verbatim, so
// the passthrough verification and the Kubernetes API server still see the
// real provider token.
type oauthProxy struct {
	verifier     *oidc.Verifier
	clientID     string
	clientSecret string
	callbackURL  string
	callbackPath string
	authURL      string
	tokenURL     string
	scopes       string
	httpClient   *http.Client
	log          *slog.Logger

	allowedRedirectPrefixes []string

	stop     chan struct{}
	mu       sync.Mutex
	clients  map[string]*proxyClient
	sessions map[string]*proxySession
	grants   map[string]*proxyGrant
}

// proxyClient is a dynamically registered MCP client (RFC 7591).
type proxyClient struct {
	redirectURIs []string
	expires      time.Time
}

// proxySession is a pending authorization request, keyed by the state sent
// to the upstream provider.
type proxySession struct {
	clientID        string
	redirectURI     string
	clientState     string
	codeChallenge   string
	challengeMethod string
	verifier        string // PKCE verifier for the upstream code exchange
	expires         time.Time
}

// proxyGrant is an issued proxy authorization code, exchanged once at /token.
type proxyGrant struct {
	accessToken     string
	expiresIn       int
	clientID        string
	redirectURI     string
	codeChallenge   string
	challengeMethod string
	expires         time.Time
}

// proxyConfig configures the oauthProxy.
type proxyConfig struct {
	clientID     string
	clientSecret string
	callbackURL  string
	scopes       []string
	log          *slog.Logger
	httpClient   *http.Client
}

// newOAuthProxy validates the configuration and returns a ready-to-use
// proxy. It fails fast on invalid configuration so errors surface at startup.
func newOAuthProxy(verifier *oidc.Verifier, cfg proxyConfig) (*oauthProxy, error) {
	if cfg.clientID == "" {
		return nil, errors.New("oidc proxy: --oidc-client-id is required when --oidc-callback-url is set")
	}

	u, err := url.Parse(cfg.callbackURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return nil, fmt.Errorf("oidc proxy: --oidc-callback-url must be an absolute URL, got %q", cfg.callbackURL)
	}

	if u.Scheme != "https" && !isLoopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("oidc proxy: --oidc-callback-url must use https (http is only allowed for loopback hosts), got %q", cfg.callbackURL)
	}

	callbackPath := u.Path
	if callbackPath == "" {
		callbackPath = "/oauth/callback"

		u.Path = callbackPath
		cfg.callbackURL = u.String()
	}

	scopes := cfg.scopes
	if len(scopes) == 0 {
		scopes = defaultProxyScopes
	}

	// The proxy hands out the upstream id_token, so the authorization
	// request must produce one.
	if !slices.Contains(scopes, "openid") {
		return nil, errors.New("oidc proxy: --oidc-scope must include 'openid' (an id_token is required for verification)")
	}

	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	log := cfg.log
	if log == nil {
		log = slog.Default()
	}

	p := &oauthProxy{
		verifier:     verifier,
		clientID:     cfg.clientID,
		clientSecret: cfg.clientSecret,
		callbackURL:  cfg.callbackURL,
		callbackPath: callbackPath,
		authURL:      verifier.AuthURL(),
		tokenURL:     verifier.TokenURL(),
		scopes:       strings.Join(scopes, " "),
		httpClient:   httpClient,
		log:          log,
		stop:         make(chan struct{}),
		clients:      map[string]*proxyClient{},
		sessions:     map[string]*proxySession{},
		grants:       map[string]*proxyGrant{},
	}

	go p.sweep()

	return p, nil
}

// Close stops the background sweeper.
func (p *oauthProxy) Close() {
	close(p.stop)
}

// handleRegister implements RFC 7591 dynamic client registration. The proxy
// issues public clients (no secret); the flow's security comes from PKCE,
// the registered redirect URIs, and the short-lived grants.
func (p *oauthProxy) handleRegister(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "failed to parse registration request")
		return
	}

	if len(req.RedirectURIs) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
		return
	}

	for _, uri := range req.RedirectURIs {
		if !allowedRedirectURI(p, uri) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri",
				fmt.Sprintf("redirect URI %q must be https, or http on a loopback host", uri))
			return
		}
	}

	clientID, err := p.registerClient(req.RedirectURIs)
	if err != nil {
		p.log.Debug("oidc proxy: client registration rejected", "reason", err.Error())
		oauthError(w, http.StatusServiceUnavailable, "server_error", "client registration limit reached, try again later")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().Unix(),
		"redirect_uris":              req.RedirectURIs,
		"token_endpoint_auth_method": "none",
	})
}

// handleAuthorize starts the authorization-code flow: it validates the
// client's request and redirects the browser to the upstream provider with
// the proxy's fixed callback URL and its own state and PKCE verifier.
func (p *oauthProxy) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	if q.Get("response_type") != "code" {
		oauthError(w, http.StatusBadRequest, "unsupported_response_type", "only response_type=code is supported")
		return
	}

	clientID := q.Get("client_id")
	if clientID == "" {
		oauthError(w, http.StatusBadRequest, "invalid_client", "client_id is required")
		return
	}

	redirectURI := q.Get("redirect_uri")

	p.mu.Lock()
	client, ok := p.clients[clientID]
	if ok && time.Now().After(client.expires) {
		delete(p.clients, clientID)
		ok = false
	}
	p.mu.Unlock()

	if ok {
		if !slices.Contains(client.redirectURIs, redirectURI) {
			// Never redirect: an unregistered redirect_uri must not be honored
			// (open-redirect protection).
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri is not registered for this client")
			return
		}
	} else {
		// Unknown client_id: MCP clients cache dynamic registrations, and
		// the registry is in-memory, so registrations do not survive server
		// restarts. Accept the request by validating the redirect URI exactly
		// as /register would — security-wise this is equivalent to the open
		// /register endpoint, and it keeps cached clients working.
		if !allowedRedirectURI(p, redirectURI) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri",
				fmt.Sprintf("redirect URI %q must be https, or http on a loopback host", redirectURI))
			return
		}

		p.log.Debug("oidc proxy: auto-registered unknown client", "client_id", clientID)
	}

	challenge := q.Get("code_challenge")
	if challenge == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "PKCE code_challenge is required")
		return
	}

	method := q.Get("code_challenge_method")
	if method == "" {
		method = "plain"
	}

	if method != "S256" && method != "plain" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code_challenge_method must be S256 or plain")
		return
	}

	upstreamState := randomToken(32)
	verifier := randomPKCEVerifier()

	const maxProxySessions = 10000

	p.mu.Lock()

	if len(p.sessions) >= maxProxySessions {
		p.cleanupLocked(time.Now())
		if len(p.sessions) >= maxProxySessions {
			p.mu.Unlock()
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "too many pending authorization requests, try again later")

			return
		}
	}

	p.sessions[upstreamState] = &proxySession{
		clientID:        clientID,
		redirectURI:     redirectURI,
		clientState:     q.Get("state"),
		codeChallenge:   challenge,
		challengeMethod: method,
		verifier:        verifier,
		expires:         time.Now().Add(proxySessionTTL),
	}
	p.mu.Unlock()

	upstream := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.clientID},
		"redirect_uri":          {p.callbackURL},
		"scope":                 {p.scopes},
		"state":                 {upstreamState},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, p.authURL+"?"+upstream.Encode(), http.StatusFound)
}

// handleCallback terminates the upstream authorization-code flow at the
// fixed redirect URI: it exchanges the code server-side, verifies the token,
// and redirects the browser back to the MCP client with a short-lived proxy
// authorization code.
func (p *oauthProxy) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	p.mu.Lock()
	session := p.sessions[q.Get("state")]
	delete(p.sessions, q.Get("state"))
	p.mu.Unlock()

	if session == nil || time.Now().After(session.expires) {
		http.Error(w, "invalid or expired authorization state", http.StatusBadRequest)
		return
	}

	if errCode := q.Get("error"); errCode != "" {
		// Forward the upstream error to the client's redirect URI.
		dest := url.Values{"error": {errCode}}
		if description := q.Get("error_description"); description != "" {
			dest.Set("error_description", description)
		}

		if session.clientState != "" {
			dest.Set("state", session.clientState)
		}

		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, redirectWithParams(session.redirectURI, dest), http.StatusFound)
		return
	}

	code := q.Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	token, expiresIn, err := p.exchangeCode(r.Context(), code, session.verifier)
	if err != nil {
		// The reason is operator-relevant (e.g. a missing --oidc-client-secret
		// surfaces as the issuer's invalid_client) and contains no token
		// material, so surface it in both the logs and the browser page.
		p.log.Warn("oidc proxy: upstream code exchange failed", "reason", err.Error())
		http.Error(w, "authorization failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	if _, err := p.verifier.Verify(r.Context(), token); err != nil {
		p.log.Warn("oidc proxy: upstream token rejected", "reason", err.Error())
		http.Error(w, "authorization failed: token rejected: "+err.Error(), http.StatusBadGateway)
		return
	}

	proxyCode := randomToken(32)

	p.mu.Lock()
	p.grants[proxyCode] = &proxyGrant{
		accessToken:     token,
		expiresIn:       expiresIn,
		clientID:        session.clientID,
		redirectURI:     session.redirectURI,
		codeChallenge:   session.codeChallenge,
		challengeMethod: session.challengeMethod,
		expires:         time.Now().Add(proxyGrantTTL),
	}
	p.mu.Unlock()

	dest := url.Values{"code": {proxyCode}}
	if session.clientState != "" {
		dest.Set("state", session.clientState)
	}

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, redirectWithParams(session.redirectURI, dest), http.StatusFound)
}

// redirectWithParams merges params into the query of the registered redirect
// URI and returns the result. A redirect URI may itself carry a query
// component; appending a second "?" would corrupt it.
func redirectWithParams(redirectURI string, params url.Values) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		// Registration validated the URI; this cannot happen.
		return redirectURI
	}

	q := u.Query()
	for key, values := range params {
		q[key] = append(q[key], values...)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// handleToken exchanges a proxy authorization code for the upstream token.
func (p *oauthProxy) handleToken(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "failed to parse request")
		return
	}

	if r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "only grant_type=authorization_code is supported")
		return
	}

	clientID := r.PostForm.Get("client_id")
	if clientID == "" {
		if basicID, _, ok := r.BasicAuth(); ok {
			clientID = basicID
		}
	}

	code := r.PostForm.Get("code")

	p.mu.Lock()
	grant := p.grants[code]
	// Authorization codes are single-use: burn it whether or not the
	// request validates, so a failed attempt cannot be retried.
	delete(p.grants, code)
	p.mu.Unlock()

	if grant == nil || time.Now().After(grant.expires) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid, expired, or already used")
		return
	}

	if grant.clientID != clientID {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}

	if grant.redirectURI != r.PostForm.Get("redirect_uri") {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}

	verifier := r.PostForm.Get("code_verifier")
	if verifier == "" || !pkceMatches(grant.codeChallenge, verifier, grant.challengeMethod) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	response := map[string]any{
		"access_token": grant.accessToken,
		"token_type":   "Bearer",
		"scope":        p.scopes,
	}
	if grant.expiresIn > 0 {
		response["expires_in"] = grant.expiresIn
	}

	writeJSON(w, http.StatusOK, response)
}

// authorizationServerMetadataDocument is the RFC 8416 metadata document
// served in proxy mode.
type authorizationServerMetadataDocument struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
}

// handleAuthorizationServerMetadata serves RFC 8416 metadata advertising the
// proxy's own endpoints.
func (p *oauthProxy) handleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	origin := originURL(r)

	writeJSON(w, http.StatusOK, authorizationServerMetadataDocument{
		Issuer:                            origin,
		AuthorizationEndpoint:             origin + "/authorize",
		TokenEndpoint:                     origin + "/token",
		RegistrationEndpoint:              origin + "/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		CodeChallengeMethodsSupported:     []string{"S256", "plain"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		ScopesSupported:                   strings.Fields(p.scopes),
	})
}

// exchangeCode exchanges the upstream authorization code at the provider's
// token endpoint. The id_token is preferred as the proxy's access token
// because it carries the iss/aud claims the verifier (and the Kubernetes API
// server) validate.
func (p *oauthProxy) exchangeCode(ctx context.Context, code, verifier string) (string, int, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.callbackURL},
		"client_id":     {p.clientID},
		"code_verifier": {verifier},
	}
	if p.clientSecret != "" {
		form.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			p.log.Debug("oidc proxy: failed to close token response body", "reason", err.Error())
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		var upstreamErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(body, &upstreamErr) == nil && upstreamErr.Error != "" {
			if upstreamErr.ErrorDescription != "" {
				return "", 0, fmt.Errorf("issuer token endpoint returned %d (%s: %s)",
					resp.StatusCode, upstreamErr.Error, upstreamErr.ErrorDescription)
			}

			return "", 0, fmt.Errorf("issuer token endpoint returned %d (%s)", resp.StatusCode, upstreamErr.Error)
		}

		return "", 0, fmt.Errorf("issuer token endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", 0, fmt.Errorf("failed to decode token response: %w", err)
	}

	token := tok.IDToken
	if token == "" {
		token = tok.AccessToken
	}

	if token == "" {
		return "", 0, errors.New("token response contains no id_token or access_token")
	}

	return token, tok.ExpiresIn, nil
}

func (p *oauthProxy) registerClient(redirectURIs []string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.clients) >= maxProxyClients {
		p.cleanupLocked(time.Now())
		if len(p.clients) >= maxProxyClients {
			return "", errors.New("client registration limit reached")
		}
	}

	clientID := randomToken(16)
	p.clients[clientID] = &proxyClient{
		redirectURIs: redirectURIs,
		expires:      time.Now().Add(proxyClientTTL),
	}

	return clientID, nil
}

func (p *oauthProxy) cleanupLocked(now time.Time) {
	for id, client := range p.clients {
		if now.After(client.expires) {
			delete(p.clients, id)
		}
	}

	for state, session := range p.sessions {
		if now.After(session.expires) {
			delete(p.sessions, state)
		}
	}

	for code, grant := range p.grants {
		if now.After(grant.expires) {
			delete(p.grants, code)
		}
	}
}

func (p *oauthProxy) sweep() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			p.cleanupLocked(time.Now())
			p.mu.Unlock()
		case <-p.stop:
			return
		}
	}
}

// allowedRedirectURI reports whether a client redirect URI may be registered:
// https anywhere, or http on a loopback host (RFC 8252).
func allowedRedirectURI(p *oauthProxy, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return false
	}

	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return true
	}

	if u.Scheme != "https" {
		return false
	}

	return slices.ContainsFunc(p.allowedRedirectPrefixes, func(prefix string) bool {
		return strings.HasPrefix(raw, prefix)
	})
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("oidc proxy: crypto/rand failed: " + err.Error())
	}

	return hex.EncodeToString(b)
}

func randomPKCEVerifier() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("oidc proxy: crypto/rand failed: " + err.Error())
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func pkceMatches(challenge, verifier, method string) bool {
	if method == "S256" {
		return challenge == pkceChallenge(verifier)
	}

	return challenge == verifier
}

func corsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// oauthError writes an RFC 6749 §5.2 error response.
func oauthError(w http.ResponseWriter, status int, code, description string) {
	payload := map[string]string{"error": code}
	if description != "" {
		payload["error_description"] = description
	}

	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	// Once the status is written there is nothing useful to do on encode
	// failure; the response is already committed.
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}
