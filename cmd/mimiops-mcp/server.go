package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/spf13/cobra"
)

func newServerCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Serve MCP protocol over HTTP/SSE",
		Long:  "Serve MCP protocol over HTTP/SSE for web/remote MCP clients",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := flags.Config()

			client, err := k8s.NewClient(&k8s.Config{
				Kubeconfig: cfg.Kubeconfig,
				Context:    cfg.Context,
				Namespace:  cfg.Namespace,
			})
			if err != nil {
				return err
			}

			return serveSSE(cmd.Context(), client, cfg)
		},
	}

	flags.AddServerFlags(cmd.Flags())

	return cmd
}

func serveSSE(ctx context.Context, client *k8s.Client, cfg *config.Config) error {
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
	fmt.Fprintf(os.Stderr, "  serving:   :%d (HTTP/SSE)\n", cfg.Port)

	// TODO(server): wire the MCP server over HTTP/SSE here, listening on cfg.Port.
	<-ctx.Done()

	return nil
}
