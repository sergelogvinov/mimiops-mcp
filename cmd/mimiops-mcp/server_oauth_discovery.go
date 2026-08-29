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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// wellKnownProtectedResource is the RFC 9728 well-known path under which
// OAuth 2.0 Protected Resource Metadata is served.
const wellKnownProtectedResource = "/.well-known/oauth-protected-resource"

// protectedResourceMetadata serves RFC 9728 OAuth 2.0 Protected Resource
// Metadata so MCP clients (VS Code, Claude, ...) can discover the
// authorization server to log in against.
//
// In passthrough mode the advertised authorization server is the configured
// OIDC issuer. In proxy mode (see oauth_proxy.go) it is this server itself,
// derived per-request so the URL matches how the client reached it.
type protectedResourceMetadata struct {
	authorizationServers func(*http.Request) []string
	// scopes are advertised in scopes_supported.
	scopes []string
}

// protectedResourceMetadataDocument is the RFC 9728 metadata document.
type protectedResourceMetadataDocument struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	ResourceName           string   `json:"resource_name,omitempty"`
}

// ServeHTTP responds with the metadata document. The resource identifier is
// derived per-request so the advertised URL matches how the client reached
// the server (also behind TLS-terminating proxies).
func (m *protectedResourceMetadata) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet, http.MethodHead:
		// served below
	default:
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	doc := protectedResourceMetadataDocument{
		Resource:               resourceURL(r),
		AuthorizationServers:   m.authorizationServers(r),
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        m.scopes,
		ResourceName:           "MimiOPS MCP Server",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(doc); err != nil {
		http.Error(w, "failed to encode metadata", http.StatusInternalServerError)
	}
}

// unauthorized writes a plain 401 with an RFC 9728 WWW-Authenticate header
// pointing MCP clients at the protected resource metadata so they can start
// the authorization flow.
func unauthorized(w http.ResponseWriter, r *http.Request) {
	metadataURL := resourceURL(r) + wellKnownProtectedResource
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q", metadataURL))
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// resourceURL returns this server's external MCP endpoint URL, honoring the
// X-Forwarded-* headers set by TLS-terminating proxies.
func resourceURL(r *http.Request) string {
	return originURL(r) + "/mcp"
}

// originURL derives this server's external origin URL from the request,
// honoring the X-Forwarded-* headers set by TLS-terminating proxies.
func originURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	if proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	}

	host := r.Host
	if forwarded := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = forwarded
	}

	return scheme + "://" + host
}

// firstHeaderValue returns the first value of a comma-separated header.
func firstHeaderValue(value string) string {
	if value == "" {
		return ""
	}

	parts := strings.SplitN(value, ",", 2)

	return strings.TrimSpace(parts[0])
}
