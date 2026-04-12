// Package mcp provides a Model Context Protocol server that exposes
// takumi workspace operations as tools for AI agents.
package mcp

import "github.com/mark3labs/mcp-go/server"

// NewServer creates a configured MCP server with all takumi tools registered.
func NewServer() *server.MCPServer {
	s := server.NewMCPServer("takumi", "0.1.0")
	registerTools(s)
	return s
}
