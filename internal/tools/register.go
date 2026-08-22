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
func RegisterTools(srv *server.MCPServer, client *k8s.Client, allowDestructive bool) {
	RegisterClusterName(srv, client)
	RegisterPodsList(srv, client)
	RegisterPodsGet(srv, client)
	RegisterPodsDescribe(srv, client)
	RegisterPodsLog(srv, client)
	RegisterJobsList(srv, client)
	RegisterJobsGet(srv, client)
	RegisterJobsDescribe(srv, client)
	RegisterJobsLog(srv, client)
	RegisterJobsCreate(srv, client)
	RegisterCronJobsList(srv, client)
	RegisterCronJobsGet(srv, client)
	RegisterCronJobsDescribe(srv, client)
	RegisterNodesList(srv, client)
	RegisterNodesGet(srv, client)
	RegisterNamespacesList(srv, client)
	RegisterNamespacesGet(srv, client)
	RegisterResourceQuotasList(srv, client)
	RegisterResourceQuotasGet(srv, client)
	RegisterLimitRangesList(srv, client)
	RegisterLimitRangesGet(srv, client)
	RegisterStorageClassesList(srv, client)
	RegisterPriorityClassesList(srv, client)
	RegisterEventsGet(srv, client)
	RegisterWorkloadsList(srv, client)
	RegisterWorkloadsGet(srv, client)
	RegisterWorkloadsDescribe(srv, client)
	RegisterHelmList(srv, client)
	RegisterHelmStatus(srv, client)
	if allowDestructive {
		RegisterPodsDelete(srv, client)
		RegisterJobsDelete(srv, client)
		RegisterCronJobsSuspend(srv, client)
		RegisterCronJobsResume(srv, client)
		RegisterWorkloadsScale(srv, client)
		RegisterHelmRollback(srv, client)
	}
}
