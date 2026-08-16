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
	if allowDestructive {
		RegisterPodsDelete(srv, client, log)
	}
}
