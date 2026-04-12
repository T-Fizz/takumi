# Onboarding an Existing Project

Step-by-step guide for adding Takumi to a project that already has code.

## 1. Initialize the Workspace

`cd` into your project root and run:

```bash
cd your-project
takumi init --agent claude
```

This creates `.takumi/`, `takumi.yaml`, and a root `takumi-pkg.yaml` with placeholder commands. Your existing files are untouched.

## 2. Configure the Root Package

Edit `takumi-pkg.yaml` with your actual build commands:

```yaml
package:
  name: your-project
  version: 0.1.0
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go test ./...
  lint:
    commands:
      - golangci-lint run
```

If your project is a single component, you're done — skip to step 4.

## 3. (Multi-Component Projects) Create Sub-Packages

If your project has multiple components (services, libraries, apps), create a `takumi-pkg.yaml` in each component directory:

```yaml
# internal/takumi-pkg.yaml
package:
  name: internal
  version: 0.1.0
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go test ./...
```

```yaml
# services/api/takumi-pkg.yaml
package:
  name: api
  version: 1.0.0
dependencies:
  - internal                    # declares: api depends on internal
phases:
  build:
    commands:
      - go build -o ../../build/api .
  test:
    commands:
      - go test ./...
```

Takumi discovers packages by scanning for `takumi-pkg.yaml` files recursively. The `dependencies` field controls build order — Takumi will build `internal` before `api`.

### Tip: Output Binaries to a Shared Directory

If your build produces binaries, output them to a directory listed in `takumi.yaml`'s ignore list so they don't invalidate the cache:

```yaml
# takumi.yaml
workspace:
  name: your-project
  ignore:
    - vendor/
    - node_modules/
    - .git/
    - build/                    # build outputs go here
```

## 4. Validate and Preview

```bash
takumi validate                  # Check all configs for errors
takumi graph                     # See dependency order
takumi build --dry-run           # Preview what will run
```

Example output:

```
Dependency Graph

  Level 0 (no deps)
    internal

  Level 1
    api ← internal
    web ← internal

3 packages in 2 levels
```

Level 0 packages build first. Packages within the same level run in parallel.

## 5. Build and Test

```bash
takumi build                     # Build in dependency order
takumi test                      # Run tests
takumi status                    # Full dashboard
```

Run the same build again — unchanged packages are skipped via content-addressed caching:

```
✓ internal cached
✓ api cached
✓ web cached
3 cached
```

## 6. (Optional) Runtime Environments

If a package needs an isolated environment (virtualenv, nvm, etc.):

```yaml
# services/ml-pipeline/takumi-pkg.yaml
package:
  name: ml-pipeline
  version: 0.1.0
runtime:
  setup:
    - python -m venv {{env_dir}}
    - "{{env_dir}}/bin/pip install -r requirements.txt"
  env:
    PATH: "{{env_dir}}/bin:$PATH"
    PYTHONPATH: ./src
phases:
  build:
    commands:
      - python -m compileall src/
  test:
    commands:
      - pytest tests/
```

```bash
takumi env setup                 # Create all environments
takumi env list                  # Check status
```

`{{env_dir}}` resolves to `.takumi/envs/<package-name>/` — a dedicated directory per package.

## 7. (Optional) Generate Documentation

```bash
takumi docs generate             # Auto-generate docs from configs
```

Creates `docs/user/` with:
- `commands.md` — CLI reference from Cobra definitions
- `skills-reference.md` — available AI skills
- `config-reference.md` — annotated YAML schemas
- `packages.md` — table of all packages with versions, deps, phases

## 8. (Optional) Track External Repos

If your workspace spans multiple git repositories:

```bash
takumi checkout git@github.com:org/shared-lib.git
takumi sync                      # Pull all tracked sources
```

This clones the repo, scans it for `takumi-pkg.yaml` files, and registers it in `takumi.yaml`.

## 9. (Optional) MCP Server for Claude Code

If you use Claude Code, you have two options:

**Per-project:** add a `.mcp.json` file to your project root:

```json
{
  "mcpServers": {
    "takumi": {
      "command": "takumi",
      "args": ["mcp", "serve"]
    }
  }
}
```

**Global:** register Takumi once for all projects:

```bash
takumi mcp install
```

This writes to `~/.claude/claude_desktop_config.json` so Takumi tools are available everywhere — even in projects that haven't run `takumi init` yet.

Either way, Claude Code can call `takumi_build`, `takumi_test`, `takumi_diagnose`, and other tools directly. See [AI Skills](ai-skills.md#mcp-server-integration) for details.
