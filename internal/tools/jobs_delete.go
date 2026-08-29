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
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobDeleteResult represents the result of deleting a Job.
type JobDeleteResult struct {
	Name      string `json:"name" jsonschema:"Name of the deleted Job"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the deleted Job"`
	Deleted   bool   `json:"deleted" jsonschema:"Whether the Job was successfully deleted"`
}

// RegisterJobsDelete adds the jobs_delete tool, which deletes a Job.
func RegisterJobsDelete(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Delete Job"),
		mcp.WithDescription("Delete a Job (cascading — also deletes owned pods)"),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("propagation_policy", mcp.Description("propagation policy"), mcp.Enum("Background", "Foreground", "Orphan"), mcp.DefaultString("Background")),
		mcp.WithOutputSchema[JobDeleteResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("jobs_delete", opts...)
	s.AddTool(tool, handlerJobsDelete(mc))
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=delete

// handlerJobsDelete returns a handler function for the jobs_delete tool.
func handlerJobsDelete(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		propagationPolicyStr := req.GetString("propagation_policy", "Background")
		propagationPolicy := metav1.DeletionPropagation(propagationPolicyStr)

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "jobs_delete called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"job", name,
			"propagation_policy", propagationPolicyStr,
		)

		// Delete the Job
		err = client.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to delete Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := JobDeleteResult{
			Name:      name,
			Namespace: namespace,
			Deleted:   true,
		}

		return mcp.NewToolResultStructured(result, formatter.ToText(result)), nil
	}
}
