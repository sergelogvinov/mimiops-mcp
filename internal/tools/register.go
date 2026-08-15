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
	if allowDestructive {
		RegisterPodsDelete(srv, client)
	}
}
