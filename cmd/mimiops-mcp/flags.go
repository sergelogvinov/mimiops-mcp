package main

import (
	"os"
	"strconv"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	flagAllowDestructive = "allow-destructive"
	flagLogLevel         = "log-level"
	flagLogFormat        = "log-format"
	flagPort             = "port"

	envAllowDestructive = "ALLOW_DESTRUCTIVE"
	envLogLevel         = "LOG_LEVEL"
	envLogFormat        = "LOG_FORMAT"
	envPort             = "PORT"

	envKubeconfig  = "KUBECONFIG"
	envContext     = "CONTEXT"
	envNamespace   = "NAMESPACE"
	envImpersonate = "AS"
)

const (
	defaultLogLevel         = "info"
	defaultLogFormat        = "text"
	defaultPort             = 8080
	defaultAllowDestructive = false
)

// Flags wraps genericclioptions.ConfigFlags and adds application-specific flags.
type Flags struct {
	configFlags *genericclioptions.ConfigFlags

	AllowDestructive bool
	LogLevel         string
	LogFormat        string
	Port             int
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
		AllowDestructive: withDefaultEnvBool(envAllowDestructive, defaultAllowDestructive),
		LogLevel:         withDefaultEnv(envLogLevel, defaultLogLevel),
		LogFormat:        withDefaultEnv(envLogFormat, defaultLogFormat),
		Port:             withDefaultEnvInt(envPort, defaultPort),
	}
}

// AddPersistentFlags adds the global flags shared by every subcommand.
func (f *Flags) AddPersistentFlags(flags *pflag.FlagSet) {
	// Add Kubernetes flags from genericclioptions
	f.configFlags.AddFlags(flags)

	// Add application-specific flags
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
		ConfigFlags:      f.configFlags,
		Port:             f.Port,
		AllowDestructive: f.AllowDestructive,
		LogLevel:         f.LogLevel,
		LogFormat:        f.LogFormat,
	}
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
