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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServicesListResult represents the result of listing services.
type ServicesListResult struct {
	Services []ServiceSummary `json:"services" jsonschema:"List of services"`
}

// RegisterServicesList adds the services_list tool, which lists services in a namespace (or all namespaces).
func RegisterServicesList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Services"),
		mcp.WithDescription("List services in a namespace (or all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithOutputSchema[ServicesListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("services_list", opts...)
	s.AddTool(tool, handlerServicesList(mc))
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// handlerServicesList returns a handler function for the services_list tool.
func handlerServicesList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		// Parse parameters
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "services_list called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		// List services
		services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector, LabelSelector: labelSelector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no services found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list services: %v", err), nil
		}

		result := ServicesListResult{
			Services: make([]ServiceSummary, 0, len(services.Items)),
		}

		// Build result
		for _, svc := range services.Items {
			result.Services = append(result.Services, toServiceSummary(&svc))
		}

		// Build fallback text
		fallbackText := "No services found"
		if len(result.Services) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
