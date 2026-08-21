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
	"slices"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// toNodeSummary converts a node to a NodeSummary.
func toNodeSummary(node *corev1.Node) NodeSummary {
	return NodeSummary{
		Name:           node.Name,
		Status:         deriveNodeStatus(node),
		Roles:          deriveNodeRoles(node),
		Age:            formatAge(node.CreationTimestamp),
		KubeletVersion: node.Status.NodeInfo.KubeletVersion,
		ImageVersion:   node.Status.NodeInfo.OSImage,
		InternalIP:     getInternalIP(node),
		NodeCapacityInfo: NodeCapacityInfo{
			CPU:    node.Status.Capacity.Cpu().String(),
			Memory: node.Status.Capacity.Memory().String(),
			Pods:   int(node.Status.Capacity.Pods().Value()),
		},
	}
}

func toNodeSpec(node *corev1.Node) NodeSpec {
	taints := make([]TaintInfo, 0, len(node.Spec.Taints))
	for _, taint := range node.Spec.Taints {
		taints = append(taints, TaintInfo{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		})
	}

	return NodeSpec{
		Unschedulable: node.Spec.Unschedulable,
		Taints:        taints,
	}
}

func toNodeConditionInfo(node *corev1.Node) []ConditionInfo {
	conditions := make([]ConditionInfo, 0, len(node.Status.Conditions))
	for _, cond := range node.Status.Conditions {
		conditions = append(conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}
	return conditions
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

func extractNodeAnnotations(annotations map[string]string) map[string]string {
	result := make(map[string]string)

	ignoreKeys := []string{
		"node.alpha.kubernetes.io/ttl",
	}
	ignoreKeysPrefix := []string{
		"extensions.talos.dev/",
		"talos.dev/owned",
	}

	for k, v := range annotations {
		if slices.Contains(ignoreKeys, k) {
			continue
		}

		ignore := false
		for _, prefix := range ignoreKeysPrefix {
			if strings.HasPrefix(k, prefix) {
				ignore = true
				break
			}
		}
		if ignore {
			continue
		}

		result[k] = v
	}

	return result
}

func extractNodeLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)

	ignoreKeys := []string{
		"kubernetes.io/role",
	}
	ignoreKeysPrefix := []string{
		"beta.kubernetes.io/",
		"failure-domain.beta.kubernetes.io/",
		"extensions.talos.dev/",
	}

	for k, v := range labels {
		if slices.Contains(ignoreKeys, k) {
			continue
		}

		ignore := false
		for _, prefix := range ignoreKeysPrefix {
			if strings.HasPrefix(k, prefix) {
				ignore = true
				break
			}
		}
		if ignore {
			continue
		}

		result[k] = v
	}

	return result
}
