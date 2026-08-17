package tools

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodGetResult represents the result of getting a pod.
type PodGetResult struct {
	PodSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the Pod"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the Pod"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the Pod"`

	Tolerations []TaintInfo     `json:"tolerations,omitempty" jsonschema:"List of tolerations"`
	Conditions  []ConditionInfo `json:"conditions,omitempty" jsonschema:"List of conditions"`
}

// RegisterPodsGet adds the pods_get tool, which gets a pod's full spec and status.
func RegisterPodsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Pod"),
		mcp.WithDescription("Get a pod's full spec and status"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[PodGetResult](),
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

		log.DebugContext(ctx, "pods_get called",
			"namespace", namespace,
			"pod", name,
		)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildPodGetResult(ctx, client, pod)
		fallbackText := fmt.Sprintf("Pod '%s' in namespace '%s' has status '%s' with ready status '%s'. Age: %s.",
			result.Name, result.Namespace, result.Status, result.Ready, result.Age)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildPodGetResult builds a PodGetResult from a Pod.
func buildPodGetResult(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *PodGetResult {
	result := &PodGetResult{
		PodSummary:  toPodSummary(ctx, client, pod),
		Labels:      pod.Labels,
		Annotations: pod.Annotations,
		Spec:        make(map[string]any),
		Tolerations: make([]TaintInfo, 0, len(pod.Spec.Tolerations)),
		Conditions:  make([]ConditionInfo, 0, len(pod.Status.Conditions)),
	}

	if result.Labels == nil {
		result.Labels = make(map[string]string)
	}
	if result.Annotations == nil {
		result.Annotations = make(map[string]string)
	}

	// Remove internal annotations
	maps.DeleteFunc(result.Annotations, func(k, _ string) bool {
		return k == "kubectl.kubernetes.io/last-applied-configuration"
	})

	// Tolerations
	for _, taint := range pod.Spec.Tolerations {
		result.Tolerations = append(result.Tolerations, TaintInfo{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		})
	}

	// Conditions
	for _, cond := range pod.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// Spec (simplified)
	result.Spec = make(map[string]any)
	result.Spec["nodeName"] = pod.Spec.NodeName
	result.Spec["restartPolicy"] = pod.Spec.RestartPolicy
	result.Spec["serviceAccountName"] = pod.Spec.ServiceAccountName

	return result
}
