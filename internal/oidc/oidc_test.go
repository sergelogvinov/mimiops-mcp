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

package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testProvider struct {
	server   *httptest.Server
	issuer   string
	key      *rsa.PrivateKey
	signer   jose.Signer
	verifier *Verifier
}

func newTestProvider(t *testing.T, clientID string, emailDomains []string) *testProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
	require.NoError(t, err)

	tp := &testProvider{key: key, signer: signer}

	jwk := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}

	var discovery, jwks []byte

	writeJSON := func(w http.ResponseWriter, payload []byte) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, discovery)
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jwks)
	})

	tp.server = httptest.NewTLSServer(mux)
	t.Cleanup(tp.server.Close)

	tp.issuer = tp.server.URL

	discovery, err = json.Marshal(map[string]any{
		"issuer":                                tp.issuer,
		"jwks_uri":                              tp.issuer + "/keys",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
	require.NoError(t, err)

	jwks, err = json.Marshal(map[string]any{"keys": []any{jwk}})
	require.NoError(t, err)

	// Route go-oidc's discovery/JWKS requests through the test server's
	// TLS client so its self-signed certificate is trusted.
	ctx := coreoidc.ClientContext(context.Background(), tp.server.Client())

	tp.verifier, err = New(ctx, Config{Issuer: tp.issuer, ClientID: clientID, EmailDomains: emailDomains})
	require.NoError(t, err)

	return tp
}

type tokenClaims struct {
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Audience      string `json:"aud"`
	Expiry        int64  `json:"exp"`
	IssuedAt      int64  `json:"iat"`
	Email         string `json:"email,omitempty"`
	EmailVerified *bool  `json:"email_verified,omitempty"`
}

func (tp *testProvider) signToken(t *testing.T, claims tokenClaims) string {
	t.Helper()

	if claims.Issuer == "" {
		claims.Issuer = tp.issuer
	}

	now := time.Now()
	if claims.IssuedAt == 0 {
		claims.IssuedAt = now.Unix()
	}
	if claims.Expiry == 0 {
		claims.Expiry = now.Add(time.Hour).Unix()
	}

	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	object, err := tp.signer.Sign(payload)
	require.NoError(t, err)

	signed, err := object.CompactSerialize()
	require.NoError(t, err)

	return signed
}

func TestNewRejectsInvalidIssuers(t *testing.T) {
	tests := []struct {
		name   string
		issuer string
	}{
		{name: "empty", issuer: ""},
		{name: "http scheme", issuer: "http://accounts.example.com"},
		{name: "not a URL", issuer: "not-a-url"},
		{name: "missing host", issuer: "https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(context.Background(), Config{Issuer: tt.issuer})
			require.Error(t, err)
		})
	}
}

func TestVerify(t *testing.T) {
	verified := true

	tests := []struct {
		name         string
		clientID     string
		emailDomains []string
		claims       func() tokenClaims
		wantErr      string
	}{
		{
			name: "valid token",
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1", Audience: "mcp-server"}
			},
		},
		{
			name:     "wrong audience",
			clientID: "mcp-server",
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1", Audience: "other-client"}
			},
			wantErr: "expected audience",
		},
		{
			name:     "matching audience",
			clientID: "mcp-server",
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1", Audience: "mcp-server"}
			},
		},
		{
			name: "expired token",
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1", Expiry: time.Now().Add(-time.Hour).Unix()}
			},
			wantErr: "token is expired",
		},
		{
			name: "wrong issuer",
			claims: func() tokenClaims {
				return tokenClaims{Issuer: "https://evil.example.com", Subject: "user-1"}
			},
			wantErr: "issued by a different provider",
		},
		{
			name:         "email domain allowed",
			emailDomains: []string{"example.com"},
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1", Email: "user@Example.com", EmailVerified: &verified}
			},
		},
		{
			name:         "email domain denied",
			emailDomains: []string{"example.com"},
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1", Email: "user@evil.example", EmailVerified: &verified}
			},
			wantErr: "not allowed",
		},
		{
			name:         "email missing",
			emailDomains: []string{"example.com"},
			claims: func() tokenClaims {
				return tokenClaims{Subject: "user-1"}
			},
			wantErr: "email claim is required",
		},
		{
			name:         "email not verified",
			emailDomains: []string{"example.com"},
			claims: func() tokenClaims {
				unverified := false
				return tokenClaims{Subject: "user-1", Email: "user@example.com", EmailVerified: &unverified}
			},
			wantErr: "not verified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp := newTestProvider(t, tt.clientID, tt.emailDomains)

			claims, err := tp.verifier.Verify(context.Background(), tp.signToken(t, tt.claims()))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "user-1", claims.Subject)
			assert.Equal(t, tp.issuer, claims.Issuer)
		})
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	tp := newTestProvider(t, "", nil)

	_, err := tp.verifier.Verify(context.Background(), "not-a-jwt")
	require.Error(t, err)
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{name: "missing", header: http.Header{}, want: ""},
		{name: "bearer", header: http.Header{"Authorization": []string{"Bearer abc"}}, want: "abc"},
		{name: "case-insensitive scheme", header: http.Header{"Authorization": []string{"bearer abc"}}, want: "abc"},
		{name: "other scheme", header: http.Header{"Authorization": []string{"Basic abc"}}, want: ""},
		{name: "no token", header: http.Header{"Authorization": []string{"Bearer"}}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BearerToken(tt.header))
		})
	}
}

func TestAuthContext(t *testing.T) {
	ctx := context.Background()

	_, ok := FromContext(ctx)
	assert.False(t, ok)

	auth := &Auth{Token: "tok", Claims: &Claims{Subject: "user-1"}}
	ctx = Inject(ctx, auth)

	got, ok := FromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, auth, got)
}
