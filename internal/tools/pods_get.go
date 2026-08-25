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

// PodGetResult represents the result of getting a pod.
type PodGetResult struct {
	PodSummary
	PodSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"Conditions"`
}

// RegisterPodsGet adds the pods_get tool, which gets a pod's full spec and status.
func RegisterPodsGet(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Pod"),
		mcp.WithDescription("Get a pod's full spec and status"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[PodGetResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("pods_get", opts...)
	s.AddTool(tool, handlerPodsGet(mc))
}

// handlerPodsGet returns a handler function for the pods_get tool.
func handlerPodsGet(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "pods_get called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"pod", name,
		)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("pod '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildPodGetResult(ctx, client, pod)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildPodGetResult builds a PodGetResult from a Pod.
func buildPodGetResult(ctx context.Context, client *k8s.Client, pod *corev1.Pod) *PodGetResult {
	result := &PodGetResult{
		PodSummary:  toPodSummary(pod),
		PodSpec:     toPodSpec(pod),
		Annotations: extractAnnotations(pod.Annotations),
		Labels:      extractLabels(pod.Labels),
		Conditions:  toPodConditionInfo(pod),
	}

	result.PodSummary.OwnerReferences, _ = ownerReferences(ctx, client, pod) //nolint:errcheck

	return result
}
