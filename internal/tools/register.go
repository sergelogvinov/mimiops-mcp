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
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// RegisterTools wires every tool into the MCP server. Read tools are always
// registered; destructive tools are only registered when allowDestructive is true.
// clusters_list is only registered in kubeconfig (multi-cluster) mode.
func RegisterTools(srv *server.MCPServer, mc *k8s.MultiClusterClient, allowDestructive bool) {
	if mc.IsMultiCluster() {
		RegisterClustersList(srv, mc)
	}
	RegisterClustersDescribe(srv, mc)
	RegisterPodsList(srv, mc)
	RegisterPodsGet(srv, mc)
	RegisterPodsDescribe(srv, mc)
	RegisterPodsLog(srv, mc)
	RegisterJobsList(srv, mc)
	RegisterJobsGet(srv, mc)
	RegisterJobsDescribe(srv, mc)
	RegisterJobsLog(srv, mc)
	RegisterJobsCreate(srv, mc)
	RegisterCronJobsList(srv, mc)
	RegisterCronJobsGet(srv, mc)
	RegisterCronJobsDescribe(srv, mc)
	RegisterNodesList(srv, mc)
	RegisterNodesGet(srv, mc)
	RegisterNamespacesList(srv, mc)
	RegisterNamespacesGet(srv, mc)
	RegisterResourceQuotasList(srv, mc)
	RegisterResourceQuotasGet(srv, mc)
	RegisterLimitRangesList(srv, mc)
	RegisterLimitRangesGet(srv, mc)
	RegisterStorageClassesList(srv, mc)
	RegisterPriorityClassesList(srv, mc)
	RegisterEventsGet(srv, mc)
	RegisterWorkloadsList(srv, mc)
	RegisterWorkloadsGet(srv, mc)
	RegisterWorkloadsDescribe(srv, mc)
	RegisterHelmList(srv, mc)
	RegisterHelmStatus(srv, mc)
	if allowDestructive {
		RegisterPodsDelete(srv, mc)
		RegisterJobsDelete(srv, mc)
		RegisterCronJobsSuspend(srv, mc)
		RegisterCronJobsResume(srv, mc)
		RegisterWorkloadsScale(srv, mc)
		RegisterHelmRollback(srv, mc)
	}
}
