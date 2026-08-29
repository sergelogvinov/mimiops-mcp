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
	"strings"

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

// LimitRangesListResult represents the result of listing limit ranges.
type LimitRangesListResult struct {
	LimitRanges []LimitRangeSummary `json:"limitranges" jsonschema:"List of limit ranges"`
}

// RegisterLimitRangesList adds the limitranges_list tool, which lists LimitRanges in a namespace (or all namespaces).
func RegisterLimitRangesList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List LimitRanges"),
		mcp.WithDescription("List LimitRanges in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithOutputSchema[LimitRangesListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("limitranges_list", opts...)
	s.AddTool(tool, handlerLimitRangesList(mc))
}

// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=limitranges/status,verbs=get;list;watch

// handlerLimitRangesList returns a handler function for the limitranges_list tool.
func handlerLimitRangesList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "limitranges_list called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
		)

		// List limit ranges
		ranges, err := client.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Limit Ranges found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list limit ranges: %v", err), nil
		}

		result := LimitRangesListResult{
			LimitRanges: make([]LimitRangeSummary, 0, len(ranges.Items)),
		}

		// Build result
		for _, lr := range ranges.Items {
			result.LimitRanges = append(result.LimitRanges, LimitRangeSummary{
				Name:      lr.Name,
				Namespace: lr.Namespace,
				Types:     deriveLimitRangeTypes(&lr),
				Age:       age.FormatAge(lr.CreationTimestamp),
			})
		}

		// Build fallback text
		fallbackText := "No LimitRanges found"
		if len(result.LimitRanges) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}

// deriveLimitRangeTypes derives the resource types from spec.limits.
func deriveLimitRangeTypes(lr *corev1.LimitRange) string {
	types := make([]string, 0)
	for _, limit := range lr.Spec.Limits {
		if limit.Type != "" {
			types = append(types, string(limit.Type))
		}
	}
	if len(types) == 0 {
		return "none"
	}
	return strings.Join(types, ", ")
}
