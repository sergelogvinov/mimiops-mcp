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

// RegisterLimitRangesGet adds the limitranges_get tool, which gets a single LimitRange's full spec.
func RegisterLimitRangesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("limitranges_get",
		mcp.WithDescription("Get a single LimitRange's full spec."),
		mcp.WithString("name", mcp.Description("limit range name"), mcp.Required()),
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

		log.DebugContext(ctx, "limitranges_get called", "namespace", namespace, "name", name)

		// Get the limit range
		lr, err := client.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get limit range '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Format output
		result, err := formatLimitRangeGet(lr, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatLimitRangeGet formats a limit range for MCP tool output.
func formatLimitRangeGet(lr *corev1.LimitRange, format string) (string, error) {
	if format == "json" {
		return formatLimitRangeGetJSON(lr)
	}
	return formatLimitRangeGetText(lr), nil
}

// formatLimitRangeGetText formats a limit range's detailed information as key-value blocks.
func formatLimitRangeGetText(lr *corev1.LimitRange) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", lr.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", lr.Namespace)
	fmt.Fprintf(&buf, "**Age:** %s\n", formatAge(lr.CreationTimestamp))

	// Spec Limits
	fmt.Fprintf(&buf, "\n### Spec Limits\n\n")
	for _, limit := range lr.Spec.Limits {
		fmt.Fprintf(&buf, "- **Type:** %s\n", limit.Type)
		if len(limit.Min) > 0 {
			fmt.Fprintf(&buf, "  - **Min:**")
			for key, val := range limit.Min {
				fmt.Fprintf(&buf, " %s=%s", key, val.String())
			}
			fmt.Fprintf(&buf, "\n")
		}
		if len(limit.Max) > 0 {
			fmt.Fprintf(&buf, "  - **Max:**")
			for key, val := range limit.Max {
				fmt.Fprintf(&buf, " %s=%s", key, val.String())
			}
			fmt.Fprintf(&buf, "\n")
		}
		if len(limit.Default) > 0 {
			fmt.Fprintf(&buf, "  - **Default:**")
			for key, val := range limit.Default {
				fmt.Fprintf(&buf, " %s=%s", key, val.String())
			}
			fmt.Fprintf(&buf, "\n")
		}
		if len(limit.DefaultRequest) > 0 {
			fmt.Fprintf(&buf, "  - **DefaultRequest:**")
			for key, val := range limit.DefaultRequest {
				fmt.Fprintf(&buf, " %s=%s", key, val.String())
			}
			fmt.Fprintf(&buf, "\n")
		}
		if limit.MaxLimitRequestRatio != nil {
			fmt.Fprintf(&buf, "  - **MaxLimitRequestRatio:**")
			for key, val := range limit.MaxLimitRequestRatio {
				fmt.Fprintf(&buf, " %s=%s", key, val.String())
			}
			fmt.Fprintf(&buf, "\n")
		}
	}

	return buf.String()
}

// formatLimitRangeGetJSON formats a limit range as JSON.
func formatLimitRangeGetJSON(lr *corev1.LimitRange) (string, error) {
	type LimitRangeInfo struct {
		Metadata map[string]any    `json:"metadata"`
		Spec     map[string]any    `json:"spec"`
		Summary  LimitRangeSummary `json:"summary"`
	}

	info := LimitRangeInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = lr.Name
	info.Metadata["namespace"] = lr.Namespace
	info.Metadata["uid"] = string(lr.UID)
	info.Metadata["creationTimestamp"] = lr.CreationTimestamp.String()
	info.Metadata["labels"] = lr.Labels
	info.Metadata["annotations"] = lr.Annotations

	// Spec
	info.Spec = make(map[string]any)
	info.Spec["limits"] = lr.Spec.Limits

	// Summary
	info.Summary = LimitRangeSummary{
		Name:      lr.Name,
		Namespace: lr.Namespace,
		Types:     deriveLimitRangeTypes(lr),
		Age:       formatAge(lr.CreationTimestamp),
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
