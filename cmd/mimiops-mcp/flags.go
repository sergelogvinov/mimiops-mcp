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
	flagAllowDestructive = "allow-destructive"
	flagLogLevel         = "log-level"
	flagPort             = "port"

	envKubeconfig = "KUBECONFIG"
	envContext    = "CONTEXT"
	envNamespace  = "NAMESPACE"
	envLogLevel   = "LOG_LEVEL"
	envPort       = "PORT"
)

const defaultLogLevel = "info"
const defaultPort = 8080

// Flags represents the command-line flags for the mimiops-mcp server.
type Flags struct {
	Kubeconfig       string
	Context          string
	Namespace        string
	AllowDestructive bool
	LogLevel         string
	Port             int
}

// DefaultFlags returns the default flags for the command,
// populated from environment variables where applicable.
func DefaultFlags() *Flags {
	return &Flags{
		Kubeconfig: withDefaultEnv(envKubeconfig, ""),
		Context:    withDefaultEnv(envContext, ""),
		Namespace:  withDefaultEnv(envNamespace, ""),
		LogLevel:   withDefaultEnv(envLogLevel, defaultLogLevel),
		Port:       withDefaultEnvInt(envPort, defaultPort),
	}
}

// AddPersistentFlags adds the global flags shared by every subcommand.
func (f *Flags) AddPersistentFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&f.Kubeconfig, flagKubeconfig, "", f.Kubeconfig, "path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	flags.StringVarP(&f.Context, flagContext, "", f.Context, "kubernetes context to use (default: current-context)")
	flags.StringVarP(&f.Namespace, flagNamespace, "n", f.Namespace, "kubernetes namespace scope (default: all namespaces)")
	flags.BoolVarP(&f.AllowDestructive, flagAllowDestructive, "", f.AllowDestructive, "allow destructive operations (default: false)")
	flags.StringVarP(&f.LogLevel, flagLogLevel, "", f.LogLevel, "log level: debug, info, warn, error (default: info)")
}

// AddPort adds the local --port flag (server subcommand only).
func (f *Flags) AddServerFlags(flags *pflag.FlagSet) {
	flags.IntVarP(&f.Port, flagPort, "", f.Port, "http/sse listen port (default: 8080)")
}

// Config returns the internal config populated from the parsed flags.
func (f *Flags) Config() *config.Config {
	return &config.Config{
		Kubeconfig:       f.Kubeconfig,
		Context:          f.Context,
		Namespace:        f.Namespace,
		Port:             f.Port,
		AllowDestructive: f.AllowDestructive,
		LogLevel:         f.LogLevel,
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
