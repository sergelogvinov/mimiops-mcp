package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/spf13/cobra"
)

func newMcpCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve MCP protocol over stdio",
		Long:  "Serve MCP protocol over stdio for desktop clients (Claude Desktop, Cursor, VS Code)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := flags.Config()

			client, err := k8s.NewClient(&k8s.Config{
				Kubeconfig:  cfg.Kubeconfig,
				Context:     cfg.Context,
				Namespace:   cfg.Namespace,
				Impersonate: cfg.Impersonate,
			})
			if err != nil {
				return err
			}

			return serveStdio(cmd.Context(), client, cfg)
		},
	}
}

func serveStdio(ctx context.Context, client *k8s.Client, cfg *config.Config) error {
	versionInfo, err := client.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Connected to Kubernetes %s\n", versionInfo.String())
	fmt.Fprintf(os.Stderr, "  context:   %s\n", client.ContextName)
	fmt.Fprintf(os.Stderr, "  cluster:   %s\n", client.ClusterName)
	fmt.Fprintf(os.Stderr, "  namespace: %s\n", client.Namespace)
	fmt.Fprintf(os.Stderr, "  user:      %s\n", client.User.Name)
	if client.User.Username != "" {
		fmt.Fprintf(os.Stderr, "  username:  %s\n", client.User.Username)
	}
	if client.User.HasToken {
		fmt.Fprintf(os.Stderr, "  auth:      token\n")
	}
	if client.User.Impersonate != "" {
		fmt.Fprintf(os.Stderr, "  impersonate: %s\n", client.User.Impersonate)
	}
	if len(client.User.ImpersonateGroups) > 0 {
		fmt.Fprintf(os.Stderr, "  impersonate-groups: %v\n", client.User.ImpersonateGroups)
	}

	// TODO(mcp): wire the MCP server over stdio here.
	<-ctx.Done()

	return nil
}
