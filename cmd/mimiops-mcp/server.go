package main

import (
	"context"
	"strconv"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newServerCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Serve MCP protocol over HTTP/SSE",
		Long:  "Serve MCP protocol over HTTP/SSE for web/remote MCP clients",
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
			defer func() {
				_ = log.Sync() //nolint:errcheck
			}()

			return serveSSE(cmd.Context(), client, cfg, log)
		},
	}

	flags.AddServerFlags(cmd.Flags())

	return cmd
}

func serveSSE(_ context.Context, client *k8s.Client, cfg *config.Config, log *zap.Logger) error {
	versionInfo, err := client.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	log.Info("connected to kubernetes",
		zap.String("version", versionInfo.String()),
		zap.String("context", client.ContextName),
		zap.String("cluster", client.ClusterName),
		zap.String("namespace", client.Namespace),
		zap.String("user", client.User.Name),
	)

	if client.User.Username != "" {
		log.Debug("kubeconfig basic-auth username", zap.String("username", client.User.Username))
	}
	if client.User.HasToken {
		log.Debug("kubeconfig token auth is in use")
	}
	if client.User.Impersonate != "" {
		log.Info("impersonating",
			zap.String("user", client.User.Impersonate),
			zap.Strings("groups", client.User.ImpersonateGroups),
		)
	}

	log.Info("serving mcp over http/sse", zap.Int("port", cfg.Port))

	srv := server.NewMCPServer("mimiops-mcp", version)
	tools.RegisterTools(srv, client, cfg.AllowDestructive)

	httpOpts := []server.StreamableHTTPOption{}
	httpServer := server.NewStreamableHTTPServer(srv, httpOpts...)

	return httpServer.Start(":" + strconv.Itoa(cfg.Port))
}
