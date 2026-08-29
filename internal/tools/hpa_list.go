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
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPAListResult represents the result of listing HorizontalPodAutoscalers.
type HPAListResult struct {
	HPA []HPASummary `json:"hpa" jsonschema:"List of HorizontalPodAutoscalers"`
}

// RegisterHPAList adds the hpa_list tool, which lists HorizontalPodAutoscalers in a namespace
// (or all namespaces).
func RegisterHPAList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List HorizontalPodAutoscalers"),
		mcp.WithDescription("List HorizontalPodAutoscalers in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithOutputSchema[HPAListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("horizontalpodautoscaler_list", opts...)
	s.AddTool(tool, handlerHPAList(mc))
}

// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=list;watch

// handlerHPAList returns a handler function for the hpa_list tool.
func handlerHPAList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "horizontalpodautoscaler_list called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		// List horizontal pod autoscalers
		hpas, err := client.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no HorizontalPodAutoscalers found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list HorizontalPodAutoscalers: %v", err), nil
		}

		result := HPAListResult{
			HPA: make([]HPASummary, 0, len(hpas.Items)),
		}

		// Build result
		for _, hpa := range hpas.Items {
			result.HPA = append(result.HPA, toHPASummary(&hpa))
		}

		// Build fallback text
		fallbackText := "No HorizontalPodAutoscalers found"
		if len(result.HPA) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}

// toHPASummary converts a HorizontalPodAutoscaler to an HPASummary.
func toHPASummary(hpa *autoscalingv2.HorizontalPodAutoscaler) HPASummary {
	minPods := int32(1)
	if hpa.Spec.MinReplicas != nil {
		minPods = *hpa.Spec.MinReplicas
	}

	ref := hpa.Spec.ScaleTargetRef

	return HPASummary{
		Name:      hpa.Name,
		Namespace: hpa.Namespace,
		Reference: fmt.Sprintf("%s/%s", ref.Kind, ref.Name),
		Targets:   hpaTargets(hpa),
		MinPods:   minPods,
		MaxPods:   hpa.Spec.MaxReplicas,
		Replicas:  hpa.Status.CurrentReplicas,
		Age:       age.FormatAge(hpa.CreationTimestamp),
	}
}

// hpaTargets formats the HPA metrics as kubectl-style "current/target" pairs,
// e.g., "50%/80%" or "<unknown>/80%".
func hpaTargets(hpa *autoscalingv2.HorizontalPodAutoscaler) string {
	if len(hpa.Spec.Metrics) == 0 {
		return ""
	}

	parts := make([]string, 0, len(hpa.Spec.Metrics))
	for _, metric := range hpa.Spec.Metrics {
		current := "<unknown>"
		for _, cm := range hpa.Status.CurrentMetrics {
			if hpaMetricMatches(metric, cm) {
				current = formatHPACurrentMetric(cm)
				break
			}
		}

		parts = append(parts, current+"/"+formatHPAMetricTarget(metric))
	}

	return strings.Join(parts, ", ")
}

// hpaMetricMatches reports whether a current metric status corresponds to the spec metric.
func hpaMetricMatches(spec autoscalingv2.MetricSpec, current autoscalingv2.MetricStatus) bool {
	if spec.Type != current.Type {
		return false
	}

	switch spec.Type { //nolint:exhaustive
	case autoscalingv2.ResourceMetricSourceType:
		return spec.Resource != nil && current.Resource != nil && spec.Resource.Name == current.Resource.Name
	case autoscalingv2.ContainerResourceMetricSourceType:
		return spec.ContainerResource != nil && current.ContainerResource != nil &&
			spec.ContainerResource.Name == current.ContainerResource.Name &&
			spec.ContainerResource.Container == current.ContainerResource.Container
	case autoscalingv2.PodsMetricSourceType:
		return spec.Pods != nil && current.Pods != nil && spec.Pods.Metric.Name == current.Pods.Metric.Name
	case autoscalingv2.ObjectMetricSourceType:
		return spec.Object != nil && current.Object != nil && spec.Object.Metric.Name == current.Object.Metric.Name
	case autoscalingv2.ExternalMetricSourceType:
		return spec.External != nil && current.External != nil && spec.External.Metric.Name == current.External.Metric.Name
	}

	return false
}

// formatHPAMetricTarget formats the target value of a spec metric.
func formatHPAMetricTarget(metric autoscalingv2.MetricSpec) string {
	switch metric.Type { //nolint:exhaustive
	case autoscalingv2.ResourceMetricSourceType:
		if metric.Resource != nil {
			if metric.Resource.Target.AverageUtilization != nil {
				return fmt.Sprintf("%d%%", *metric.Resource.Target.AverageUtilization)
			}
			if metric.Resource.Target.AverageValue != nil {
				return metric.Resource.Target.AverageValue.String()
			}
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if metric.ContainerResource != nil {
			if metric.ContainerResource.Target.AverageUtilization != nil {
				return fmt.Sprintf("%d%%", *metric.ContainerResource.Target.AverageUtilization)
			}
			if metric.ContainerResource.Target.AverageValue != nil {
				return metric.ContainerResource.Target.AverageValue.String()
			}
		}
	case autoscalingv2.PodsMetricSourceType:
		if metric.Pods != nil && metric.Pods.Target.AverageValue != nil {
			return metric.Pods.Target.AverageValue.String()
		}
	case autoscalingv2.ObjectMetricSourceType:
		if metric.Object != nil && metric.Object.Target.Value != nil {
			return metric.Object.Target.Value.String()
		}
	case autoscalingv2.ExternalMetricSourceType:
		if metric.External != nil {
			if metric.External.Target.Value != nil {
				return metric.External.Target.Value.String()
			}
			if metric.External.Target.AverageValue != nil {
				return metric.External.Target.AverageValue.String()
			}
		}
	}

	return "<none>"
}

// formatHPACurrentMetric formats the current value of a metric status.
func formatHPACurrentMetric(metric autoscalingv2.MetricStatus) string {
	switch metric.Type { //nolint:exhaustive
	case autoscalingv2.ResourceMetricSourceType:
		if metric.Resource != nil {
			if metric.Resource.Current.AverageUtilization != nil {
				return fmt.Sprintf("%d%%", *metric.Resource.Current.AverageUtilization)
			}
			if metric.Resource.Current.AverageValue != nil {
				return metric.Resource.Current.AverageValue.String()
			}
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if metric.ContainerResource != nil {
			if metric.ContainerResource.Current.AverageUtilization != nil {
				return fmt.Sprintf("%d%%", *metric.ContainerResource.Current.AverageUtilization)
			}
			if metric.ContainerResource.Current.AverageValue != nil {
				return metric.ContainerResource.Current.AverageValue.String()
			}
		}
	case autoscalingv2.PodsMetricSourceType:
		if metric.Pods != nil && metric.Pods.Current.AverageValue != nil {
			return metric.Pods.Current.AverageValue.String()
		}
	case autoscalingv2.ObjectMetricSourceType:
		if metric.Object != nil && metric.Object.Current.Value != nil {
			return metric.Object.Current.Value.String()
		}
	case autoscalingv2.ExternalMetricSourceType:
		if metric.External != nil {
			if metric.External.Current.Value != nil {
				return metric.External.Current.Value.String()
			}
			if metric.External.Current.AverageValue != nil {
				return metric.External.Current.AverageValue.String()
			}
		}
	}

	return "<unknown>"
}
