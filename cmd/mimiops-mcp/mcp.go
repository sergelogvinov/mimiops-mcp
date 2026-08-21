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

			client, err := k8s.NewClient(cfg)
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
