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
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/sergelogvinov/mimiops-mcp/internal/oidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUpstream is a minimal OIDC provider: discovery, JWKS, and a token
// endpoint that returns a signed id_token for the fixed code "fake-auth-code".
type fakeUpstream struct {
	server   *httptest.Server
	issuer   string
	clientID string
	signer   jose.Signer

	mu        sync.Mutex
	tokenForm url.Values
}

const fakeAuthCode = "fake-auth-code"

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
	require.NoError(t, err)

	up := &fakeUpstream{signer: signer, clientID: "upstream-client"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                up.issuer,
			"jwks_uri":                              up.issuer + "/keys",
			"authorization_endpoint":                up.issuer + "/authorize",
			"token_endpoint":                        up.issuer + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"keys": []any{jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())

		up.mu.Lock()
		cloned := url.Values{}
		for k, v := range r.PostForm {
			cloned[k] = append([]string(nil), v...)
		}
		up.tokenForm = cloned
		up.mu.Unlock()

		if r.PostForm.Get("code") != fakeAuthCode {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "unknown code")
			return
		}

		claims := map[string]any{
			"iss":            up.issuer,
			"sub":            "user-1",
			"aud":            up.clientID,
			"exp":            time.Now().Add(time.Hour).Unix(),
			"iat":            time.Now().Unix(),
			"email":          "user@example.com",
			"email_verified": true,
		}

		payload, err := json.Marshal(claims)
		require.NoError(t, err)

		object, err := up.signer.Sign(payload)
		require.NoError(t, err)

		signed, err := object.CompactSerialize()
		require.NoError(t, err)

		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "upstream-access-token",
			"id_token":     signed,
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        "openid profile email",
		})
	})

	up.server = httptest.NewTLSServer(mux)
	t.Cleanup(up.server.Close)

	up.issuer = up.server.URL

	return up
}

func (up *fakeUpstream) recordedTokenForm() url.Values {
	up.mu.Lock()
	defer up.mu.Unlock()

	return up.tokenForm
}

// newProxyTestServer builds the full OIDC handler stack (metadata + OAuth
// proxy + /mcp behind auth) against a fake upstream provider.
func newProxyTestServer(t *testing.T) (*httptest.Server, *fakeUpstream, *oidc.Verifier) {
	t.Helper()

	up := newFakeUpstream(t)

	ctx := coreoidc.ClientContext(context.Background(), up.server.Client())
	verifier, err := oidc.New(ctx, oidc.Config{Issuer: up.issuer, ClientID: up.clientID})
	require.NoError(t, err)

	proxy, err := newOAuthProxy(verifier, proxyConfig{
		clientID:    up.clientID,
		callbackURL: up.issuer + "/oauth/callback",
		log:         slog.Default(),
		httpClient:  up.server.Client(),
	})
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	handler := newOIDCHandler(http.NotFoundHandler(), oidcHandlerConfig{
		verifier: verifier,
		issuer:   up.issuer,
		proxy:    proxy,
		log:      slog.Default(),
	})

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts, up, verifier
}

