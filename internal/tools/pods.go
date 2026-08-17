package tools

import (
	"context"
	"fmt"
	"io"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
func toPodSummary(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod) PodSummary {
	node := pod.Spec.NodeName
	if node == "" {
		node = "<pending>"
	}

	ownerRefs, _ := ownerReferences(ctx, client, pod) //nolint:errcheck

	return PodSummary{
		Namespace:       pod.Namespace,
		Name:            pod.Name,
		Ready:           formatReady(pod.Status),
		Status:          string(pod.Status.Phase),
		Restarts:        containerRestartCount(pod.Status),
		Age:             formatAge(pod.CreationTimestamp),
		Node:            node,
		OwnerReferences: ownerRefs,
	}
}

// fetchPodLogStream fetches logs from a single pod and returns a LogStream.
func fetchPodLogStream(ctx context.Context, client *k8s.Client, namespace, podName, container string, tail, sinceSeconds int, previous bool) (LogStream, error) {
	// Get pod to check container names
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return LogStream{}, fmt.Errorf("failed to get pod '%s': %v", podName, err)
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

	return LogStream{
		Pod:       podName,
		Container: container,
		Logs:      string(logContent),
	}, nil
}
