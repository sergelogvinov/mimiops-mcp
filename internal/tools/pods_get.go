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

// PodGetResult represents the result of getting a pod.
type PodGetResult struct {
	PodSummary
	PodSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
	Tolerations []TolerationInfo  `json:"tolerations,omitempty" jsonschema:"Tolerations"`
	Conditions  []ConditionInfo   `json:"conditions,omitempty" jsonschema:"Conditions"`
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
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("pod '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildPodGetResult(ctx, client, pod)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildPodGetResult builds a PodGetResult from a Pod.
func buildPodGetResult(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *PodGetResult {
	result := &PodGetResult{
		PodSummary:  toPodSummary(ctx, client, pod),
		PodSpec:     toPodSpec(pod),
		Annotations: extractAnnotations(pod.Annotations),
		Labels:      extractLabels(pod.Labels),
		Tolerations: make([]TolerationInfo, 0, len(pod.Spec.Tolerations)),
		Conditions:  make([]ConditionInfo, 0, len(pod.Status.Conditions)),
	}

	// Tolerations
	for _, toleration := range pod.Spec.Tolerations {
		result.Tolerations = append(result.Tolerations, TolerationInfo{
			Key:    toleration.Key,
			Value:  toleration.Value,
			Effect: string(toleration.Effect),
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

	return result
}
