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

// ReconcileResult is the result of flux_reconcile.
type ReconcileResult struct {
	Kind             string `json:"kind" jsonschema:"Kind of the reconciled resource"`
	Namespace        string `json:"namespace" jsonschema:"Namespace"`
	Name             string `json:"name" jsonschema:"Name"`
	RequestedAt      string `json:"requestedAt" jsonschema:"Timestamp written to reconcile.fluxcd.io/requestedAt"`
	Ready            bool   `json:"ready" jsonschema:"Whether the resource is currently ready"`
	Message          string `json:"message,omitempty" jsonschema:"Current ready condition message"`
	SourceReconciled bool   `json:"sourceReconciled,omitempty" jsonschema:"Whether the source was also reconciled (--with-source)"`
}

// RegisterFluxReconcile adds the flux_reconcile tool, which triggers an
// immediate Flux reconciliation of a GitRepository, HelmRelease, or
// Kustomization by writing the reconcile.fluxcd.io/requestedAt annotation.
func RegisterFluxReconcile(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Reconcile Flux Resource"),
		mcp.WithDescription("Trigger an immediate Flux reconciliation of a GitRepository, HelmRelease, or Kustomization (fire-and-forget; poll with the describe tool)"),
		mcp.WithString("kind", mcp.Description("resource kind to reconcile"), mcp.Required(), mcp.Enum("gitrepository", "helmrelease", "kustomization")),
		mcp.WithString("name", mcp.Description("resource name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithBoolean("with_source", mcp.Description("also reconcile the referenced source (helmrelease/kustomization only)"), mcp.DefaultBool(false)),
		mcp.WithOutputSchema[ReconcileResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("flux_reconcile", opts...)
	s.AddTool(tool, handlerFluxReconcile(mc))
}

// handlerFluxReconcile returns a handler function for the flux_reconcile tool.
func handlerFluxReconcile(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		kind := req.GetString("kind", "")
		if kind == "" {
			return mcp.NewToolResultError("missing required parameter 'kind'"), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		withSource := req.GetBool("with_source", false)

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "flux_reconcile called",
			"cluster", client.ClusterName,
			"kind", kind,
			"namespace", namespace,
			"name", name,
			"with_source", withSource,
		)

		fluxClient, err := NewFluxClient(client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Flux client: %v", err), nil
		}

		result, err := fluxClient.Reconcile(ctx, kind, name, namespace, withSource)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
