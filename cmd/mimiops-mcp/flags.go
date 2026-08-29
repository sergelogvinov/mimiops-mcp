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
	"os"
	"strconv"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	flagExtensions       = "extensions"
	flagAllowDestructive = "allow-destructive"
	flagLogLevel         = "log-level"
	flagLogFormat        = "log-format"
	flagPort             = "port"
	flagOIDCIssuer       = "oidc-issuer"
	flagOIDCClientID     = "oidc-client-id"
	flagOIDCClientSecret = "oidc-client-secret"
	flagOIDCScope        = "oidc-scope"
	flagOIDCEmailDomains = "oidc-email-domains"
	flagOIDCCallbackURL  = "oidc-callback-url"

	envExtensions       = "EXTENSIONS"
	envAllowDestructive = "ALLOW_DESTRUCTIVE"
	envLogLevel         = "LOG_LEVEL"
	envLogFormat        = "LOG_FORMAT"
	envPort             = "PORT"
	envOIDCIssuer       = "OIDC_ISSUER"
	envOIDCClientID     = "OIDC_CLIENT_ID"
	envOIDCClientSecret = "OIDC_CLIENT_SECRET"
	envOIDCScope        = "OIDC_SCOPE"
	envOIDCEmailDomains = "OIDC_EMAIL_DOMAINS"
	envOIDCCallbackURL  = "OIDC_CALLBACK_URL"

	envKubeconfig  = "KUBECONFIG"
	envContext     = "CONTEXT"
	envNamespace   = "NAMESPACE"
	envImpersonate = "AS"
)

const (
	defaultExtensions       = "helm"
	defaultAllowDestructive = false
	defaultLogLevel         = "info"
	defaultLogFormat        = "text"
	defaultPort             = 8080
	defaultOutputFormat     = "text"

	defaultOIDCIssuer       = ""
	defaultOIDCClientID     = ""
	defaultOIDCClientSecret = ""
	defaultOIDCScope        = "openid profile email"
	defaultOIDCEmailDomains = ""
	defaultOIDCCallbackURL  = ""
)

// Flags wraps genericclioptions.ConfigFlags and adds application-specific flags.
type Flags struct {
	configFlags *genericclioptions.ConfigFlags

	Extensions       string
	AllowDestructive bool
	LogLevel         string
	LogFormat        string
	Port             int
	Output           string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCScope        string
	OIDCEmailDomains string
	OIDCCallbackURL  string
}

// DefaultFlags returns the default flags for the command,
// populated from environment variables where applicable.
func DefaultFlags() *Flags {
	configFlags := genericclioptions.NewConfigFlags(true)

	// Set defaults from environment variables
	kubeconfig := withDefaultEnv(envKubeconfig, "")
	context := withDefaultEnv(envContext, "")
	namespace := withDefaultEnv(envNamespace, "")
	impersonate := withDefaultEnv(envImpersonate, "")

	configFlags.KubeConfig = &kubeconfig
	configFlags.Context = &context
	configFlags.Namespace = &namespace
	configFlags.Impersonate = &impersonate

	return &Flags{
		configFlags:      configFlags,
		Extensions:       withDefaultEnv(envExtensions, defaultExtensions),
		AllowDestructive: withDefaultEnvBool(envAllowDestructive, defaultAllowDestructive),
		LogLevel:         withDefaultEnv(envLogLevel, defaultLogLevel),
		LogFormat:        withDefaultEnv(envLogFormat, defaultLogFormat),
		Port:             withDefaultEnvInt(envPort, defaultPort),
		Output:           defaultOutputFormat,

		OIDCIssuer:       withDefaultEnv(envOIDCIssuer, defaultOIDCIssuer),
		OIDCClientID:     withDefaultEnv(envOIDCClientID, defaultOIDCClientID),
		OIDCClientSecret: withDefaultEnv(envOIDCClientSecret, defaultOIDCClientSecret),
		OIDCScope:        withDefaultEnv(envOIDCScope, defaultOIDCScope),
		OIDCEmailDomains: withDefaultEnv(envOIDCEmailDomains, defaultOIDCEmailDomains),
		OIDCCallbackURL:  withDefaultEnv(envOIDCCallbackURL, defaultOIDCCallbackURL),
	}
}

