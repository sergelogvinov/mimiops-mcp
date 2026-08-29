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
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceResourceQuota is a ResourceQuota in a namespace with its resources.
type NamespaceResourceQuota struct {
	Name           string          `json:"name" jsonschema:"Name of the resource quota"`
	ResourceQuotas []ResourceQuota `json:"resourceQuotas" jsonschema:"ResourceQuotas (resource, used, hard)"`
}

// NamespaceLimitRange is a LimitRange in a namespace with its typed limits.
type NamespaceLimitRange struct {
	Name   string            `json:"name" jsonschema:"Name of the LimitRange"`
	Types  string            `json:"types" jsonschema:"Resource types in the LimitRange"`
	Limits []LimitRangeLimit `json:"limits" jsonschema:"Limits of the LimitRange"`
}

// NamespaceDescribeResult represents the result of describing a namespace.
type NamespaceDescribeResult struct {
	NamespaceSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Finalizers     []string          `json:"finalizers,omitempty" jsonschema:"Finalizers"`
	Conditions     []ConditionInfo   `json:"conditions,omitempty" jsonschema:"Conditions"`
	ResourceQuotas []ResourceQuota   `json:"resourcequotas,omitempty" jsonschema:"Resource quota in the namespace"`
	LimitRange     []LimitRangeLimit `json:"limitranges,omitempty" jsonschema:"Limit ranges in the namespace"`
}

// RegisterNamespacesDescribe adds the namespaces_describe tool, which describes a namespace
// including its ResourceQuotas and LimitRanges.
func RegisterNamespacesDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Namespace"),
		mcp.WithDescription("Describe a namespace including its ResourceQuotas and LimitRanges."),
		mcp.WithString("name", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[NamespaceDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("namespaces_describe", opts...)
	s.AddTool(tool, handlerNamespacesDescribe(mc))
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=limitranges/status,verbs=get;list;watch

// handlerNamespacesDescribe returns a handler function for the namespaces_describe tool.
func handlerNamespacesDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "namespaces_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
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

		// Get resource quotas in the namespace
		quota, err := client.CoreV1().ResourceQuotas(name).Get(ctx, "default", metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			log.WarnContext(ctx, "failed to get resource quota for namespace", "namespace", name, "err", err)
		}

		// Get limit ranges in the namespace
		limitRange, err := client.CoreV1().LimitRanges(name).Get(ctx, "default", metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			log.WarnContext(ctx, "failed to get limit range for namespace", "namespace", name, "err", err)
		}

		result := buildNamespaceDescribeResult(ns, quota, limitRange)
		return mcp.NewToolResultStructured(result, formatter.ToText(result)), nil
	}
}

// buildNamespaceDescribeResult builds a NamespaceDescribeResult from a Namespace and its
// ResourceQuotas and LimitRanges.
func buildNamespaceDescribeResult(ns *corev1.Namespace, quota *corev1.ResourceQuota, limitRange *corev1.LimitRange) *NamespaceDescribeResult {
	result := &NamespaceDescribeResult{
		NamespaceSummary: NamespaceSummary{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    age.FormatAge(ns.CreationTimestamp),
		},
		Annotations: extractAnnotations(ns.Annotations),
		Labels:      extractLabels(ns.Labels),
		Finalizers:  ns.Finalizers,
	}

	for _, cond := range ns.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	if quota != nil {
		result.ResourceQuotas = extractResourceQuotas(quota)
	}

	if limitRange != nil {
		result.LimitRange = toLimitRangeLimits(limitRange)
	}

	return result
}
