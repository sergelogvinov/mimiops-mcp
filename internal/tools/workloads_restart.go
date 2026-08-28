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
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// restartedAtAnnotation is the pod template annotation bumped by
// `kubectl rollout restart` to trigger a controlled rollout of new pods.
const restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"

// WorkloadsRestartResult represents the result of a rollout restart.
type WorkloadsRestartResult struct {
	Restarted []WorkloadSummary `json:"restarted" jsonschema:"List of restarted workloads"`
}

// RegisterWorkloadsRestart adds the workloads_restart tool, which performs a
// rollout restart of a workload (by name) or of all workloads matching a label
// selector. This is a mutating tool and requires --allow-destructive to be enabled.
func RegisterWorkloadsRestart(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Restart Workloads"),
		mcp.WithDescription("Rollout restart a Deployment, StatefulSet, or DaemonSet by name, or restart all workloads matching a label selector"),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("name", mcp.Description("workload name; omit to restart all workloads matching label_selector")),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithString("label_selector", mcp.Description("label selector filter (required when name is omitted)")),
		mcp.WithOutputSchema[WorkloadsRestartResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("workloads_restart", opts...)
	s.AddTool(tool, handlerWorkloadsRestart(mc))
}

// handlerWorkloadsRestart returns the handler function for the workloads_restart tool.
func handlerWorkloadsRestart(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		name := req.GetString("name", "")
		labelSelector := req.GetString("label_selector", "")
		if name == "" && labelSelector == "" {
			return mcp.NewToolResultError("either 'name' or 'label_selector' is required"), nil
		}

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "workloads_restart called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
			"kind", kind,
			"label_selector", labelSelector,
		)

		var restarted []WorkloadSummary

		if name != "" {
			resolvedKind, err := resolveWorkloadKind(ctx, client, namespace, name, kind)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if err := restartWorkload(ctx, client, namespace, name, resolvedKind); err != nil {
				return mcp.NewToolResultErrorf("failed to restart %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
			}

			workload, err := getWorkloadByKind(ctx, client, namespace, name, resolvedKind)
			if err != nil {
				return mcp.NewToolResultErrorf("failed to get %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
			}

			restarted = append(restarted, toWorkloadSummary(workload))
		} else {
			var summaries []WorkloadSummary

			if kind != "" {
				summaries, err = listWorkloadsByKind(ctx, client, namespace, kind, labelSelector)
			} else {
				summaries, err = listAllWorkloads(ctx, client, namespace, labelSelector)
			}
			if err != nil {
				return mcp.NewToolResultErrorf("failed to list workloads in namespace '%s': %v", namespace, err), nil
			}

			if len(summaries) == 0 {
				return mcp.NewToolResultErrorf("no workloads matching label selector '%s' in namespace '%s'", labelSelector, namespace), nil
			}

			for _, w := range summaries {
				if err := restartWorkload(ctx, client, w.Namespace, w.Name, w.Kind); err != nil {
					return mcp.NewToolResultErrorf("failed to restart %s '%s' in namespace '%s': %v", w.Kind, w.Name, w.Namespace, err), nil
				}

				restarted = append(restarted, w)
			}
		}

		result := WorkloadsRestartResult{
			Restarted: restarted,
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=patch

// restartWorkload triggers a rollout restart of a workload by patching the
// restartedAt annotation in its pod template (same mechanism as kubectl rollout restart).
func restartWorkload(ctx context.Context, client *k8s.Client, namespace, name, kind string) error {
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"%s":%q}}}}}`,
		restartedAtAnnotation, time.Now().Format(time.RFC3339))

	switch kind {
	case "deployment":
		_, err := client.AppsV1().Deployments(namespace).Patch(ctx, name, k8stypes.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		return err
	case "statefulset":
		_, err := client.AppsV1().StatefulSets(namespace).Patch(ctx, name, k8stypes.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		return err
	case "daemonset":
		_, err := client.AppsV1().DaemonSets(namespace).Patch(ctx, name, k8stypes.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("invalid kind '%s'", kind)
	}
}