// AddPersistentFlags adds the global flags shared by every subcommand.
func (f *Flags) AddPersistentFlags(flags *pflag.FlagSet) {
	// Add Kubernetes flags from genericclioptions
	f.configFlags.AddFlags(flags)

	// Add application-specific flags
	flags.StringVarP(&f.Extensions, flagExtensions, "", f.Extensions, "comma-separated list of extensions to enable, or 'all' (default: all)")
	flags.BoolVarP(&f.AllowDestructive, flagAllowDestructive, "", f.AllowDestructive, "allow destructive operations (default: false)")
	flags.StringVarP(&f.LogLevel, flagLogLevel, "", f.LogLevel, "log level: debug, info, warn, error (default: info)")
	flags.StringVarP(&f.LogFormat, flagLogFormat, "", f.LogFormat, "log output format: text, json (default: text)")
	flags.StringVarP(&f.OIDCIssuer, flagOIDCIssuer, "", f.OIDCIssuer, "OIDC issuer URL; enables OIDC authentication when set (env: OIDC_ISSUER)")
	flags.StringVarP(&f.OIDCClientID, flagOIDCClientID, "", f.OIDCClientID, "client ID that must be present in the token's aud claim (env: OIDC_CLIENT_ID)")
	flags.StringVarP(&f.OIDCEmailDomains, flagOIDCEmailDomains, "", f.OIDCEmailDomains, "comma-separated list of allowed email domains, empty allows all (env: OIDC_EMAIL_DOMAINS)")
	flags.StringVarP(&f.OIDCCallbackURL, flagOIDCCallbackURL, "", f.OIDCCallbackURL, "fixed OAuth callback URL registered with the issuer; enables the OAuth proxy flow when set (env: OIDC_CALLBACK_URL)")
	flags.StringVarP(&f.OIDCClientSecret, flagOIDCClientSecret, "", f.OIDCClientSecret, "issuer client secret for the authorization-code exchange in proxy mode (env: OIDC_CLIENT_SECRET)")
	flags.StringVarP(&f.OIDCScope, flagOIDCScope, "", f.OIDCScope, "space-separated OAuth scopes requested from the issuer; must include openid (default: openid profile email) (env: OIDC_SCOPE)")
}

// AddServerFlags adds the flags for the "server" subcommand.
func (f *Flags) AddServerFlags(flags *pflag.FlagSet) {
	flags.IntVarP(&f.Port, flagPort, "", f.Port, "http/sse listen port (default: 8080)")
}

// AddToolFlags adds the flags for the "tool" subcommand.
func (f *Flags) AddToolFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&f.Output, "output", "o", defaultOutputFormat, "output format: text, json, yaml")
}

// Config returns the internal config populated from the parsed flags.
func (f *Flags) Config() (*config.Config, error) {
	emailDomains, err := config.ParseEmailDomains(f.OIDCEmailDomains)
	if err != nil {
		return nil, err
	}

	scopes := config.ParseScopes(f.OIDCScope)
	if len(scopes) == 0 {
		scopes = config.ParseScopes(defaultOIDCScope)
	}

	return &config.Config{
		ConfigFlags:      f.configFlags,
		Port:             f.Port,
		Extensions:       f.Extensions,
		AllowDestructive: f.AllowDestructive,
		LogLevel:         f.LogLevel,
		LogFormat:        f.LogFormat,

		OIDCIssuer:       f.OIDCIssuer,
		OIDCClientID:     f.OIDCClientID,
		OIDCClientSecret: f.OIDCClientSecret,
		OIDCScope:        scopes,
		OIDCEmailDomains: emailDomains,

		OIDCCallbackURL: f.OIDCCallbackURL,
	}, nil
}

// ToRawKubeConfigLoader returns the underlying ConfigFlags for k8s client creation.
func (f *Flags) ToRawKubeConfigLoader() *genericclioptions.ConfigFlags {
	return f.configFlags
}

func withDefaultEnv(key string, def string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return def
}

func withDefaultEnvInt(key string, def int) int {
	if val, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return def
}

func withDefaultEnvBool(key string, def bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		// Parse common truthy/falsy values
		switch val {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return def
}
