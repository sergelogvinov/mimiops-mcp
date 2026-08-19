package tools

import (
	"context"
	"log/slog"
	"maps"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

// WorkloadGetResult represents the result of getting a workload.
type WorkloadGetResult struct {
	WorkloadSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the Workload"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the Workload"`
}

// RegisterWorkloadsGet adds the workloads_get tool, which gets a single workload's
// full spec and status (Deployment, StatefulSet, or DaemonSet).
func RegisterWorkloadsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Workload"),
		mcp.WithDescription("Get a single workload's full spec and status (Deployment, StatefulSet, or DaemonSet)"),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithOutputSchema[WorkloadGetResult](),
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

		log.DebugContext(ctx, "workloads_get called",
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
			return mcp.NewToolResultErrorf("failed to get %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
		}

		result := buildWorkloadGetResult(workload)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildWorkloadGetResult builds a WorkloadGetResult from a workload object.
func buildWorkloadGetResult(workload any) *WorkloadGetResult {
	result := &WorkloadGetResult{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	switch w := workload.(type) {
	case *appsv1.Deployment:
		result.WorkloadSummary = toWorkloadSummaryDeployment(*w)
		maps.Copy(result.Labels, w.Labels)
		maps.Copy(result.Annotations, w.Annotations)

		// result.Replicas = Replicas{
		// 	Ready:   int(w.Status.ReadyReplicas),
		// 	Desired: int(*w.Spec.Replicas),
		// }
		// result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		// result.Service = w.Name
		// result.Strategy = string(w.Spec.Strategy.Type)
		// result.Age = formatAge(w.CreationTimestamp)
		// result.Spec = buildSpecMap(w.Spec)
		// result.Status = buildStatusMap(w.Status)

	case *appsv1.StatefulSet:
		result.WorkloadSummary = toWorkloadSummaryStatefulSet(*w)
		maps.Copy(result.Labels, w.Labels)
		maps.Copy(result.Annotations, w.Annotations)

		// result.Replicas = Replicas{
		// 	Ready:   int(w.Status.ReadyReplicas),
		// 	Desired: int(*w.Spec.Replicas),
		// }
		// result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		// result.Service = w.Spec.ServiceName
		// result.Strategy = string(w.Spec.UpdateStrategy.Type)
		// result.Age = formatAge(w.CreationTimestamp)
		// result.Spec = buildSpecMap(w.Spec)
		// result.Status = buildStatusMap(w.Status)

	case *appsv1.DaemonSet:
		result.WorkloadSummary = toWorkloadSummaryDaemonSet(*w)
		maps.Copy(result.Labels, w.Labels)
		maps.Copy(result.Annotations, w.Annotations)

		// result.Replicas = Replicas{
		// 	Ready:   int(w.Status.NumberReady),
		// 	Desired: int(w.Status.DesiredNumberScheduled),
		// }
		// result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		// result.Strategy = string(w.Spec.UpdateStrategy.Type)
		// result.Age = formatAge(w.CreationTimestamp)
		// result.Spec = buildSpecMap(w.Spec)
		// result.Status = buildStatusMap(w.Status)
	}

	return result
}

// buildSpecMap builds a map representation of a workload spec.
func buildSpecMap(spec any) map[string]any {
	switch s := spec.(type) {
	case appsv1.DeploymentSpec:
		result := make(map[string]any)
		result["replicas"] = *s.Replicas
		result["selector"] = s.Selector
		result["template"] = s.Template
		result["strategy"] = s.Strategy
		result["minReadySeconds"] = s.MinReadySeconds
		result["progressDeadlineSeconds"] = s.ProgressDeadlineSeconds
		return result
	case appsv1.StatefulSetSpec:
		result := make(map[string]any)
		result["replicas"] = *s.Replicas
		result["selector"] = s.Selector
		result["template"] = s.Template
		result["serviceName"] = s.ServiceName
		result["updateStrategy"] = s.UpdateStrategy
		result["minReadySeconds"] = s.MinReadySeconds
		return result
	case appsv1.DaemonSetSpec:
		result := make(map[string]any)
		result["selector"] = s.Selector
		result["template"] = s.Template
		result["updateStrategy"] = s.UpdateStrategy
		result["minReadySeconds"] = s.MinReadySeconds
		return result
	}
	return nil
}

// buildStatusMap builds a map representation of a workload status.
func buildStatusMap(status any) map[string]any {
	switch s := status.(type) {
	case appsv1.DeploymentStatus:
		result := make(map[string]any)
		result["replicas"] = s.Replicas
		result["readyReplicas"] = s.ReadyReplicas
		result["updatedReplicas"] = s.UpdatedReplicas
		result["availableReplicas"] = s.AvailableReplicas
		result["conditions"] = s.Conditions
		result["collisionCount"] = s.CollisionCount
		return result
	case appsv1.StatefulSetStatus:
		result := make(map[string]any)
		result["replicas"] = s.Replicas
		result["readyReplicas"] = s.ReadyReplicas
		result["currentReplicas"] = s.CurrentReplicas
		result["updatedReplicas"] = s.UpdatedReplicas
		result["conditions"] = s.Conditions
		result["collisionCount"] = s.CollisionCount
		return result
	case appsv1.DaemonSetStatus:
		result := make(map[string]any)
		result["desiredNumberScheduled"] = s.DesiredNumberScheduled
		result["numberReady"] = s.NumberReady
		result["updatedNumberScheduled"] = s.UpdatedNumberScheduled
		result["numberUnavailable"] = s.NumberUnavailable
		result["conditions"] = s.Conditions
		return result
	}
	return nil
}
