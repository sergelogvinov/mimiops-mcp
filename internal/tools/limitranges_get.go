package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LimitRangeGetResult represents the result of getting a LimitRange.
type LimitRangeGetResult struct {
	LimitRangeSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the LimitRange"`
}

// RegisterLimitRangesGet adds the limitranges_get tool, which gets a single LimitRange's full spec.
func RegisterLimitRangesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("limitranges_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get LimitRange"),
		mcp.WithDescription("Get a single LimitRange's full spec and status."),
		mcp.WithString("name", mcp.Description("LimitRange name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[LimitRangeGetResult](),
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

		log.DebugContext(ctx, "limitranges_get called", "namespace", namespace, "name", name)

		// Get the limit range
		lr, err := client.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("limit range '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get limit range '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildLimitRangeGetResult(lr)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildLimitRangeGetResult builds a LimitRangeGetResult from a LimitRange.
func buildLimitRangeGetResult(lr *corev1.LimitRange) *LimitRangeGetResult {
	result := &LimitRangeGetResult{
		LimitRangeSummary: LimitRangeSummary{
			Name:      lr.Name,
			Namespace: lr.Namespace,
			Types:     deriveLimitRangeTypes(lr),
			Age:       formatAge(lr.CreationTimestamp),
		},
		Annotations: extractAnnotations(lr.Annotations),
		Labels:      extractLabels(lr.Labels),
		Spec:        make(map[string]any),
	}

	// Spec (simplified)
	result.Spec["limits"] = lr.Spec.Limits

	return result
}
