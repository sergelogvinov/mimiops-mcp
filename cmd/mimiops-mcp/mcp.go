package main

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools"
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

			// Build the logger here so an invalid --log-level / --log-format
			// errors out before we block on the transport.
			log, err := newLogger(cfg)
			if err != nil {
				return err
			}

			return serveStdio(cmd.Context(), client, cfg, log)
		},
	}
}

func serveStdio(_ context.Context, client *k8s.Client, cfg *config.Config, log *slog.Logger) error {
	versionInfo, err := client.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	log.Info("connected to kubernetes",
		"version", versionInfo.String(),
		"context", client.ContextName,
		"cluster", client.ClusterName,
		"namespace", client.Namespace,
		"user", client.User.Name,
	)

	if client.User.Username != "" {
		log.Debug("kubeconfig basic-auth username", "username", client.User.Username)
	}
	if client.User.HasToken {
		log.Debug("kubeconfig token auth is in use")
	}
	if client.User.Impersonate != "" {
		log.Info("impersonating",
			"user", client.User.Impersonate,
			"groups", client.User.ImpersonateGroups,
		)
	}

	log.Info("serving mcp over stdio")

	opts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithLogger(log),
	}

	srv := server.NewMCPServer("mimiops-mcp", version, opts...)
	tools.RegisterTools(srv, client, log, cfg.AllowDestructive)

	return server.ServeStdio(srv)
}
