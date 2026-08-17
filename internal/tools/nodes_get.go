package tools

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeGetResult represents the result of getting a node.
type NodeGetResult struct {
	NodeSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the node"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the node"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the node"`
	Status      map[string]any    `json:"status" jsonschema:"Status of the node"`

	Allocated  *AllocatedInfo  `json:"allocated,omitempty" jsonschema:"Allocated resource information"`
	Taints     []TaintInfo     `json:"taints,omitempty" jsonschema:"List of taints"`
	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"List of conditions"`
	Addresses  []AddressInfo   `json:"addresses,omitempty" jsonschema:"List of addresses"`
	NodeInfo   *NodeInfo       `json:"node_info,omitempty" jsonschema:"Node information"`
	Pods       []PodSummary    `json:"pods,omitempty" jsonschema:"List of pods running on the node"`
}

// TaintInfo represents a node taint.
type TaintInfo struct {
	Key    string `json:"key" jsonschema:"Key of the taint"`
	Value  string `json:"value" jsonschema:"Value of the taint"`
	Effect string `json:"effect" jsonschema:"Effect of the taint"`
}

// AddressInfo represents a node address.
type AddressInfo struct {
	Type    string `json:"type" jsonschema:"Type of the address"`
	Address string `json:"address" jsonschema:"Address value"`
}

// NodeInfo represents node information.
type NodeInfo struct {
	KubeletVersion          string `json:"kubelet_version" jsonschema:"Kubelet version"`
	OSImage                 string `json:"os_image" jsonschema:"OS image"`
	ContainerRuntimeVersion string `json:"container_runtime_version" jsonschema:"Container runtime version"`
	KernelVersion           string `json:"kernel_version" jsonschema:"Kernel version"`
}

// AllocatedInfo holds allocated resource information for a node.
type AllocatedInfo struct {
	RequestsCPU    string `json:"requests_cpu" jsonschema:"CPU requests (used/allocatable)"`
	RequestsMemory string `json:"requests_memory" jsonschema:"Memory requests (used/allocatable)"`
	LimitsCPU      string `json:"limits_cpu" jsonschema:"CPU limits (used/allocatable)"`
	LimitsMemory   string `json:"limits_memory" jsonschema:"Memory limits (used/allocatable)"`
	AllocatableCPU string `json:"allocatable_cpu" jsonschema:"Allocatable CPU"`
	AllocatableMem string `json:"allocatable_memory" jsonschema:"Allocatable memory"`
	PodCount       int    `json:"pod_count" jsonschema:"Number of pods"`
}

// RegisterNodesGet adds the nodes_get tool, which gets detailed information about a single node.
func RegisterNodesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("nodes_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		// mcp.WithToolTitle("Get Node"),
		mcp.WithDescription("Get detailed information about a single node"),
		mcp.WithString("name", mcp.Description("node name"), mcp.Required()),
		mcp.WithOutputSchema[NodeGetResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		log.DebugContext(ctx, "nodes_get called", "name", name)

		// Get the node
		node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
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

		// Build fallback text
		fallbackText := fmt.Sprintf("Node '%s' has status '%s' with roles '%s'. CPU: %s, Memory: %s. Pod count: %d.",
			result.Name, result.Status, result.Roles, result.Allocated.RequestsCPU, result.Allocated.RequestsMemory, result.Allocated.PodCount)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildNodeGetResult builds a NodeGetResult from a Node.
func buildNodeGetResult(node *corev1.Node, pods []corev1.Pod) *NodeGetResult {
	result := &NodeGetResult{}

	result.Labels = node.Labels
	result.Annotations = node.Annotations

	if result.Annotations == nil {
		result.Annotations = make(map[string]string)
	}
	maps.DeleteFunc(result.Annotations, func(k, _ string) bool {
		return k == "kubectl.kubernetes.io/last-applied-configuration"
	})

	// Spec (simplified)
	result.Spec = make(map[string]any)
	result.Spec["unschedulable"] = node.Spec.Unschedulable
	result.Spec["taints"] = node.Spec.Taints

	// Status (simplified)
	result.Status = make(map[string]any)
	result.Status["capacity"] = node.Status.Capacity
	result.Status["allocatable"] = node.Status.Allocatable
	result.Status["conditions"] = node.Status.Conditions
	result.Status["addresses"] = node.Status.Addresses
	result.Status["nodeInfo"] = node.Status.NodeInfo

	// Summary
	result.NodeSummary = NodeSummary{
		Name:       node.Name,
		Status:     deriveNodeStatus(node),
		Roles:      deriveNodeRoles(node),
		Age:        formatAge(node.CreationTimestamp),
		Version:    node.Status.NodeInfo.KubeletVersion,
		InternalIP: getInternalIP(node),
	}

	// Allocated resources
	allocMap := computeNodeAllocations(pods)
	alloc := allocMap[node.Name]
	if alloc != nil {
		result.Allocated = &AllocatedInfo{
			RequestsCPU:    alloc.RequestsCPU,
			RequestsMemory: alloc.RequestsMemory,
			LimitsCPU:      alloc.LimitsCPU,
			LimitsMemory:   alloc.LimitsMemory,
			AllocatableCPU: node.Status.Allocatable.Cpu().String(),
			AllocatableMem: node.Status.Allocatable.Memory().String(),
			PodCount:       len(pods),
		}
	}

	// Taints
	result.Taints = make([]TaintInfo, 0, len(node.Spec.Taints))
	for _, taint := range node.Spec.Taints {
		result.Taints = append(result.Taints, TaintInfo{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		})
	}

	// Conditions
	result.Conditions = make([]ConditionInfo, 0, len(node.Status.Conditions))
	for _, cond := range node.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// Addresses
	result.Addresses = make([]AddressInfo, 0, len(node.Status.Addresses))
	for _, addr := range node.Status.Addresses {
		result.Addresses = append(result.Addresses, AddressInfo{
			Type:    string(addr.Type),
			Address: addr.Address,
		})
	}

	// Node Info
	result.NodeInfo = &NodeInfo{
		KubeletVersion:          node.Status.NodeInfo.KubeletVersion,
		OSImage:                 node.Status.NodeInfo.OSImage,
		ContainerRuntimeVersion: node.Status.NodeInfo.ContainerRuntimeVersion,
		KernelVersion:           node.Status.NodeInfo.KernelVersion,
	}

	// Pods
	if len(pods) > 0 {
		cappedPods := pods
		if len(pods) > 15 {
			cappedPods = pods[:15]
		}
		for _, pod := range cappedPods {
			result.Pods = append(result.Pods, toPodSummary(pod))
		}
	}

	return result
}
