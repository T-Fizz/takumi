package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWorkspace_Valid(t *testing.T) {
	cfg := DefaultWorkspaceConfig("my-project")
	findings := ValidateWorkspace(cfg)
	assert.Empty(t, findings)
}

func TestValidateWorkspace_EmptyName(t *testing.T) {
	cfg := DefaultWorkspaceConfig("")
	findings := ValidateWorkspace(cfg)
	assert.Len(t, findings, 1)
	assert.Equal(t, SeverityError, findings[0].Severity)
	assert.Equal(t, "workspace.name", findings[0].Field)
}

// TestValidateWorkspace_WhitespaceNameRejectsAsEmpty pins down that a name
// consisting only of whitespace is treated as an empty name (not silently accepted).
func TestValidateWorkspace_WhitespaceNameRejectsAsEmpty(t *testing.T) {
	cfg := DefaultWorkspaceConfig("   \t  ")
	findings := ValidateWorkspace(cfg)
	require.Len(t, findings, 1)
	assert.Equal(t, SeverityError, findings[0].Severity)
	assert.Equal(t, "workspace.name", findings[0].Field)
	assert.Equal(t, "must not be empty", findings[0].Message)
}

// TestValidateWorkspace_WhitespaceSourceURLRejectsAsEmpty pins down that a
// source URL of only whitespace is treated as missing.
func TestValidateWorkspace_WhitespaceSourceURLRejectsAsEmpty(t *testing.T) {
	cfg := DefaultWorkspaceConfig("test")
	cfg.Workspace.Sources["repo"] = Source{URL: "  \t ", Path: "/some/path"}
	findings := ValidateWorkspace(cfg)
	require.Len(t, findings, 1)
	assert.Equal(t, SeverityError, findings[0].Severity)
	assert.Equal(t, "workspace.sources.repo.url", findings[0].Field)
}

func TestValidateWorkspace_InvalidAgent(t *testing.T) {
	cfg := DefaultWorkspaceConfig("test")
	cfg.Workspace.AI.Agent = "chatgpt"
	findings := ValidateWorkspace(cfg)
	assert.Len(t, findings, 1)
	assert.Equal(t, SeverityError, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "chatgpt")
}

func TestValidateWorkspace_ValidAgents(t *testing.T) {
	for _, agent := range []string{"claude", "cursor", "copilot", "windsurf", "cline", "kiro", "none"} {
		cfg := DefaultWorkspaceConfig("test")
		cfg.Workspace.AI.Agent = agent
		findings := ValidateWorkspace(cfg)
		assert.Empty(t, findings, "agent %q should be valid", agent)
	}
}

func TestValidateWorkspace_SourceMissingURL(t *testing.T) {
	cfg := DefaultWorkspaceConfig("test")
	cfg.Workspace.Sources["repo"] = Source{Path: "/some/path"}
	findings := ValidateWorkspace(cfg)
	assert.Len(t, findings, 1)
	assert.Equal(t, SeverityError, findings[0].Severity)
	assert.Contains(t, findings[0].Field, "url")
}

func TestValidateWorkspace_SourceMissingPath(t *testing.T) {
	cfg := DefaultWorkspaceConfig("test")
	cfg.Workspace.Sources["repo"] = Source{URL: "git@example.com:repo.git"}
	findings := ValidateWorkspace(cfg)
	assert.Len(t, findings, 1)
	assert.Equal(t, SeverityWarning, findings[0].Severity)
	assert.Contains(t, findings[0].Field, "path")
}

func TestValidatePackage_Valid(t *testing.T) {
	cfg := DefaultPackageConfig("my-pkg")
	findings := ValidatePackage(cfg)
	assert.Empty(t, findings)
}

func TestValidatePackage_EmptyName(t *testing.T) {
	cfg := DefaultPackageConfig("")
	findings := ValidatePackage(cfg)
	hasNameError := false
	for _, f := range findings {
		if f.Field == "package.name" && f.Severity == SeverityError {
			hasNameError = true
		}
	}
	assert.True(t, hasNameError, "should have error for empty package name")
}

func TestValidatePackage_InvalidVersion(t *testing.T) {
	cfg := DefaultPackageConfig("test")
	cfg.Package.Version = "not-semver"
	findings := ValidatePackage(cfg)
	hasVersionWarning := false
	for _, f := range findings {
		if f.Field == "package.version" && f.Severity == SeverityWarning {
			hasVersionWarning = true
		}
	}
	assert.True(t, hasVersionWarning, "should warn about invalid semver")
}

