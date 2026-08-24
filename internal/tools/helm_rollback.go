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
	"github.com/sergelogvinov/mimiops-mcp/internal/helm"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
)

// HelmRollbackResult represents the result of rolling back a Helm release.
type HelmRollbackResult struct {
	Rollback helm.RollbackResult `json:"rollback" jsonschema:"Helm rollback result"`
}

// RegisterHelmRollback adds the helm_rollback tool, which rolls back a Helm release to the previous revision.
func RegisterHelmRollback(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Rollback Helm Release"),
		mcp.WithDescription("Roll back a Helm release to the previous revision (one back)"),
		mcp.WithString("name", mcp.Description("release name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[HelmRollbackResult](),
	}, clusterOptions(mc)...)

	tool := mcp.NewTool("helm_rollback", opts...)
	s.AddTool(tool, handlerHelmRollback(mc))
}

// handlerHelmRollback returns a handler function for the helm_rollback tool.
func handlerHelmRollback(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "helm_rollback called",
			"cluster", client.ClusterName,
			"name", name,
			"namespace", namespace,
		)

		// Create Helm client
		helmClient, err := helm.NewHelmClient(client, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Helm client: %v", err), nil
		}

		// First, get the current revision to calculate the previous revision
		currentRelease, err := helmClient.GetRelease(name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get current status for release: %v", err), nil
		}

		// Calculate previous revision (current - 1)
		previousRevision := currentRelease.Revision
		newRevision := previousRevision - 1

		if newRevision < 1 {
			return mcp.NewToolResultErrorf("release '%s' in namespace '%s' is at revision %d — nothing to roll back to", name, namespace, previousRevision), nil
		}

		// Perform the rollback to the previous revision
		err = helmClient.Rollback(name, newRevision)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to rollback release to revision %d: %v", newRevision, err), nil
		}

		// Get the status after rollback
		afterRollback, err := helmClient.GetRelease(name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get status after rollback for release: %v", err), nil
		}

		result := HelmRollbackResult{
			Rollback: helm.RollbackResult{
				Name:             name,
				Namespace:        namespace,
				PreviousRevision: previousRevision,
				NewRevision:      afterRollback.Revision,
				Status:           afterRollback.Status,
			},
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
