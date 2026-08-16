package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// RegisterClusterName adds the cluster_name tool, which reports the name of the
// cluster resolved from the active kubeconfig context. The value is captured
// once at startup by k8s.NewClient, so no cluster API call (and therefore no
// RBAC) is needed at call time.
func RegisterClusterName(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cluster_name",
		mcp.WithDescription("Return the name of the connected cluster"),
	)
	s.AddTool(tool, func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.InfoContext(ctx, "cluster_name called", "cluster", client.ClusterName)

		return mcp.NewToolResultText(client.ClusterName), nil
	})
}