func TestValidatePackage_ValidVersions(t *testing.T) {
	for _, v := range []string{"0.1.0", "1.0.0", "2.3.4", "1.0.0-alpha", "1.0.0-rc.1"} {
		cfg := DefaultPackageConfig("test")
		cfg.Package.Version = v
		findings := ValidatePackage(cfg)
		for _, f := range findings {
			assert.NotEqual(t, "package.version", f.Field, "version %q should be valid", v)
		}
	}
}

func TestValidatePackage_EmptyVersion(t *testing.T) {
	cfg := DefaultPackageConfig("test")
	cfg.Package.Version = ""
	findings := ValidatePackage(cfg)
	hasWarning := false
	for _, f := range findings {
		if f.Field == "package.version" {
			hasWarning = true
		}
	}
	assert.True(t, hasWarning, "should warn about empty version")
}

func TestValidatePackage_NullPhase(t *testing.T) {
	cfg := DefaultPackageConfig("test")
	cfg.Phases["lint"] = nil
	findings := ValidatePackage(cfg)
	hasError := false
	for _, f := range findings {
		if f.Field == "phases.lint" && f.Severity == SeverityError {
			hasError = true
		}
	}
	assert.True(t, hasError, "should error on null phase")
}

func TestValidatePackage_EmptyPhaseCommands(t *testing.T) {
	cfg := DefaultPackageConfig("test")
	cfg.Phases["lint"] = &Phase{Commands: []string{}}
	findings := ValidatePackage(cfg)
	hasWarning := false
	for _, f := range findings {
		if f.Field == "phases.lint.commands" && f.Severity == SeverityWarning {
			hasWarning = true
		}
	}
	assert.True(t, hasWarning, "should warn about empty commands")
}

func TestValidatePackage_RuntimeNoSetup(t *testing.T) {
	cfg := DefaultPackageConfig("test")
	cfg.Runtime = &Runtime{Env: map[string]string{"FOO": "bar"}}
	findings := ValidatePackage(cfg)
	hasWarning := false
	for _, f := range findings {
		if f.Field == "runtime.setup" {
			hasWarning = true
		}
	}
	assert.True(t, hasWarning, "should warn about runtime with no setup commands")
}

func TestValidateVersionSet_Valid(t *testing.T) {
	cfg := &VersionSetConfig{
		VersionSet: VersionSet{
			Name:     "release-q1",
			Strategy: "strict",
			Packages: map[string]string{"react": "18.0.0"},
		},
	}
	findings := ValidateVersionSet(cfg)
	assert.Empty(t, findings)
}

func TestValidateVersionSet_InvalidStrategy(t *testing.T) {
	cfg := &VersionSetConfig{
		VersionSet: VersionSet{
			Name:     "test",
			Strategy: "yolo",
			Packages: map[string]string{"a": "1.0.0"},
		},
	}
	findings := ValidateVersionSet(cfg)
	assert.Len(t, findings, 1)
	assert.Equal(t, SeverityError, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "yolo")
}

func TestValidateVersionSet_EmptyPackages(t *testing.T) {
	cfg := &VersionSetConfig{
		VersionSet: VersionSet{
			Name:     "test",
			Strategy: "strict",
		},
	}
	findings := ValidateVersionSet(cfg)
	hasWarning := false
	for _, f := range findings {
		if f.Field == "version-set.packages" {
			hasWarning = true
		}
	}
	assert.True(t, hasWarning, "should warn about empty packages")
}

func TestValidateVersionSet_EmptyStrategy(t *testing.T) {
	// No strategy set — the strategy check should be skipped entirely
	cfg := &VersionSetConfig{
		VersionSet: VersionSet{
			Name:     "release",
			Packages: map[string]string{"react": "18.0.0"},
		},
	}
	findings := ValidateVersionSet(cfg)
	assert.Empty(t, findings, "no findings when strategy is empty and config is otherwise valid")
}

func TestValidateVersionSet_EmptyName(t *testing.T) {
	cfg := &VersionSetConfig{
		VersionSet: VersionSet{
			Strategy: "strict",
			Packages: map[string]string{"a": "1.0.0"},
		},
	}
	findings := ValidateVersionSet(cfg)
	hasNameWarning := false
	for _, f := range findings {
		if f.Field == "version-set.name" {
			hasNameWarning = true
		}
	}
	assert.True(t, hasNameWarning, "should warn about empty name")
}

func TestFinding_String(t *testing.T) {
	f := Finding{Severity: SeverityError, Field: "workspace.name", Message: "must not be empty"}
	assert.Equal(t, "error: workspace.name — must not be empty", f.String())

	f2 := Finding{Severity: SeverityWarning, Message: "something"}
	assert.Equal(t, "warning: something", f2.String())
}
