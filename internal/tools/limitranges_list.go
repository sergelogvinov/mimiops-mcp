package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterLimitRangesList adds the limitranges_list tool, which lists LimitRanges in a namespace (or all namespaces).
func RegisterLimitRangesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("limitranges_list",
		mcp.WithDescription("List LimitRanges in a namespace (or all namespaces)."),
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

		log.DebugContext(ctx, "limitranges_list called", "namespace", namespace)

		// List limit ranges
		ranges, err := client.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list limit ranges in namespace '%s': %v", namespace, err), nil
		}

		// Format output
		result, err := formatLimitRangesList(ranges.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatLimitRangesList formats a list of limit ranges for MCP tool output.
func formatLimitRangesList(ranges []corev1.LimitRange, format string) (string, error) {
	if format == "json" {
		return formatLimitRangesListJSON(ranges)
	}
	return formatLimitRangesListText(ranges), nil
}

// formatLimitRangesListText formats a list of limit ranges as a markdown table.
func formatLimitRangesListText(ranges []corev1.LimitRange) string {
	if len(ranges) == 0 {
		return "No limit ranges found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAMESPACE | NAME | TYPES | AGE |\n")
	buf.WriteString("|-----------|------|-------|-----|\n")

	for _, lr := range ranges {
		namespace := lr.Namespace
		name := lr.Name
		age := formatAge(lr.CreationTimestamp)
		types := deriveLimitRangeTypes(&lr)

		fmt.Fprintf(&buf, "| %s | %s | %s | %s |\n", namespace, name, types, age)
	}

	return buf.String()
}

// formatLimitRangesListJSON formats a list of limit ranges as JSON.
func formatLimitRangesListJSON(ranges []corev1.LimitRange) (string, error) {
	summaries := make([]LimitRangeSummary, 0, len(ranges))
	for _, lr := range ranges {
		summary := LimitRangeSummary{
			Name:      lr.Name,
			Namespace: lr.Namespace,
			Types:     deriveLimitRangeTypes(&lr),
			Age:       formatAge(lr.CreationTimestamp),
		}
		summaries = append(summaries, summary)
	}

	result := struct {
		LimitRanges []LimitRangeSummary `json:"limitranges"`
	}{
		LimitRanges: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// deriveLimitRangeTypes derives the resource types from spec.limits.
func deriveLimitRangeTypes(lr *corev1.LimitRange) string {
	types := make([]string, 0)
	for _, limit := range lr.Spec.Limits {
		if limit.Type != "" {
			types = append(types, string(limit.Type))
		}
	}
	if len(types) == 0 {
		return "none"
	}
	return strings.Join(types, ", ")
}

// LimitRangeSummary is the trimmed representation of a limit range used by limitranges_list.
type LimitRangeSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Types     string `json:"types"`
	Age       string `json:"age"`
}
