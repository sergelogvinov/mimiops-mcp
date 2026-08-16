package tools

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

// ownerReferences extracts the pod's owner references as simplified structs.
func ownerReferences(pod *corev1.Pod) []OwnerReference {
	refs := make([]OwnerReference, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		refs = append(refs, OwnerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
		})
	}
	return refs
}

// ownerReferencesMeta extracts owner references from a PartialObjectMetadata
// (used by pods_list's partial metadata responses).
func ownerReferencesMeta(pod *metav1.PartialObjectMetadata) []OwnerReference {
	refs := make([]OwnerReference, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		refs = append(refs, OwnerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
		})
	}
	return refs
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
