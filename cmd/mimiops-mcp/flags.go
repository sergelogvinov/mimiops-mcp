package main

import (
	"os"
	"strconv"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/spf13/pflag"
)

const (
	flagKubeconfig       = "kubeconfig"
	flagContext          = "context"
	flagNamespace        = "namespace"
	flagImpersonate      = "impersonate"
	flagAllowDestructive = "allow-destructive"
	flagLogLevel         = "log-level"
	flagLogFormat        = "log-format"
	flagPort             = "port"

	envKubeconfig  = "KUBECONFIG"
	envContext     = "CONTEXT"
	envNamespace   = "NAMESPACE"
	envImpersonate = "IMPERSONATE"
	envLogLevel    = "LOG_LEVEL"
	envLogFormat   = "LOG_FORMAT"
	envPort        = "PORT"
)

const (
	defaultLogLevel  = "info"
	defaultLogFormat = "text"
	defaultPort      = 8080
)

// Flags represents the command-line flags for the mimiops-mcp server.
type Flags struct {
	Kubeconfig       string
	Context          string
	Namespace        string
	Impersonate      string
	AllowDestructive bool
	LogLevel         string
	LogFormat        string
	Port             int
}

// DefaultFlags returns the default flags for the command,
// populated from environment variables where applicable.
func DefaultFlags() *Flags {
	return &Flags{
		Kubeconfig:  withDefaultEnv(envKubeconfig, ""),
		Context:     withDefaultEnv(envContext, ""),
		Namespace:   withDefaultEnv(envNamespace, ""),
		Impersonate: withDefaultEnv(envImpersonate, ""),
		LogLevel:    withDefaultEnv(envLogLevel, defaultLogLevel),
		LogFormat:   withDefaultEnv(envLogFormat, defaultLogFormat),
		Port:        withDefaultEnvInt(envPort, defaultPort),
	}
}

// AddPersistentFlags adds the global flags shared by every subcommand.
func (f *Flags) AddPersistentFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&f.Kubeconfig, flagKubeconfig, "", f.Kubeconfig, "path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	flags.StringVarP(&f.Context, flagContext, "", f.Context, "kubernetes context to use (default: current-context)")
	flags.StringVarP(&f.Namespace, flagNamespace, "n", f.Namespace, "kubernetes namespace scope (default: all namespaces)")
	flags.StringVarP(&f.Impersonate, flagImpersonate, "", f.Impersonate, "username to impersonate for the operation (default: kubeconfig act-as)")
	flags.BoolVarP(&f.AllowDestructive, flagAllowDestructive, "", f.AllowDestructive, "allow destructive operations (default: false)")
	flags.StringVarP(&f.LogLevel, flagLogLevel, "", f.LogLevel, "log level: debug, info, warn, error (default: info)")
	flags.StringVarP(&f.LogFormat, flagLogFormat, "", f.LogFormat, "log output format: text, json (default: text)")
}

// AddServerFlags adds the flags for the "server" subcommand.
func (f *Flags) AddServerFlags(flags *pflag.FlagSet) {
	flags.IntVarP(&f.Port, flagPort, "", f.Port, "http/sse listen port (default: 8080)")
}

// Config returns the internal config populated from the parsed flags.
func (f *Flags) Config() *config.Config {
	return &config.Config{
		Kubeconfig:       f.Kubeconfig,
		Context:          f.Context,
		Namespace:        f.Namespace,
		Impersonate:      f.Impersonate,
		Port:             f.Port,
		AllowDestructive: f.AllowDestructive,
		LogLevel:         f.LogLevel,
		LogFormat:        f.LogFormat,
	}
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
