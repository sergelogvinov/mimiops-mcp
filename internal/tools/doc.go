// Package tools implements the MCP tool catalog for mimiops-mcp.
//
// Every tool lives in its own file (cluster.go, pods.go, pods_log.go,
// pods_delete.go) and exposes a Register* function that defines its name,
// description, input schema, and handler together. This register.go is
// the central place that lists every tool and wires them into the MCP server.
package tools
