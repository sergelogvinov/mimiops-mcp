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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LimitRangeDescribeResult represents the result of describing a LimitRange.
type LimitRangeDescribeResult struct {
	LimitRangeSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Spec LimitRangeSpec `json:"spec" jsonschema:"Spec of the LimitRange"`
}

// RegisterLimitRangesDescribe adds the limitranges_describe tool, which describes a single LimitRange.
func RegisterLimitRangesDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe LimitRange"),
		mcp.WithDescription("Describe a single LimitRange's full spec."),
		mcp.WithString("name", mcp.Description("LimitRange name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[LimitRangeDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("limitranges_describe", opts...)
	s.AddTool(tool, handlerLimitRangesDescribe(mc))
}

// handlerLimitRangesDescribe returns a handler function for the limitranges_describe tool.
func handlerLimitRangesDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "limitranges_describe called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
		)

		// Get the limit range
		lr, err := client.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("limit range '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get limit range '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildLimitRangeDescribeResult(lr)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildLimitRangeDescribeResult builds a LimitRangeDescribeResult from a LimitRange.
func buildLimitRangeDescribeResult(lr *corev1.LimitRange) *LimitRangeDescribeResult {
	return &LimitRangeDescribeResult{
		LimitRangeSummary: LimitRangeSummary{
			Name:      lr.Name,
			Namespace: lr.Namespace,
			Types:     deriveLimitRangeTypes(lr),
			Age:       age.FormatAge(lr.CreationTimestamp),
		},
		Annotations: extractAnnotations(lr.Annotations),
		Labels:      extractLabels(lr.Labels),
		Spec: LimitRangeSpec{
			Limits: toLimitRangeLimits(lr),
		},
	}
}

// toLimitRangeLimits converts a LimitRange's spec.limits to the typed representation.
func toLimitRangeLimits(lr *corev1.LimitRange) []LimitRangeLimit {
	limits := make([]LimitRangeLimit, 0, len(lr.Spec.Limits))
	for _, limit := range lr.Spec.Limits {
		limits = append(limits, LimitRangeLimit{
			Type:                 string(limit.Type),
			Min:                  resourceListToStringMap(limit.Min),
			Max:                  resourceListToStringMap(limit.Max),
			Default:              resourceListToStringMap(limit.Default),
			DefaultRequest:       resourceListToStringMap(limit.DefaultRequest),
			MaxLimitRequestRatio: resourceListToStringMap(limit.MaxLimitRequestRatio),
		})
	}

	return limits
}

// resourceListToStringMap converts a corev1.ResourceList to a map of quantity strings.
func resourceListToStringMap(rl corev1.ResourceList) map[string]string {
	if len(rl) == 0 {
		return nil
	}

	result := make(map[string]string, len(rl))
	for name, qty := range rl {
		result[string(name)] = qty.String()
	}

	return result
}
