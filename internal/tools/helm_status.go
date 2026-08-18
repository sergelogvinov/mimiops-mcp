package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
		fallbackText := fmt.Sprintf("Helm release '%s' in namespace '%s': %s (revision %d)", name, namespace, release.Status, release.Revision)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
