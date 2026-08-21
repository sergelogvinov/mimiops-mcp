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
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// WorkloadGetResult represents the result of getting a workload.
type WorkloadGetResult struct {
	WorkloadSummary
	WorkloadSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
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
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("%s '%s' not found in namespace '%s'", resolvedKind, name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
		}

		result := buildWorkloadGetResult(workload)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildWorkloadGetResult builds a WorkloadGetResult from a workload object.
func buildWorkloadGetResult(workload any) *WorkloadGetResult {
	result := &WorkloadGetResult{}

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
