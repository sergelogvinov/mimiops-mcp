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
	"log/slog"
	"net/http"

	"github.com/sergelogvinov/mimiops-mcp/internal/oidc"
)

// tokenVerifier verifies OIDC bearer tokens (implemented by *oidc.Verifier;
// an interface so tests can stub it).
type tokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*oidc.Claims, error)
}

// oidcHandlerConfig configures newOIDCHandler.
type oidcHandlerConfig struct {
	// verifier validates Bearer tokens on /mcp.
	verifier tokenVerifier
	// issuer is the upstream OIDC issuer, advertised as the authorization
	// server in passthrough mode.
	issuer string
	// proxy, when non-nil, enables the OAuth proxy flow: the server
	// advertises itself as the authorization server.
	proxy *oauthProxy
	// scopes are the OAuth scopes advertised to and requested from clients.
	scopes []string
	log    *slog.Logger
}

// newOIDCHandler builds the HTTP routing for OIDC-enabled mode: the MCP
// endpoint behind Bearer-token verification, plus the RFC 9728 Protected
// Resource Metadata endpoints so MCP clients (VS Code, ...) can discover
// how to authenticate. In proxy mode the RFC 8416 Authorization Server
// Metadata and the OAuth endpoints (register/authorize/callback/token) are
// served as well.
//
// Clients probe the well-known paths in several styles for an MCP endpoint
// like https://host/mcp — all of them are served:
//
//	/.well-known/oauth-protected-resource            (origin, RFC 9728 §3.1)
//	/.well-known/oauth-protected-resource/mcp        (path-inserted, RFC 9728 §3.1)
//	/mcp/.well-known/oauth-protected-resource        (appended to the endpoint URL)
func newOIDCHandler(next http.Handler, cfg oidcHandlerConfig) http.Handler {
	var authorizationServers func(*http.Request) []string
	if cfg.proxy != nil {
		authorizationServers = func(r *http.Request) []string {
			return []string{originURL(r)}
		}
	} else {
		authorizationServers = func(_ *http.Request) []string {
			return []string{cfg.issuer}
		}
	}

	metadata := &protectedResourceMetadata{
		authorizationServers: authorizationServers,
		scopes:               cfg.scopes,
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", withOIDCAuth(cfg.verifier, cfg.log, next))
	mux.Handle(wellKnownProtectedResource, metadata)
	mux.Handle(wellKnownProtectedResource+"/", metadata)
	mux.Handle("/mcp"+wellKnownProtectedResource, metadata)

	if cfg.proxy != nil {
		mux.HandleFunc("/authorize", cfg.proxy.handleAuthorize)
		mux.HandleFunc("/token", cfg.proxy.handleToken)
		mux.HandleFunc("/register", cfg.proxy.handleRegister)
		mux.Handle(cfg.proxy.callbackPath, http.HandlerFunc(cfg.proxy.handleCallback))

		asMetadata := http.HandlerFunc(cfg.proxy.handleAuthorizationServerMetadata)
		mux.Handle(wellKnownAuthorizationServer, asMetadata)
		mux.Handle(wellKnownAuthorizationServer+"/", asMetadata)
		mux.Handle("/mcp"+wellKnownAuthorizationServer, asMetadata)
	}

	return mux
}

// withOIDCAuth wraps an http.Handler with Bearer-token verification. Invalid
// or missing tokens get a plain 401 Unauthorized; the token material and
// claims never appear in the response or the logs.
func withOIDCAuth(verifier tokenVerifier, log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := oidc.BearerToken(r.Header)
		if token == "" {
			unauthorized(w, r)
			return
		}

		claims, err := verifier.Verify(r.Context(), token)
		if err != nil {
			// Debug level only, and never with the token material.
			log.Debug("oidc: rejected token", "reason", err.Error())
			unauthorized(w, r)
			return
		}

		next.ServeHTTP(w, r.WithContext(oidc.Inject(r.Context(), &oidc.Auth{
			Token:  token,
			Claims: claims,
		})))
	})
}
