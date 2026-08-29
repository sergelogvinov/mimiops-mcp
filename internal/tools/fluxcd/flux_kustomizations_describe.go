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

// KustomizationDescribeResult is the result of flux_kustomizations_describe.
type KustomizationDescribeResult struct {
	KustomizationSummary

	Labels      map[string]string `json:"labels,omitempty" jsonschema:"Labels"`
	Annotations map[string]string `json:"annotations,omitempty" jsonschema:"Annotations"`
	Conditions  []ConditionInfo   `json:"conditions,omitempty" jsonschema:"Conditions"`

	SourceRef             string `json:"sourceRef,omitempty" jsonschema:"Source reference (kind/name)"`
	Path                  string `json:"path,omitempty" jsonschema:"Path within the source"`
	Prune                 bool   `json:"prune" jsonschema:"Whether resources are pruned"`
	LastAppliedRevision   string `json:"lastAppliedRevision,omitempty" jsonschema:"Last applied revision"`
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty" jsonschema:"Last attempted revision"`
}

// RegisterKustomizationsDescribe adds the flux_kustomizations_describe tool,
// which provides a rich structured summary of a Flux Kustomization.
func RegisterKustomizationsDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Flux Kustomization"),
		mcp.WithDescription("Flux Kustomization summary (source ref, path, prune, revisions, conditions)."),
		mcp.WithString("name", mcp.Description("Kustomization name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[KustomizationDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("flux_kustomizations_describe", opts...)
	s.AddTool(tool, handlerKustomizationsDescribe(mc))
}

// handlerKustomizationsDescribe returns a handler function for the flux_kustomizations_describe tool.
func handlerKustomizationsDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "flux_kustomizations_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"name", name,
		)

		fluxClient, err := NewFluxClient(client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Flux client: %v", err), nil
		}

		result, err := fluxClient.DescribeKustomization(ctx, name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
