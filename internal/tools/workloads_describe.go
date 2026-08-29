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
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadDescribeResult represents the result of describing a workload.
type WorkloadDescribeResult struct {
	WorkloadSummary
	WorkloadSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"Conditions"`
	Events     []EventSummary  `json:"events,omitempty" jsonschema:"List of events"`

	Pods []PodSummary `json:"pods,omitempty" jsonschema:"List of pods owned by the workload"`
}

// RegisterWorkloadsDescribe adds the workloads_describe tool, which provides
// a rich structured summary of a workload: replicas, conditions, selector,
// strategy, update history.
func RegisterWorkloadsDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Workload"),
		mcp.WithDescription("Workload summary (replicas, conditions, selector, strategy, update history)."),
		mcp.WithString("name", mcp.Description("workload name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("kind", mcp.Description("kind: deployment, statefulset, or daemonset"), mcp.Enum("deployment", "statefulset", "daemonset")),
		mcp.WithOutputSchema[WorkloadDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("workloads_describe", opts...)
	s.AddTool(tool, handlerWorkloadsDescribe(mc))
}

// +kubebuilder:rbac:groups="",resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=daemonsets,verbs=get;list;watch

// handlerWorkloadsDescribe returns the handler function for the workloads_describe tool.
func handlerWorkloadsDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		kind := req.GetString("kind", "")
		if kind != "" && kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return mcp.NewToolResultErrorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "workloads_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"name", name,
			"kind", kind,
		)

		// Resolve kind if not provided
		resolvedKind, err := resolveWorkloadKind(ctx, client, namespace, name, kind)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Get the workload
		workload, err := getWorkloadByKind(ctx, client, namespace, name, resolvedKind)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("%s '%s' not found in namespace '%s'", resolvedKind, name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get %s '%s' in namespace '%s': %v", resolvedKind, name, namespace, err), nil
		}

		result := buildWorkloadDescribeResult(ctx, workload, client)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch

// buildWorkloadDescribeResult builds a WorkloadDescribeResult from a workload object.
func buildWorkloadDescribeResult(ctx context.Context, workload any, client *k8s.Client) *WorkloadDescribeResult {
	result := &WorkloadDescribeResult{}
	kind := ""

	switch w := workload.(type) {
	case *appsv1.Deployment:
		kind = "Deployment"
		result.WorkloadSummary = toWorkloadSummaryDeployment(w)
		result.Labels = extractLabels(w.Labels)
		result.Annotations = extractAnnotations(w.Annotations)
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.Strategy.Type)
		result.PodSpec = toPodSpec(&w.Spec.Template.Spec)

		for _, cond := range w.Status.Conditions {
			result.Conditions = append(result.Conditions, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}

	case *appsv1.StatefulSet:
		kind = "StatefulSet"
		result.WorkloadSummary = toWorkloadSummaryStatefulSet(w)
		result.Labels = extractLabels(w.Labels)
		result.Annotations = extractAnnotations(w.Annotations)
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
		result.PodSpec = toPodSpec(&w.Spec.Template.Spec)

		for _, cond := range w.Status.Conditions {
			result.Conditions = append(result.Conditions, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}

	case *appsv1.DaemonSet:
		kind = "DaemonSet"
		result.WorkloadSummary = toWorkloadSummaryDaemonSet(w)
		result.Labels = extractLabels(w.Labels)
		result.Annotations = extractAnnotations(w.Annotations)
		result.Selector = formatMatchLabels(w.Spec.Selector.MatchLabels)
		result.UpdateStrategy = string(w.Spec.UpdateStrategy.Type)
		result.PodSpec = toPodSpec(&w.Spec.Template.Spec)

		for _, cond := range w.Status.Conditions {
			result.Conditions = append(result.Conditions, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}
	}

	pods, err := client.CoreV1().Pods(result.Namespace).List(ctx, metav1.ListOptions{LabelSelector: result.Selector})
	if err == nil {
		result.Pods = make([]PodSummary, 0, len(pods.Items))
		for _, pod := range pods.Items {
			node := pod.Spec.NodeName
			if node == "" {
				node = "<pending>"
			}

			podInfo := PodSummary{
				Name:     pod.Name,
				Ready:    formatReady(pod.Status),
				Status:   string(pod.Status.Phase),
				Restarts: containerRestartCount(pod.Status),
				Age:      age.FormatAge(pod.CreationTimestamp),
				Node:     node,
			}

			result.Pods = append(result.Pods, podInfo)
		}
	}

	// List events for the workload
	if kind != "" {
		events, err := client.CoreV1().Events(result.Namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", kind, result.Name),
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return result
		}

		result.Events = make([]EventSummary, 0, len(events.Items))
		for _, e := range events.Items {
			firstSeen := ""
			if !e.FirstTimestamp.IsZero() {
				firstSeen = age.FormatAge(e.FirstTimestamp)
			}

			result.Events = append(result.Events, EventSummary{
				Namespace: e.Namespace,
				FirstSeen: firstSeen,
				Age:       age.FormatEventAge(e),
				Message:   e.Message,
				Reason:    e.Reason,
				Type:      e.Type,
			})
		}

		if len(result.Events) > 50 {
			result.Events = result.Events[:50]
		}
	}

	return result
}
