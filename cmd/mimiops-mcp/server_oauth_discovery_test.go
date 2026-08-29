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
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sergelogvinov/mimiops-mcp/internal/oidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubVerifier lets routing tests exercise the 401 path without a real
// OIDC provider.
type stubVerifier struct{}

func (stubVerifier) Verify(_ context.Context, _ string) (*oidc.Claims, error) {
	return nil, assert.AnError
}

func TestProtectedResourceMetadata(t *testing.T) {
	issuer := "https://sso.example.com/realms/main"
	handler := &protectedResourceMetadata{
		authorizationServers: func(_ *http.Request) []string { return []string{issuer} },
	}

	tests := []struct {
		name         string
		headers      map[string]string
		host         string
		wantResource string
	}{
		{
			name:         "plain",
			host:         "mcp.example.com",
			wantResource: "http://mcp.example.com/mcp",
		},
		{
			name:         "forwarded proto and host",
			headers:      map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "public.example.com"},
			host:         "internal:8080",
			wantResource: "https://public.example.com/mcp",
		},
		{
			name:         "multiple forwarded values",
			headers:      map[string]string{"X-Forwarded-Proto": "https, http"},
			host:         "mcp.example.com",
			wantResource: "https://mcp.example.com/mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+tt.host+"/.well-known/oauth-protected-resource", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))

			var doc protectedResourceMetadataDocument
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
			assert.Equal(t, tt.wantResource, doc.Resource)
			assert.Equal(t, []string{"https://sso.example.com/realms/main"}, doc.AuthorizationServers)
			assert.Contains(t, doc.BearerMethodsSupported, "header")
		})
	}

	t.Run("options preflight", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/.well-known/oauth-protected-resource", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/.well-known/oauth-protected-resource", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestUnauthorizedIncludesDiscoveryHeader(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://mcp.example.com/mcp", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	unauthorized(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t,
		`Bearer resource_metadata="https://mcp.example.com/mcp/.well-known/oauth-protected-resource"`,
		rec.Header().Get("WWW-Authenticate"))
}

func TestOIDCHandlerDiscoveryPaths(t *testing.T) {
	// Clients probe the well-known path in several styles relative to the
	// MCP endpoint URL; every one of them must serve the metadata.
	paths := []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
		"/mcp/.well-known/oauth-protected-resource",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			var log slog.Logger

			handler := newOIDCHandler(http.NotFoundHandler(), oidcHandlerConfig{
				verifier: stubVerifier{},
				issuer:   "https://sso.example.com",
				log:      &log,
			})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://mcp.example.com"+path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var doc protectedResourceMetadataDocument
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
			assert.Equal(t, []string{"https://sso.example.com"}, doc.AuthorizationServers)
		})
	}
}

func TestOIDCHandlerRejectsUnauthenticatedMCPRequests(t *testing.T) {
	var log slog.Logger

	handler := newOIDCHandler(http.NotFoundHandler(), oidcHandlerConfig{
		verifier: stubVerifier{},
		issuer:   "https://sso.example.com",
		log:      &log,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://mcp.example.com/mcp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t,
		rec.Header().Get("WWW-Authenticate"),
		"/.well-known/oauth-protected-resource")
}
