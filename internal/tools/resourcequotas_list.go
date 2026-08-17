package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceQuotasListResult represents the result of listing resource quotas.
type ResourceQuotasListResult struct {
	ResourceQuotas []ResourceQuotaSummary `json:"resourcequotas" jsonschema:"List of resource quotas"`
}

// RegisterResourceQuotasList adds the resourcequotas_list tool, which lists ResourceQuotas in a namespace (or all namespaces).
func RegisterResourceQuotasList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("resourcequotas_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List ResourceQuotas"),
		mcp.WithDescription("List ResourceQuotas in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithOutputSchema[ResourceQuotasListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		log.DebugContext(ctx, "resourcequotas_list called", "namespace", namespace)

		// List resource quotas
		quotas, err := client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list resource quotas in namespace '%s': %v", namespace, err), nil
		}

		result := ResourceQuotasListResult{
			ResourceQuotas: make([]ResourceQuotaSummary, 0, len(quotas.Items)),
		}

		// Build result
		for _, quota := range quotas.Items {
			summary := ResourceQuotaSummary{
				Name:      quota.Name,
				Namespace: quota.Namespace,
				Age:       formatAge(quota.CreationTimestamp),
			}

			used := quota.Status.Hard
			usedStatus := quota.Status.Used

			summary.RequestsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU)
			summary.RequestsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory)
			summary.LimitsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU)
			summary.LimitsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory)

			result.ResourceQuotas = append(result.ResourceQuotas, summary)
		}

		// Build fallback text
		var fallbackText string
		switch len(result.ResourceQuotas) {
		case 0:
			fallbackText = "No resource quotas found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 resource quota: %s in namespace %s", result.ResourceQuotas[0].Name, result.ResourceQuotas[0].Namespace)
		default:
			fallbackText = fmt.Sprintf("Found %d resource quotas", len(result.ResourceQuotas))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// getQuotaValueDisplay returns the used/hard display for a resource.
func getQuotaValueDisplay(usedStatus, used corev1.ResourceList, resource corev1.ResourceName) string {
	hard, hasHard := used[resource]
	if !hasHard {
		return "-"
	}

	usedVal, hasUsed := usedStatus[resource]
	if !hasUsed {
		return "-" + "/" + hard.String()
	}

	return usedVal.String() + "/" + hard.String()
}
