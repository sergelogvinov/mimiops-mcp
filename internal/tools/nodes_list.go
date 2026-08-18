package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodesListResult represents the result of listing nodes.
type NodesListResult struct {
	Nodes []NodeSummary `json:"nodes" jsonschema:"List of nodes"`
}

// RegisterNodesList adds the nodes_list tool, which lists cluster nodes and their status.
func RegisterNodesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("nodes_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Nodes"),
		mcp.WithDescription("List cluster nodes and their status"),
		mcp.WithOutputSchema[NodesListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.DebugContext(ctx, "nodes_list called")

		// List nodes
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no nodes found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list nodes: %v", err), nil
		}

		result := NodesListResult{
			Nodes: make([]NodeSummary, 0, len(nodes.Items)),
		}

		// Build result
		for _, node := range nodes.Items {
			result.Nodes = append(result.Nodes, toNodeSummary(node))
		}

		// Build fallback text
		var fallbackText string
		switch len(result.Nodes) {
		case 0:
			fallbackText = "No nodes found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 node: %s (%s)", result.Nodes[0].Name, result.Nodes[0].Status)
		default:
			fallbackText = fmt.Sprintf("Found %d nodes", len(result.Nodes))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
