package tools

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
