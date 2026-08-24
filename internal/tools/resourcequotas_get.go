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

// ResourceQuotaGetResult represents the result of getting a resource quota.
type ResourceQuotaGetResult struct {
	ResourceQuotaSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the resource quota"`
}

// RegisterResourceQuotasGet adds the resourcequotas_get tool, which gets a single ResourceQuota's full spec and status.
func RegisterResourceQuotasGet(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get ResourceQuota"),
		mcp.WithDescription("Get a single ResourceQuota's full spec and status."),
		mcp.WithString("name", mcp.Description("resource quota name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[ResourceQuotaGetResult](),
	}, clusterOptions(mc)...)

	tool := mcp.NewTool("resourcequotas_get", opts...)
	s.AddTool(tool, handlerResourceQuotasGet(mc))
}

// handlerResourceQuotasGet returns a handler function for the resourcequotas_get tool.
func handlerResourceQuotasGet(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := resolveCluster(mc, req)
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
		log.DebugContext(ctx, "resourcequotas_get called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
		)

		// Get the resource quota
		quota, err := client.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("resource quota '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get resource quota '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildResourceQuotaGetResult(quota)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildResourceQuotaGetResult builds a ResourceQuotaGetResult from a ResourceQuota.
func buildResourceQuotaGetResult(quota *corev1.ResourceQuota) *ResourceQuotaGetResult {
	used := quota.Status.Hard
	usedStatus := quota.Status.Used

	result := ResourceQuotaGetResult{
		ResourceQuotaSummary: ResourceQuotaSummary{
			Name:           quota.Name,
			Namespace:      quota.Namespace,
			Age:            formatAge(quota.CreationTimestamp),
			RequestsCPU:    getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU),
			RequestsMemory: getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory),
			LimitsCPU:      getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU),
			LimitsMemory:   getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory),
		},
		Annotations: extractAnnotations(quota.Annotations),
		Labels:      extractLabels(quota.Labels),
		Spec:        make(map[string]any),
	}

	// Spec (simplified)
	result.Spec["hard"] = quota.Spec.Hard

	return &result
}
