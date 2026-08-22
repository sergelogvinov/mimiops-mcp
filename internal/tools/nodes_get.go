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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeGetResult represents the result of getting a node.
type NodeGetResult struct {
	NodeSummary
	NodeSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"List of conditions"`
}

// NodeAllocations holds computed resource allocations for a node.
type NodeAllocations struct {
	RequestsCPU       string `json:"requests_cpu" jsonschema:"CPU requests (used/allocatable)"`
	RequestsMemory    string `json:"requests_memory" jsonschema:"Memory requests (used/allocatable)"`
	LimitsCPU         string `json:"limits_cpu" jsonschema:"CPU limits (used/allocatable)"`
	LimitsMemory      string `json:"limits_memory" jsonschema:"Memory limits (used/allocatable)"`
	AllocatableCPU    string `json:"allocatable_cpu" jsonschema:"Allocatable CPU"`
	AllocatableMemory string `json:"allocatable_memory" jsonschema:"Allocatable memory"`
}

// RegisterNodesGet adds the nodes_get tool, which gets detailed information about a single node.
func RegisterNodesGet(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("nodes_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Node"),
		mcp.WithDescription("Get detailed information about a single node"),
		mcp.WithString("name", mcp.Description("node name"), mcp.Required()),
		mcp.WithOutputSchema[NodeGetResult](),
	)
	s.AddTool(tool, handlerNodesGet(client))
}

// handlerNodesGet returns a handler function for the nodes_get tool.
func handlerNodesGet(client *k8s.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "nodes_get called", "name", name)

		node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("node '%s' not found", name), nil
			}
			return mcp.NewToolResultErrorf("failed to get node '%s': %v", name, err), nil
		}

		// List pods on this node for allocated resources (always needed)
		pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", name),
		})
		if err != nil {
			log.WarnContext(ctx, "failed to list pods for node", "node", name, "err", err)
		}

		// Build result
		result := buildNodeGetResult(node, pods.Items)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildNodeGetResult builds a NodeGetResult from a Node.
func buildNodeGetResult(node *corev1.Node, _ []corev1.Pod) *NodeGetResult {
	result := &NodeGetResult{
		NodeSummary: toNodeSummary(node),
		NodeSpec:    toNodeSpec(node),
		Annotations: extractNodeAnnotations(node.Annotations),
		Labels:      extractNodeLabels(node.Labels),
		Conditions:  toNodeConditionInfo(node),
	}

	return result
}
