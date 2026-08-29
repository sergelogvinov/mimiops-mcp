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

// Package oidc implements verification of OIDC access tokens presented by MCP
// HTTP clients. Verified tokens are forwarded verbatim to the Kubernetes API
// server, which remains the authoritative verifier and RBAC enforcer.
package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strings"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
)

// Config configures the Verifier.
type Config struct {
	// Issuer is the OIDC issuer URL, e.g. "https://accounts.google.com".
	// A non-empty value enables OIDC authentication.
	Issuer string
	// ClientID that must be present in the token's "aud" claim. When empty,
	// the audience check is skipped (some providers omit "aud").
	ClientID string
	// EmailDomains restricts access to verified emails under these domains.
	// Empty means all domains are allowed.
	EmailDomains []string
}

// Claims are the verified token claims the server cares about.
type Claims struct {
	// Issuer is the verified "iss" claim.
	Issuer string
	// Subject is the verified "sub" claim.
	Subject string
	// Email is the "email" claim, if present.
	Email string
	// EmailVerified reports the "email_verified" claim.
	EmailVerified bool
	// Audience lists the verified "aud" claim values.
	Audience []string
}

// Verifier validates OIDC access tokens against a single issuer. Discovery
// and JWKS fetching are performed once at construction and cached by the
// underlying go-oidc provider (including re-fetch on unknown "kid").
type Verifier struct {
	verifier     *coreoidc.IDTokenVerifier
	clientID     string
	emailDomains []string
	authURL      string
	tokenURL     string
}

// New performs OIDC discovery against the issuer and returns a ready-to-use
// Verifier. It fails fast on an invalid issuer URL or unreachable discovery
// endpoint so configuration errors surface at startup.
func New(ctx context.Context, cfg Config) (*Verifier, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("oidc: issuer is required")
	}

	u, err := url.Parse(cfg.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("oidc: issuer must be a valid https:// URL, got %q", cfg.Issuer)
	}

	provider, err := coreoidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discovery failed for issuer %s: %w", cfg.Issuer, err)
	}

	endpoint := provider.Endpoint()

	return &Verifier{
		verifier: provider.Verifier(&coreoidc.Config{
			ClientID: cfg.ClientID,
			// Without a configured client ID there is nothing to check
			// "aud" against; the API server re-verifies the token anyway.
			SkipClientIDCheck: cfg.ClientID == "",
		}),
		clientID:     cfg.ClientID,
		emailDomains: cfg.EmailDomains,
		authURL:      endpoint.AuthURL,
		tokenURL:     endpoint.TokenURL,
	}, nil
}

// AuthURL returns the provider's authorization endpoint, as advertised by
// its discovery document.
func (v *Verifier) AuthURL() string {
	return v.authURL
}

// TokenURL returns the provider's token endpoint, as advertised by its
// discovery document.
func (v *Verifier) TokenURL() string {
	return v.tokenURL
}

// Verify validates the raw JWT: signature (via the provider's JWKS, restricted
// to the discovery-advertised algorithms), issuer, audience (when a client ID
// is configured), expiry and not-before. When email domains are configured,
// the token must carry a verified email under one of the allowed domains.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (*Claims, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: token verification failed: %w", err)
	}

	var payload struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&payload); err != nil {
		return nil, fmt.Errorf("oidc: failed to unmarshal claims: %w", err)
	}

	claims := &Claims{
		Issuer:        idToken.Issuer,
		Subject:       idToken.Subject,
		Email:         payload.Email,
		EmailVerified: payload.EmailVerified,
		Audience:      idToken.Audience,
	}

	if len(v.emailDomains) > 0 {
		if payload.Email == "" {
			return nil, errors.New("oidc: email claim is required but missing")
		}

		if !payload.EmailVerified {
			return nil, errors.New("oidc: email claim is not verified")
		}

		domain, err := emailDomain(payload.Email)
		if err != nil {
			return nil, fmt.Errorf("oidc: invalid email claim: %w", err)
		}

		if !slices.Contains(v.emailDomains, domain) {
			return nil, fmt.Errorf("oidc: email domain %q is not allowed", domain)
		}
	}

	return claims, nil
}

// emailDomain returns the lowercased domain part of an email address.
func emailDomain(email string) (string, error) {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", err
	}

	at := strings.LastIndex(addr.Address, "@")
	if at < 0 || at == len(addr.Address)-1 {
		return "", fmt.Errorf("missing domain in %q", email)
	}

	return strings.ToLower(addr.Address[at+1:]), nil
}
