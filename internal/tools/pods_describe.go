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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PodDescribeResult represents the result of describing a pod.
type PodDescribeResult struct {
	PodSummary
	PodSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"Conditions"`
	Usage      *ResourceUsage  `json:"usage,omitempty" jsonschema:"Resource usage from the metrics API"`
	Events     []EventSummary  `json:"events,omitempty" jsonschema:"List of events"`
}

// RegisterPodsDescribe adds the pods_describe tool, which provides a structured pod summary.
func RegisterPodsDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Pod"),
		mcp.WithDescription("Pod summary (conditions, container statuses, usage, node, tolerations)"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[PodDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("pods_describe", opts...)
	s.AddTool(tool, handlerPodsDescribe(mc))
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=metrics.k8s.io,resources=pods,verbs=get;list;watch

// handlerPodsDescribe returns a handler function for the pods_describe tool.
func handlerPodsDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "pods_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"pod", name,
		)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("pod '%s' in namespace '%s' not found", name, namespace), nil
			}

			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildPodDescribeResult(ctx, client, pod)
		return mcp.NewToolResultStructured(result, formatter.ToText(result)), nil
	}
}

// fetchPodUsage fetches the pod's resource usage. It prefers the windowed
// averages from custom.metrics.k8s.io and falls back to the point-in-time
// values from metrics.k8s.io. It returns nil when neither API is available,
// so the describe result degrades gracefully without usage data.
func fetchPodUsage(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *ResourceUsage {
	if usage := fetchCustomUsage(ctx, client, schema.GroupKind{Kind: "Pod"}, pod.Namespace, pod.Name, pod.UID); usage != nil {
		return usage
	}

	return fetchPodMetricsUsage(ctx, client, pod)
}

// fetchPodMetricsUsage fetches the pod's current resource usage from metrics.k8s.io.
// It returns nil when the metrics API is unavailable (e.g., no metrics-server).
func fetchPodMetricsUsage(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *ResourceUsage {
	log := logger.FromContext(ctx)

	metricsClient, err := client.Metrics()
	if err != nil {
		log.DebugContext(ctx, "failed to create metrics client", "pod", pod.Name, "err", err)
		return nil
	}

	podMetrics, err := metricsClient.MetricsV1beta1().PodMetricses(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		// The metrics API is optional; NotFound usually means metrics-server is not installed.
		log.DebugContext(ctx, "pod metrics not available", "pod", pod.Name, "err", err)
		return nil
	}

	return sumContainerUsage(podMetrics.Containers, true)
}

// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=events/status,verbs=list;watch

// buildPodDescribeResult builds a PodDescribeResult from a Pod.
func buildPodDescribeResult(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *PodDescribeResult {
	result := &PodDescribeResult{
		PodSummary:  toPodSummary(pod),
		PodSpec:     toPodSpec(pod.Name, &pod.Spec),
		Annotations: extractAnnotations(pod.Annotations),
		Labels:      extractLabels(pod.Labels),
		Conditions:  toPodConditionInfo(pod),
		Usage:       fetchPodUsage(ctx, client, pod),
	}

	result.PodSpec.QOSClass = string(pod.Status.QOSClass)
	result.PodSummary.OwnerReferences, _ = ownerReferences(ctx, client, pod) //nolint:errcheck

	// List events
	events, err := client.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return result
	}

	result.Events = make([]EventSummary, 0, len(events.Items))
	for _, e := range events.Items {
		if e.InvolvedObject.Kind == "Pod" && e.InvolvedObject.Name == pod.Name {
			firstSeen := ""
			if !e.FirstTimestamp.IsZero() {
				firstSeen = age.FormatAge(e.FirstTimestamp)
			}

			result.Events = append(result.Events, EventSummary{
				FirstSeen: firstSeen,
				Age:       age.FormatEventAge(e),
				Message:   fmt.Sprintf("%s: %s", e.InvolvedObject.FieldPath, e.Message),
				Reason:    e.Reason,
				Type:      e.Type,
			})
		}
	}

	if len(result.Events) > 50 {
		result.Events = result.Events[:50]
	}

	return result
}
