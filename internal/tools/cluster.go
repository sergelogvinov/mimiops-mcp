package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// ClusterNameResult represents the result of getting the cluster name.
type ClusterNameResult struct {
	Name string `json:"name" jsonschema:"Name of the cluster"`
}

// RegisterClusterName adds the cluster_name tool, which reports the name of the
// cluster resolved from the active kubeconfig context. The value is captured
// once at startup by k8s.NewClient, so no cluster API call (and therefore no
// RBAC) is needed at call time.
func RegisterClusterName(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cluster_name",
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Cluster Name"),
		mcp.WithDescription("Return the name of the connected cluster"),
		mcp.WithOutputSchema[ClusterNameResult](),
	)
	s.AddTool(tool, func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.InfoContext(ctx, "cluster_name called", "cluster", client.ClusterName)

		result := ClusterNameResult{Name: client.ClusterName}
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}
