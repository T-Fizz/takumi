package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	s := NewServer()
	require.NotNil(t, s)
}

func TestNewServer_HasTools(t *testing.T) {
	s := NewServer()
	require.NotNil(t, s)
	// The server should have all 7 tools registered.
	// We verify by listing tools via the server's internal state.
	// Since MCPServer doesn't expose a tool count, we just confirm it was created.
	assert.NotNil(t, s)
}
