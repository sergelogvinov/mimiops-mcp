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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterWorkloadsList adds the workloads_list tool, which lists Deployments,
// StatefulSets, or DaemonSets in a namespace (or all namespaces).
func RegisterWorkloadsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_list",
		mcp.WithDescription("List Deployments, StatefulSets, or DaemonSets in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		labelSelector := req.GetString("label_selector", "")
		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "workloads_list called",
			"namespace", namespace,
			"kind", kind,
			"label_selector", labelSelector,
		)

		var summaries []WorkloadSummary
		var err error

		if kind != "" {
			// List specific kind
			summaries, err = listWorkloadsByKindAndFormat(ctx, client, namespace, kind, labelSelector)
		} else {
			// List all kinds
			summaries, err = listAllWorkloads(ctx, client, namespace, labelSelector)
		}

		if err != nil {
			return mcp.NewToolResultErrorf("failed to list workloads in namespace '%s': %v", namespace, err), nil
		}

		result, err := formatWorkloadsList(summaries, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// listWorkloadsByKindAndFormat lists workloads of a specific kind.
func listWorkloadsByKindAndFormat(ctx context.Context, client *k8s.Client, namespace, kind string, labelSelector string) ([]WorkloadSummary, error) {
	opts := metav1.ListOptions{}
	if labelSelector != "" {
		opts.LabelSelector = labelSelector
	}

	var summaries []WorkloadSummary

	switch kind {
	case "deployment":
		deployments, err := client.AppsV1().Deployments(namespace).List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments: %v", err)
		}
		for _, d := range deployments.Items {
			summaries = append(summaries, toWorkloadSummaryDeployment(d))
		}
	case "statefulset":
		statefulsets, err := client.AppsV1().StatefulSets(namespace).List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list statefulsets: %v", err)
		}
		for _, s := range statefulsets.Items {
			summaries = append(summaries, toWorkloadSummaryStatefulSet(s))
		}
	case "daemonset":
		daemonsets, err := client.AppsV1().DaemonSets(namespace).List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list daemonsets: %v", err)
		}
		for _, d := range daemonsets.Items {
			summaries = append(summaries, toWorkloadSummaryDaemonSet(d))
		}
	}

	return summaries, nil
}

// formatWorkloadsList formats a list of workloads for MCP tool output.
func formatWorkloadsList(summaries []WorkloadSummary, format string) (string, error) {
	if format == "json" {
		return formatWorkloadsListJSON(summaries)
	}
	return formatWorkloadsListText(summaries), nil
}

// formatWorkloadsListText formats a list of workloads as a markdown table.
func formatWorkloadsListText(summaries []WorkloadSummary) string {
	if len(summaries) == 0 {
		return "No workloads found."
	}

	var buf bytes.Buffer
	buf.WriteString("| KIND | NAMESPACE | NAME | READY | DESIRED | AGE |\n")
	buf.WriteString("|------|-----------|------|-------|---------|-----|\n")

	for _, w := range summaries {
		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %d | %s |\n",
			w.Kind,
			w.Namespace,
			w.Name,
			w.Ready,
			w.Desired,
			w.Age,
		)
	}

	return buf.String()
}

// formatWorkloadsListJSON formats a list of workloads as JSON.
func formatWorkloadsListJSON(summaries []WorkloadSummary) (string, error) {
	result := struct {
		Workloads []WorkloadSummary `json:"workloads"`
	}{
		Workloads: summaries,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
