package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Severity indicates how critical a validation finding is.
type Severity int

const (
	SeverityError   Severity = iota // Must fix — config is invalid
	SeverityWarning                 // Should fix — may cause unexpected behavior
)

// Finding is a single validation issue.
type Finding struct {
	Severity Severity
	Field    string // e.g. "workspace.name", "package.version"
	Message  string
}

func (f Finding) String() string {
	prefix := "error"
	if f.Severity == SeverityWarning {
		prefix = "warning"
	}
	if f.Field != "" {
		return fmt.Sprintf("%s: %s — %s", prefix, f.Field, f.Message)
	}
	return fmt.Sprintf("%s: %s", prefix, f.Message)
}

var validAgents = map[string]bool{
	"claude": true, "cursor": true, "copilot": true,
	"windsurf": true, "cline": true, "none": true,
}

var validStrategies = map[string]bool{
	"strict": true, "prefer-latest": true, "prefer-pinned": true,
}

// semverPattern matches basic semver: major.minor.patch with optional pre-release.
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)

// ValidateWorkspace checks a workspace config for structural issues.
func ValidateWorkspace(cfg *WorkspaceConfig) []Finding {
	var findings []Finding

	if strings.TrimSpace(cfg.Workspace.Name) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Field:    "workspace.name",
			Message:  "must not be empty",
		})
	}

	if agent := cfg.Workspace.AI.Agent; agent != "" {
		if !validAgents[agent] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Field:    "workspace.ai.agent",
				Message:  fmt.Sprintf("%q is not a supported agent (claude, cursor, copilot, windsurf, cline, none)", agent),
			})
		}
	}

	for name, src := range cfg.Workspace.Sources {
		if strings.TrimSpace(src.URL) == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Field:    fmt.Sprintf("workspace.sources.%s.url", name),
				Message:  "must not be empty",
			})
		}
		if strings.TrimSpace(src.Path) == "" {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Field:    fmt.Sprintf("workspace.sources.%s.path", name),
				Message:  "not set — will use source name as path",
			})
		}
	}

	return findings
}

// ValidatePackage checks a package config for structural issues.
func ValidatePackage(cfg *PackageConfig) []Finding {
	var findings []Finding

	if strings.TrimSpace(cfg.Package.Name) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Field:    "package.name",
			Message:  "must not be empty",
		})
	}

	if strings.TrimSpace(cfg.Package.Version) == "" {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Field:    "package.version",
			Message:  "not set",
		})
	} else if !semverPattern.MatchString(cfg.Package.Version) {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Field:    "package.version",
			Message:  fmt.Sprintf("%q is not valid semver (expected x.y.z)", cfg.Package.Version),
		})
	}

	for phaseName, phase := range cfg.Phases {
		if phase == nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Field:    fmt.Sprintf("phases.%s", phaseName),
				Message:  "is null — define commands or remove the phase",
			})
			continue
		}
		if len(phase.Commands) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Field:    fmt.Sprintf("phases.%s.commands", phaseName),
				Message:  "has no commands",
			})
		}
	}

	if cfg.Runtime != nil && len(cfg.Runtime.Setup) == 0 {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Field:    "runtime.setup",
			Message:  "runtime defined but no setup commands — environment will be empty",
		})
	}

	return findings
}

// ValidateVersionSet checks a version-set config for structural issues.
func ValidateVersionSet(cfg *VersionSetConfig) []Finding {
	var findings []Finding

	if strings.TrimSpace(cfg.VersionSet.Name) == "" {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Field:    "version-set.name",
			Message:  "not set",
		})
	}

	if s := cfg.VersionSet.Strategy; s != "" {
		if !validStrategies[s] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Field:    "version-set.strategy",
				Message:  fmt.Sprintf("%q is not valid (strict, prefer-latest, prefer-pinned)", s),
			})
		}
	}

	if len(cfg.VersionSet.Packages) == 0 {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Field:    "version-set.packages",
			Message:  "no versions pinned",
		})
	}

	return findings
}
