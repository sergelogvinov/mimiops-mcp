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

// Package config defines the configuration structure for the mimiops-mcp server.
package config

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Config holds the configuration for the mimiops-mcp server.
type Config struct {
	ConfigFlags      *genericclioptions.ConfigFlags
	Port             int
	Extensions       string
	AllowDestructive bool
	LogLevel         string
	LogFormat        string

	// OIDCIssuer is the OIDC issuer URL. A non-empty value enables OIDC
	// authentication with passthrough to the Kubernetes API server.
	OIDCIssuer string
	// OIDCClientID must be present in the token's "aud" claim when set.
	OIDCClientID string
	// OIDCEmailDomains restricts access to verified emails under these
	// domains. Empty means all domains are allowed.
	OIDCEmailDomains []string
	// OIDCScope is the scope list requested from the issuer in proxy mode
	// and advertised to MCP clients. Defaults to openid, profile, email.
	OIDCScope []string
	// OIDCCallbackURL is the fixed redirect URI registered with the upstream
	// provider. A non-empty value enables the OAuth proxy mode (the server
	// terminates the authorization-code flow itself).
	OIDCCallbackURL string
	// OIDCClientSecret is the upstream client secret used for the
	// authorization-code exchange in proxy mode. Optional (public clients).
	OIDCClientSecret string
}

// ParseScopes normalizes a comma- or space-separated scope list: entries
// are trimmed, empty entries dropped, and duplicates removed, preserving
// order. Returns nil for an empty input.
func ParseScopes(value string) []string {
	fields := strings.Fields(strings.ReplaceAll(value, ",", " "))
	if len(fields) == 0 {
		return nil
	}

	scopes := make([]string, 0, len(fields))
	for _, scope := range fields {
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}

	return scopes
}

// ParseEmailDomains normalizes a comma-separated list of email domains:
// entries are trimmed, lowercased, and empty entries dropped. It returns an
// error when the input is non-empty but yields no domains.
func ParseEmailDomains(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	domains := make([]string, 0, 4)
	for part := range strings.SplitSeq(value, ",") {
		domain := strings.ToLower(strings.TrimSpace(part))
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	if len(domains) == 0 {
		return nil, fmt.Errorf("invalid email domains %q: expected at least one non-empty domain", value)
	}

	return domains, nil
}
