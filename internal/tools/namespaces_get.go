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

// RegisterNamespacesGet adds the namespaces_get tool, which gets a single namespace's full spec and status.
func RegisterNamespacesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("namespaces_get",
		mcp.WithDescription("Get a single namespace's full spec and status."),
		mcp.WithString("name", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "namespaces_get called", "namespace", name)

		// Get the namespace
		ns, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get namespace '%s': %v", name, err), nil
		}

		// Format output
		result, err := formatNamespaceGet(ns, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatNamespaceGet formats a namespace for MCP tool output.
func formatNamespaceGet(ns *corev1.Namespace, format string) (string, error) {
	if format == "json" {
		return formatNamespaceGetJSON(ns)
	}
	return formatNamespaceGetText(ns), nil
}

// formatNamespaceGetText formats a namespace's detailed information as key-value blocks.
func formatNamespaceGetText(ns *corev1.Namespace) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", ns.Name)
	fmt.Fprintf(&buf, "**Status:** %s\n", ns.Status.Phase)

	// Labels
	if len(ns.Labels) > 0 {
		fmt.Fprintf(&buf, "\n### Labels\n\n")
		for k, v := range ns.Labels {
			fmt.Fprintf(&buf, "- **%s:** %s\n", k, v)
		}
	}

	// Annotations
	if len(ns.Annotations) > 0 {
		fmt.Fprintf(&buf, "\n### Annotations\n\n")
		for k, v := range ns.Annotations {
			fmt.Fprintf(&buf, "- **%s:** %s\n", k, v)
		}
	}

	// Finalizers
	if len(ns.Finalizers) > 0 {
		fmt.Fprintf(&buf, "\n### Finalizers\n\n")
		for _, f := range ns.Finalizers {
			fmt.Fprintf(&buf, "- %s\n", f)
		}
	}

	// Conditions
	if len(ns.Status.Conditions) > 0 {
		fmt.Fprintf(&buf, "\n### Conditions\n\n")
		for _, cond := range ns.Status.Conditions {
			fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
		}
	}

	fmt.Fprintf(&buf, "\n**Age:** %s\n", formatAge(ns.CreationTimestamp))

	return buf.String()
}

// formatNamespaceGetJSON formats a namespace as JSON.
func formatNamespaceGetJSON(ns *corev1.Namespace) (string, error) {
	type NamespaceInfo struct {
		Metadata map[string]any   `json:"metadata"`
		Spec     map[string]any   `json:"spec"`
		Status   map[string]any   `json:"status"`
		Summary  NamespaceSummary `json:"summary"`
	}

	info := NamespaceInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = ns.Name
	info.Metadata["uid"] = string(ns.UID)
	info.Metadata["creationTimestamp"] = ns.CreationTimestamp.String()
	info.Metadata["labels"] = ns.Labels
	info.Metadata["annotations"] = ns.Annotations

	// Spec (simplified)
	info.Spec = make(map[string]any)

	// Status (simplified)
	info.Status = make(map[string]any)
	info.Status["phase"] = ns.Status.Phase
	info.Status["conditions"] = ns.Status.Conditions
	info.Status["finalizers"] = ns.Finalizers

	// Summary
	info.Summary = NamespaceSummary{
		Name:   ns.Name,
		Status: string(ns.Status.Phase),
		Age:    formatAge(ns.CreationTimestamp),
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
