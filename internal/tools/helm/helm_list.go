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

package toolshelm

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/helm"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
)

// HelmListResult represents the result of listing Helm releases.
type HelmListResult struct {
	Releases []ReleaseSummary `json:"releases" jsonschema:"List of Helm releases"`
}

// RegisterHelmList adds the helm_list tool, which lists Helm releases in a namespace.
func RegisterHelmList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Helm Releases"),
		mcp.WithDescription("List Helm releases in a namespace"),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("status_filter", mcp.Description("status filter or empty for all"), mcp.Enum("failed", "deployed", ""), mcp.DefaultString("")),
		mcp.WithOutputSchema[HelmListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("helm_list", opts...)
	s.AddTool(tool, handlerHelmList(mc))
}

// handlerHelmList returns a handler function for the helm_list tool.
func handlerHelmList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		labelSelector := req.GetString("label_selector", "")
		statusFilter := req.GetString("status_filter", "")
		outputFormat := req.GetString("format", "text")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "helm_list called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"label_selector", labelSelector,
			"status_filter", statusFilter,
			"format", outputFormat,
		)

		// Create Helm client
		helmClient, err := helm.NewHelmClient(client, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Helm client: %v", err), nil
		}

		// List releases
		releases, err := helmClient.ListReleases(namespace, labelSelector, statusFilter)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list releases: %v", err), nil
		}

		result := HelmListResult{
			Releases: make([]ReleaseSummary, 0, len(releases)),
		}

		for _, r := range releases {
			result.Releases = append(result.Releases, toHelmSummary(&r))
		}

		// Build fallback text
		fallbackText := "No Helm releases found"
		if len(result.Releases) > 0 {
			fallbackText = formatter.ToText(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
