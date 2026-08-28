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
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeUsage holds the current resource usage of a node from the metrics API.
type NodeUsage struct {
	CPU           string `json:"cpu" jsonschema:"Current CPU usage"`
	CPUPercent    string `json:"cpu_percent" jsonschema:"CPU usage percentage of allocatable"`
	Memory        string `json:"memory" jsonschema:"Current memory usage"`
	MemoryPercent string `json:"memory_percent" jsonschema:"Memory usage percentage of allocatable"`
}

// NodeDescribeResult represents the result of describing a node.
type NodeDescribeResult struct {
	NodeSummary
	NodeSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Addresses          []NodeAddressInfo `json:"addresses,omitempty" jsonschema:"Node addresses"`
	Conditions         []ConditionInfo   `json:"conditions,omitempty" jsonschema:"List of conditions"`
	AllocatedResources NodeAllocations   `json:"allocated_resources" jsonschema:"Allocated resources (requests and limits vs allocatable)"`
	Usage              *NodeUsage        `json:"usage,omitempty" jsonschema:"Current resource usage from the metrics API"`
	Pods               []NodePodInfo     `json:"pods,omitempty" jsonschema:"Pods running on the node (capped at 20)"`
	Events             []EventSummary    `json:"events,omitempty" jsonschema:"List of events"`
}

// RegisterNodesDescribe adds the nodes_describe tool, which provides a structured node summary.
func RegisterNodesDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Node"),
		mcp.WithDescription("Node summary (conditions, addresses, taints, allocated resources, current usage, pods, events)"),
		mcp.WithString("name", mcp.Description("node name"), mcp.Required()),
		mcp.WithOutputSchema[NodeDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("nodes_describe", opts...)
	s.AddTool(tool, handlerNodesDescribe(mc))
}

// handlerNodesDescribe returns a handler function for the nodes_describe tool.
func handlerNodesDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "nodes_describe called",
			"cluster", client.ClusterName,
			"name", name,
		)

		node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("node '%s' not found", name), nil
			}
			return mcp.NewToolResultErrorf("failed to get node '%s': %v", name, err), nil
		}

		// List pods on this node for allocated resources and the pod table
		pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", name),
		})
		if err != nil {
			log.WarnContext(ctx, "failed to list pods for node", "node", name, "err", err)
		}

		result := buildNodeDescribeResult(ctx, client, node, pods.Items)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildNodeDescribeResult builds a NodeDescribeResult from a Node and its pods.
func buildNodeDescribeResult(ctx context.Context, client *k8s.Client, node *corev1.Node, pods []corev1.Pod) *NodeDescribeResult {
	result := &NodeDescribeResult{
		NodeSummary:        toNodeSummary(node),
		NodeSpec:           toNodeSpec(node),
		Annotations:        extractNodeAnnotations(node.Annotations),
		Labels:             extractNodeLabels(node.Labels),
		Addresses:          extractNodeAddresses(node.Status.Addresses),
		Conditions:         toNodeConditionInfo(node),
		AllocatedResources: computeNodeAllocations(node, pods),
		Usage:              fetchNodeUsage(ctx, client, node),
		Pods:               toNodePodInfoList(pods, 20),
	}

	// List events for the node
	events, err := client.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=Node,involvedObject.name=%s", node.Name),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return result
	}

	result.Events = make([]EventSummary, 0, len(events.Items))
	for _, e := range events.Items {
		firstSeen := ""
		if !e.FirstTimestamp.IsZero() {
			firstSeen = age.FormatAge(e.FirstTimestamp)
		}

		result.Events = append(result.Events, EventSummary{
			FirstSeen: firstSeen,
			Age:       age.FormatEventAge(e),
			Message:   e.Message,
			Reason:    e.Reason,
			Type:      e.Type,
		})
	}

	if len(result.Events) > 50 {
		result.Events = result.Events[:50]
	}

	return result
}

