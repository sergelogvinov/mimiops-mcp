package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceGetResult represents the result of getting a namespace.
type NamespaceGetResult struct {
	NamespaceSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the namespace"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the namespace"`
	Finalizers  []string          `json:"finalizers" jsonschema:"List of finalizers"`
	Conditions  []ConditionInfo   `json:"conditions" jsonschema:"List of conditions"`
}

// RegisterNamespacesGet adds the namespaces_get tool, which gets a single namespace's full spec and status.
func RegisterNamespacesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("namespaces_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get namespace"),
		mcp.WithDescription("Get a namespace full spec and status."),
		mcp.WithString("name", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[NamespaceGetResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		log.DebugContext(ctx, "namespaces_get called", "namespace", name)

		// Get the namespace
		ns, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("namespace '%s' not found", name), nil
			}
			return mcp.NewToolResultErrorf("failed to get namespace '%s': %v", name, err), nil
		}

		// Build result
		result := buildNamespaceGetResult(ns)

		// Build fallback text
		fallbackText := fmt.Sprintf("Namespace '%s' has status '%s'. Age: %s.", result.Name, result.Status, result.Age)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildNamespaceGetResult builds a NamespaceGetResult from a Namespace.
func buildNamespaceGetResult(ns *corev1.Namespace) *NamespaceGetResult {
	result := &NamespaceGetResult{
		NamespaceSummary: NamespaceSummary{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    formatAge(ns.CreationTimestamp),
		},
		Labels:      ns.Labels,
		Annotations: ns.Annotations,
		Finalizers:  ns.Finalizers,
		Conditions:  make([]ConditionInfo, 0, len(ns.Status.Conditions)),
	}

	// Conditions
	for _, c := range ns.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	return result
}
