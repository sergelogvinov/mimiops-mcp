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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LimitRangeGetResult represents the result of getting a LimitRange.
type LimitRangeGetResult struct {
	LimitRangeSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the LimitRange"`
}

// RegisterLimitRangesGet adds the limitranges_get tool, which gets a single LimitRange's full spec.
func RegisterLimitRangesGet(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("limitranges_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get LimitRange"),
		mcp.WithDescription("Get a single LimitRange's full spec and status."),
		mcp.WithString("name", mcp.Description("LimitRange name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[LimitRangeGetResult](),
	)
	s.AddTool(tool, handlerLimitRangesGet(client))
}

// handlerLimitRangesGet returns a handler function for the limitranges_get tool.
func handlerLimitRangesGet(client *k8s.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "limitranges_get called", "namespace", namespace, "name", name)

		// Get the limit range
		lr, err := client.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("limit range '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get limit range '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildLimitRangeGetResult(lr)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildLimitRangeGetResult builds a LimitRangeGetResult from a LimitRange.
func buildLimitRangeGetResult(lr *corev1.LimitRange) *LimitRangeGetResult {
	result := &LimitRangeGetResult{
		LimitRangeSummary: LimitRangeSummary{
			Name:      lr.Name,
			Namespace: lr.Namespace,
			Types:     deriveLimitRangeTypes(lr),
			Age:       formatAge(lr.CreationTimestamp),
		},
		Annotations: extractAnnotations(lr.Annotations),
		Labels:      extractLabels(lr.Labels),
		Spec:        make(map[string]any),
	}

	// Spec (simplified)
	result.Spec["limits"] = lr.Spec.Limits

	return result
}
