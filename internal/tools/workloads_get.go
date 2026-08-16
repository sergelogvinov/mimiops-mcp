package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

// RegisterWorkloadsGet adds the workloads_get tool, which gets a single workload's
// full spec and status (Deployment, StatefulSet, or DaemonSet).
func RegisterWorkloadsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_get",
		mcp.WithDescription("Get a single workload's full spec and status (Deployment, StatefulSet, or DaemonSet)."),
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

		result, err := formatWorkloadGet(workload, resolvedKind, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatWorkloadGet formats a workload for MCP tool output.
func formatWorkloadGet(workload any, kind string, format string) (string, error) {
	if format == "json" {
		return formatWorkloadGetJSON(workload, kind)
	}
	return formatWorkloadGetText(workload, kind), nil
}

// formatWorkloadGetText formats a workload's detailed information as key-value blocks.
func formatWorkloadGetText(workload any, kind string) string {
	var buf bytes.Buffer

	switch w := workload.(type) {
	case *appsv1.Deployment:
		fmt.Fprintf(&buf, "**KIND:** %s\n", kind)
		fmt.Fprintf(&buf, "**NAME:** %s\n", w.Name)
		fmt.Fprintf(&buf, "**NAMESPACE:** %s\n", w.Namespace)
		fmt.Fprintf(&buf, "**SERVICE:** %s\n", w.Name) // Service name typically matches deployment name
		fmt.Fprintf(&buf, "**REPLICAS:** %s\n", formatDeploymentReady(*w))
		fmt.Fprintf(&buf, "**SELECTOR:** %s\n", formatMatchLabels(w.Spec.Selector.MatchLabels))
		fmt.Fprintf(&buf, "**UPDATE STRATEGY:** %s\n", w.Spec.Strategy.Type)
		if w.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
			fmt.Fprintf(&buf, "  - Rolling Update:\n")
			if w.Spec.Strategy.RollingUpdate != nil {
				if w.Spec.Strategy.RollingUpdate.MaxSurge != nil {
					fmt.Fprintf(&buf, "    - Max Surge: %v\n", w.Spec.Strategy.RollingUpdate.MaxSurge)
				}
				if w.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
					fmt.Fprintf(&buf, "    - Max Unavailable: %v\n", w.Spec.Strategy.RollingUpdate.MaxUnavailable)
				}
			}
		}
		fmt.Fprintf(&buf, "**AGE:** %s\n", formatAge(w.CreationTimestamp))

		// Containers
		buf.WriteString("\n### Containers\n\n")
		for _, container := range w.Spec.Template.Spec.Containers {
			fmt.Fprintf(&buf, "- **%s**: image=%s", container.Name, container.Image)
			if len(container.Ports) > 0 {
				ports := make([]string, len(container.Ports))
				for i, p := range container.Ports {
					ports[i] = fmt.Sprintf("%d", p.ContainerPort)
				}
				fmt.Fprintf(&buf, ", ports=%s", joinStrings(ports))
			}
			fmt.Fprintf(&buf, "\n")
		}

		// Conditions
		if len(w.Status.Conditions) > 0 {
			fmt.Fprintf(&buf, "\n### Conditions\n\n")
			for _, cond := range w.Status.Conditions {
				fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
			}
		}

	case *appsv1.StatefulSet:
		fmt.Fprintf(&buf, "**KIND:** %s\n", kind)
		fmt.Fprintf(&buf, "**NAME:** %s\n", w.Name)
		fmt.Fprintf(&buf, "**NAMESPACE:** %s\n", w.Namespace)
		fmt.Fprintf(&buf, "**SERVICE:** %s\n", w.Spec.ServiceName)
		fmt.Fprintf(&buf, "**REPLICAS:** %s\n", formatStatefulSetReady(*w))
		fmt.Fprintf(&buf, "**SELECTOR:** %s\n", formatMatchLabels(w.Spec.Selector.MatchLabels))
		fmt.Fprintf(&buf, "**UPDATE STRATEGY:** %s\n", w.Spec.UpdateStrategy.Type)
		if w.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType {
			fmt.Fprintf(&buf, "  - Rolling Update:\n")
			if w.Spec.UpdateStrategy.RollingUpdate != nil {
				fmt.Fprintf(&buf, "    - Partition: %d\n", w.Spec.UpdateStrategy.RollingUpdate.Partition)
			}
		}
		fmt.Fprintf(&buf, "**AGE:** %s\n", formatAge(w.CreationTimestamp))

		// Containers
		fmt.Fprintf(&buf, "\n### Containers\n\n")
		for _, container := range w.Spec.Template.Spec.Containers {
			fmt.Fprintf(&buf, "- **%s**: image=%s", container.Name, container.Image)
			if len(container.Ports) > 0 {
				ports := make([]string, len(container.Ports))
				for i, p := range container.Ports {
					ports[i] = fmt.Sprintf("%d", p.ContainerPort)
				}
				fmt.Fprintf(&buf, ", ports=%s", joinStrings(ports))
			}
			fmt.Fprintf(&buf, "\n")
		}

		// Conditions
		if len(w.Status.Conditions) > 0 {
			fmt.Fprintf(&buf, "\n### Conditions\n\n")
			for _, cond := range w.Status.Conditions {
				fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
			}
		}

	case *appsv1.DaemonSet:
		fmt.Fprintf(&buf, "**KIND:** %s\n", kind)
		fmt.Fprintf(&buf, "**NAME:** %s\n", w.Name)
		fmt.Fprintf(&buf, "**NAMESPACE:** %s\n", w.Namespace)
		fmt.Fprintf(&buf, "**REPLICAS:** %s\n", formatDaemonSetReady(*w))
		fmt.Fprintf(&buf, "**SELECTOR:** %s\n", formatMatchLabels(w.Spec.Selector.MatchLabels))
		fmt.Fprintf(&buf, "**UPDATE STRATEGY:** %s\n", w.Spec.UpdateStrategy.Type)
		if w.Spec.UpdateStrategy.Type == appsv1.RollingUpdateDaemonSetStrategyType {
			fmt.Fprintf(&buf, "  - Rolling Update:\n")
			if w.Spec.UpdateStrategy.RollingUpdate != nil {
				fmt.Fprintf(&buf, "    - Max Unavailable: %v\n", w.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
			}
		}
		fmt.Fprintf(&buf, "**AGE:** %s\n", formatAge(w.CreationTimestamp))

		// Containers
		fmt.Fprintf(&buf, "\n### Containers\n\n")
		for _, container := range w.Spec.Template.Spec.Containers {
			fmt.Fprintf(&buf, "- **%s**: image=%s", container.Name, container.Image)
			if len(container.Ports) > 0 {
				ports := make([]string, len(container.Ports))
				for i, p := range container.Ports {
					ports[i] = fmt.Sprintf("%d", p.ContainerPort)
				}
				fmt.Fprintf(&buf, ", ports=%s", joinStrings(ports))
			}
			fmt.Fprintf(&buf, "\n")
		}

		// Conditions
		if len(w.Status.Conditions) > 0 {
			fmt.Fprintf(&buf, "\n### Conditions\n\n")
			for _, cond := range w.Status.Conditions {
				fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
			}
		}
	}

	return buf.String()
}

