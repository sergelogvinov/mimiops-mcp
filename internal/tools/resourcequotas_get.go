package tools

import (
	"context"
	"log/slog"
	"maps"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceQuotaGetResult represents the result of getting a resource quota.
type ResourceQuotaGetResult struct {
	ResourceQuotaSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the resource quota"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the resource quota"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the resource quota"`
	Status      map[string]any    `json:"status" jsonschema:"Status of the resource quota"`
}

// RegisterResourceQuotasGet adds the resourcequotas_get tool, which gets a single ResourceQuota's full spec and status.
func RegisterResourceQuotasGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("resourcequotas_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get ResourceQuota"),
		mcp.WithDescription("Get a single ResourceQuota's full spec and status."),
		mcp.WithString("name", mcp.Description("resource quota name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[ResourceQuotaGetResult](),
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

		log.DebugContext(ctx, "resourcequotas_get called", "namespace", namespace, "name", name)

		// Get the resource quota
		quota, err := client.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("resource quota '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get resource quota '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildResourceQuotaGetResult(quota)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildResourceQuotaGetResult builds a ResourceQuotaGetResult from a ResourceQuota.
func buildResourceQuotaGetResult(quota *corev1.ResourceQuota) *ResourceQuotaGetResult {
	used := quota.Status.Hard
	usedStatus := quota.Status.Used

	result := ResourceQuotaGetResult{
		ResourceQuotaSummary: ResourceQuotaSummary{
			Name:           quota.Name,
			Namespace:      quota.Namespace,
			Age:            formatAge(quota.CreationTimestamp),
			RequestsCPU:    getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU),
			RequestsMemory: getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory),
			LimitsCPU:      getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU),
			LimitsMemory:   getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory),
		},
		Labels:      quota.Labels,
		Annotations: quota.Annotations,
	}

	if result.Labels == nil {
		result.Labels = make(map[string]string)
	}
	if result.Annotations == nil {
		result.Annotations = make(map[string]string)
	}

	maps.DeleteFunc(result.Annotations, func(k, _ string) bool {
		return k == "kubectl.kubernetes.io/last-applied-configuration"
	})

	// Spec
	result.Spec = make(map[string]any)
	result.Spec["hard"] = quota.Spec.Hard

	// Status
	result.Status = make(map[string]any)
	result.Status["hard"] = quota.Status.Hard
	result.Status["used"] = quota.Status.Used

	return &result
}
