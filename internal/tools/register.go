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
	if allowDestructive {
		RegisterPodsDelete(srv, client, log)
		RegisterCronJobsSuspend(srv, client, log)
		RegisterCronJobsResume(srv, client, log)
	}
}
