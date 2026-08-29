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

package toolsfluxcd

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
)

// ReconciliationResult is the result of flux_reconciliation.
type ReconciliationResult struct {
	Kind      string `json:"kind" jsonschema:"Kind of the resource"`
	Namespace string `json:"namespace" jsonschema:"Namespace"`
	Name      string `json:"name" jsonschema:"Name"`
	Action    string `json:"action" jsonschema:"Action applied: suspend or resume"`
	Suspended bool   `json:"suspended" jsonschema:"Whether the resource is now suspended"`
}

// RegisterFluxReconciliation adds the flux_reconciliation tool, which suspends
// or resumes a Flux HelmRelease or Kustomization by toggling spec.suspend.
func RegisterFluxReconciliation(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Suspend/Resume Flux Resource"),
		mcp.WithDescription("Suspend or resume a Flux HelmRelease or Kustomization (toggles spec.suspend)"),
		mcp.WithString("kind", mcp.Description("resource kind"), mcp.Required(), mcp.Enum("helmrelease", "kustomization")),
		mcp.WithString("action", mcp.Description("action to apply"), mcp.Required(), mcp.Enum("suspend", "resume")),
		mcp.WithString("name", mcp.Description("resource name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[ReconciliationResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("flux_reconciliation", opts...)
	s.AddTool(tool, handlerFluxReconciliation(mc))
}

// handlerFluxReconciliation returns a handler function for the flux_reconciliation tool.
func handlerFluxReconciliation(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		kind := req.GetString("kind", "")
		if kind == "" {
			return mcp.NewToolResultError("missing required parameter 'kind'"), nil
		}

		action := req.GetString("action", "")
		if action == "" {
			return mcp.NewToolResultError("missing required parameter 'action'"), nil
		}
		if action != "suspend" && action != "resume" {
			return mcp.NewToolResultError("invalid parameter 'action': must be one of suspend, resume"), nil
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
		log.DebugContext(ctx, "flux_reconciliation called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"kind", kind,
			"action", action,
			"namespace", namespace,
			"name", name,
		)

		fluxClient, err := NewFluxClient(client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Flux client: %v", err), nil
		}

		var result *ReconciliationResult
		if action == "suspend" {
			result, err = fluxClient.Suspend(ctx, kind, name, namespace)
		} else {
			result, err = fluxClient.Resume(ctx, kind, name, namespace)
		}
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
