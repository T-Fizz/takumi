package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
)

func TestAgentByName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"claude", true},
		{"cursor", true},
		{"copilot", true},
		{"windsurf", true},
		{"cline", true},
		{"none", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := AgentByName(tt.name)
			if tt.want {
				require.NotNil(t, agent)
				assert.Equal(t, tt.name, agent.Name)
			} else {
				assert.Nil(t, agent)
			}
		})
	}
}

func TestAgentNames(t *testing.T) {
	names := agentNames()
	for _, a := range SupportedAgents {
		assert.Contains(t, names, a.Name)
	}
}

func TestPromptAgentSelection_MockedForTests(t *testing.T) {
	// The real promptAgentSelection uses huh (interactive TUI).
	// We verify the mock mechanism works — same pattern used by all init tests.
	original := promptAgentSelection
	promptAgentSelection = func() (*AgentType, error) {
		return AgentByName("cursor"), nil
	}
	t.Cleanup(func() { promptAgentSelection = original })

	agent, err := promptAgentSelection()
	require.NoError(t, err)
	assert.Equal(t, "cursor", agent.Name)
	assert.Equal(t, "Cursor", agent.Label)
}

func TestSetupAgentConfig_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), includeLine)
}

func TestSetupAgentConfig_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("cursor")

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".cursor", "rules"))
	require.NoError(t, err)
	assert.Contains(t, string(data), includeLine)
}

func TestSetupAgentConfig_AppendsIfFileExists(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")

	existing := "# My existing Claude config\n\nSome rules here.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(existing), 0644))

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "My existing Claude config")
	assert.Contains(t, content, includeLine)
}

func TestSetupAgentConfig_SkipsIfAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")

	existing := "# Config\n" + includeLine + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(existing), 0644))

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(data), "file should not be modified")
}

func TestSetupAgentConfig_NoneAgent(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("none")

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	// No files should be created
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSetupAgentConfig_CopilotNestedPath(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("copilot")

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".github", "copilot-instructions.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), includeLine)
}

func TestWriteTakumiMD(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))

	captureStdout(t, func() {
		err := writeTakumiMD(dir, "test-project")
		require.NoError(t, err)
	})

	data, err := os.ReadFile(filepath.Join(dir, ".takumi", "TAKUMI.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "test-project")
	assert.Contains(t, content, "takumi status")
	assert.Contains(t, content, "takumi build")
	assert.Contains(t, content, "Workflow")
}

func TestTakumiMDContent(t *testing.T) {
	content := takumiMDContent("my-workspace")
	assert.Contains(t, content, "my-workspace")
	assert.Contains(t, content, "Commands")
	assert.Contains(t, content, "Workflow")
	assert.Contains(t, content, "Config locations")
	assert.Contains(t, content, "Rules")
}

func TestInitWorkspace_WithAgent(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")

	out := captureStdout(t, func() {
		err := initWorkspace(dir, "test-ws", agent)
		require.NoError(t, err)
	})

	// TAKUMI.md should exist
	assert.FileExists(t, filepath.Join(dir, ".takumi", "TAKUMI.md"))

	// Agent config file should exist
	assert.FileExists(t, filepath.Join(dir, "CLAUDE.md"))

	// Agent stored in workspace config
	cfg, err := config.LoadWorkspaceConfig(filepath.Join(dir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "claude", cfg.Workspace.AI.Agent)

	assert.Contains(t, out, "TAKUMI.md")
	assert.Contains(t, out, "CLAUDE.md")
}

func TestInitWorkspace_WithNoneAgent(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("none")

	captureStdout(t, func() {
		err := initWorkspace(dir, "test-ws", agent)
		require.NoError(t, err)
	})

	// TAKUMI.md should still exist
	assert.FileExists(t, filepath.Join(dir, ".takumi", "TAKUMI.md"))

	// No agent config files
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))

	// Agent should NOT be in config
	cfg, err := config.LoadWorkspaceConfig(filepath.Join(dir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Workspace.AI.Agent)
}

func TestRunInit_WithAgentFlag(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)

	initCmd.Flags().Set("agent", "cursor")
	t.Cleanup(func() { initCmd.Flags().Set("agent", "") })

	captureStdout(t, func() {
		err := runInit(initCmd, nil)
		require.NoError(t, err)
	})

	// Cursor config created
	assert.FileExists(t, filepath.Join(dir, ".cursor", "rules"))

	// TAKUMI.md created
	assert.FileExists(t, filepath.Join(dir, ".takumi", "TAKUMI.md"))

	// Agent stored in config
	cfg, err := config.LoadWorkspaceConfig(filepath.Join(dir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "cursor", cfg.Workspace.AI.Agent)
}

func TestRunInit_WithInvalidAgentFlag(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)

	initCmd.Flags().Set("agent", "invalid-agent")
	t.Cleanup(func() { initCmd.Flags().Set("agent", "") })

	err := runInit(initCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestRunInit_RootFlagWithAgent(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)

	initCmd.Flags().Set("root", "my-project")
	initCmd.Flags().Set("agent", "windsurf")
	t.Cleanup(func() {
		initCmd.Flags().Set("root", "")
		initCmd.Flags().Set("agent", "")
	})

	captureStdout(t, func() {
		err := runInit(initCmd, nil)
		require.NoError(t, err)
	})

	projectDir := filepath.Join(dir, "my-project")
	assert.FileExists(t, filepath.Join(projectDir, ".windsurfrules"))
	assert.FileExists(t, filepath.Join(projectDir, ".takumi", "TAKUMI.md"))

	cfg, err := config.LoadWorkspaceConfig(filepath.Join(projectDir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "windsurf", cfg.Workspace.AI.Agent)
}
