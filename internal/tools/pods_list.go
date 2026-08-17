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

		// Build list options
		opts := metav1.ListOptions{}
		if labelSelector != "" {
			opts.LabelSelector = labelSelector
		}
		if fieldSelector != "" {
			opts.FieldSelector = fieldSelector
		}

		// List pods
		pods, err := client.CoreV1().Pods(namespace).List(ctx, opts)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list pods in namespace '%s': %v", namespace, err), nil
		}

		result := PodsListResult{
			Pods: make([]PodSummary, 0, len(pods.Items)),
		}

		// Build result
		for _, pod := range pods.Items {
			result.Pods = append(result.Pods, toPodSummary(pod))
		}

		// Build fallback text
		var fallbackText string
		switch len(result.Pods) {
		case 0:
			fallbackText = "No pods found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 pod: %s in namespace %s (%s)", result.Pods[0].Name, result.Pods[0].Namespace, result.Pods[0].Status)
		default:
			fallbackText = fmt.Sprintf("Found %d pods", len(result.Pods))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// formatReady extracts the ready status from pod status.
func formatReady(status corev1.PodStatus) string {
	ready := 0
	total := len(status.ContainerStatuses)
	for _, cs := range status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

// containerRestartCount returns the total restart count for all containers in the pod.
func containerRestartCount(status corev1.PodStatus) int32 {
	total := int32(0)
	for _, cs := range status.ContainerStatuses {
		total += cs.RestartCount
	}
	return total
}

// toPodSummary converts a pod to a PodSummary.
func toPodSummary(pod corev1.Pod) PodSummary {
	node := pod.Spec.NodeName
	if node == "" {
		node = "<pending>"
	}

	return PodSummary{
		Namespace:       pod.Namespace,
		Name:            pod.Name,
		Ready:           formatReady(pod.Status),
		Status:          string(pod.Status.Phase),
		Restarts:        containerRestartCount(pod.Status),
		Age:             formatAge(pod.CreationTimestamp),
		Node:            node,
		OwnerReferences: ownerReferences(&pod),
	}
}
