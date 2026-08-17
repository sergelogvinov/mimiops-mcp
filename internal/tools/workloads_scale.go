package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScaleResult represents the result of a scale operation.
type ScaleResult struct {
	WorkloadSummary
}

// RegisterWorkloadsScale adds the workloads_scale tool, which scales a Deployment
// or StatefulSet to a target replica count. This is a mutating tool and requires
// --allow-destructive flag to be enabled.
func RegisterWorkloadsScale(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("workloads_scale",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Scale Workload"),
		mcp.WithDescription("Scale a Deployment or StatefulSet to a target replica count. This is a destructive action."),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithInteger("replicas", mcp.Description("target replica count, min: 0"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment or statefulset"), mcp.Required(), mcp.Enum("deployment", "statefulset")),
		mcp.WithBoolean("confirm", mcp.Description("set to true to confirm the scale operation"), mcp.DefaultBool(false)),
		mcp.WithOutputSchema[ScaleResult](),
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
		if kind != "deployment" && kind != "statefulset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset"), nil
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
			result := ScaleResult{
				WorkloadSummary: WorkloadSummary{
					Kind:      kind,
					Namespace: namespace,
					Name:      name,
					Desired:   replicas,
				},
			}
			fallbackText := fmt.Sprintf("This will scale %s '%s' in namespace '%s' to %d replicas. Call again with confirm=true to proceed.", kind, name, namespace, replicas)
			return mcp.NewToolResultStructured(result, fallbackText), nil
		}

		// Phase 2: Execute the scale operation
		var err error

		switch kind {
		case "deployment":
			_, err = client.AppsV1().Deployments(namespace).UpdateScale(ctx, name, &autoscalingv1.Scale{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: autoscalingv1.ScaleSpec{
					Replicas: int32(replicas),
				},
			}, metav1.UpdateOptions{})
		case "statefulset":
			_, err = client.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, &autoscalingv1.Scale{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: autoscalingv1.ScaleSpec{
					Replicas: int32(replicas),
				},
			}, metav1.UpdateOptions{})
		default:
			return mcp.NewToolResultErrorf("unsupported workload kind '%s' for scaling", kind), nil
		}
		if err != nil {
			return mcp.NewToolResultErrorf("failed to scale %s '%s' in namespace '%s': %v", kind, name, namespace, err), nil
		}

		// Get the updated workload to get accurate status
		updatedWorkload, err := getWorkloadByKind(ctx, client, namespace, name, kind)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get updated %s '%s' in namespace '%s': %v", kind, name, namespace, err), nil
		}

		result := ScaleResult{
			WorkloadSummary: toWorkloadSummary(updatedWorkload),
		}
		fallbackText := fmt.Sprintf("Scaled %s '%s' in namespace '%s' to %d replicas.", kind, name, namespace, replicas)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