func registerProxyClient(t *testing.T, ts *httptest.Server, redirectURI string) string {
	t.Helper()

	body := fmt.Sprintf(`{"redirect_uris":[%q]}`, redirectURI)
	resp := doRequest(t, ts, http.MethodPost, "/register", body, "")
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var reg struct {
		ClientID string `json:"client_id"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &reg))
	require.NotEmpty(t, reg.ClientID)

	return reg.ClientID
}

func doRequest(t *testing.T, ts *httptest.Server, method, path, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request

	var err error

	if body == "" {
		req = httptest.NewRequestWithContext(context.Background(), method, ts.URL+path, nil)
	} else {
		req = httptest.NewRequestWithContext(context.Background(), method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
	}
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)

	return rec
}

func TestOAuthProxyFullFlow(t *testing.T) {
	ts, up, verifier := newProxyTestServer(t)

	clientRedirect := "http://localhost:53742/callback"

	// 1. Dynamic client registration.
	clientID := registerProxyClient(t, ts, clientRedirect)

	// 2. Authorize: the proxy must redirect to the upstream provider with
	// its own callback URL, state, and PKCE challenge.
	pkceVerifier := randomPKCEVerifier()
	authorizeURL := "/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {clientRedirect},
		"scope":                 {"openid profile email"},
		"state":                 {"client-state"},
		"code_challenge":        {pkceChallenge(pkceVerifier)},
		"code_challenge_method": {"S256"},
	}.Encode()

	resp := doRequest(t, ts, http.MethodGet, authorizeURL, "", "")
	require.Equal(t, http.StatusFound, resp.Code, resp.Body.String())

	location, err := url.Parse(resp.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, up.issuer+"/authorize", location.Scheme+"://"+location.Host+location.Path)

	upstreamQuery := location.Query()
	assert.Equal(t, "code", upstreamQuery.Get("response_type"))
	assert.Equal(t, up.clientID, upstreamQuery.Get("client_id"))
	assert.Equal(t, up.issuer+"/oauth/callback", upstreamQuery.Get("redirect_uri"))
	assert.Equal(t, "S256", upstreamQuery.Get("code_challenge_method"))
	assert.NotEmpty(t, upstreamQuery.Get("state"))
	assert.NotEmpty(t, upstreamQuery.Get("code_challenge"))
	upstreamState := upstreamQuery.Get("state")

	// 3. Callback: the upstream provider redirects the browser back with a
	// code; the proxy exchanges it server-side and redirects to the client.
	resp = doRequest(t, ts, http.MethodGet, "/oauth/callback?"+url.Values{
		"code":  {fakeAuthCode},
		"state": {upstreamState},
	}.Encode(), "", "")
	require.Equal(t, http.StatusFound, resp.Code, resp.Body.String())

	location, err = url.Parse(resp.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, clientRedirect, location.Scheme+"://"+location.Host+location.Path)
	assert.Equal(t, "client-state", location.Query().Get("state"))

	proxyCode := location.Query().Get("code")
	require.NotEmpty(t, proxyCode)

	// The upstream exchange used the proxy's callback URL and PKCE verifier.
	tokenForm := up.recordedTokenForm()
	require.NotNil(t, tokenForm)
	assert.Equal(t, fakeAuthCode, tokenForm.Get("code"))
	assert.Equal(t, up.issuer+"/oauth/callback", tokenForm.Get("redirect_uri"))
	assert.Equal(t, up.clientID, tokenForm.Get("client_id"))

	// 4. Token exchange: the client redeems the proxy code with PKCE.
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {proxyCode},
		"redirect_uri":  {clientRedirect},
		"client_id":     {clientID},
		"code_verifier": {pkceVerifier},
	}
	resp = doRequest(t, ts, http.MethodPost, "/token", form.Encode(), "application/x-www-form-urlencoded")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &tok))
	assert.Equal(t, "Bearer", tok.TokenType)
	assert.Equal(t, 3600, tok.ExpiresIn)

	// The proxy hands out the upstream id_token, and it must pass the same
	// verification the /mcp endpoint applies.
	claims, err := verifier.Verify(context.Background(), tok.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)

	// 5. The proxy code is single-use.
	resp = doRequest(t, ts, http.MethodPost, "/token", form.Encode(), "application/x-www-form-urlencoded")
	require.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "invalid_grant")
}

func TestOAuthProxyAuthorizeValidation(t *testing.T) {
	ts, up, _ := newProxyTestServer(t)

	clientRedirect := "http://localhost:53742/callback"
	clientID := registerProxyClient(t, ts, clientRedirect)

	tests := []struct {
		name   string
		params url.Values
		want   string
	}{
		{
			name: "missing client id",
			params: url.Values{
				"response_type": {"code"},
				"redirect_uri":  {clientRedirect}, "code_challenge": {"x"},
			},
			want: "invalid_client",
		},
		{
			name: "unregistered redirect uri",
			params: url.Values{
				"response_type": {"code"}, "client_id": {clientID},
				"redirect_uri": {"https://evil.example.com/callback"}, "code_challenge": {"x"},
			},
			want: "invalid_redirect_uri",
		},
		{
			name: "unknown client with disallowed redirect uri",
			params: url.Values{
				"response_type": {"code"}, "client_id": {"stale-cached-client"},
				"redirect_uri": {"ftp://evil.example.com/callback"}, "code_challenge": {"x"},
			},
			want: "invalid_redirect_uri",
		},
		{
			name: "missing pkce",
			params: url.Values{
				"response_type": {"code"}, "client_id": {clientID},
				"redirect_uri": {clientRedirect},
			},
			want: "invalid_request",
		},
		{
			name: "unsupported response type",
			params: url.Values{
				"response_type": {"token"}, "client_id": {clientID},
				"redirect_uri": {clientRedirect}, "code_challenge": {"x"},
			},
			want: "unsupported_response_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, ts, http.MethodGet, "/authorize?"+tt.params.Encode(), "", "")
			require.Equal(t, http.StatusBadRequest, resp.Code)
			assert.Contains(t, resp.Body.String(), tt.want)
		})
	}

	// An unknown client_id with a valid redirect URI is auto-registered, so
	// clients with cached registrations keep working across restarts.
	t.Run("unknown client with valid redirect uri is accepted", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {"stale-cached-client"},
			"redirect_uri": {clientRedirect}, "code_challenge": {pkceChallenge(randomPKCEVerifier())},
			"code_challenge_method": {"S256"},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code, resp.Body.String())
		assert.Contains(t, resp.Header().Get("Location"), up.issuer+"/authorize")
	})
}

func TestOAuthProxyTokenValidation(t *testing.T) {
	ts, _, _ := newProxyTestServer(t)

	clientRedirect := "http://localhost:53742/callback"
	clientID := registerProxyClient(t, ts, clientRedirect)

	// Run a full flow to obtain a valid proxy code.
	pkceVerifier := randomPKCEVerifier()

	resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
		"response_type": {"code"}, "client_id": {clientID},
		"redirect_uri": {clientRedirect}, "state": {"s"},
		"code_challenge": {pkceChallenge(pkceVerifier)}, "code_challenge_method": {"S256"},
	}.Encode(), "", "")
	require.Equal(t, http.StatusFound, resp.Code)

	location, err := url.Parse(resp.Header().Get("Location"))
	require.NoError(t, err)

	resp = doRequest(t, ts, http.MethodGet, "/oauth/callback?"+url.Values{
		"code": {fakeAuthCode}, "state": {location.Query().Get("state")},
	}.Encode(), "", "")
	require.Equal(t, http.StatusFound, resp.Code)

	location, err = url.Parse(resp.Header().Get("Location"))
	require.NoError(t, err)

	proxyCode := location.Query().Get("code")

	tests := []struct {
		name string
		form url.Values
	}{
		{
			name: "wrong pkce verifier",
			form: url.Values{
				"grant_type": {"authorization_code"}, "code": {proxyCode},
				"redirect_uri": {clientRedirect}, "client_id": {clientID},
				"code_verifier": {randomPKCEVerifier()},
			},
		},
		{
			name: "wrong redirect uri",
			form: url.Values{
				"grant_type": {"authorization_code"}, "code": {proxyCode},
				"redirect_uri": {"http://localhost:1/callback"}, "client_id": {clientID},
				"code_verifier": {pkceVerifier},
			},
		},
		{
			name: "wrong client",
			form: url.Values{
				"grant_type": {"authorization_code"}, "code": {proxyCode},
				"redirect_uri": {clientRedirect}, "client_id": {"someone-else"},
				"code_verifier": {pkceVerifier},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, ts, http.MethodPost, "/token", tt.form.Encode(), "application/x-www-form-urlencoded")
			require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
			assert.Contains(t, resp.Body.String(), "invalid_grant")
		})
	}
}

func TestOAuthProxyRedirectURIMergesQuery(t *testing.T) {
	ts, _, _ := newProxyTestServer(t)

	// A registered redirect URI may itself carry a query component; the
	// proxy must merge its parameters into it, not append a second "?".
	clientRedirect := "http://localhost:53742/callback?foo=bar"
	clientID := registerProxyClient(t, ts, clientRedirect)

	resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
		"response_type": {"code"}, "client_id": {clientID},
		"redirect_uri": {clientRedirect}, "state": {"client-state"},
		"code_challenge": {pkceChallenge(randomPKCEVerifier())}, "code_challenge_method": {"S256"},
	}.Encode(), "", "")
	require.Equal(t, http.StatusFound, resp.Code, resp.Body.String())

	location, err := url.Parse(resp.Header().Get("Location"))
	require.NoError(t, err)

	resp = doRequest(t, ts, http.MethodGet, "/oauth/callback?"+url.Values{
		"code": {fakeAuthCode}, "state": {location.Query().Get("state")},
	}.Encode(), "", "")
	require.Equal(t, http.StatusFound, resp.Code, resp.Body.String())

	location, err = url.Parse(resp.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:53742/callback", location.Scheme+"://"+location.Host+location.Path)
	assert.Equal(t, "bar", location.Query().Get("foo"))
	assert.NotEmpty(t, location.Query().Get("code"))
	assert.Equal(t, "client-state", location.Query().Get("state"))
}

func TestOAuthProxyCallbackErrors(t *testing.T) {
	ts, _, _ := newProxyTestServer(t)

	t.Run("exchange failure surfaces the issuer error", func(t *testing.T) {
		clientRedirect := "http://localhost:53742/callback"
		clientID := registerProxyClient(t, ts, clientRedirect)

		resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {clientID},
			"redirect_uri": {clientRedirect}, "state": {"s"},
			"code_challenge": {pkceChallenge(randomPKCEVerifier())}, "code_challenge_method": {"S256"},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code)

		location, err := url.Parse(resp.Header().Get("Location"))
		require.NoError(t, err)

		// The fake upstream rejects any code but fakeAuthCode with
		// invalid_grant; the proxy must surface that reason.
		resp = doRequest(t, ts, http.MethodGet, "/oauth/callback?"+url.Values{
			"code": {"rejected-code"}, "state": {location.Query().Get("state")},
		}.Encode(), "", "")
		require.Equal(t, http.StatusBadGateway, resp.Code, resp.Body.String())
		assert.Contains(t, resp.Body.String(), "invalid_grant")
	})

	t.Run("unknown state", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/oauth/callback?code=x&state=nope", "", "")
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("upstream error is forwarded to the client", func(t *testing.T) {
		clientRedirect := "http://localhost:53742/callback"
		clientID := registerProxyClient(t, ts, clientRedirect)

		resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {clientID},
			"redirect_uri": {clientRedirect}, "state": {"client-state"},
			"code_challenge": {pkceChallenge(randomPKCEVerifier())}, "code_challenge_method": {"S256"},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code)

		location, err := url.Parse(resp.Header().Get("Location"))
		require.NoError(t, err)

		resp = doRequest(t, ts, http.MethodGet, "/oauth/callback?"+url.Values{
			"error": {"access_denied"}, "state": {location.Query().Get("state")},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code)

		location, err = url.Parse(resp.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:53742/callback", location.Scheme+"://"+location.Host+location.Path)
		assert.Equal(t, "access_denied", location.Query().Get("error"))
		assert.Equal(t, "client-state", location.Query().Get("state"))
	})
}

func TestOAuthProxyMetadata(t *testing.T) {
	ts, _, _ := newProxyTestServer(t)

	t.Run("authorization server metadata", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/.well-known/oauth-authorization-server", "", "")
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

		var doc authorizationServerMetadataDocument
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &doc))
		assert.Equal(t, ts.URL, doc.Issuer)
		assert.Equal(t, ts.URL+"/authorize", doc.AuthorizationEndpoint)
		assert.Equal(t, ts.URL+"/token", doc.TokenEndpoint)
		assert.Equal(t, ts.URL+"/register", doc.RegistrationEndpoint)
		assert.Contains(t, doc.CodeChallengeMethodsSupported, "S256")
	})

	t.Run("protected resource metadata advertises the proxy", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/.well-known/oauth-protected-resource", "", "")
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

		var doc protectedResourceMetadataDocument
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &doc))
		assert.Equal(t, []string{ts.URL}, doc.AuthorizationServers)
	})
}

func TestOAuthProxyScopes(t *testing.T) {
	up := newFakeUpstream(t)

	ctx := coreoidc.ClientContext(context.Background(), up.server.Client())
	verifier, err := oidc.New(ctx, oidc.Config{Issuer: up.issuer, ClientID: up.clientID})
	require.NoError(t, err)

	proxy, err := newOAuthProxy(verifier, proxyConfig{
		clientID:    up.clientID,
		callbackURL: up.issuer + "/oauth/callback",
		scopes:      []string{"openid", "email"},
		log:         slog.Default(),
		httpClient:  up.server.Client(),
	})
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	handler := newOIDCHandler(http.NotFoundHandler(), oidcHandlerConfig{
		verifier: verifier,
		issuer:   up.issuer,
		proxy:    proxy,
		scopes:   []string{"openid", "email"},
		log:      slog.Default(),
	})

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	clientRedirect := "http://localhost:53742/callback"
	clientID := registerProxyClient(t, ts, clientRedirect)

	t.Run("authorize requests the configured scopes", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {clientID},
			"redirect_uri": {clientRedirect}, "state": {"s"},
			"code_challenge": {pkceChallenge(randomPKCEVerifier())}, "code_challenge_method": {"S256"},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code, resp.Body.String())

		location, err := url.Parse(resp.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "openid email", location.Query().Get("scope"))
	})

	t.Run("metadata advertises the configured scopes", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/.well-known/oauth-authorization-server", "", "")
		require.Equal(t, http.StatusOK, resp.Code)

		var doc authorizationServerMetadataDocument
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &doc))
		assert.Equal(t, []string{"openid", "email"}, doc.ScopesSupported)

		resp = doRequest(t, ts, http.MethodGet, "/.well-known/oauth-protected-resource", "", "")
		require.Equal(t, http.StatusOK, resp.Code)

		var prDoc protectedResourceMetadataDocument
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &prDoc))
		assert.Equal(t, []string{"openid", "email"}, prDoc.ScopesSupported)
	})

	t.Run("token response echoes the configured scopes", func(t *testing.T) {
		// Run a full flow to redeem a code.
		pkceVerifier := randomPKCEVerifier()

		resp := doRequest(t, ts, http.MethodGet, "/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {clientID},
			"redirect_uri": {clientRedirect}, "state": {"s"},
			"code_challenge": {pkceChallenge(pkceVerifier)}, "code_challenge_method": {"S256"},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code)

		location, err := url.Parse(resp.Header().Get("Location"))
		require.NoError(t, err)

		resp = doRequest(t, ts, http.MethodGet, "/oauth/callback?"+url.Values{
			"code": {fakeAuthCode}, "state": {location.Query().Get("state")},
		}.Encode(), "", "")
		require.Equal(t, http.StatusFound, resp.Code)

		location, err = url.Parse(resp.Header().Get("Location"))
		require.NoError(t, err)

		form := url.Values{
			"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")},
			"redirect_uri": {clientRedirect}, "client_id": {clientID},
			"code_verifier": {pkceVerifier},
		}
		resp = doRequest(t, ts, http.MethodPost, "/token", form.Encode(), "application/x-www-form-urlencoded")
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

		var tok struct {
			Scope string `json:"scope"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &tok))
		assert.Equal(t, "openid email", tok.Scope)
	})
}

