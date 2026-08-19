package tools

import (
	"bytes"
	"context"
	"fmt"
	"slices"
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

	var result bytes.Buffer
	first := true
	for k, v := range labels {
		if !first {
			result.WriteString(",")
		}
		fmt.Fprintf(&result, "%s=%s", k, v)
		first = false
	}

	return result.String()
}

func extractAnnotations(annotations map[string]string) map[string]string {
	result := make(map[string]string)

	ignoreKeys := []string{
		defaultContainerAnnotation,
		"kubectl.kubernetes.io/last-applied-configuration",
		"deployment.kubernetes.io/revision",
	}
	ignoreKeysPrefix := []string{
		"prometheus.io/",
		"meta.helm.sh/",
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

func extractLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)

	ignoreKeys := []string{
		"pod-template-hash",
		"controller-revision-hash",
		"statefulset.kubernetes.io/pod-name",
		"controller-uid",
		"job-name",
		"app.kubernetes.io/version",
		"app.kubernetes.io/managed-by",
	}
	ignoreKeysPrefix := []string{
		"batch.kubernetes.io/",
		"helm.sh/",
		"kustomize.toolkit.fluxcd.io/",
		"helm.toolkit.fluxcd.io/",
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

func toPodSpec(pod *corev1.Pod) PodSpec {
	return PodSpec{
		RestartPolicy:     string(pod.Spec.RestartPolicy),
		ServiceAccount:    pod.Spec.ServiceAccountName,
		PriorityClassName: pod.Spec.PriorityClassName,
		InitContainers:    toContainerInfoList(pod.Spec.InitContainers),
		Containers:        toContainerInfoList(pod.Spec.Containers),
		Volumes:           extractVolumeNames(pod.Spec.Volumes),
	}
}

func toContainerInfoList(containers []corev1.Container) []ContainerInfo {
	infoList := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		infoList = append(infoList, ContainerInfo{
			Name:  c.Name,
			Image: c.Image,
			Ports: extractContainerPorts(c.Ports),
		})
	}
	return infoList
}

func extractContainerPorts(ports []corev1.ContainerPort) []string {
	portList := make([]string, 0, len(ports))
	for _, p := range ports {
		portList = append(portList, fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
	}
	return portList
}

func extractVolumeNames(volumes []corev1.Volume) []string {
	volumeNames := make([]string, 0, len(volumes))
	for _, v := range volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	return volumeNames
}
