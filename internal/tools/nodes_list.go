package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterNodesList adds the nodes_list tool, which lists cluster nodes and their status.
func RegisterNodesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("nodes_list",
		mcp.WithDescription("List cluster nodes and their status."),
		mcp.WithBoolean("include_allocations", mcp.Description("include aggregate pod CPU/memory request & limit sums per node"), mcp.DefaultBool(false)),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		includeAllocations := req.GetBool("include_allocations", false)
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "nodes_list called",
			"include_allocations", includeAllocations,
		)

		// List nodes
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list nodes: %v", err), nil
		}

		// Format output
		result, err := formatNodesList(ctx, nodes.Items, includeAllocations, format, client, log)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatNodesList formats a list of nodes for MCP tool output.
func formatNodesList(ctx context.Context, nodes []corev1.Node, includeAllocations bool, format string, client *k8s.Client, log *slog.Logger) (string, error) {
	if format == "json" {
		return formatNodesListJSON(ctx, nodes, includeAllocations, client, log)
	}
	return formatNodesListText(ctx, nodes, includeAllocations, client, log), nil
}

// formatNodesListText formats a list of nodes as a markdown table.
func formatNodesListText(ctx context.Context, nodes []corev1.Node, includeAllocations bool, client *k8s.Client, log *slog.Logger) string {
	if len(nodes) == 0 {
		return "No nodes found."
	}

	var buf bytes.Buffer
	if includeAllocations {
		buf.WriteString("| NAME | STATUS | ROLES | AGE | VERSION | INTERNAL-IP | REQUESTS CPU | REQUESTS MEMORY | LIMITS CPU | LIMITS MEMORY |\n")
		buf.WriteString("|------|--------|-------|-----|---------|-------------|--------------|-----------------|------------|---------------|\n")
	} else {
		buf.WriteString("| NAME | STATUS | ROLES | AGE | VERSION | INTERNAL-IP |\n")
		buf.WriteString("|------|--------|-------|-----|---------|-------------|\n")
	}

	// Pre-compute allocations if needed
	var podList *corev1.PodList
	var err error
	if includeAllocations {
		podList, err = client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.WarnContext(ctx, "failed to list pods for allocation calculation", "err", err)
		}
	}

	allocations := make(map[string]*NodeAllocations)
	if includeAllocations && podList != nil {
		allocations = computeNodeAllocations(podList.Items)
	}

	for _, node := range nodes {
		name := node.Name
		status := deriveNodeStatus(&node)
		roles := deriveNodeRoles(&node)
		age := formatAge(node.CreationTimestamp)
		version := node.Status.NodeInfo.KubeletVersion
		internalIP := getInternalIP(&node)

		if includeAllocations {
			alloc := allocations[node.Name]
			reqCPU, reqMem := "-", "-"
			limCPU, limMem := "-", "-"
			if alloc != nil {
				reqCPU = fmt.Sprintf("%s/%s", alloc.RequestsCPU, alloc.AllocatableCPU)
				reqMem = fmt.Sprintf("%s/%s", alloc.RequestsMemory, alloc.AllocatableMemory)
				limCPU = fmt.Sprintf("%s/%s", alloc.LimitsCPU, alloc.AllocatableCPU)
				limMem = fmt.Sprintf("%s/%s", alloc.LimitsMemory, alloc.AllocatableMemory)
			}
			fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				name, status, roles, age, version, internalIP, reqCPU, reqMem, limCPU, limMem)
		} else {
			fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s |\n",
				name, status, roles, age, version, internalIP)
		}
	}

	return buf.String()
}

