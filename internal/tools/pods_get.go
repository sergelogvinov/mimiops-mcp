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

// RegisterPodsGet adds the pods_get tool, which gets a full pod spec and status.
func RegisterPodsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_get",
		mcp.WithDescription("Get full pod spec and status."),
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

		log.DebugContext(ctx, "pods_get called",
			"namespace", namespace,
			"pod", name,
		)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := formatPodDescribe(pod, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatPodDescribe formats a pod for MCP tool output.
func formatPodDescribe(pod *corev1.Pod, format string) (string, error) {
	if format == "json" {
		return formatPodDescribeJSON(pod)
	}
	return formatPodDescribeText(pod), nil
}

// formatPodDescribeText formats a pod's detailed information as key-value blocks.
func formatPodDescribeText(pod *corev1.Pod) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", pod.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", pod.Namespace)
	fmt.Fprintf(&buf, "**Node:** %s\n", pod.Spec.NodeName)
	fmt.Fprintf(&buf, "**Status:** %s\n", pod.Status.Phase)
	fmt.Fprintf(&buf, "**Ready:** %s\n", formatReady(pod.Status))
	fmt.Fprintf(&buf, "**Restarts:** %d\n", containerRestartCount(pod.Status))
	fmt.Fprintf(&buf, "**Age:** %s\n", formatAge(pod.CreationTimestamp.Time))

	// Containers
	fmt.Fprintf(&buf, "\n### Containers\n\n")
	for _, container := range pod.Spec.Containers {
		fmt.Fprintf(&buf, "- **%s**: image=%s\n", container.Name, container.Image)
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

	return buf.String()
}

// formatPodDescribeJSON formats a pod as JSON.
func formatPodDescribeJSON(pod *corev1.Pod) (string, error) {
	type PodInfo struct {
		Metadata map[string]any `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
		Summary  PodSummary     `json:"summary"`
	}

	info := PodInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = pod.Name
	info.Metadata["namespace"] = pod.Namespace
	info.Metadata["uid"] = string(pod.UID)
	info.Metadata["creationTimestamp"] = pod.CreationTimestamp.String()
	info.Metadata["labels"] = pod.Labels
	info.Metadata["annotations"] = pod.Annotations

	// Spec (simplified)
	info.Spec = make(map[string]any)
	info.Spec["nodeName"] = pod.Spec.NodeName
	info.Spec["containers"] = pod.Spec.Containers

	// Status (simplified)
	info.Status = make(map[string]any)
	info.Status["phase"] = pod.Status.Phase
	info.Status["restartCount"] = containerRestartCount(pod.Status)
	info.Status["conditions"] = pod.Status.Conditions
	info.Status["containerStatuses"] = pod.Status.ContainerStatuses

	// Summary
	info.Summary = toPodSummary(*pod)

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
