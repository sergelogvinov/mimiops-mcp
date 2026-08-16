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

// RegisterResourceQuotasGet adds the resourcequotas_get tool, which gets a single ResourceQuota's full spec and status.
func RegisterResourceQuotasGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("resourcequotas_get",
		mcp.WithDescription("Get a single ResourceQuota's full spec and status."),
		mcp.WithString("name", mcp.Description("resource quota name"), mcp.Required()),
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

		log.DebugContext(ctx, "resourcequotas_get called", "namespace", namespace, "name", name)

		// Get the resource quota
		quota, err := client.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get resource quota '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Format output
		result, err := formatResourceQuotaGet(quota, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatResourceQuotaGet formats a resource quota for MCP tool output.
func formatResourceQuotaGet(quota *corev1.ResourceQuota, format string) (string, error) {
	if format == "json" {
		return formatResourceQuotaGetJSON(quota)
	}
	return formatResourceQuotaGetText(quota), nil
}

// formatResourceQuotaGetText formats a resource quota's detailed information as key-value blocks.
func formatResourceQuotaGetText(quota *corev1.ResourceQuota) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", quota.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", quota.Namespace)
	fmt.Fprintf(&buf, "**Age:** %s\n", formatAge(quota.CreationTimestamp))

	// Spec Hard limits
	fmt.Fprintf(&buf, "\n### Spec Hard Limits\n\n")
	for key, val := range quota.Spec.Hard {
		fmt.Fprintf(&buf, "- **%s:** %s\n", key, val.String())
	}

	// Status Used
	fmt.Fprintf(&buf, "\n### Status Used\n\n")
	for key, val := range quota.Status.Used {
		fmt.Fprintf(&buf, "- **%s:** %s\n", key, val.String())
	}

	return buf.String()
}

// formatResourceQuotaGetJSON formats a resource quota as JSON.
func formatResourceQuotaGetJSON(quota *corev1.ResourceQuota) (string, error) {
	type ResourceQuotaInfo struct {
		Metadata map[string]any       `json:"metadata"`
		Spec     map[string]any       `json:"spec"`
		Status   map[string]any       `json:"status"`
		Summary  ResourceQuotaSummary `json:"summary"`
	}

	info := ResourceQuotaInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = quota.Name
	info.Metadata["namespace"] = quota.Namespace
	info.Metadata["uid"] = string(quota.UID)
	info.Metadata["creationTimestamp"] = quota.CreationTimestamp.String()
	info.Metadata["labels"] = quota.Labels
	info.Metadata["annotations"] = quota.Annotations

	// Spec
	info.Spec = make(map[string]any)
	info.Spec["hard"] = quota.Spec.Hard

	// Status
	info.Status = make(map[string]any)
	info.Status["hard"] = quota.Status.Hard
	info.Status["used"] = quota.Status.Used

	// Summary
	used := quota.Status.Hard
	usedStatus := quota.Status.Used

	info.Summary = ResourceQuotaSummary{
		Name:      quota.Name,
		Namespace: quota.Namespace,
		Age:       formatAge(quota.CreationTimestamp),
	}

	info.Summary.RequestsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU)
	info.Summary.RequestsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory)
	info.Summary.LimitsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU)
	info.Summary.LimitsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory)

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
