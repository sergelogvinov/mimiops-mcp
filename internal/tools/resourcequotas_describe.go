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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceQuotaDescribeResult represents the result of describing a resource quota.
type ResourceQuotaDescribeResult struct {
	ResourceQuotaSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	ResourceQuotas []ResourceQuota `json:"resourceQuotas" jsonschema:"ResourceQuotas (resource, used, hard)"`
}

// RegisterResourceQuotasDescribe adds the resourcequotas_describe tool
func RegisterResourceQuotasDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe ResourceQuota"),
		mcp.WithDescription("Describe ResourceQuota used and hard limits"),
		mcp.WithString("name", mcp.Description("resource quota name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[ResourceQuotaDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("resourcequotas_describe", opts...)
	s.AddTool(tool, handlerResourceQuotasDescribe(mc))
}

// handlerResourceQuotasDescribe returns a handler function for the resourcequotas_describe tool.
func handlerResourceQuotasDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "resourcequotas_describe called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
		)

		quota, err := client.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("resource quota '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get resource quota '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildResourceQuotaDescribeResult(quota)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildResourceQuotaDescribeResult builds a ResourceQuotaDescribeResult from a ResourceQuota.
func buildResourceQuotaDescribeResult(quota *corev1.ResourceQuota) *ResourceQuotaDescribeResult {
	result := ResourceQuotaDescribeResult{
		Annotations: extractAnnotations(quota.Annotations),
		Labels:      extractLabels(quota.Labels),
		ResourceQuotaSummary: ResourceQuotaSummary{
			Name:      quota.Name,
			Namespace: quota.Namespace,
			Age:       formatAge(quota.CreationTimestamp),
		},
		ResourceQuotas: extractResourceQuotas(quota),
	}

	return &result
}

func extractResourceQuotas(quota *corev1.ResourceQuota) []ResourceQuota {
	resourceQuotas := make([]ResourceQuota, 0, len(quota.Status.Hard))

	usedStatus := quota.Status.Used

	for resourceName, hardLimit := range quota.Status.Hard {
		usedValue := usedStatus[resourceName]
		resourceQuotas = append(resourceQuotas, ResourceQuota{
			Resource: string(resourceName),
			Used:     usedValue.String(),
			Hard:     hardLimit.String(),
		})
	}

	return resourceQuotas
}
