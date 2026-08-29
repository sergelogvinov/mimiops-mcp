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
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
)

// OCIRepositoryDescribeResult is the result of flux_ocirepositories_describe.
type OCIRepositoryDescribeResult struct {
	SourceSummary

	Labels      map[string]string `json:"labels,omitempty" jsonschema:"Labels"`
	Annotations map[string]string `json:"annotations,omitempty" jsonschema:"Annotations"`
	Conditions  []ConditionInfo   `json:"conditions,omitempty" jsonschema:"Conditions"`

	Interval               string `json:"interval,omitempty" jsonschema:"Reconciliation interval"`
	Timeout                string `json:"timeout,omitempty" jsonschema:"Fetch timeout"`
	Provider               string `json:"provider,omitempty" jsonschema:"Authentication provider"`
	LayerSelector          string `json:"layerSelector,omitempty" jsonschema:"OCI layer selector (mediaType/operation)"`
	Digest                 string `json:"digest,omitempty" jsonschema:"Monitored reference digest"`
	Artifact               string `json:"artifact,omitempty" jsonschema:"Last artifact path"`
	LastHandledReconcileAt string `json:"lastHandledReconcileAt,omitempty" jsonschema:"Last handled reconcile request"`
}

// RegisterOCIRepositoriesDescribe adds the flux_ocirepositories_describe tool,
// which provides a rich structured summary of a Flux OCIRepository source.
func RegisterOCIRepositoriesDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Flux OCI Repository"),
		mcp.WithDescription("Flux OCIRepository summary (URL, ref, provider, layer selector, artifact, conditions)."),
		mcp.WithString("name", mcp.Description("OCIRepository name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[OCIRepositoryDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("flux_ocirepositories_describe", opts...)
	s.AddTool(tool, handlerOCIRepositoriesDescribe(mc))
}

// handlerOCIRepositoriesDescribe returns a handler function for the flux_ocirepositories_describe tool.
func handlerOCIRepositoriesDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "flux_ocirepositories_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"name", name,
		)

		fluxClient, err := NewFluxClient(client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Flux client: %v", err), nil
		}

		result, err := fluxClient.DescribeOCIRepository(ctx, name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToText(result)), nil
	}
}
