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
	"context"
	"fmt"
	"io"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// formatReady extracts the ready status from pod status.
func formatReady(status corev1.PodStatus) string {
	ready := 0
	total := len(status.ContainerStatuses)
	for _, cs := range status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

// containerRestartCount returns the total restart count for all containers in the pod.
func containerRestartCount(status corev1.PodStatus) int32 {
	total := int32(0)
	for _, cs := range status.ContainerStatuses {
		total += cs.RestartCount
	}
	return total
}

// toPodSummary converts a pod to a PodSummary.
func toPodSummary(pod *corev1.Pod) PodSummary {
	node := pod.Spec.NodeName
	if node == "" {
		node = "<pending>"
	}

	return PodSummary{
		Namespace:       pod.Namespace,
		Name:            pod.Name,
		Ready:           formatReady(pod.Status),
		Status:          string(pod.Status.Phase),
		Restarts:        containerRestartCount(pod.Status),
		Age:             age.FormatAge(pod.CreationTimestamp),
		Node:            node,
		Zone:            pod.Labels["topology.kubernetes.io/zone"],
		OwnerReferences: ownerReferencesParent(pod),
	}
}

func toPodConditionInfo(pod *corev1.Pod) []ConditionInfo {
	conditions := make([]ConditionInfo, 0, len(pod.Status.Conditions))
	for _, cond := range pod.Status.Conditions {
		conditions = append(conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}
	return conditions
}

// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

// fetchPodLogStream fetches logs from a single pod and returns a LogStream.
func fetchPodLogStream(ctx context.Context, client *k8s.Client, namespace, podName, container, stream string, tail, sinceSeconds int, previous bool) (LogStream, error) {
	// Get pod to check container names
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return LogStream{}, err
	}

	if container == "" {
		if pod.Annotations != nil {
			if defaultContainer, ok := pod.Annotations[defaultContainerAnnotation]; ok && defaultContainer != "" {
				container = defaultContainer
			}
		}
		if container == "" {
			container = pod.Spec.Containers[0].Name
		}
	}

	// Get log options
	tailInt64 := int64(tail)
	logOpts := &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailInt64,
		Previous:  previous,
	}

	if stream != "" {
		logOpts.Stream = &stream
	}

	if sinceSeconds > 0 {
		sinceSecondsInt64 := int64(sinceSeconds)
		logOpts.SinceSeconds = &sinceSecondsInt64
	}

	// Fetch logs
	logReq := client.CoreV1().Pods(namespace).GetLogs(podName, logOpts)
	logStream, err := logReq.Stream(ctx)
	if err != nil {
		return LogStream{}, fmt.Errorf("failed to fetch logs for pod '%s' container '%s': %v", podName, container, err)
	}
	defer logStream.Close() //nolint:errcheck

	// Read all logs
	logContent, err := io.ReadAll(logStream)
	if err != nil {
		return LogStream{}, fmt.Errorf("failed to read logs for pod '%s' container '%s': %v", podName, container, err)
	}

	if client.Sanitizer() != nil {
		logContent = []byte(client.Sanitizer().Sanitize(string(logContent)))
	}

	return LogStream{
		Pod:       podName,
		Container: container,
		Logs:      string(logContent),
	}, nil
}
