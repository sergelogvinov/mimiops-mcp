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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespacesListResult represents the result of listing namespaces.
type NamespacesListResult struct {
	Namespaces []NamespaceSummary `json:"namespaces" jsonschema:"List of namespaces"`
}

// RegisterNamespacesList adds the namespaces_list tool, which lists all namespaces.
func RegisterNamespacesList(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("namespaces_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Namespaces"),
		mcp.WithDescription("List all namespaces in the cluster"),
		mcp.WithOutputSchema[NamespacesListResult](),
	)
	s.AddTool(tool, handlerNamespacesList(client))
}

// handlerNamespacesList returns a handler function for the namespaces_list tool.
func handlerNamespacesList(client *k8s.Client) func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "namespaces_list called")

		// List namespaces
		namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Namespaces found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list namespaces: %v", err), nil
		}

		result := NamespacesListResult{
			Namespaces: make([]NamespaceSummary, 0, len(namespaces.Items)),
		}

		// Build result
		for _, ns := range namespaces.Items {
			result.Namespaces = append(result.Namespaces, NamespaceSummary{
				Name:   ns.Name,
				Status: string(ns.Status.Phase),
				Age:    formatAge(ns.CreationTimestamp),
			})
		}

		// Build fallback text
		fallbackText := "No Namespaces found"
		if len(result.Namespaces) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
