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

// NamespaceGetResult represents the result of getting a namespace.
type NamespaceGetResult struct {
	NamespaceSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
	Finalizers  []string          `json:"finalizers" jsonschema:"Finalizers"`
	Conditions  []ConditionInfo   `json:"conditions,omitempty" jsonschema:"Conditions"`
}

// RegisterNamespacesGet adds the namespaces_get tool, which gets a single namespace's full spec and status.
func RegisterNamespacesGet(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get namespace"),
		mcp.WithDescription("Get a namespace full spec and status."),
		mcp.WithString("name", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[NamespaceGetResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("namespaces_get", opts...)
	s.AddTool(tool, handlerNamespacesGet(mc))
}

// handlerNamespacesGet returns a handler function for the namespaces_get tool.
func handlerNamespacesGet(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "namespaces_get called",
			"cluster", client.ClusterName,
			"namespace", name,
		)

		// Get the namespace
		ns, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("namespace '%s' not found", name), nil
			}
			return mcp.NewToolResultErrorf("failed to get namespace '%s': %v", name, err), nil
		}

		result := buildNamespaceGetResult(ns)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildNamespaceGetResult builds a NamespaceGetResult from a Namespace.
func buildNamespaceGetResult(ns *corev1.Namespace) *NamespaceGetResult {
	result := &NamespaceGetResult{
		NamespaceSummary: NamespaceSummary{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    formatAge(ns.CreationTimestamp),
		},
		Annotations: extractAnnotations(ns.Annotations),
		Labels:      extractLabels(ns.Labels),
		Finalizers:  ns.Finalizers,
		Conditions:  make([]ConditionInfo, 0, len(ns.Status.Conditions)),
	}

	// Conditions
	for _, cond := range ns.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	return result
}
