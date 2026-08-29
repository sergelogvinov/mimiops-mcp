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
	"net/http"
	"strings"
)

type authContextKey struct{}

// Auth carries the verified raw token and its claims for a single request.
// The token is forwarded verbatim to the Kubernetes API server.
type Auth struct {
	// Token is the raw JWT as presented by the client.
	Token string
	// Claims are the locally verified token claims.
	Claims *Claims
}

// Inject adds the verified auth to the request context.
func Inject(ctx context.Context, auth *Auth) context.Context {
	return context.WithValue(ctx, authContextKey{}, auth)
}

// FromContext retrieves the verified auth from the request context, if any.
func FromContext(ctx context.Context) (*Auth, bool) {
	auth, ok := ctx.Value(authContextKey{}).(*Auth)
	return auth, ok
}

// BearerToken extracts the bearer token from an Authorization header.
func BearerToken(header http.Header) string {
	value := header.Get("Authorization")
	if value == "" {
		return ""
	}

	parts := strings.SplitN(value, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}