func TestNewOAuthProxyValidation(t *testing.T) {
	verifier := &oidc.Verifier{} // endpoints are not used by validation

	tests := []struct {
		name        string
		callbackURL string
		clientID    string
		scopes      []string
	}{
		{name: "missing client id", callbackURL: "https://mcp.example.com/oauth/callback", clientID: ""},
		{name: "not absolute", callbackURL: "/oauth/callback", clientID: "client"},
		{name: "plain http on non-loopback host", callbackURL: "http://mcp.example.com/oauth/callback", clientID: "client"},
		{name: "scopes without openid", callbackURL: "https://mcp.example.com/oauth/callback", clientID: "client", scopes: []string{"profile"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOAuthProxy(verifier, proxyConfig{
				clientID:    tt.clientID,
				callbackURL: tt.callbackURL,
				scopes:      tt.scopes,
			})
			require.Error(t, err)
		})
	}

	t.Run("http on loopback host is allowed", func(t *testing.T) {
		_, err := newOAuthProxy(verifier, proxyConfig{
			clientID:    "client",
			callbackURL: "http://127.0.0.1:8080/oauth/callback",
		})
		require.NoError(t, err)
	})
}

func TestPKCEHelpers(t *testing.T) {
	verifier := randomPKCEVerifier()
	assert.Len(t, verifier, 43, "RFC 7636 minimum verifier length")

	challenge := pkceChallenge(verifier)
	assert.True(t, pkceMatches(challenge, verifier, "S256"))
	assert.False(t, pkceMatches(challenge, verifier+"x", "S256"))
	assert.True(t, pkceMatches(verifier, verifier, "plain"))

	// base64url without padding
	assert.NotContains(t, challenge, "=")
	_, err := base64.RawURLEncoding.DecodeString(challenge)
	require.NoError(t, err)
}
