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

// PodsListResult represents the result of listing pods.
type PodsListResult struct {
	Pods []PodSummary `json:"pods" jsonschema:"List of pods"`
}

// RegisterPodsList adds the pods_list tool, which lists pods in a namespace (or all namespaces).
func RegisterPodsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Pods"),
		mcp.WithDescription("List pods in a namespace (or all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithOutputSchema[PodsListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Parse parameters
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")

		log.DebugContext(ctx, "pods_list called",
			"namespace", namespace,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		// List pods
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector, LabelSelector: labelSelector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no pods found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list pods: %v", err), nil
		}

		result := PodsListResult{
			Pods: make([]PodSummary, 0, len(pods.Items)),
		}

		// Build result
		for _, pod := range pods.Items {
			result.Pods = append(result.Pods, toPodSummary(ctx, client, &pod))
		}

		// Build fallback text
		fallbackText := "No pods found"
		if len(result.Pods) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
