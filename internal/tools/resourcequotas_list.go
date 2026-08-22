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

// ResourceQuotasListResult represents the result of listing resource quotas.
type ResourceQuotasListResult struct {
	ResourceQuotas []ResourceQuotaSummary `json:"resourcequotas" jsonschema:"List of resource quotas"`
}

// RegisterResourceQuotasList adds the resourcequotas_list tool, which lists ResourceQuotas in a namespace (or all namespaces).
func RegisterResourceQuotasList(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("resourcequotas_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List ResourceQuotas"),
		mcp.WithDescription("List ResourceQuotas in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithOutputSchema[ResourceQuotasListResult](),
	)
	s.AddTool(tool, handlerResourceQuotasList(client))
}

// handlerResourceQuotasList returns a handler function for the resourcequotas_list tool.
func handlerResourceQuotasList(client *k8s.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "resourcequotas_list called", "namespace", namespace)

		// List resource quotas
		quotas, err := client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Resource Quotas found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list resource quotas: %v", err), nil
		}

		result := ResourceQuotasListResult{
			ResourceQuotas: make([]ResourceQuotaSummary, 0, len(quotas.Items)),
		}

		// Build result
		for _, quota := range quotas.Items {
			summary := ResourceQuotaSummary{
				Name:      quota.Name,
				Namespace: quota.Namespace,
				Age:       formatAge(quota.CreationTimestamp),
			}

			used := quota.Status.Hard
			usedStatus := quota.Status.Used

			summary.RequestsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceCPU)
			summary.RequestsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceMemory)
			summary.LimitsCPU = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsCPU)
			summary.LimitsMemory = getQuotaValueDisplay(usedStatus, used, corev1.ResourceLimitsMemory)

			result.ResourceQuotas = append(result.ResourceQuotas, summary)
		}

		// Build fallback text
		fallbackText := "No resource quotas found"
		if len(result.ResourceQuotas) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}

// getQuotaValueDisplay returns the used/hard display for a resource.
func getQuotaValueDisplay(usedStatus, used corev1.ResourceList, resource corev1.ResourceName) string {
	hard, hasHard := used[resource]
	if !hasHard {
		return "-"
	}

	usedVal, hasUsed := usedStatus[resource]
	if !hasUsed {
		return "-" + "/" + hard.String()
	}

	return usedVal.String() + "/" + hard.String()
}
