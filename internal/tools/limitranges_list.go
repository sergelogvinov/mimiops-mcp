package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LimitRangesListResult represents the result of listing limit ranges.
type LimitRangesListResult struct {
	LimitRanges []LimitRangeSummary `json:"limitranges" jsonschema:"List of limit ranges"`
}

// RegisterLimitRangesList adds the limitranges_list tool, which lists LimitRanges in a namespace (or all namespaces).
func RegisterLimitRangesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("limitranges_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List LimitRanges"),
		mcp.WithDescription("List LimitRanges in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithOutputSchema[LimitRangesListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		log.DebugContext(ctx, "limitranges_list called", "namespace", namespace)

		// List limit ranges
		ranges, err := client.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list limit ranges in namespace '%s': %v", namespace, err), nil
		}

		result := LimitRangesListResult{
			LimitRanges: make([]LimitRangeSummary, 0, len(ranges.Items)),
		}

		// Build result
		for _, lr := range ranges.Items {
			result.LimitRanges = append(result.LimitRanges, LimitRangeSummary{
				Name:      lr.Name,
				Namespace: lr.Namespace,
				Types:     deriveLimitRangeTypes(&lr),
				Age:       formatAge(lr.CreationTimestamp),
			})
		}

		var fallbackText string
		switch len(result.LimitRanges) {
		case 0:
			fallbackText = "No limit ranges found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 limit range: %s in namespace %s (%s)", result.LimitRanges[0].Name, result.LimitRanges[0].Namespace, result.LimitRanges[0].Types)
		default:
			fallbackText = fmt.Sprintf("Found %d limit ranges", len(result.LimitRanges))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// deriveLimitRangeTypes derives the resource types from spec.limits.
func deriveLimitRangeTypes(lr *corev1.LimitRange) string {
	types := make([]string, 0)
	for _, limit := range lr.Spec.Limits {
		if limit.Type != "" {
			types = append(types, string(limit.Type))
		}
	}
	if len(types) == 0 {
		return "none"
	}
	return strings.Join(types, ", ")
}
