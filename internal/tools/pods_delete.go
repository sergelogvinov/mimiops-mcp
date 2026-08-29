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
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodDeleteResult represents the result of deleting a pod.
type PodDeleteResult struct {
	Name      string `json:"name" jsonschema:"Name of the deleted pod"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the deleted pod"`
	Deleted   bool   `json:"deleted" jsonschema:"Whether the pod was successfully deleted"`
}

// RegisterPodsDelete adds the pods_delete tool, which deletes a pod.
func RegisterPodsDelete(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Delete Pod"),
		mcp.WithDescription("Delete a pod"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithInteger("grace_period_seconds", mcp.Description("grace period in seconds"), mcp.DefaultNumber(30)),
		mcp.WithOutputSchema[PodDeleteResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("pods_delete", opts...)
	s.AddTool(tool, handlerPodsDelete(mc))
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=delete

// handlerPodsDelete returns a handler function for the pods_delete tool.
func handlerPodsDelete(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
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

		gracePeriodSeconds := req.GetInt("grace_period_seconds", 30)

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "pods_delete called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"pod", name,
			"grace_period_seconds", gracePeriodSeconds,
		)

		// Delete the pod - convert int to int64
		gracePeriodSecondsInt64 := int64(gracePeriodSeconds)
		err = client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSecondsInt64,
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("pod '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to delete pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Pod '%s' in namespace '%s' deleted successfully.", name, namespace)), nil
	}
}
