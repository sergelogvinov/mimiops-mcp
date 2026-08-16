package tools

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterPodsDescribe adds the pods_describe tool, which provides a human-readable pod summary.
func RegisterPodsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_describe",
		mcp.WithDescription("Human-readable pod summary (conditions, container statuses, node, tolerations)."),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
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

		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "pods_describe called",
			"namespace", namespace,
			"pod", name,
		)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := formatPodDescribeWithEvents(pod, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatPodDescribeWithEvents formats a pod's detailed information including events.
func formatPodDescribeWithEvents(pod *corev1.Pod, format string) (string, error) {
	if format == "json" {
		return formatPodDescribeJSON(pod)
	}
	return formatPodDescribeTextWithEvents(pod), nil
}

// formatPodDescribeTextWithEvents formats a pod's detailed information as key-value blocks with events.
func formatPodDescribeTextWithEvents(pod *corev1.Pod) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", pod.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", pod.Namespace)
	fmt.Fprintf(&buf, "**Node:** %s\n", pod.Spec.NodeName)
	fmt.Fprintf(&buf, "**Status:** %s\n", pod.Status.Phase)
	fmt.Fprintf(&buf, "**Ready:** %s\n", formatReady(pod.Status))
	fmt.Fprintf(&buf, "**Restarts:** %d\n", containerRestartCount(pod.Status))
	fmt.Fprintf(&buf, "**Age:** %s\n", formatAge(pod.CreationTimestamp))

	// Containers
	fmt.Fprintf(&buf, "\n### Containers\n\n")
	for _, container := range pod.Spec.Containers {
		fmt.Fprintf(&buf, "- **%s**: image=%s\n", container.Name, container.Image)
	}

	// Container Statuses
	fmt.Fprintf(&buf, "\n### Container Statuses\n\n")
	for _, cs := range pod.Status.ContainerStatuses {
		fmt.Fprintf(&buf, "- **%s**:\n", cs.Name)
		fmt.Fprintf(&buf, "  - Image: %s\n", cs.Image)
		fmt.Fprintf(&buf, "  - Ready: %v\n", cs.Ready)
		fmt.Fprintf(&buf, "  - Restart Count: %d\n", cs.RestartCount)
		switch {
		case cs.State.Waiting != nil:
			fmt.Fprintf(&buf, "  - State: Waiting (%s): %s\n", cs.State.Waiting.Reason, cs.State.Waiting.Message)
		case cs.State.Terminated != nil:
			fmt.Fprintf(&buf, "  - State: Terminated (%s): %s\n", cs.State.Terminated.Reason, cs.State.Terminated.Message)
		case cs.State.Running != nil:
			fmt.Fprintf(&buf, "  - State: Running\n")
		}
	}

	// Conditions
	if len(pod.Status.Conditions) > 0 {
		fmt.Fprintf(&buf, "\n### Conditions\n\n")
		for _, cond := range pod.Status.Conditions {
			fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
		}
	}

	// Owner References
	if len(pod.OwnerReferences) > 0 {
		fmt.Fprintf(&buf, "\n### Owner References\n\n")
		for _, ref := range pod.OwnerReferences {
			fmt.Fprintf(&buf, "- %s/%s\n", ref.Kind, ref.Name)
		}
	}

	// Events (simplified - just count for now, can be expanded with event listing)
	fmt.Fprintf(&buf, "\n### Events\n\n")
	fmt.Fprintf(&buf, "Events are available for this pod. Use `pods_events` to list events.\n")

	return buf.String()
}
