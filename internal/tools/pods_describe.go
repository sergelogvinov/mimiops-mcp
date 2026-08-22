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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodDescribeResult represents the result of describing a pod.
type PodDescribeResult struct {
	PodSummary
	PodSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"Conditions"`
	Events     []EventSummary  `json:"events,omitempty" jsonschema:"List of events"`
}

// RegisterPodsDescribe adds the pods_describe tool, which provides a structured pod summary.
func RegisterPodsDescribe(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("pods_describe",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Pod"),
		mcp.WithDescription("Pod summary (conditions, container statuses, node, tolerations)"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[PodDescribeResult](),
	)
	s.AddTool(tool, handlerPodsDescribe(client))
}

// handlerPodsDescribe returns a handler function for the pods_describe tool.
func handlerPodsDescribe(client *k8s.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildPodDescribeResult builds a PodDescribeResult from a Pod.
func buildPodDescribeResult(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *PodDescribeResult {
	result := &PodDescribeResult{
		PodSummary:  toPodSummary(ctx, client, pod),
		PodSpec:     toPodSpec(pod),
		Annotations: extractAnnotations(pod.Annotations),
		Labels:      extractLabels(pod.Labels),
		Conditions:  toPodConditionInfo(pod),
	}

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
				firstSeen = formatAge(e.FirstTimestamp)
			}

			result.Events = append(result.Events, EventSummary{
				FirstSeen: firstSeen,
				Age:       formatEventAge(e),
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
