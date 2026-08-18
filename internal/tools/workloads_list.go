package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		var fallbackText string
		switch len(result.Workloads) {
		case 0:
			fallbackText = "No workloads found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 workload: %s (%s) in namespace %s", result.Workloads[0].Name, result.Workloads[0].Kind, result.Workloads[0].Namespace)
		default:
			fallbackText = fmt.Sprintf("Found %d workloads", len(result.Workloads))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// listWorkloadsByKind lists workloads of a specific kind.
func listWorkloadsByKind(ctx context.Context, client *k8s.Client, namespace, kind string, labelSelector string) (summaries []WorkloadSummary, err error) {
	opts := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	switch kind {
	case "deployment":
		deployments, err := client.AppsV1().Deployments(namespace).List(ctx, opts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no deployments found")
			}
			return nil, fmt.Errorf("failed to list deployments: %v", err)
		}
		for _, d := range deployments.Items {
			summaries = append(summaries, toWorkloadSummaryDeployment(d))
		}
	case "statefulset":
		statefulsets, err := client.AppsV1().StatefulSets(namespace).List(ctx, opts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no statefulsets found")
			}
			return nil, fmt.Errorf("failed to list statefulsets: %v", err)
		}
		for _, s := range statefulsets.Items {
			summaries = append(summaries, toWorkloadSummaryStatefulSet(s))
		}
	case "daemonset":
		daemonsets, err := client.AppsV1().DaemonSets(namespace).List(ctx, opts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no daemonsets found")
			}
			return nil, fmt.Errorf("failed to list daemonsets: %v", err)
		}
		for _, d := range daemonsets.Items {
			summaries = append(summaries, toWorkloadSummaryDaemonSet(d))
		}
	}

	return summaries, nil
}
