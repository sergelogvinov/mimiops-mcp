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
	"net/http"
	"strconv"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools"
	"github.com/spf13/cobra"
)

func newServerCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Serve MCP protocol over HTTP/SSE",
		Long:  "Serve MCP protocol over HTTP/SSE for web/remote MCP clients",
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

			return serveSSE(cmd.Context(), mc, cfg, log)
		},
	}

	flags.AddServerFlags(cmd.Flags())

	return cmd
}

func serveSSE(_ context.Context, mc *k8s.MultiClusterClient, cfg *config.Config, log *slog.Logger) error {
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

		// mcClient, err := mc.GetCluster(cluster.Name)
		// if err != nil {
		// 	log.Error("failed to get client for cluster", "cluster", cluster.Name, "error", err)
		// 	continue
		// }

		// log.Debug("cluster details",
		// 	"contexts", cluster.Contexts,
		// 	"namespace", mcClient.Namespace,
		// 	"user", mcClient.User.Name,
		// 	"username", mcClient.User.Username,
		// 	"impersonate", mcClient.User.Impersonate,
		// 	"impersonateGroups", mcClient.User.ImpersonateGroups,
		// 	"hasToken", mcClient.User.HasToken,
		// )
	}

	log.Info("serving mcp over http/stream", "port", cfg.Port)

	opts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithLogger(log),
	}

	srv := server.NewMCPServer("mimiops-mcp", version, opts...)
	if err := tools.RegisterTools(srv, mc, cfg.Extensions, cfg.AllowDestructive); err != nil {
		return err
	}

	// Set up HTTP server with context injection for logging
	// Currently, this is the only way to inject context into the HTTP handler for logging purposes.
	httpOpts := []server.StreamableHTTPOption{
		server.WithHTTPContextFunc(func(ctx context.Context, _ *http.Request) context.Context {
			return logger.Inject(ctx, log)
		}),
	}

	httpServer := server.NewStreamableHTTPServer(srv, httpOpts...)

	return httpServer.Start(":" + strconv.Itoa(cfg.Port))
}
