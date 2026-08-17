package tools

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// toNodeSummary converts a pod to a NodeSummary.
func toNodeSummary(node corev1.Node) NodeSummary {
	return NodeSummary{
		Name:           node.Name,
		Status:         deriveNodeStatus(&node),
		Roles:          deriveNodeRoles(&node),
		Age:            formatAge(node.CreationTimestamp),
		KubeletVersion: node.Status.NodeInfo.KubeletVersion,
		ImageVersion:   node.Status.NodeInfo.OSImage,
		InternalIP:     getInternalIP(&node),
		Capacity: NodeCapacityInfo{
			CPU:    node.Status.Capacity.Cpu().String(),
			Memory: node.Status.Capacity.Memory().String(),
			Pods:   int(node.Status.Capacity.Pods().Value()),
		},
	}
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
func deriveNodeRoles(node *corev1.Node) []string {
	roles := make([]string, 0)

	for label := range node.Labels {
		if role, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok {
			roles = append(roles, role)
		}
	}

	if len(roles) == 0 {
		return []string{}
	}

	// Sort roles for consistent output
	sort.Strings(roles)
	return roles
}

// getInternalIP gets the internal IP address from node addresses.
func getInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
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
