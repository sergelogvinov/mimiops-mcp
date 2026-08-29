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

package toolskarpenter

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
)

// NodePoolListResult is the result of karpenter_nodepools_list.
type NodePoolListResult struct {
	NodePools []NodePoolSummary `json:"nodePools" jsonschema:"List of NodePool resources"`
}

// RegisterNodePoolList adds the karpenter_nodepools_list tool, which lists
// Karpenter NodePool resources (karpenter.sh/v1) with their node class, node
// count, readiness, weight, and CPU/memory usage against limits.
func RegisterNodePoolList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Karpenter NodePools"),
		mcp.WithDescription("List Karpenter NodePool resources (karpenter.sh/v1) with node class, node count, readiness, weight, and CPU/memory provisioned vs limits"),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter (e.g. metadata.name=foo)")),
		mcp.WithOutputSchema[NodePoolListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("karpenter_nodepools_list", opts...)
	s.AddTool(tool, handlerNodePoolList(mc))
}

// handlerNodePoolList returns a handler function for the karpenter_nodepools_list tool.
func handlerNodePoolList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "karpenter_nodepools_list called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		karpenterClient, err := NewKarpenterClient(client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Karpenter client: %v", err), nil
		}

		pools, err := karpenterClient.ListNodePools(ctx, labelSelector, fieldSelector)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		result := NodePoolListResult{NodePools: pools}

		fallbackText := "No NodePool found"
		if len(result.NodePools) > 0 {
			fallbackText = formatter.ToText(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
