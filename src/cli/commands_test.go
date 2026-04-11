package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImplementedCommands verifies implemented commands run in a workspace.
func TestImplementedCommands(t *testing.T) {
	dir := t.TempDir()
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"build clean", []string{"build", "clean"}, "Cleaned", false},
		{"sync", []string{"sync"}, "No tracked sources", false},
		{"ai context", []string{"ai", "context"}, "Regenerated AI context", false},
		{"ai review", []string{"ai", "review"}, "Review Prompt", false},
		{"ai optimize", []string{"ai", "optimize"}, "Optimization Prompt", false},
		{"ai onboard", []string{"ai", "onboard"}, "Onboarding Prompt", false},
		{"ai skill list", []string{"ai", "skill", "list"}, "Available Skills", false},
		{"ai skill show operator", []string{"ai", "skill", "show", "operator"}, "Skill: operator", false},
		{"ai skill run operator", []string{"ai", "skill", "run", "operator"}, "Running: operator", false},
		{"docs generate", []string{"docs", "generate"}, "Generating Documentation", false},
		{"docs hook remove no git", []string{"docs", "hook", "remove"}, "No pre-commit hook", false},
		{"docs hook install no git", []string{"docs", "hook", "install"}, "", true},
		{"ai diagnose missing pkg", []string{"ai", "diagnose", "pkg"}, "", true},
		{"validate", []string{"validate"}, "All configs valid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				rootCmd.SetArgs(tt.args)
				err := rootCmd.Execute()
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
			if tt.want != "" {
				assert.Contains(t, out, tt.want)
			}
		})
	}
}

// TestBuildCmd_Flags verifies the --affected flag is registered.
func TestBuildCmd_Flags(t *testing.T) {
	f := buildCmd.Flags().Lookup("affected")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

// TestTestCmd_Flags verifies the --affected flag is registered.
func TestTestCmd_Flags(t *testing.T) {
	f := testCmd.Flags().Lookup("affected")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

// TestCheckoutCmd_Flags verifies flags are registered.
func TestCheckoutCmd_Flags(t *testing.T) {
	assert.NotNil(t, checkoutCmd.Flags().Lookup("branch"))
	assert.NotNil(t, checkoutCmd.Flags().Lookup("path"))
}

// TestRemoveCmd_Flags verifies the --delete flag is registered.
func TestRemoveCmd_Flags(t *testing.T) {
	f := removeCmd.Flags().Lookup("delete")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

// TestAffectedCmd_Flags verifies the --since flag is registered.
func TestAffectedCmd_Flags(t *testing.T) {
	f := affectedCmd.Flags().Lookup("since")
	require.NotNil(t, f)
}

// TestDocsGenerateCmd_Flags verifies the --ai flag is registered.
func TestDocsGenerateCmd_Flags(t *testing.T) {
	f := docsGenerateCmd.Flags().Lookup("ai")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

// TestVersionSetCmd_Alias verifies the "vs" alias is registered.
func TestVersionSetCmd_Alias(t *testing.T) {
	assert.Contains(t, versionSetCmd.Aliases, "vs")
}

// TestCommandTree_SubcommandsRegistered verifies the full command tree.
func TestCommandTree_SubcommandsRegistered(t *testing.T) {
	rootSubs := commandNames(rootCmd.Commands())
	for _, name := range []string{"init", "build", "test", "run", "checkout", "remove",
		"sync", "env", "graph", "status", "affected", "version-set", "ai", "docs", "validate"} {
		assert.Contains(t, rootSubs, name, "root should have %q subcommand", name)
	}

	buildSubs := commandNames(buildCmd.Commands())
	for _, name := range []string{"clean"} {
		assert.Contains(t, buildSubs, name, "build should have %q subcommand", name)
	}

	envSubs := commandNames(envCmd.Commands())
	for _, name := range []string{"setup", "clean", "list"} {
		assert.Contains(t, envSubs, name, "env should have %q subcommand", name)
	}

	aiSubs := commandNames(aiCmd.Commands())
	for _, name := range []string{"context", "diagnose", "review", "optimize", "onboard", "skill"} {
		assert.Contains(t, aiSubs, name, "ai should have %q subcommand", name)
	}

	skillSubs := commandNames(aiSkillCmd.Commands())
	for _, name := range []string{"list", "show", "run"} {
		assert.Contains(t, skillSubs, name, "ai skill should have %q subcommand", name)
	}

	docsSubs := commandNames(docsCmd.Commands())
	for _, name := range []string{"generate", "hook"} {
		assert.Contains(t, docsSubs, name, "docs should have %q subcommand", name)
	}

	hookSubs := commandNames(docsHookCmd.Commands())
	for _, name := range []string{"install", "remove"} {
		assert.Contains(t, hookSubs, name, "docs hook should have %q subcommand", name)
	}
}

func commandNames(cmds []*cobra.Command) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name()
	}
	return names
}

// captureStdout redirects os.Stdout during fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	return buf.String()
}