// fetchNodeUsage fetches the node's current resource usage from the metrics API.
// It returns nil when the metrics API is unavailable (e.g., no metrics-server),
// so the describe result degrades gracefully without usage data.
func fetchNodeUsage(ctx context.Context, client *k8s.Client, node *corev1.Node) *NodeUsage {
	log := logger.FromContext(ctx)

	metricsClient, err := client.Metrics()
	if err != nil {
		log.DebugContext(ctx, "failed to create metrics client", "node", node.Name, "err", err)
		return nil
	}

	nodeMetrics, err := metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		// The metrics API is optional; NotFound usually means metrics-server is not installed.
		log.DebugContext(ctx, "node metrics not available", "node", node.Name, "err", err)
		return nil
	}

	usage := &NodeUsage{}
	if q, ok := nodeMetrics.Usage[corev1.ResourceCPU]; ok {
		usage.CPU = fmt.Sprintf("%dm", q.MilliValue())
		usage.CPUPercent = quantityPercent(&q, node.Status.Allocatable.Cpu())
	}
	if q, ok := nodeMetrics.Usage[corev1.ResourceMemory]; ok {
		usage.Memory = q.String()
		usage.MemoryPercent = quantityPercent(&q, node.Status.Allocatable.Memory())
	}

	return usage
}

// computeNodeAllocations sums pod requests and limits on the node and expresses
// them against the node's allocatable resources, kubectl-style (e.g., "650m (16%)").
func computeNodeAllocations(node *corev1.Node, pods []corev1.Pod) NodeAllocations {
	var reqCPU, reqMem, limCPU, limMem resource.Quantity

	for _, pod := range pods {
		for _, c := range pod.Spec.Containers {
			if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				reqCPU.Add(q)
			}
			if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				reqMem.Add(q)
			}
			if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				limCPU.Add(q)
			}
			if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				limMem.Add(q)
			}
		}
	}

	allocCPU := node.Status.Allocatable.Cpu()
	allocMem := node.Status.Allocatable.Memory()

	return NodeAllocations{
		RequestsCPU:       fmt.Sprintf("%s (%s)", reqCPU.String(), quantityPercent(&reqCPU, allocCPU)),
		RequestsMemory:    fmt.Sprintf("%s (%s)", reqMem.String(), quantityPercent(&reqMem, allocMem)),
		LimitsCPU:         fmt.Sprintf("%s (%s)", limCPU.String(), quantityPercent(&limCPU, allocCPU)),
		LimitsMemory:      fmt.Sprintf("%s (%s)", limMem.String(), quantityPercent(&limMem, allocMem)),
		AllocatableCPU:    allocCPU.String(),
		AllocatableMemory: allocMem.String(),
	}
}

// quantityPercent returns the percentage of used relative to total, e.g., "16%".
func quantityPercent(used, total *resource.Quantity) string {
	if total == nil || total.IsZero() {
		return "0%"
	}

	pct := float64(used.MilliValue()) / float64(total.MilliValue()) * 100

	return fmt.Sprintf("%.0f%%", pct)
}

// toNodePodInfoList converts pods to NodePodInfo, capped at maxPods.
func toNodePodInfoList(pods []corev1.Pod, maxPods int) []NodePodInfo {
	result := make([]NodePodInfo, 0, min(len(pods), maxPods))

	for i, pod := range pods {
		if i >= maxPods {
			break
		}

		result = append(result, NodePodInfo{
			Namespace:      pod.Namespace,
			Name:           pod.Name,
			Phase:          string(pod.Status.Phase),
			CPURequests:    podResourceTotal(pod, corev1.ResourceCPU, true),
			CPULimits:      podResourceTotal(pod, corev1.ResourceCPU, false),
			MemoryRequests: podResourceTotal(pod, corev1.ResourceMemory, true),
			MemoryLimits:   podResourceTotal(pod, corev1.ResourceMemory, false),
		})
	}

	return result
}

// podResourceTotal sums a resource across all containers of a pod.
// It returns an empty string when the total is zero.
func podResourceTotal(pod corev1.Pod, name corev1.ResourceName, requests bool) string {
	var total resource.Quantity

	for _, c := range pod.Spec.Containers {
		rl := c.Resources.Limits
		if requests {
			rl = c.Resources.Requests
		}

		if q, ok := rl[name]; ok {
			total.Add(q)
		}
	}

	if total.IsZero() {
		return ""
	}

	return total.String()
}
