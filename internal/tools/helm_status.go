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

package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/helm"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// HelmStatusResult represents the result of getting Helm release status.
type HelmStatusResult struct {
	Release helm.ReleaseStatus `json:"release" jsonschema:"Helm release status and history"`
}

// RegisterHelmStatus adds the helm_status tool, which gets the status of a Helm release.
func RegisterHelmStatus(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("helm_status",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Helm Release Status"),
		mcp.WithDescription("Get the status of a Helm release, including the last 3 revisions of history"),
		mcp.WithString("name", mcp.Description("release name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[HelmStatusResult](),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		log.DebugContext(ctx, "helm_status called",
			"name", name,
			"namespace", namespace,
		)

		// Create Helm client
		helmClient, err := helm.NewHelmClient(client, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Helm client: %v", err), nil
		}

		// Get release status
		release, err := helmClient.GetRelease(name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get status for release: %v", err), nil
		}

		// Get release history (last 3 revisions)
		history, err := helmClient.GetReleaseHistory(name, namespace, 3)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get history for release: %v", err), nil
		}

		release.History = history

		result := HelmStatusResult{
			Release: *release,
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}
