package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// WorkloadDescribeResult represents the result of describing a workload.
type WorkloadDescribeResult struct {
	WorkloadSummary
	WorkloadSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
}

// RegisterWorkloadsDescribe adds the workloads_describe tool, which provides
// a rich structured summary of a workload: replicas, conditions, selector,
// strategy, update history.
func RegisterWorkloadsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_describe",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Workload"),
		mcp.WithDescription("Workload summary (replicas, conditions, selector, strategy, update history)."),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithOutputSchema[WorkloadDescribeResult](),
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

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		log.DebugContext(ctx, "workloads_describe called",
			"namespace", namespace,
			"name", name,
			"kind", kind,
		)

		// Resolve kind if not provided
		resolvedKind, err := resolveWorkloadKind(ctx, client, namespace, name, kind)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Get the workload
		workload, err := getWorkloadByKind(ctx, client, namespace, name, resolvedKind)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("%s '%s' not found in namespace '%s'", resolvedKind, name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
		}

		result := buildWorkloadDescribeResult(workload)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildWorkloadDescribeResult builds a WorkloadDescribeResult from a workload object.
func buildWorkloadDescribeResult(workload any) *WorkloadDescribeResult {
	result := &WorkloadDescribeResult{}

	switch w := workload.(type) {
	case *appsv1.Deployment:
		result.WorkloadSummary = toWorkloadSummaryDeployment(w)
		result.Labels = extractLabels(w.Labels)
		result.Annotations = extractAnnotations(w.Annotations)
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.Strategy.Type)

	case *appsv1.StatefulSet:
		result.WorkloadSummary = toWorkloadSummaryStatefulSet(w)
		result.Labels = extractLabels(w.Labels)
		result.Annotations = extractAnnotations(w.Annotations)
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)

	case *appsv1.DaemonSet:
		result.WorkloadSummary = toWorkloadSummaryDaemonSet(w)
		result.Labels = extractLabels(w.Labels)
		result.Annotations = extractAnnotations(w.Annotations)
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
	}

	return result
}
