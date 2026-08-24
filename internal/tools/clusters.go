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
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// clusterOptions returns tool options for the `cluster` parameter, filtered of
// nil entries for modes where the parameter is hidden.
func clusterOptions(mc *k8s.MultiClusterClient) []mcp.ToolOption {
	if mc == nil || !mc.IsMultiCluster() {
		return nil
	}

	opt := []mcp.ToolOption{
		mcp.WithString("cluster", mcp.Description("target cluster name from the kubeconfig (see clusters_list); leave empty for the active cluster"), mcp.Required()),
	}

	return opt
}

// resolveCluster resolves the target client for a tool call. An empty cluster
// name selects the active cluster. In in-cluster mode the cluster parameter is
// not exposed and the active client is always returned.
func resolveCluster(mc *k8s.MultiClusterClient, req mcp.CallToolRequest) (*k8s.Client, error) {
	cluster := ""
	if mc.IsMultiCluster() {
		cluster = req.GetString("cluster", "")
	}

	return mc.GetCluster(cluster)
}
