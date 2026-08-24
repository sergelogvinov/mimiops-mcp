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
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
)

// ClustersDescribeResult represents the result of describing a cluster.
type ClustersDescribeResult struct {
	Name        string   `json:"name" jsonschema:"Name of the cluster"`
	Namespace   string   `json:"namespace" jsonschema:"Namespace of the cluster"`
	ContextName string   `json:"context_name" jsonschema:"Name of the kubeconfig context"`
	APIVersions []string `json:"api_versions" jsonschema:"API versions served by the cluster (group/version or v1)"`
}

// RegisterClustersDescribe adds the clusters_describe tool, which returns the
// API versions served by a cluster. In in-cluster mode the cluster parameter
// is hidden and the tool always describes the current cluster.
func RegisterClustersDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Cluster"),
		mcp.WithDescription("Return the API versions served by a cluster"),
		mcp.WithOutputSchema[ClustersDescribeResult](),
	}, clusterOptions(mc)...)

	tool := mcp.NewTool("clusters_describe", opts...)
	s.AddTool(tool, handlerClustersDescribe(mc))
}

// handlerClustersDescribe returns a handler function for the clusters_describe tool.
func handlerClustersDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log := logger.FromContext(ctx)

		client, err := resolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		log.DebugContext(ctx, "clusters_describe called", "cluster", client.ClusterName)

		apiVersions, err := apiVersionsForCluster(ctx, client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to fetch API versions for cluster %q: %v", client.ClusterName, err), nil
		}

		result := ClustersDescribeResult{
			Name:        client.ClusterName,
			Namespace:   client.Namespace,
			ContextName: client.ContextName,
			APIVersions: apiVersions,
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// apiVersionsForCluster fetches the aggregated list of API versions
// (group/version, plus the legacy core "v1") served by a cluster.
func apiVersionsForCluster(_ context.Context, client *k8s.Client) ([]string, error) {
	groupList, err := client.Discovery().ServerGroups()
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	versions := make([]string, 0, len(groupList.Groups)+1)
	for _, group := range groupList.Groups {
		for _, version := range group.Versions {
			switch {
			case version.GroupVersion == "":
				// Skip empty group/version entries.
				continue
			case strings.HasPrefix(version.GroupVersion, "v1"):
				// Skip the legacy core group, which is reported as "v1" without a group prefix.
				continue
			case strings.HasSuffix(version.GroupVersion, "k8s.io/v1"):
				// Skip the internal k8s.io group, which is not relevant to users.
				continue
			case strings.HasPrefix(version.GroupVersion, "apps/"), strings.HasPrefix(version.GroupVersion, "autoscaling/"):
				// Skip the apps group, which is handled separately.
				continue
			}

			versions = append(versions, version.GroupVersion)
		}
	}

	sort.Strings(versions)

	return versions, nil
}
