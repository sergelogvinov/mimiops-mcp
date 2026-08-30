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

// +kubebuilder:rbac:groups="",resources=replicasets,verbs=get;list;watch

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

func ownerReferencesParent(pod *corev1.Pod) []OwnerReference {
	refs := make([]OwnerReference, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		ownerRef := OwnerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
		}

		refs = append(refs, ownerRef)
	}
	return refs
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
		"deployment.kubernetes.io/revision",
		"checksum/config",
		"checksum/secret",
		"checksum/configmap",
	}
	ignoreKeysPrefix := []string{
		"kubectl.kubernetes.io/",
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
		"job-name",
		"controller-uid",
		"statefulset.kubernetes.io/pod-name",
		"apps.kubernetes.io/pod-index",
		"batch.kubernetes.io/controller-uid",
		"app.kubernetes.io/version",
	}
	ignoreKeysPrefix := []string{
		"batch.kubernetes.io/",
		"helm.sh/",
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

// toPodSpec converts a PodSpec into a PodSpec summary. podName is the name of
// the concrete Pod the spec belongs to; it may be empty when only a controller
// template is described.
func toPodSpec(podName string, spec *corev1.PodSpec) PodSpec {
	tolerations := make([]TolerationInfo, 0, len(spec.Tolerations))
	for _, toleration := range spec.Tolerations {
		tolerations = append(tolerations, TolerationInfo{
			Key:    toleration.Key,
			Value:  toleration.Value,
			Effect: string(toleration.Effect),
		})
	}

	return PodSpec{
		RestartPolicy:     string(spec.RestartPolicy),
		ServiceAccount:    spec.ServiceAccountName,
		PriorityClassName: spec.PriorityClassName,
		InitContainers:    toContainerInfoList(spec.InitContainers),
		Containers:        toContainerInfoList(spec.Containers),
		Volumes:           extractVolumeNames(podName, spec.Volumes),
		NodeSelector:      spec.NodeSelector,
		Tolerations:       tolerations,
	}
}

func toContainerInfoList(containers []corev1.Container) []ContainerInfo {
	infoList := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		infoList = append(infoList, ContainerInfo{
			Name:     c.Name,
			Image:    c.Image,
			Ports:    extractContainerPorts(c.Ports),
			Requests: extractResourceList(c.Resources.Requests),
			Limits:   extractResourceList(c.Resources.Limits),
		})
	}
	return infoList
}

func extractResourceList(resources corev1.ResourceList) map[string]string {
	result := make(map[string]string, len(resources))
	for name, quantity := range resources {
		result[string(name)] = quantity.String()
	}
	return result
}

func extractContainerPorts(ports []corev1.ContainerPort) []string {
	portList := make([]string, 0, len(ports))
	for _, p := range ports {
		portList = append(portList, fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
	}
	return portList
}

// extractVolumeNames converts volumes into VolumesInfo summaries. podName is
// the name of the concrete Pod that mounts the volumes: generic ephemeral
// volumes derive their PVC name from it. It may be empty when the pod name is
// unknown, in which case the derived claim name is left unset.
func extractVolumeNames(podName string, volumes []corev1.Volume) []VolumesInfo {
	volumeInfos := make([]VolumesInfo, 0, len(volumes))
	for _, v := range volumes {
		if strings.HasPrefix(v.Name, "kube-api-access-") {
			continue
		}

		volumeInfos = append(volumeInfos, VolumesInfo{
			Name: v.Name,
			Type: func() string {
				switch {
				case v.VolumeSource.Secret != nil:
					return "Secret"
				case v.VolumeSource.ConfigMap != nil:
					return "ConfigMap"
				case v.VolumeSource.PersistentVolumeClaim != nil:
					return "PersistentVolumeClaim"
				case v.VolumeSource.EmptyDir != nil:
					return "EmptyDir"
				case v.VolumeSource.HostPath != nil:
					return "HostPath"
				case v.VolumeSource.Projected != nil:
					return "Projected"
				case v.VolumeSource.Ephemeral != nil:
					return "Ephemeral"
				default:
					return ""
				}
			}(),
			SecretName: func() string {
				if v.VolumeSource.Secret != nil {
					return v.VolumeSource.Secret.SecretName
				}
				return ""
			}(),
			ConfigMapName: func() string {
				if v.VolumeSource.ConfigMap != nil {
					return v.VolumeSource.ConfigMap.Name
				}
				return ""
			}(),
			PersistentVolumeClaimName: func() string {
				if v.VolumeSource.PersistentVolumeClaim != nil {
					return v.VolumeSource.PersistentVolumeClaim.ClaimName
				}
				if v.VolumeSource.Ephemeral != nil && v.VolumeSource.Ephemeral.VolumeClaimTemplate != nil {
					if v.VolumeSource.Ephemeral.VolumeClaimTemplate.Name == "" && podName != "" {
						return fmt.Sprintf("%s-%s", podName, v.Name)
					}
					return v.VolumeSource.Ephemeral.VolumeClaimTemplate.Name
				}
				return ""
			}(),
			Optional: func() bool {
				if v.VolumeSource.Secret != nil {
					return v.VolumeSource.Secret.Optional != nil && *v.VolumeSource.Secret.Optional
				}
				if v.VolumeSource.ConfigMap != nil {
					return v.VolumeSource.ConfigMap.Optional != nil && *v.VolumeSource.ConfigMap.Optional
				}
				return false
			}(),
			StorageClass: func() string {
				if v.VolumeSource.Ephemeral != nil && v.VolumeSource.Ephemeral.VolumeClaimTemplate != nil {
					if v.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName != nil {
						return *v.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName
					}
					return ""
				}
				return ""
			}(),
			Labels: func() map[string]string {
				if v.VolumeSource.Ephemeral != nil && v.VolumeSource.Ephemeral.VolumeClaimTemplate != nil {
					return extractLabels(v.VolumeSource.Ephemeral.VolumeClaimTemplate.Labels)
				}
				return nil
			}(),
			Annotations: func() map[string]string {
				if v.VolumeSource.Ephemeral != nil && v.VolumeSource.Ephemeral.VolumeClaimTemplate != nil {
					return extractAnnotations(v.VolumeSource.Ephemeral.VolumeClaimTemplate.Annotations)
				}
				return nil
			}(),
		})
	}
	return volumeInfos
}
