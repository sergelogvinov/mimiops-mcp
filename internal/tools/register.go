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
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// RegisterTools wires every tool into the MCP server. Read tools are always
// registered; destructive tools are only registered when allowDestructive is true.
func RegisterTools(srv *server.MCPServer, client *k8s.Client, log *slog.Logger, allowDestructive bool) {
	RegisterClusterName(srv, client, log)
	RegisterPodsList(srv, client, log)
	RegisterPodsGet(srv, client, log)
	RegisterPodsDescribe(srv, client, log)
	RegisterPodsLog(srv, client, log)
	RegisterJobsList(srv, client, log)
	RegisterJobsGet(srv, client, log)
	RegisterJobsDescribe(srv, client, log)
	RegisterJobsLog(srv, client, log)
	RegisterJobsCreate(srv, client, log)
	RegisterCronJobsList(srv, client, log)
	RegisterCronJobsGet(srv, client, log)
	RegisterCronJobsDescribe(srv, client, log)
	RegisterNodesList(srv, client, log)
	RegisterNodesGet(srv, client, log)
	RegisterNamespacesList(srv, client, log)
	RegisterNamespacesGet(srv, client, log)
	RegisterResourceQuotasList(srv, client, log)
	RegisterResourceQuotasGet(srv, client, log)
	RegisterLimitRangesList(srv, client, log)
	RegisterLimitRangesGet(srv, client, log)
	RegisterStorageClassesList(srv, client, log)
	RegisterPriorityClassesList(srv, client, log)
	RegisterEventsGet(srv, client, log)
	RegisterWorkloadsList(srv, client, log)
	RegisterWorkloadsGet(srv, client, log)
	RegisterWorkloadsDescribe(srv, client, log)
	RegisterHelmList(srv, client, log)
	RegisterHelmStatus(srv, client, log)
	if allowDestructive {
		RegisterPodsDelete(srv, client, log)
		RegisterJobsDelete(srv, client, log)
		RegisterCronJobsSuspend(srv, client, log)
		RegisterCronJobsResume(srv, client, log)
		RegisterWorkloadsScale(srv, client, log)
		RegisterHelmRollback(srv, client, log)
	}
}
