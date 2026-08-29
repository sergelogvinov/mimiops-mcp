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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty", value: "", want: nil},
		{name: "space separated", value: "openid profile email", want: []string{"openid", "profile", "email"}},
		{name: "comma separated", value: "openid,email", want: []string{"openid", "email"}},
		{name: "mixed separators and extra spaces", value: " openid ,  email  profile ", want: []string{"openid", "email", "profile"}},
		{name: "deduplicates", value: "openid openid email", want: []string{"openid", "email"}}, //nolint:dupword
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseScopes(tt.value))
		})
	}
}

func TestParseEmailDomains(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{name: "empty", value: "", want: nil},
		{name: "whitespace only", value: "   ", want: nil},
		{name: "single domain", value: "example.com", want: []string{"example.com"}},
		{name: "multiple domains", value: "example.com,example.org", want: []string{"example.com", "example.org"}},
		{name: "trims and lowercases", value: " Example.COM ,  example.org ", want: []string{"example.com", "example.org"}},
		{name: "drops empty entries", value: "example.com,,example.org,", want: []string{"example.com", "example.org"}},
		{name: "only commas", value: " , , ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEmailDomains(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
