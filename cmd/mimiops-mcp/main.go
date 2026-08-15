// Package main implements the mimiops-mcp command-line tool.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootCmd := newRootCmd()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		errorString := err.Error()
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", errorString)

		if strings.Contains(errorString, "arg(s)") {
			fmt.Fprintln(os.Stderr, rootCmd.UsageString())
		}

		cancel()
		os.Exit(1) //nolint:gocritic
	}
}

// newLogger builds the zap logger from the parsed config, honoring the
// --log-level and --log-format flags.
func newLogger(cfg *config.Config) (*zap.Logger, error) {
	return logger.New(logger.Options{
		Level:  logger.Level(cfg.LogLevel),
		Format: logger.Format(cfg.LogFormat),
	})
}

func newRootCmd() *cobra.Command {
	flags := DefaultFlags()

	rootCmd := &cobra.Command{
		Use:           "mimiops-mcp",
		Short:         "MimiOPS MCP Server - Kubernetes tooling",
		Long:          "MimiOPS MCP Server provides Kubernetes tooling via MCP protocol",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global (persistent) flags, inherited by every subcommand.
	flags.AddPersistentFlags(rootCmd.PersistentFlags())

	rootCmd.AddCommand(
		newVersionCmd(),
		newMcpCmd(flags),
		newServerCmd(flags),
	)

	return rootCmd
}
