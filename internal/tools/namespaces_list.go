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

// RegisterNamespacesList adds the namespaces_list tool, which lists all namespaces.
func RegisterNamespacesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("namespaces_list",
		mcp.WithDescription("List all namespaces."),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "namespaces_list called")

		// List namespaces
		namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list namespaces: %v", err), nil
		}

		// Format output
		result, err := formatNamespacesList(namespaces.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatNamespacesList formats a list of namespaces for MCP tool output.
func formatNamespacesList(namespaces []corev1.Namespace, format string) (string, error) {
	if format == "json" {
		return formatNamespacesListJSON(namespaces)
	}
	return formatNamespacesListText(namespaces), nil
}

// formatNamespacesListText formats a list of namespaces as a markdown table.
func formatNamespacesListText(namespaces []corev1.Namespace) string {
	if len(namespaces) == 0 {
		return "No namespaces found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAME | STATUS | AGE |\n")
	buf.WriteString("|------|--------|-----|\n")

	for _, ns := range namespaces {
		name := ns.Name
		status := string(ns.Status.Phase)
		age := formatAge(ns.CreationTimestamp)

		fmt.Fprintf(&buf, "| %s | %s | %s |\n", name, status, age)
	}

	return buf.String()
}

// formatNamespacesListJSON formats a list of namespaces as JSON.
func formatNamespacesListJSON(namespaces []corev1.Namespace) (string, error) {
	summaries := make([]NamespaceSummary, 0, len(namespaces))
	for _, ns := range namespaces {
		summaries = append(summaries, NamespaceSummary{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    formatAge(ns.CreationTimestamp),
		})
	}

	result := struct {
		Namespaces []NamespaceSummary `json:"namespaces"`
	}{
		Namespaces: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// NamespaceSummary is the trimmed representation of a namespace used by namespaces_list.
type NamespaceSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}
