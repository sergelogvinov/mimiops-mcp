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
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPADescribeResult represents the result of describing a HorizontalPodAutoscaler.
type HPADescribeResult struct {
	HPASummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	DesiredReplicas int32           `json:"desired_replicas" jsonschema:"Desired number of replicas"`
	LastScaleTime   string          `json:"last_scale_time,omitempty" jsonschema:"Time since the last scale event"`
	Behavior        string          `json:"behavior,omitempty" jsonschema:"Scaling behavior policies"`
	Conditions      []ConditionInfo `json:"conditions,omitempty" jsonschema:"List of conditions"`
	Events          []EventSummary  `json:"events,omitempty" jsonschema:"List of events"`
}

// RegisterHPADescribe adds the hpa_describe tool, which describes a single HorizontalPodAutoscaler.
func RegisterHPADescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe HorizontalPodAutoscaler"),
		mcp.WithDescription("Describe a single HorizontalPodAutoscaler (conditions, behavior, desired replicas, events)."),
		mcp.WithString("name", mcp.Description("HorizontalPodAutoscaler name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[HPADescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("horizontalpodautoscaler_describe", opts...)
	s.AddTool(tool, handlerHPADescribe(mc))
}

// handlerHPADescribe returns a handler function for the hpa_describe tool.
func handlerHPADescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
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
		log.DebugContext(ctx, "horizontalpodautoscaler_describe called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
		)

		// Get the horizontal pod autoscaler
		hpa, err := client.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("horizontal pod autoscaler '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get horizontal pod autoscaler '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildHPADescribeResult(ctx, client, hpa)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildHPADescribeResult builds an HPADescribeResult from a HorizontalPodAutoscaler.
func buildHPADescribeResult(ctx context.Context, client *k8s.Client, hpa *autoscalingv2.HorizontalPodAutoscaler) *HPADescribeResult {
	lastScaleTime := ""
	if hpa.Status.LastScaleTime != nil {
		lastScaleTime = formatAge(*hpa.Status.LastScaleTime)
	}

	conditions := make([]ConditionInfo, 0, len(hpa.Status.Conditions))
	for _, cond := range hpa.Status.Conditions {
		conditions = append(conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	result := &HPADescribeResult{
		HPASummary:      toHPASummary(hpa),
		Annotations:     extractAnnotations(hpa.Annotations),
		Labels:          extractLabels(hpa.Labels),
		DesiredReplicas: hpa.Status.DesiredReplicas,
		LastScaleTime:   lastScaleTime,
		Behavior:        hpaBehaviorString(hpa.Spec.Behavior),
		Conditions:      conditions,
	}

	// List events for the horizontal pod autoscaler
	events, err := client.CoreV1().Events(hpa.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=HorizontalPodAutoscaler,involvedObject.name=%s", hpa.Name),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return result
	}

	result.Events = make([]EventSummary, 0, len(events.Items))
	for _, e := range events.Items {
		firstSeen := ""
		if !e.FirstTimestamp.IsZero() {
			firstSeen = formatAge(e.FirstTimestamp)
		}

		result.Events = append(result.Events, EventSummary{
			Namespace: e.Namespace,
			FirstSeen: firstSeen,
			Age:       formatEventAge(e),
			Message:   e.Message,
			Reason:    e.Reason,
			Type:      e.Type,
		})
	}

	if len(result.Events) > 50 {
		result.Events = result.Events[:50]
	}

	return result
}

// hpaBehaviorString renders the scaling behavior as a compact summary, e.g.:
// "scaleUp: Percent:100/15s (select: Max); scaleDown: disabled".
func hpaBehaviorString(behavior *autoscalingv2.HorizontalPodAutoscalerBehavior) string {
	if behavior == nil {
		return ""
	}

	var parts []string
	if behavior.ScaleUp != nil {
		parts = append(parts, "scaleUp: "+hpaScalingRulesString(behavior.ScaleUp))
	}
	if behavior.ScaleDown != nil {
		parts = append(parts, "scaleDown: "+hpaScalingRulesString(behavior.ScaleDown))
	}

	return strings.Join(parts, "; ")
}

// hpaScalingRulesString renders one direction of the scaling behavior.
func hpaScalingRulesString(rules *autoscalingv2.HPAScalingRules) string {
	if rules.SelectPolicy != nil && *rules.SelectPolicy == autoscalingv2.DisabledPolicySelect {
		return "disabled"
	}

	policies := make([]string, 0, len(rules.Policies))
	for _, p := range rules.Policies {
		policies = append(policies, fmt.Sprintf("%s:%d/%ds", p.Type, p.Value, p.PeriodSeconds))
	}

	s := strings.Join(policies, ", ")
	if s == "" {
		s = "default"
	}

	if rules.SelectPolicy != nil {
		s += fmt.Sprintf(" (select: %s)", *rules.SelectPolicy)
	}
	if rules.StabilizationWindowSeconds != nil && *rules.StabilizationWindowSeconds > 0 {
		s += fmt.Sprintf(" (stabilization: %ds)", *rules.StabilizationWindowSeconds)
	}

	return s
}
