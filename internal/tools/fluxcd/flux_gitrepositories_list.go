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

package toolsfluxcd

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
)

// GitRepositoryListResult is the result of flux_gitrepositories_list.
type GitRepositoryListResult struct {
	GitRepositories []SourceSummary `json:"gitRepositories" jsonschema:"List of GitRepository resources"`
}

// RegisterGitRepositoriesList adds the flux_gitrepositories_list tool, which
// lists Flux GitRepository sources in a namespace (or across all namespaces).
func RegisterGitRepositoriesList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Flux Git Repositories"),
		mcp.WithDescription("List Flux GitRepository sources in a namespace (empty for all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter (e.g. metadata.name=foo)")),
		mcp.WithOutputSchema[GitRepositoryListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("flux_gitrepositories_list", opts...)
	s.AddTool(tool, handlerGitRepositoriesList(mc))
}

// handlerGitRepositoriesList returns a handler function for the flux_gitrepositories_list tool.
func handlerGitRepositoriesList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "flux_gitrepositories_list called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		fluxClient, err := NewFluxClient(client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Flux client: %v", err), nil
		}

		repositories, err := fluxClient.ListGitRepositories(ctx, namespace, labelSelector, fieldSelector)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		result := GitRepositoryListResult{GitRepositories: repositories}

		fallbackText := "No GitRepository found"
		if len(result.GitRepositories) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
