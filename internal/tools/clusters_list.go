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
)

// ClusterListEntry is one cluster entry in clusters_list output.
type ClusterListEntry struct {
	Name   string `json:"name" jsonschema:"Name of the cluster"`
	Server string `json:"server" jsonschema:"API server endpoint"`
}

// ClustersListResult represents the result of listing clusters from kubeconfig.
type ClustersListResult struct {
	Clusters []ClusterListEntry `json:"clusters" jsonschema:"List of clusters"`
}

// RegisterClustersList adds the clusters_list tool, which lists clusters known
// to the kubeconfig. It is only registered in kubeconfig (multi-cluster) mode.
func RegisterClustersList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	tool := mcp.NewTool("clusters_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Clusters"),
		mcp.WithDescription("List clusters from kubeconfig and contexts that reference them"),
		mcp.WithOutputSchema[ClustersListResult](),
	)
	s.AddTool(tool, handlerClustersList(mc))
}

// handlerClustersList returns a handler function for the clusters_list tool.
func handlerClustersList(mc *k8s.MultiClusterClient) func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "clusters_list called")

		entries := mc.ListClusters()

		result := ClustersListResult{Clusters: make([]ClusterListEntry, 0, len(entries))}
		for _, e := range entries {
			c := ClusterListEntry{
				Name:   e.Name,
				Server: e.Server,
			}

			result.Clusters = append(result.Clusters, c)
		}

		fallbackText := "No clusters found"
		if len(result.Clusters) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