// formatWorkloadGetJSON formats a workload as JSON.
func formatWorkloadGetJSON(workload any, _ string) (string, error) {
	var details WorkloadDetails

	switch w := workload.(type) {
	case *appsv1.Deployment:
		details = buildWorkloadDetails("deployment", w)
	case *appsv1.StatefulSet:
		details = buildWorkloadDetails("statefulset", w)
	case *appsv1.DaemonSet:
		details = buildWorkloadDetails("daemonset", w)
	}

	data, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// buildWorkloadDetails builds a WorkloadDetails from a workload object.
func buildWorkloadDetails(kind string, workload any) WorkloadDetails {
	var details WorkloadDetails

	switch w := workload.(type) {
	case *appsv1.Deployment:
		details.Kind = kind
		details.Namespace = w.Namespace
		details.Name = w.Name
		details.Replicas = Replicas{
			Ready:   int(w.Status.ReadyReplicas),
			Desired: int(*w.Spec.Replicas),
		}
		details.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		details.Service = w.Name
		details.Strategy = string(w.Spec.Strategy.Type)
		details.Age = formatAge(w.CreationTimestamp)
		details.Spec = buildSpecMap(w.Spec)
		details.Status = buildStatusMap(w.Status)

	case *appsv1.StatefulSet:
		details.Kind = kind
		details.Namespace = w.Namespace
		details.Name = w.Name
		details.Replicas = Replicas{
			Ready:   int(w.Status.ReadyReplicas),
			Desired: int(*w.Spec.Replicas),
		}
		details.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		details.Service = w.Spec.ServiceName
		details.Strategy = string(w.Spec.UpdateStrategy.Type)
		details.Age = formatAge(w.CreationTimestamp)
		details.Spec = buildSpecMap(w.Spec)
		details.Status = buildStatusMap(w.Status)

	case *appsv1.DaemonSet:
		details.Kind = kind
		details.Namespace = w.Namespace
		details.Name = w.Name
		details.Replicas = Replicas{
			Ready:   int(w.Status.NumberReady),
			Desired: int(w.Status.DesiredNumberScheduled),
		}
		details.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		details.Strategy = string(w.Spec.UpdateStrategy.Type)
		details.Age = formatAge(w.CreationTimestamp)
		details.Spec = buildSpecMap(w.Spec)
		details.Status = buildStatusMap(w.Status)
	}

	return details
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

// joinStrings joins a slice of strings with a comma and space.
func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString(strs[0])
	for i := 1; i < len(strs); i++ {
		result.WriteString(", ")
		result.WriteString(strs[i])
	}
	return result.String()
}
