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

// HelmListResult represents the result of listing Helm releases.
type HelmListResult struct {
	Releases []helm.ReleaseSummary `json:"releases" jsonschema:"List of Helm releases"`
}

// RegisterHelmList adds the helm_list tool, which lists Helm releases in a namespace.
func RegisterHelmList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("helm_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Helm Releases"),
		mcp.WithDescription("List Helm releases in a namespace"),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("status_filter", mcp.Description("status filter or empty for all"), mcp.Enum("failed", "deployed", ""), mcp.DefaultString("")),
		mcp.WithOutputSchema[HelmListResult](),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		labelSelector := req.GetString("label_selector", "")
		statusFilter := req.GetString("status_filter", "")
		outputFormat := req.GetString("format", "text")

		log.DebugContext(ctx, "helm_list called",
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
			Releases: releases,
		}

		// Build fallback text
		fallbackText := "No Helm releases found"
		if len(result.Releases) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
