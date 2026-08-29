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
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/oidc"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools"
	"github.com/spf13/cobra"
)

func newServerCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Serve MCP protocol over HTTP/SSE",
		Long:  "Serve MCP protocol over HTTP/SSE for web/remote MCP clients",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := flags.Config()
			if err != nil {
				return err
			}

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

			// OIDC is enabled only when --oidc-issuer is set. Discovery is
			// performed here so a misconfigured issuer fails fast at startup.
			// A non-empty --oidc-callback-url additionally enables the OAuth
			// proxy flow for clients that cannot use the issuer directly.
			var verifier *oidc.Verifier
			var proxy *oauthProxy

			if cfg.OIDCIssuer != "" {
				verifier, err = oidc.New(cmd.Context(), oidc.Config{
					Issuer:       cfg.OIDCIssuer,
					ClientID:     cfg.OIDCClientID,
					EmailDomains: cfg.OIDCEmailDomains,
				})
				if err != nil {
					return err
				}

				if cfg.OIDCCallbackURL != "" {
					proxy, err = newOAuthProxy(verifier, proxyConfig{
						clientID:     cfg.OIDCClientID,
						clientSecret: cfg.OIDCClientSecret,
						callbackURL:  cfg.OIDCCallbackURL,
						scopes:       cfg.OIDCScope,
						log:          log,
					})
					if err != nil {
						return err
					}
				}
			}

			return serveSSE(cmd.Context(), mc, cfg, log, verifier, proxy)
		},
	}

	flags.AddServerFlags(cmd.Flags())

	return cmd
}

func serveSSE(_ context.Context, mc *k8s.MultiClusterClient, cfg *config.Config, log *slog.Logger, verifier *oidc.Verifier, proxy *oauthProxy) error {
	log.Info("server config",
		"allowDestructive", cfg.AllowDestructive,
		"multiCluster", mc.IsMultiCluster(),
		"clusters", len(mc.ListClusters()),
		"oidc", verifier != nil,
		"oidcIssuer", cfg.OIDCIssuer,
		"oidcProxy", proxy != nil,
		"oidcCallbackURL", cfg.OIDCCallbackURL,
	)

	for _, cluster := range mc.ListClusters() {
		log.Debug("cluster",
			"name", cluster.Name,
			"server", cluster.Server,
			"default", cluster.IsCurrent,
		)
	}

	log.Info("serving mcp over http/stream", "port", cfg.Port)

	opts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithLogger(log),
	}

	srv := server.NewMCPServer("MimiOPS MCP Server", version, opts...)
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

	if verifier == nil {
		return httpServer.Start(":" + strconv.Itoa(cfg.Port))
	}

	// mcp-go v0.58.0 has no HTTP middleware option, but StreamableHTTPServer
	// implements http.Handler: wrap it with an auth middleware that rejects
	// unauthenticated requests with 401 before any tool handler runs, and
	// injects the verified token into the request context for the
	// per-request k8s client (see MultiClusterClient.GetClusterForRequest).
	httpSrv := &http.Server{
		Addr: ":" + strconv.Itoa(cfg.Port),
		// Bound the request-header read only: the deadline expires before the
		// handler (and thus the OIDC auth layer) runs, and ReadTimeout /
		// WriteTimeout stay unset so SSE/streaming responses are not cut off.
		ReadHeaderTimeout: 10 * time.Second,
		Handler: newOIDCHandler(httpServer, oidcHandlerConfig{
			verifier: verifier,
			issuer:   cfg.OIDCIssuer,
			proxy:    proxy,
			scopes:   cfg.OIDCScope,
			log:      log,
		}),
	}

	return httpSrv.ListenAndServe()
}
