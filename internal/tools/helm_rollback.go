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

// HelmRollbackResult represents the result of rolling back a Helm release.
type HelmRollbackResult struct {
	Rollback helm.RollbackResult `json:"rollback" jsonschema:"Helm rollback result"`
}

// RegisterHelmRollback adds the helm_rollback tool, which rolls back a Helm release to the previous revision.
func RegisterHelmRollback(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("helm_rollback",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Rollback Helm Release"),
		mcp.WithDescription("Roll back a Helm release to the previous revision (one back)"),
		mcp.WithString("name", mcp.Description("release name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[HelmRollbackResult](),
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

		log.DebugContext(ctx, "helm_rollback called",
			"name", name,
			"namespace", namespace,
		)

		// Create Helm client
		helmClient, err := helm.NewHelmClient(client, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Helm client: %v", err), nil
		}

		// First, get the current revision to calculate the previous revision
		currentRelease, err := helmClient.GetRelease(name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get current status for release: %v", err), nil
		}

		// Calculate previous revision (current - 1)
		previousRevision := currentRelease.Revision
		newRevision := previousRevision - 1

		if newRevision < 1 {
			return mcp.NewToolResultErrorf("release '%s' in namespace '%s' is at revision %d — nothing to roll back to", name, namespace, previousRevision), nil
		}

		// Perform the rollback to the previous revision
		err = helmClient.Rollback(name, newRevision)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to rollback release to revision %d: %v", newRevision, err), nil
		}

		// Get the status after rollback
		afterRollback, err := helmClient.GetRelease(name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get status after rollback for release: %v", err), nil
		}

		result := HelmRollbackResult{
			Rollback: helm.RollbackResult{
				Name:             name,
				Namespace:        namespace,
				PreviousRevision: previousRevision,
				NewRevision:      afterRollback.Revision,
				Status:           afterRollback.Status,
			},
		}

		fallbackText := fmt.Sprintf("Rolled back release '%s' in namespace '%s' to revision %d", name, namespace, newRevision)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
