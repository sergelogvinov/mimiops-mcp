package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespacesListResult represents the result of listing namespaces.
type NamespacesListResult struct {
	Namespaces []NamespaceSummary `json:"namespaces" jsonschema:"List of namespaces"`
}

// RegisterNamespacesList adds the namespaces_list tool, which lists all namespaces.
func RegisterNamespacesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("namespaces_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Namespaces"),
		mcp.WithDescription("List all namespaces in the cluster"),
		mcp.WithOutputSchema[NamespacesListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.DebugContext(ctx, "namespaces_list called")

		// List namespaces
		namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Namespaces found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list namespaces: %v", err), nil
		}

		result := NamespacesListResult{
			Namespaces: make([]NamespaceSummary, 0, len(namespaces.Items)),
		}

		// Build result
		for _, ns := range namespaces.Items {
			result.Namespaces = append(result.Namespaces, NamespaceSummary{
				Name:   ns.Name,
				Status: string(ns.Status.Phase),
				Age:    formatAge(ns.CreationTimestamp),
			})
		}

		// Build fallback text
		fallbackText := "No Namespaces found"
		if len(result.Namespaces) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
