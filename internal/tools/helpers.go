package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

// ownerReferences extracts the pod's owner references as simplified structs.
// If an owner is a ReplicaSet, it fetches the ReplicaSet and returns its owner references
// to get the actual workload (e.g., Deployment) that owns the pod.
func ownerReferences(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod) ([]OwnerReference, error) {
	refs := make([]OwnerReference, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		ownerRef := OwnerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
		}

		// If the owner is a ReplicaSet, fetch it to get its owner references
		if ref.Kind == "ReplicaSet" {
			rs, err := client.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
			if err == nil && rs != nil && len(rs.OwnerReferences) > 0 {
				// Use the ReplicaSet's owner references instead
				refs = append(refs, ownerReferencesFromMetav1(rs.OwnerReferences)...)
				continue
			}
		}

		refs = append(refs, ownerRef)
	}
	return refs, nil
}

// ownerReferencesFromMetav1 converts metav1.OwnerReference to OwnerReference.
func ownerReferencesFromMetav1(refs []metav1.OwnerReference) []OwnerReference {
	result := make([]OwnerReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, OwnerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
		})
	}
	return result
}

// formatAge calculates the age from creation time.
func formatAge(created metav1.Time) string {
	now := time.Now()
	diff := now.Sub(created.Time)

	if diff < time.Minute {
		return "0s"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd", int(diff.Hours()/24))
}

// formatAgeMin calculates the age from creation time with hours and minutes granularity.
func formatAgeMin(created metav1.Time) string {
	now := time.Now()
	diff := now.Sub(created.Time)

	if diff < time.Minute {
		return "0s"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		minutes := int(diff.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dd", int(diff.Hours()/24))
}

// formatDuration calculates the duration between two times.
func formatDuration(end, start metav1.Time) string {
	endTime := end.Time
	startTime := start.Time
	diff := endTime.Sub(startTime)
	if diff < time.Second {
		return "0s"
	}
	if diff < time.Minute {
		return fmt.Sprintf("%ds", int(diff.Seconds()))
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd", int(diff.Hours()/24))
}

// formatMatchLabels converts match labels to a comma-separated string.
func formatMatchLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	var result strings.Builder
	first := true
	for k, v := range labels {
		if !first {
			result.WriteString(", ")
		}
		fmt.Fprintf(&result, "%s=%s", k, v)
		first = false
	}

	return result.String()
}

// extractContainerInfo extracts container information from a pod spec.
func extractContainerInfo(containers []corev1.Container) []ContainerInfo {
	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		ports := make([]int32, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, p.ContainerPort)
		}
		result = append(result, ContainerInfo{
			Name:  c.Name,
			Image: c.Image,
			Ports: ports,
		})
	}
	return result
}
