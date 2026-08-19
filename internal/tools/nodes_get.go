package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeGetResult represents the result of getting a node.
type NodeGetResult struct {
	NodeSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the node"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the node"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the node"`

	Allocated  *AllocatedInfo  `json:"allocated,omitempty" jsonschema:"Allocated resource information"`
	Taints     []TaintInfo     `json:"taints,omitempty" jsonschema:"List of taints"`
	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"List of conditions"`

	// Pods     []PodSummary `json:"pods,omitempty" jsonschema:"List of pods running on the node"`
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
func RegisterNodesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("nodes_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Node"),
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
	})
}

// buildNodeGetResult builds a NodeGetResult from a Node.
func buildNodeGetResult(node *corev1.Node, pods []corev1.Pod) *NodeGetResult {
	result := &NodeGetResult{
		NodeSummary: toNodeSummary(*node),
		Labels:      node.Labels,
		Annotations: node.Annotations,
		Spec:        make(map[string]any),
		Taints:      make([]TaintInfo, 0, len(node.Spec.Taints)),
		Conditions:  make([]ConditionInfo, 0, len(node.Status.Conditions)),
	}

	if result.Labels == nil {
		result.Labels = make(map[string]string)
	}
	if result.Annotations == nil {
		result.Annotations = make(map[string]string)
	}

	// Spec (simplified)
	result.Spec["unschedulable"] = node.Spec.Unschedulable

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
	for _, taint := range node.Spec.Taints {
		result.Taints = append(result.Taints, TaintInfo{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		})
	}

	// Conditions
	for _, cond := range node.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// Pods
	// if len(pods) > 0 {
	// 	cappedPods := pods
	// 	if len(pods) > 15 {
	// 		cappedPods = pods[:15]
	// 	}
	// 	for _, pod := range cappedPods {
	// 		result.Pods = append(result.Pods, toPodSummary(pod))
	// 	}
	// }

	return result
}
