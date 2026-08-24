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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadsListResult represents the result of listing workloads.
type WorkloadsListResult struct {
	Workloads []WorkloadSummary `json:"workloads" jsonschema:"List of workloads"`
}

// RegisterWorkloadsList adds the workloads_list tool, which lists Deployments,
// StatefulSets, or DaemonSets in a namespace (or all namespaces).
func RegisterWorkloadsList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Workloads"),
		mcp.WithDescription("List Deployments, StatefulSets, or DaemonSets in a namespace (or all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithOutputSchema[WorkloadsListResult](),
	}, clusterOptions(mc)...)

	tool := mcp.NewTool("workloads_list", opts...)
	s.AddTool(tool, handlerWorkloadsList(mc))
}

// handlerWorkloadsList returns the handler function for the workloads_list tool.
func handlerWorkloadsList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := resolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		labelSelector := req.GetString("label_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "workloads_list called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"kind", kind,
			"label_selector", labelSelector,
		)

		var summaries []WorkloadSummary

		if kind != "" {
			summaries, err = listWorkloadsByKind(ctx, client, namespace, kind, labelSelector)
		} else {
			summaries, err = listAllWorkloads(ctx, client, namespace, labelSelector)
		}
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list workloads in namespace '%s': %v", namespace, err), nil
		}

		result := WorkloadsListResult{
			Workloads: summaries,
		}

		// Build fallback text
		fallbackText := "No workloads found"
		if len(result.Workloads) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
