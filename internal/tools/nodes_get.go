package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterNodesGet adds the nodes_get tool, which gets detailed information about a single node.
func RegisterNodesGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("nodes_get",
		mcp.WithDescription("Get detailed information about a single node."),
		mcp.WithString("name", mcp.Description("node name"), mcp.Required()),
		mcp.WithBoolean("include_pods", mcp.Description("include a summary of pods running on the node"), mcp.DefaultBool(false)),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		includePods := req.GetBool("include_pods", false)
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "nodes_get called",
			"name", name,
			"include_pods", includePods,
		)

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

		// Format output
		result, err := formatNodeGet(node, pods.Items, includePods, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatNodeGet formats a node for MCP tool output.
func formatNodeGet(node *corev1.Node, pods []corev1.Pod, includePods bool, format string) (string, error) {
	if format == "json" {
		return formatNodeGetJSON(node, pods, includePods)
	}
	return formatNodeGetText(node, pods, includePods), nil
}

// formatNodeGetText formats a node's detailed information as key-value blocks.
func formatNodeGetText(node *corev1.Node, pods []corev1.Pod, includePods bool) string {
	var buf bytes.Buffer

	name := node.Name
	status := deriveNodeStatus(node)
	roles := deriveNodeRoles(node)
	age := formatAge(node.CreationTimestamp)

	fmt.Fprintf(&buf, "**Name:** %s\n", name)
	fmt.Fprintf(&buf, "**Status:** %s\n", status)
	fmt.Fprintf(&buf, "**Roles:** %s\n", roles)
	fmt.Fprintf(&buf, "**Age:** %s\n", age)

	// Taints
	if len(node.Spec.Taints) > 0 {
		fmt.Fprintf(&buf, "\n### Taints\n\n")
		for _, taint := range node.Spec.Taints {
			fmt.Fprintf(&buf, "- %s=%s: %s\n", taint.Key, taint.Value, taint.Effect)
		}
	}

	// Schedulable
	fmt.Fprintf(&buf, "\n### Scheduling\n\n")
	fmt.Fprintf(&buf, "- **Schedulable:** %t\n", !node.Spec.Unschedulable)
	fmt.Fprintf(&buf, "- **Unschedulable:** %t\n", node.Spec.Unschedulable)

	// Capacity
	fmt.Fprintf(&buf, "\n### Capacity\n\n")
	fmt.Fprintf(&buf, "- **CPU:** %s\n", node.Status.Capacity.Cpu().String())
	fmt.Fprintf(&buf, "- **Memory:** %s\n", node.Status.Capacity.Memory().String())
	fmt.Fprintf(&buf, "- **Pods:** %s\n", node.Status.Capacity.Pods().String())

	// Allocatable
	fmt.Fprintf(&buf, "\n### Allocatable\n\n")
	fmt.Fprintf(&buf, "- **CPU:** %s\n", node.Status.Allocatable.Cpu().String())
	fmt.Fprintf(&buf, "- **Memory:** %s\n", node.Status.Allocatable.Memory().String())
	fmt.Fprintf(&buf, "- **Pods:** %s\n", node.Status.Allocatable.Pods().String())

	// Allocated Resources
	fmt.Fprintf(&buf, "\n### Allocated Resources\n\n")
	allocMap := computeNodeAllocations(pods)
	alloc := allocMap[node.Name]
	allocatableCPU := node.Status.Allocatable.Cpu().String()
	allocatableMem := node.Status.Allocatable.Memory().String()

	if alloc != nil {
		fmt.Fprintf(&buf, "- **CPU Requests:** %s / %s\n", alloc.RequestsCPU, allocatableCPU)
		fmt.Fprintf(&buf, "- **CPU Limits:** %s / %s\n", alloc.LimitsCPU, allocatableCPU)
		fmt.Fprintf(&buf, "- **Memory Requests:** %s / %s\n", alloc.RequestsMemory, allocatableMem)
		fmt.Fprintf(&buf, "- **Memory Limits:** %s / %s\n", alloc.LimitsMemory, allocatableMem)
	} else {
		fmt.Fprintf(&buf, "- **CPU Requests:** - / %s\n", allocatableCPU)
		fmt.Fprintf(&buf, "- **CPU Limits:** - / %s\n", allocatableCPU)
		fmt.Fprintf(&buf, "- **Memory Requests:** - / %s\n", allocatableMem)
		fmt.Fprintf(&buf, "- **Memory Limits:** - / %s\n", allocatableMem)
	}

	fmt.Fprintf(&buf, "- **Pod Count:** %d\n", len(pods))

	// Conditions
	if len(node.Status.Conditions) > 0 {
		fmt.Fprintf(&buf, "\n### Conditions\n\n")
		for _, cond := range node.Status.Conditions {
			fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
		}
	}

	// Addresses
	if len(node.Status.Addresses) > 0 {
		fmt.Fprintf(&buf, "\n### Addresses\n\n")
		for _, addr := range node.Status.Addresses {
			fmt.Fprintf(&buf, "- **%s:** %s\n", addr.Type, addr.Address)
		}
	}

	// Node Info
	fmt.Fprintf(&buf, "\n### Node Info\n\n")
	fmt.Fprintf(&buf, "- **Kubelet Version:** %s\n", node.Status.NodeInfo.KubeletVersion)
	fmt.Fprintf(&buf, "- **OS Image:** %s\n", node.Status.NodeInfo.OSImage)
	fmt.Fprintf(&buf, "- **Container Runtime Version:** %s\n", node.Status.NodeInfo.ContainerRuntimeVersion)
	fmt.Fprintf(&buf, "- **Kernel Version:** %s\n", node.Status.NodeInfo.KernelVersion)

	// Pods (if requested)
	if includePods && len(pods) > 0 {
		fmt.Fprintf(&buf, "\n### Pods\n\n")
		cappedPods := pods
		if len(pods) > 15 {
			cappedPods = pods[:15]
		}
		for _, pod := range cappedPods {
			podStatus := string(pod.Status.Phase)
			if len(pod.Status.Conditions) > 0 {
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady {
						podStatus = fmt.Sprintf("%s (Ready: %v)", pod.Status.Phase, cond.Status)
						break
					}
				}
			}
			fmt.Fprintf(&buf, "- **%s:** %s\n", pod.Name, podStatus)
		}
		if len(pods) > 15 {
			fmt.Fprintf(&buf, "\n... and %d more pods\n", len(pods)-15)
		}
	}

	return buf.String()
}

