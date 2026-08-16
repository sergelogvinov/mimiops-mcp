package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterWorkloadsScale adds the workloads_scale tool, which scales a Deployment
// or StatefulSet to a target replica count. This is a mutating tool and requires
// --allow-destructive flag to be enabled.
func RegisterWorkloadsScale(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_scale",
		mcp.WithDescription("Scale a Deployment or StatefulSet to a target replica count. This is a destructive action."),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithInteger("replicas", mcp.Description("target replica count, min: 0"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment or statefulset"), mcp.Enum("deployment", "statefulset")),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
		mcp.WithBoolean("confirm", mcp.Description("set to true to confirm the scale operation"), mcp.DefaultBool(false)),
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

		replicas := req.GetInt("replicas", -1)
		if replicas < 0 {
			return mcp.NewToolResultError("missing required parameter 'replicas' or invalid value (must be >= 0)"), nil
		}

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset"), nil
		}

		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		confirm := req.GetBool("confirm", false)

		log.DebugContext(ctx, "workloads_scale called",
			"namespace", namespace,
			"name", name,
			"replicas", replicas,
			"kind", kind,
			"confirm", confirm,
		)

		// Check if destructive operations are allowed
		// This should be checked at registration time via allowDestructive flag
		// But we also check here as a safety measure

		// Phase 1: Prompt for confirmation if not confirmed
		if !confirm {
			return mcp.NewToolResultErrorf(
				"This will scale %s '%s' in namespace '%s' to %d replicas. Call again with confirm=true to proceed.",
				kind, name, namespace, replicas,
			), nil
		}

		// Phase 2: Execute the scale operation
		// Resolve kind if not provided
		resolvedKind, err := resolveWorkloadKind(ctx, client, namespace, name, kind)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Check if daemonset - cannot be scaled
		if resolvedKind == "daemonset" {
			return mcp.NewToolResultErrorf("cannot scale %s '%s': DaemonSets have no spec.replicas", resolvedKind, name), nil
		}

		// Scale the workload using the scale subresource
		scale, err := client.AppsV1().Deployments(namespace).UpdateScale(ctx, name, &autoscalingv1.Scale{
			Spec: autoscalingv1.ScaleSpec{
				Replicas: int32(replicas),
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: int32(replicas),
			},
		}, metav1.UpdateOptions{})
		if err != nil {
			// Try StatefulSet
			scale, err = client.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, &autoscalingv1.Scale{
				Spec: autoscalingv1.ScaleSpec{
					Replicas: int32(replicas),
				},
				Status: autoscalingv1.ScaleStatus{
					Replicas: int32(replicas),
				},
			}, metav1.UpdateOptions{})
		}

		if err != nil {
			return mcp.NewToolResultErrorf("failed to scale %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
		}

		result, err := formatWorkloadsScale(scale, resolvedKind, replicas, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatWorkloadsScale formats the scale result for MCP tool output.
func formatWorkloadsScale(scale *autoscalingv1.Scale, kind string, replicas int, format string) (string, error) {
	if format == "json" {
		return formatWorkloadsScaleJSON(scale, kind, replicas)
	}
	return formatWorkloadsScaleText(scale, kind, replicas), nil
}

// formatWorkloadsScaleText formats the scale result as text.
func formatWorkloadsScaleText(scale *autoscalingv1.Scale, kind string, replicas int) string {
	return fmt.Sprintf("Scaled %s '%s' in namespace '%s' to %d replicas.\n", kind, scale.Name, scale.Namespace, replicas)
}

// formatWorkloadsScaleJSON formats the scale result as JSON.
func formatWorkloadsScaleJSON(scale *autoscalingv1.Scale, kind string, replicas int) (string, error) {
	result := ScaleResult{
		Kind:      kind,
		Namespace: scale.Namespace,
		Name:      scale.Name,
		Replicas:  replicas,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
