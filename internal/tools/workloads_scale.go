/*
Copyright 2026 Serge Logvinov.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
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
func RegisterWorkloadsScale(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Scale Workload"),
		mcp.WithDescription("Scale a Deployment or StatefulSet to a target replica count. This is a destructive action."),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithInteger("replicas", mcp.Description("target replica count, min: 0"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment or statefulset"), mcp.Required(), mcp.Enum("deployment", "statefulset")),
		mcp.WithOutputSchema[ScaleResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("workloads_scale", opts...)
	s.AddTool(tool, handlerWorkloadsScale(mc))
}

// +kubebuilder:rbac:groups=apps,resources=deployments/scale,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets/scale,verbs=get;update;patch

// handlerWorkloadsScale returns the handler function for the workloads_scale tool.
func handlerWorkloadsScale(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

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

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "workloads_scale called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
			"replicas", replicas,
			"kind", kind,
		)

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

		result := ScaleResult{WorkloadSummary: toWorkloadSummary(updatedWorkload)}
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
