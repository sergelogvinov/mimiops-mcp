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

// PodDescribeResult represents the result of describing a pod.
type PodDescribeResult struct {
	PodSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the Pod"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the Pod"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the Pod"`

	Containers []ContainerInfo `json:"containers" jsonschema:"List of containers in the pod"`
}

// RegisterPodsDescribe adds the pods_describe tool, which provides a structured pod summary.
func RegisterPodsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_describe",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Pod"),
		mcp.WithDescription("Pod summary (conditions, container statuses, node, tolerations)"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[PodDescribeResult](),
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

		log.DebugContext(ctx, "pods_describe called",
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

		result := buildPodDescribeResult(ctx, client, pod)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildPodDescribeResult builds a PodDescribeResult from a Pod.
func buildPodDescribeResult(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *PodDescribeResult {
	result := &PodDescribeResult{
		PodSummary:  toPodSummary(ctx, client, pod),
		Labels:      pod.Labels,
		Annotations: pod.Annotations,
		Spec:        make(map[string]any),
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

	// Spec (simplified)
	result.Spec["nodeName"] = pod.Spec.NodeName
	result.Spec["restartPolicy"] = pod.Spec.RestartPolicy
	result.Spec["serviceAccountName"] = pod.Spec.ServiceAccountName

	// Containers
	result.Containers = extractContainerInfo(pod.Spec.Containers)

	return result
}
