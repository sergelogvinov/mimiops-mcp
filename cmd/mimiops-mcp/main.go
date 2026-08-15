// Package main implements the mimiops-mcp command-line tool.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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

		os.Exit(1)
	}
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
