package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterPodsList adds the pods_list tool, which lists pods in a namespace (or all namespaces).
func RegisterPodsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_list",
		mcp.WithDescription("List pods in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; empty = all namespaces"), mcp.Required()),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Parse parameters
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")
		format := req.GetString("format", "text")

		// Validate format
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "pods_list called",
			"namespace", namespace,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		// Use metav1.NamespaceAll for empty namespace (all namespaces)
		ns := namespace
		if ns == "" {
			ns = metav1.NamespaceAll
		}

		// Build list options
		opts := metav1.ListOptions{}
		if labelSelector != "" {
			opts.LabelSelector = labelSelector
		}
		if fieldSelector != "" {
			opts.FieldSelector = fieldSelector
		}

		// List pods
		pods, err := client.CoreV1().Pods(ns).List(ctx, opts)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list pods in namespace '%s': %v", ns, err), nil
		}

		// Format output
		result, err := formatPodsList(pods.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatPodsList formats a list of pods for MCP tool output.
func formatPodsList(pods []corev1.Pod, format string) (string, error) {
	if format == "json" {
		return formatPodsListJSON(pods)
	}
	return formatPodsListText(pods), nil
}

// formatPodsListText formats a list of pods as a markdown table.
func formatPodsListText(pods []corev1.Pod) string {
	if len(pods) == 0 {
		return "No pods found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAMESPACE | NAME | READY | STATUS | RESTARTS | AGE | NODE |\n")
	buf.WriteString("|-----------|------|-------|--------|----------|-----|------|\n")

	for _, pod := range pods {
		age := formatAge(pod.CreationTimestamp)
		ready := formatReady(pod.Status)
		node := pod.Spec.NodeName
		if node == "" {
			node = "<pending>"
		}

		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %d | %s | %s |\n",
			pod.Namespace,
			pod.Name,
			ready,
			pod.Status.Phase,
			containerRestartCount(pod.Status),
			age,
			node,
		)
	}

	return buf.String()
}

// formatPodsListJSON formats a list of pods as JSON.
func formatPodsListJSON(pods []corev1.Pod) (string, error) {
	summaries := make([]PodSummary, 0, len(pods))
	for _, pod := range pods {
		summaries = append(summaries, toPodSummary(pod))
	}

	result := struct {
		Pods []PodSummary `json:"pods"`
	}{
		Pods: summaries,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
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
	ready := formatReady(pod.Status)
	age := formatAge(pod.CreationTimestamp)
	node := pod.Spec.NodeName
	if node == "" {
		node = "<pending>"
	}

	return PodSummary{
		Namespace:       pod.Namespace,
		Name:            pod.Name,
		Ready:           ready,
		Status:          string(pod.Status.Phase),
		Restarts:        containerRestartCount(pod.Status),
		Age:             age,
		Node:            node,
		OwnerReferences: ownerReferences(&pod),
	}
}