// formatNodeGetJSON formats a node as JSON.
func formatNodeGetJSON(node *corev1.Node, pods []corev1.Pod, includePods bool) (string, error) {
	type NodeInfo struct {
		Metadata  map[string]any `json:"metadata"`
		Spec      map[string]any `json:"spec"`
		Status    map[string]any `json:"status"`
		Summary   NodeSummary    `json:"summary"`
		Allocated *AllocatedInfo `json:"allocated,omitempty"`
		Pods      []PodSummary   `json:"pods,omitempty"`
	}

	info := NodeInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = node.Name
	info.Metadata["uid"] = string(node.UID)
	info.Metadata["creationTimestamp"] = node.CreationTimestamp.String()
	info.Metadata["labels"] = node.Labels
	info.Metadata["annotations"] = node.Annotations

	// Spec (simplified)
	info.Spec = make(map[string]any)
	info.Spec["unschedulable"] = node.Spec.Unschedulable
	info.Spec["taints"] = node.Spec.Taints

	// Status (simplified)
	info.Status = make(map[string]any)
	info.Status["capacity"] = node.Status.Capacity
	info.Status["allocatable"] = node.Status.Allocatable
	info.Status["conditions"] = node.Status.Conditions
	info.Status["addresses"] = node.Status.Addresses
	info.Status["nodeInfo"] = node.Status.NodeInfo

	// Summary
	info.Summary = NodeSummary{
		Name:       node.Name,
		Status:     deriveNodeStatus(node),
		Roles:      []string{deriveNodeRoles(node)},
		Age:        formatAge(node.CreationTimestamp),
		Version:    node.Status.NodeInfo.KubeletVersion,
		InternalIP: getInternalIP(node),
	}

	// Allocated resources
	allocMap := computeNodeAllocations(pods)
	alloc := allocMap[node.Name]
	if alloc != nil {
		info.Allocated = &AllocatedInfo{
			RequestsCPU:    alloc.RequestsCPU,
			RequestsMemory: alloc.RequestsMemory,
			LimitsCPU:      alloc.LimitsCPU,
			LimitsMemory:   alloc.LimitsMemory,
			AllocatableCPU: node.Status.Allocatable.Cpu().String(),
			AllocatableMem: node.Status.Allocatable.Memory().String(),
			PodCount:       len(pods),
		}
	}

	// Pods
	if includePods && len(pods) > 0 {
		cappedPods := pods
		if len(pods) > 15 {
			cappedPods = pods[:15]
		}
		for _, pod := range cappedPods {
			info.Pods = append(info.Pods, toPodSummary(pod))
		}
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AllocatedInfo holds allocated resource information for a node.
type AllocatedInfo struct {
	RequestsCPU    string `json:"requests_cpu"`
	RequestsMemory string `json:"requests_memory"`
	LimitsCPU      string `json:"limits_cpu"`
	LimitsMemory   string `json:"limits_memory"`
	AllocatableCPU string `json:"allocatable_cpu"`
	AllocatableMem string `json:"allocatable_memory"`
	PodCount       int    `json:"pod_count"`
}
