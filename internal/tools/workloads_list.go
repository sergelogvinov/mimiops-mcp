package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadsListResult represents the result of listing workloads.
type WorkloadsListResult struct {
	Workloads []WorkloadSummary `json:"workloads" jsonschema:"List of workloads"`
}

// RegisterWorkloadsList adds the workloads_list tool, which lists Deployments,
// StatefulSets, or DaemonSets in a namespace (or all namespaces).
func RegisterWorkloadsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Workloads"),
		mcp.WithDescription("List Deployments, StatefulSets, or DaemonSets in a namespace (or all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithOutputSchema[WorkloadsListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		labelSelector := req.GetString("label_selector", "")

		log.DebugContext(ctx, "workloads_list called",
			"namespace", namespace,
			"kind", kind,
			"label_selector", labelSelector,
		)

		var summaries []WorkloadSummary
		var err error

		if kind != "" {
			summaries, err = listWorkloadsByKind(ctx, client, namespace, kind, labelSelector)
		} else {
			summaries, err = listAllWorkloads(ctx, client, namespace, labelSelector)
		}
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list workloads in namespace '%s': %v", namespace, err), nil
		}

		result := WorkloadsListResult{
			Workloads: summaries,
		}

		// Build fallback text
		fallbackText := "No workloads found"
		if len(result.Workloads) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
