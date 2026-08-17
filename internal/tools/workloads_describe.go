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
	appsv1 "k8s.io/api/apps/v1"
)

// RegisterWorkloadsDescribe adds the workloads_describe tool, which provides
// a rich human-readable summary of a workload: replicas, conditions, selector,
// strategy, update history.
func RegisterWorkloadsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_describe",
		mcp.WithDescription("Rich human-readable summary of a workload: replicas, conditions, selector, strategy, update history."),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
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

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
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

		result, err := formatWorkloadsDescribe(workload, resolvedKind, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatWorkloadsDescribe formats a workload for MCP tool output.
func formatWorkloadsDescribe(workload any, kind string, format string) (string, error) {
	if format == "json" {
		return formatWorkloadsDescribeJSON(workload, kind)
	}
	return formatWorkloadsDescribeText(workload, kind), nil
}

// formatWorkloadsDescribeText formats a workload's detailed information as key-value blocks.
//
//nolint:gocyclo,cyclop
func formatWorkloadsDescribeText(workload any, kind string) string {
	var buf bytes.Buffer

	switch w := workload.(type) {
	case *appsv1.Deployment:
		fmt.Fprintf(&buf, "**KIND:** %s\n", kind)
		fmt.Fprintf(&buf, "**NAME:** %s\n", w.Name)
		fmt.Fprintf(&buf, "**NAMESPACE:** %s\n", w.Namespace)
		fmt.Fprintf(&buf, "**REPLICAS:** %s\n", formatDeploymentReady(*w))
		fmt.Fprintf(&buf, "**SELECTOR:** %s\n", formatMatchLabels(w.Spec.Selector.MatchLabels))
		fmt.Fprintf(&buf, "**UPDATE STRATEGY:** %s\n", w.Spec.Strategy.Type)
		if w.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && w.Spec.Strategy.RollingUpdate != nil {
			fmt.Fprintf(&buf, "  - Max Surge: %v\n", w.Spec.Strategy.RollingUpdate.MaxSurge)
			fmt.Fprintf(&buf, "  - Max Unavailable: %v\n", w.Spec.Strategy.RollingUpdate.MaxUnavailable)
		}
		fmt.Fprintf(&buf, "**UPDATE HISTORY:** %d\n", w.Status.ObservedGeneration)
		fmt.Fprintf(&buf, "**REVISION HISTORY LIMIT:** %d\n", w.Spec.RevisionHistoryLimit)
		fmt.Fprintf(&buf, "**AGE:** %s\n", formatAge(w.CreationTimestamp))

		// Conditions
		if len(w.Status.Conditions) > 0 {
			fmt.Fprintf(&buf, "\n### Conditions\n\n")
			for _, cond := range w.Status.Conditions {
				fmt.Fprintf(&buf, "- **%s**: %s", cond.Type, cond.Status)
				if cond.Reason != "" {
					fmt.Fprintf(&buf, " (%s)", cond.Reason)
				}
				fmt.Fprintf(&buf, "\n")
			}
		}

		// Pod Template
		fmt.Fprintf(&buf, "\n### Pod Template\n\n")
		fmt.Fprintf(&buf, "- **Labels:** %s\n", formatMatchLabels(w.Spec.Template.Labels))
		fmt.Fprintf(&buf, "- **Restart Policy:** %s\n", w.Spec.Template.Spec.RestartPolicy)
		if w.Spec.Template.Spec.ServiceAccountName != "" {
			fmt.Fprintf(&buf, "- **Service Account:** %s\n", w.Spec.Template.Spec.ServiceAccountName)
		}

		// Containers
		fmt.Fprintf(&buf, "\n#### Containers\n\n")
		for _, container := range w.Spec.Template.Spec.Containers {
			fmt.Fprintf(&buf, "- **%s**: image=%s", container.Name, container.Image)
			if len(container.Ports) > 0 {
				ports := make([]string, len(container.Ports))
				for i, p := range container.Ports {
					ports[i] = fmt.Sprintf("%d", p.ContainerPort)
				}
				fmt.Fprintf(&buf, ", ports=%s", joinStrings(ports))
			}
			if len(container.Args) > 0 {
				fmt.Fprintf(&buf, ", args=%s", joinStrings(container.Args))
			}
			fmt.Fprintf(&buf, "\n")
		}

	case *appsv1.StatefulSet:
		fmt.Fprintf(&buf, "**KIND:** %s\n", kind)
		fmt.Fprintf(&buf, "**NAME:** %s\n", w.Name)
		fmt.Fprintf(&buf, "**NAMESPACE:** %s\n", w.Namespace)
		fmt.Fprintf(&buf, "**REPLICAS:** %s\n", formatStatefulSetReady(*w))
		fmt.Fprintf(&buf, "**SELECTOR:** %s\n", formatMatchLabels(w.Spec.Selector.MatchLabels))
		fmt.Fprintf(&buf, "**SERVICE:** %s\n", w.Spec.ServiceName)
		fmt.Fprintf(&buf, "**UPDATE STRATEGY:** %s\n", w.Spec.UpdateStrategy.Type)
		if w.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType && w.Spec.UpdateStrategy.RollingUpdate != nil {
			fmt.Fprintf(&buf, "  - Partition: %d\n", w.Spec.UpdateStrategy.RollingUpdate.Partition)
		}
		fmt.Fprintf(&buf, "**UPDATE HISTORY:** %d\n", w.Status.ObservedGeneration)
		fmt.Fprintf(&buf, "**REVISION HISTORY LIMIT:** %d\n", w.Spec.RevisionHistoryLimit)
		fmt.Fprintf(&buf, "**AGE:** %s\n", formatAge(w.CreationTimestamp))

		// Conditions
		if len(w.Status.Conditions) > 0 {
			fmt.Fprintf(&buf, "\n### Conditions\n\n")
			for _, cond := range w.Status.Conditions {
				fmt.Fprintf(&buf, "- **%s**: %s", cond.Type, cond.Status)
				if cond.Reason != "" {
					fmt.Fprintf(&buf, " (%s)", cond.Reason)
				}
				fmt.Fprintf(&buf, "\n")
			}
		}

		// Pod Template
		fmt.Fprintf(&buf, "\n### Pod Template\n\n")
		fmt.Fprintf(&buf, "- **Labels:** %s\n", formatMatchLabels(w.Spec.Template.Labels))
		fmt.Fprintf(&buf, "- **Restart Policy:** %s\n", w.Spec.Template.Spec.RestartPolicy)
		if w.Spec.Template.Spec.ServiceAccountName != "" {
			fmt.Fprintf(&buf, "- **Service Account:** %s\n", w.Spec.Template.Spec.ServiceAccountName)
		}

		// Containers
		fmt.Fprintf(&buf, "\n#### Containers\n\n")
		for _, container := range w.Spec.Template.Spec.Containers {
			fmt.Fprintf(&buf, "- **%s**: image=%s", container.Name, container.Image)
			if len(container.Ports) > 0 {
				ports := make([]string, len(container.Ports))
				for i, p := range container.Ports {
					ports[i] = fmt.Sprintf("%d", p.ContainerPort)
				}
				fmt.Fprintf(&buf, ", ports=%s", joinStrings(ports))
			}
			if len(container.Args) > 0 {
				fmt.Fprintf(&buf, ", args=%s", joinStrings(container.Args))
			}
			fmt.Fprintf(&buf, "\n")
		}

	case *appsv1.DaemonSet:
		fmt.Fprintf(&buf, "**KIND:** %s\n", kind)
		fmt.Fprintf(&buf, "**NAME:** %s\n", w.Name)
		fmt.Fprintf(&buf, "**NAMESPACE:** %s\n", w.Namespace)
		fmt.Fprintf(&buf, "**REPLICAS:** %s\n", formatDaemonSetReady(*w))
		fmt.Fprintf(&buf, "**SELECTOR:** %s\n", formatMatchLabels(w.Spec.Selector.MatchLabels))
		fmt.Fprintf(&buf, "**UPDATE STRATEGY:** %s\n", w.Spec.UpdateStrategy.Type)
		if w.Spec.UpdateStrategy.Type == appsv1.RollingUpdateDaemonSetStrategyType && w.Spec.UpdateStrategy.RollingUpdate != nil {
			fmt.Fprintf(&buf, "  - Max Unavailable: %v\n", w.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
		}
		fmt.Fprintf(&buf, "**UPDATE HISTORY:** %d\n", w.Status.ObservedGeneration)
		fmt.Fprintf(&buf, "**REVISION HISTORY LIMIT:** %d\n", w.Spec.RevisionHistoryLimit)
		fmt.Fprintf(&buf, "**AGE:** %s\n", formatAge(w.CreationTimestamp))

		// Conditions
		if len(w.Status.Conditions) > 0 {
			fmt.Fprintf(&buf, "\n### Conditions\n\n")
			for _, cond := range w.Status.Conditions {
				fmt.Fprintf(&buf, "- **%s**: %s", cond.Type, cond.Status)
				if cond.Reason != "" {
					fmt.Fprintf(&buf, " (%s)", cond.Reason)
				}
				fmt.Fprintf(&buf, "\n")
			}
		}

		// Pod Template
		fmt.Fprintf(&buf, "\n### Pod Template\n\n")
		fmt.Fprintf(&buf, "- **Labels:** %s\n", formatMatchLabels(w.Spec.Template.Labels))
		fmt.Fprintf(&buf, "- **Restart Policy:** %s\n", w.Spec.Template.Spec.RestartPolicy)
		if w.Spec.Template.Spec.ServiceAccountName != "" {
			fmt.Fprintf(&buf, "- **Service Account:** %s\n", w.Spec.Template.Spec.ServiceAccountName)
		}

		// Containers
		fmt.Fprintf(&buf, "\n#### Containers\n\n")
		for _, container := range w.Spec.Template.Spec.Containers {
			fmt.Fprintf(&buf, "- **%s**: image=%s", container.Name, container.Image)
			if len(container.Ports) > 0 {
				ports := make([]string, len(container.Ports))
				for i, p := range container.Ports {
					ports[i] = fmt.Sprintf("%d", p.ContainerPort)
				}
				fmt.Fprintf(&buf, ", ports=%s", joinStrings(ports))
			}
			if len(container.Args) > 0 {
				fmt.Fprintf(&buf, ", args=%s", joinStrings(container.Args))
			}
			fmt.Fprintf(&buf, "\n")
		}
	}

	return buf.String()
}

// formatWorkloadsDescribeJSON formats a workload as JSON.
func formatWorkloadsDescribeJSON(workload any, _ string) (string, error) {
	var describe WorkloadDescribe

	switch w := workload.(type) {
	case *appsv1.Deployment:
		describe = buildWorkloadDescribe("deployment", w)
	case *appsv1.StatefulSet:
		describe = buildWorkloadDescribe("statefulset", w)
	case *appsv1.DaemonSet:
		describe = buildWorkloadDescribe("daemonset", w)
	}

	data, err := json.MarshalIndent(describe, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// buildWorkloadDescribe builds a WorkloadDescribe from a workload object.
func buildWorkloadDescribe(kind string, workload any) WorkloadDescribe {
	var describe WorkloadDescribe

	switch w := workload.(type) {
	case *appsv1.Deployment:
		describe.Kind = kind
		describe.Namespace = w.Namespace
		describe.Name = w.Name
		describe.Replicas = Replicas{
			Ready:   int(w.Status.ReadyReplicas),
			Desired: int(*w.Spec.Replicas),
		}
		describe.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		describe.Service = w.Name
		describe.UpdateStrategy = string(w.Spec.Strategy.Type)
		describe.Conditions = buildConditions(w.Status.Conditions)
		describe.UpdateHistory = int(w.Status.ObservedGeneration)
		describe.RevisionHistory = int(*w.Spec.RevisionHistoryLimit)
		describe.PodTemplate = buildPodTemplate(w.Spec)
		describe.Age = formatAge(w.CreationTimestamp)

	case *appsv1.StatefulSet:
		describe.Kind = kind
		describe.Namespace = w.Namespace
		describe.Name = w.Name
		describe.Replicas = Replicas{
			Ready:   int(w.Status.ReadyReplicas),
			Desired: int(*w.Spec.Replicas),
		}
		describe.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		describe.Service = w.Spec.ServiceName
		describe.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
		describe.Conditions = buildConditions(w.Status.Conditions)
		describe.UpdateHistory = int(w.Status.ObservedGeneration)
		describe.RevisionHistory = int(*w.Spec.RevisionHistoryLimit)
		describe.PodTemplate = buildPodTemplate(w.Spec)
		describe.Age = formatAge(w.CreationTimestamp)

	case *appsv1.DaemonSet:
		describe.Kind = kind
		describe.Namespace = w.Namespace
		describe.Name = w.Name
		describe.Replicas = Replicas{
			Ready:   int(w.Status.NumberReady),
			Desired: int(w.Status.DesiredNumberScheduled),
		}
		describe.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		describe.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
		describe.Conditions = buildConditions(w.Status.Conditions)
		describe.UpdateHistory = int(w.Status.ObservedGeneration)
		describe.RevisionHistory = int(*w.Spec.RevisionHistoryLimit)
		describe.PodTemplate = buildPodTemplate(w.Spec)
		describe.Age = formatAge(w.CreationTimestamp)
	}

	return describe
}

// buildConditions builds a slice of Condition from workload conditions.
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