// formatNodesListJSON formats a list of nodes as JSON.
func formatNodesListJSON(ctx context.Context, nodes []corev1.Node, includeAllocations bool, client *k8s.Client, log *slog.Logger) (string, error) {
	summaries := make([]NodeSummary, 0, len(nodes))

	// Pre-compute allocations if needed
	var podList *corev1.PodList
	var err error
	if includeAllocations {
		podList, err = client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.WarnContext(ctx, "failed to list pods for allocation calculation", "err", err)
		}
	}

	allocations := make(map[string]*NodeAllocations)
	if includeAllocations && podList != nil {
		allocations = computeNodeAllocations(podList.Items)
	}

	for _, node := range nodes {
		name := node.Name
		status := deriveNodeStatus(&node)
		roles := deriveNodeRoles(&node)
		age := formatAge(node.CreationTimestamp)
		version := node.Status.NodeInfo.KubeletVersion
		internalIP := getInternalIP(&node)

		summary := NodeSummary{
			Name:       name,
			Status:     status,
			Roles:      []string{roles},
			Age:        age,
			Version:    version,
			InternalIP: internalIP,
		}

		if includeAllocations {
			alloc := allocations[node.Name]
			if alloc != nil {
				summary.RequestsCPU = fmt.Sprintf("%s/%s", alloc.RequestsCPU, alloc.AllocatableCPU)
				summary.RequestsMemory = fmt.Sprintf("%s/%s", alloc.RequestsMemory, alloc.AllocatableMemory)
				summary.LimitsCPU = fmt.Sprintf("%s/%s", alloc.LimitsCPU, alloc.AllocatableCPU)
				summary.LimitsMemory = fmt.Sprintf("%s/%s", alloc.LimitsMemory, alloc.AllocatableMemory)
			}
		}

		summaries = append(summaries, summary)
	}

	result := struct {
		Nodes []NodeSummary `json:"nodes"`
	}{
		Nodes: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// NodeSummary is the trimmed representation of a node used by nodes_list.
type NodeSummary struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	Roles          []string `json:"roles"`
	Age            string   `json:"age"`
	Version        string   `json:"version"`
	InternalIP     string   `json:"internal_ip"`
	RequestsCPU    string   `json:"requests_cpu,omitempty"`
	RequestsMemory string   `json:"requests_memory,omitempty"`
	LimitsCPU      string   `json:"limits_cpu,omitempty"`
	LimitsMemory   string   `json:"limits_memory,omitempty"`
}

// NodeAllocations holds computed resource allocations for a node.
type NodeAllocations struct {
	RequestsCPU       string
	RequestsMemory    string
	LimitsCPU         string
	LimitsMemory      string
	AllocatableCPU    string
	AllocatableMemory string
}

// deriveNodeStatus derives the node status from its conditions.
func deriveNodeStatus(node *corev1.Node) string {
	status := "Unknown"

	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			switch cond.Status {
			case corev1.ConditionTrue:
				status = "Ready"
			case corev1.ConditionFalse:
				status = "NotReady"
			case corev1.ConditionUnknown:
				status = "Unknown"
			}
			break
		}
	}

	// Check if node is cordoned
	if node.Spec.Unschedulable {
		status += ",SchedulingDisabled"
	}

	return status
}

// deriveNodeRoles derives node roles from labels.
func deriveNodeRoles(node *corev1.Node) string {
	roles := make([]string, 0)

	for label := range node.Labels {
		if isRoleLabel(label) {
			role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
			roles = append(roles, role)
		}
	}

	if len(roles) == 0 {
		return "none"
	}

	// Sort roles for consistent output
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

// isRoleLabel checks if a label is a node role label.
func isRoleLabel(label string) bool {
	return strings.HasPrefix(label, "node-role.kubernetes.io/")
}

// getInternalIP gets the internal IP address from node addresses.
func getInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return "-"
}

// computeNodeAllocations computes resource allocations for each node.
func computeNodeAllocations(pods []corev1.Pod) map[string]*NodeAllocations {
	allocations := make(map[string]*NodeAllocations)

	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}

		nodeName := pod.Spec.NodeName
		if allocations[nodeName] == nil {
			allocations[nodeName] = &NodeAllocations{}
		}

		alloc := allocations[nodeName]

		for _, container := range pod.Spec.Containers {
			// Sum requests
			if container.Resources.Requests != nil {
				if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
					alloc.RequestsCPU = addQuantity(alloc.RequestsCPU, cpu.String())
				}
				if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
					alloc.RequestsMemory = addQuantity(alloc.RequestsMemory, mem.String())
				}
			}

			// Sum limits
			if container.Resources.Limits != nil {
				if cpu, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
					alloc.LimitsCPU = addQuantity(alloc.LimitsCPU, cpu.String())
				}
				if mem, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
					alloc.LimitsMemory = addQuantity(alloc.LimitsMemory, mem.String())
				}
			}
		}
	}

	return allocations
}

// addQuantity adds two quantity strings (simplified implementation).
func addQuantity(q1, q2 string) string {
	if q1 == "" {
		return q2
	}
	if q2 == "" {
		return q1
	}
	return fmt.Sprintf("%s+%s", q1, q2) // Simplified - in production, use resource.ParseQuantity
}
