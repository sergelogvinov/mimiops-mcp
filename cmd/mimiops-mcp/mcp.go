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
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
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

			mc, err := k8s.NewMultiClusterClient(cfg)
			if err != nil {
				return err
			}

			// Build the logger here so an invalid --log-level / --log-format
			// errors out before we block on the transport.
			log, err := newLogger(cfg)
			if err != nil {
				return err
			}

			return serveStdio(cmd.Context(), mc, cfg, log)
		},
	}
}

func serveStdio(_ context.Context, mc *k8s.MultiClusterClient, cfg *config.Config, log *slog.Logger) error {
	log.Info("server config",
		"allowDestructive", cfg.AllowDestructive,
		"multiCluster", mc.IsMultiCluster(),
		"clusters", len(mc.ListClusters()),
	)

	for _, cluster := range mc.ListClusters() {
		log.Debug("cluster",
			"name", cluster.Name,
			"server", cluster.Server,
			"default", cluster.IsCurrent,
		)
	}

	log.Info("serving mcp over stdio")

	opts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithLogger(log),
	}

	srv := server.NewMCPServer("mimiops-mcp", version, opts...)
	if err := tools.RegisterTools(srv, mc, cfg.Extensions, cfg.AllowDestructive); err != nil {
		return err
	}

	return server.ServeStdio(srv, server.WithStdioContextFunc(func(ctx context.Context) context.Context {
		return logger.Inject(ctx, log)
	}))
}
