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

// RegisterResourceQuotasList adds the resourcequotas_list tool, which lists ResourceQuotas in a namespace (or all namespaces).
func RegisterResourceQuotasList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("resourcequotas_list",
		mcp.WithDescription("List ResourceQuotas in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "resourcequotas_list called", "namespace", namespace)

		// List resource quotas
		quotas, err := client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list resource quotas in namespace '%s': %v", namespace, err), nil
		}

		// Format output
		result, err := formatResourceQuotasList(quotas.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatResourceQuotasList formats a list of resource quotas for MCP tool output.
func formatResourceQuotasList(quotas []corev1.ResourceQuota, format string) (string, error) {
	if format == "json" {
		return formatResourceQuotasListJSON(quotas)
	}
	return formatResourceQuotasListText(quotas), nil
}

// formatResourceQuotasListText formats a list of resource quotas as a markdown table.
func formatResourceQuotasListText(quotas []corev1.ResourceQuota) string {
	if len(quotas) == 0 {
		return "No resource quotas found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAMESPACE | NAME | REQUESTS CPU | REQUESTS MEMORY | LIMITS CPU | LIMITS MEMORY | AGE |\n")
	buf.WriteString("|-----------|------|--------------|-----------------|------------|---------------|-----|\n")

	for _, quota := range quotas {
		namespace := quota.Namespace
		name := quota.Name
		age := formatAge(quota.CreationTimestamp)

		// Get used/hard values for CPU and memory
		used := quota.Status.Hard
		usedStatus := quota.Status.Used

		reqCPU := getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU)
		reqMem := getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory)
		limCPU := getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU)
		limMem := getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory)

		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s | %s |\n",
			namespace, name, reqCPU, reqMem, limCPU, limMem, age)
	}

	return buf.String()
}

// formatResourceQuotasListJSON formats a list of resource quotas as JSON.
func formatResourceQuotasListJSON(quotas []corev1.ResourceQuota) (string, error) {
	summaries := make([]ResourceQuotaSummary, 0, len(quotas))
	for _, quota := range quotas {
		used := quota.Status.Hard
		usedStatus := quota.Status.Used

		summary := ResourceQuotaSummary{
			Name:      quota.Name,
			Namespace: quota.Namespace,
			Age:       formatAge(quota.CreationTimestamp),
		}

		summary.RequestsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU)
		summary.RequestsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory)
		summary.LimitsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU)
		summary.LimitsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory)

		summaries = append(summaries, summary)
	}

	result := struct {
		ResourceQuotas []ResourceQuotaSummary `json:"resourcequotas"`
	}{
		ResourceQuotas: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
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

// ResourceQuotaSummary is the trimmed representation of a resource quota used by resourcequotas_list.
type ResourceQuotaSummary struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	RequestsCPU    string `json:"requests_cpu,omitempty"`
	RequestsMemory string `json:"requests_memory,omitempty"`
	LimitsCPU      string `json:"limits_cpu,omitempty"`
	LimitsMemory   string `json:"limits_memory,omitempty"`
	Age            string `json:"age"`
}
