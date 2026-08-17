package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

// WorkloadDescribeResult represents the result of describing a workload.
type WorkloadDescribeResult struct {
	WorkloadDescribe
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
			return mcp.NewToolResultErrorf("failed to get %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
		}

		result := buildWorkloadDescribeResult(workload, resolvedKind)
		fallbackText := fmt.Sprintf("%s '%s' in namespace '%s' has %d/%d replicas ready/desired. Age: %s.",
			resolvedKind, result.Name, result.Namespace, result.Replicas.Ready, result.Replicas.Desired, result.Age)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildWorkloadDescribeResult builds a WorkloadDescribeResult from a workload object.
func buildWorkloadDescribeResult(workload any, kind string) *WorkloadDescribeResult {
	result := &WorkloadDescribeResult{}

	switch w := workload.(type) {
	case *appsv1.Deployment:
		result.Kind = kind
		result.Namespace = w.Namespace
		result.Name = w.Name
		result.Replicas = Replicas{
			Ready:   int(w.Status.ReadyReplicas),
			Desired: int(*w.Spec.Replicas),
		}
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.Service = w.Name
		result.UpdateStrategy = string(w.Spec.Strategy.Type)
		result.Conditions = buildConditions(w.Status.Conditions)
		result.UpdateHistory = int(w.Status.ObservedGeneration)
		result.RevisionHistory = int(*w.Spec.RevisionHistoryLimit)
		result.PodTemplate = buildPodTemplate(w.Spec)
		result.Age = formatAge(w.CreationTimestamp)

	case *appsv1.StatefulSet:
		result.Kind = kind
		result.Namespace = w.Namespace
		result.Name = w.Name
		result.Replicas = Replicas{
			Ready:   int(w.Status.ReadyReplicas),
			Desired: int(*w.Spec.Replicas),
		}
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.Service = w.Spec.ServiceName
		result.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
		result.Conditions = buildConditions(w.Status.Conditions)
		result.UpdateHistory = int(w.Status.ObservedGeneration)
		result.RevisionHistory = int(*w.Spec.RevisionHistoryLimit)
		result.PodTemplate = buildPodTemplate(w.Spec)
		result.Age = formatAge(w.CreationTimestamp)

	case *appsv1.DaemonSet:
		result.Kind = kind
		result.Namespace = w.Namespace
		result.Name = w.Name
		result.Replicas = Replicas{
			Ready:   int(w.Status.NumberReady),
			Desired: int(w.Status.DesiredNumberScheduled),
		}
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
		result.Conditions = buildConditions(w.Status.Conditions)
		result.UpdateHistory = int(w.Status.ObservedGeneration)
		result.RevisionHistory = int(*w.Spec.RevisionHistoryLimit)
		result.PodTemplate = buildPodTemplate(w.Spec)
		result.Age = formatAge(w.CreationTimestamp)
	}

	return result
}

// buildConditions builds a slice of ConditionInfo from workload conditions.
// This function handles all three workload types' condition types.
func buildConditions(conditions any) []ConditionInfo {
	switch c := conditions.(type) {
	case []appsv1.DeploymentCondition:
		result := make([]ConditionInfo, 0, len(c))
		for _, cond := range c {
			result = append(result, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}
		return result
	case []appsv1.StatefulSetCondition:
		result := make([]ConditionInfo, 0, len(c))
		for _, cond := range c {
			result = append(result, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}
		return result
	case []appsv1.DaemonSetCondition:
		result := make([]ConditionInfo, 0, len(c))
		for _, cond := range c {
			result = append(result, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}
		return result
	}
	return nil
}

// buildPodTemplate builds a PodTemplate from a pod template spec.
func buildPodTemplate(template any) PodTemplate {
	var result PodTemplate
	switch t := template.(type) {
	case appsv1.DeploymentSpec:
		result = PodTemplate{
			Labels:         t.Template.Labels,
			RestartPolicy:  string(t.Template.Spec.RestartPolicy),
			ServiceAccount: t.Template.Spec.ServiceAccountName,
			Containers:     extractContainerInfo(t.Template.Spec.Containers),
		}
	case appsv1.StatefulSetSpec:
		result = PodTemplate{
			Labels:         t.Template.Labels,
			RestartPolicy:  string(t.Template.Spec.RestartPolicy),
			ServiceAccount: t.Template.Spec.ServiceAccountName,
			Containers:     extractContainerInfo(t.Template.Spec.Containers),
		}
	case appsv1.DaemonSetSpec:
		result = PodTemplate{
			Labels:         t.Template.Labels,
			RestartPolicy:  string(t.Template.Spec.RestartPolicy),
			ServiceAccount: t.Template.Spec.ServiceAccountName,
			Containers:     extractContainerInfo(t.Template.Spec.Containers),
		}
	}
	return result
}
