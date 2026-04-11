# AI Skills

Takumi's AI skills system generates structured prompts that teach AI assistants about your workspace. Skills collect context automatically and produce output tailored for LLM consumption.

## Agent Setup

During `takumi init`, Takumi asks which AI agent you use and creates the appropriate config file:

| Agent | Config File |
|-------|-------------|
| Claude | `CLAUDE.md` |
| Cursor | `.cursor/rules` |
| Copilot | `.github/copilot-instructions.md` |
| Windsurf | `.windsurfrules` |
| Cline | `.clinerules` |

All agents reference `.takumi/TAKUMI.md`, which contains the **operator** skill — a command reference, workflow guidance, and workspace rules. Pass `--agent claude` (or any agent name) to skip the interactive prompt.

To regenerate context files after workspace changes:

```bash
takumi ai context
```

## Built-in Skills

Takumi ships six built-in skills, embedded in the binary.

### operator

Core instructions written into `.takumi/TAKUMI.md`. Included automatically in the AI agent config — you don't invoke it directly. Contains the command reference, recommended workflow, config file locations, and workspace rules.

### diagnose

Triage a build or test failure. Reads the most recent log from `.takumi/logs/`, collects the git diff, dependency chain, and environment status, then renders a diagnostic prompt.

```bash
takumi ai diagnose api
```

**Auto-context collected:** `last_error_output`, `changed_files`, `dependency_chain`, `package_config`, `env_status`

Output asks the AI to determine:
1. Root cause category (syntax, missing-dep, version-conflict, env, config, test-regression)
2. Exact files and lines to fix
3. Suggested fix (code or command)

### review

Summarize workspace changes for code review. Collects the current git diff, affected packages, and test results.

```bash
takumi ai review
```

**Auto-context collected:** `git_diff`, `affected_packages`, `test_results`

Output asks the AI for:
1. One-line summary
2. Per-package breakdown of what changed and why it matters
3. Risk areas (breaking changes, missing tests, version impacts)
4. Suggested reviewers

### optimize

Analyze build telemetry and suggest performance improvements. Reads `.takumi/metrics.json` and the dependency graph.

```bash
takumi ai optimize
```

**Auto-context collected:** `build_metrics`, `package_graph`

Output asks the AI to identify:
1. Slowest packages and why
2. Parallelism opportunities
3. Phases that could be skipped
4. Specific commands to optimize

### onboard

Generate a workspace briefing for a new developer or a fresh AI session. Collects all configs, the dependency graph, version set, and AI instructions.

```bash
takumi ai onboard
```

**Auto-context collected:** `workspace_config`, `all_package_configs`, `dependency_graph`, `version_set`, `ai_instructions`

Output covers:
1. What the project is
2. Package map in dependency order
3. How to get started (clone, env setup, build, test)
4. Key conventions and gotchas
5. Common tasks across all packages

### doc-writer

Generate or update user-facing documentation. Used internally by `takumi docs generate --ai`.

```bash
takumi docs generate --ai
```

**Auto-context collected:** `command_tree`, `config_schemas`, `recent_commits`, `existing_docs`

## Skill Commands

```bash
takumi ai skill list              # List all skills with source labels
takumi ai skill show diagnose     # Print a skill's YAML definition
takumi ai skill run diagnose      # Render the skill with live workspace context
```

The `list` command shows each skill's source: **built-in**, **workspace**, or **package**.

## Custom Skills

You can define skills at two levels:

### Workspace Skills

Create YAML files in `.takumi/skills/`:

```yaml
# .takumi/skills/deploy-checklist.yaml
skill:
  name: deploy-checklist
  description: "Pre-deploy validation checklist"

  auto_context:
    - affected_packages
    - test_results

  prompt: |
    Generate a deploy checklist for these packages: {{affected_packages}}

    Test results: {{test_results}}

    Check:
    1. All tests pass
    2. No version-set violations
    3. Database migrations are backward-compatible
    4. Rollback plan exists

  max_tokens: 400
```

### Package-Level Tasks

Define tasks in a package's `takumi-pkg.yaml` under the `ai` section:

```yaml
# services/api/takumi-pkg.yaml
ai:
  description: "REST API service"
  notes:
    - "Uses OpenAPI codegen in build pre-step"
    - "Integration tests need DATABASE_URL"
  tasks:
    add-endpoint:
      description: "Add a new API endpoint"
      steps:
        - "Add route in routes.go"
        - "Add handler in handlers/"
        - "Update OpenAPI spec"
        - "Run takumi build api && takumi test api"
```

Package-level `ai.description` and `ai.notes` are included in the `onboard` skill output. The `tasks` section provides step-by-step recipes that AI assistants can follow.

## Skill YAML Schema

```yaml
skill:
  name: <string>              # Identifier (used in commands)
  description: <string>       # One-line purpose

  auto_context:               # Context items collected automatically
    - <context_key>

  prompt: |                   # Template with {{variable}} placeholders
    Your prompt here.
    {{variable}} is substituted at render time.

  output_format: <string>     # Optional: "structured", "markdown"
  max_tokens: <int>           # Optional token limit hint
```

### Variable Substitution

Skill prompts use `{{variable}}` placeholders. At render time, Takumi replaces each placeholder with the collected context value. Unmatched placeholders are left as-is.

## Workflow

A typical AI-assisted workflow using skills:

```bash
# 1. Check workspace state
takumi status

# 2. Build and hit a failure
takumi build --affected
# ✗ api failed

# 3. Diagnose the failure
takumi ai diagnose api
# → Renders prompt with error output, changed files, deps
# → Paste into your AI agent or pipe to clipboard

# 4. Fix, rebuild, test
takumi build api
takumi test api

# 5. Review before committing
takumi ai review
# → Renders prompt with diff, affected packages, test results
```
