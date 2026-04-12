# Configuration Reference

Takumi uses three YAML config files. All are optional except `takumi.yaml`.

## takumi.yaml — Workspace Config

Lives at the workspace root. Created by `takumi init`.

```yaml
workspace:
  name: my-project              # Required. Workspace display name.

  ignore:                       # Directories to skip during package scan and cache hashing.
    - vendor/
    - node_modules/
    - .git/
    - build/

  sources:                      # Tracked git repositories (managed by checkout/sync).
    shared-lib:
      url: git@github.com:org/shared-lib.git
      branch: main
      path: ./shared-lib        # Local clone path relative to workspace root.

  version-set:
    file: takumi-versions.yaml  # Path to version pinning file.

  settings:
    parallel: true              # Run independent packages concurrently (default: true).

  ai:
    agent: claude               # AI agent. Valid: claude, cursor, copilot, windsurf, cline, none.
    instructions: takumi-ai.yaml  # Optional custom AI instructions file.
```

### Ignore Patterns

Entries in `ignore` match directory names at any depth. Trailing `/` is stripped for matching. Always-ignored: `.takumi/`, `.git/`.

## takumi-pkg.yaml — Package Config

Lives in each package directory. Discovered recursively by scanning the workspace.

```yaml
package:
  name: api                     # Required. Package name (used in deps, logs, cache).
  version: 1.2.0                # Semver recommended (validated by `takumi validate`).

dependencies:                   # Other Takumi packages this depends on.
  - shared-lib                  # Must match the `name` field of another package.
  - utils

runtime:                        # Optional. Isolated environment per package.
  setup:                        # Commands to create the environment.
    - python -m venv {{env_dir}}
    - "{{env_dir}}/bin/pip install -r requirements.txt"
  env:                          # Env vars injected into all phase commands.
    PATH: "{{env_dir}}/bin:$PATH"
    PYTHONPATH: ./src
    NODE_ENV: development

phases:                         # Named build phases. Any name works.
  build:
    pre:                        # Run before main commands.
      - echo "generating types"
    commands:                   # Main phase commands. Required if phase is defined.
      - go build -o ../../build/api .
    post:                       # Run after main commands.
      - echo "copying assets"
  test:
    commands:
      - go test ./...
  lint:
    commands:
      - golangci-lint run
  deploy:
    pre:
      - docker build -t api .
    commands:
      - kubectl apply -f k8s/

ai:                             # Optional. AI context for this package.
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
        - "Run takumi build api && takumi test api"
```

### Variable Substitution

`{{env_dir}}` is replaced with `.takumi/envs/<package-name>/` in both `runtime.setup` commands and `runtime.env` values.

### Phase Execution Order

For each phase, commands run in order: `pre` → `commands` → `post`. If any command exits non-zero, execution stops immediately.

### Dependencies

Dependencies control build order only. They must reference other Takumi packages by name. External dependencies (not in the workspace) are silently ignored — they don't block builds.

## takumi-versions.yaml — Version Set Config

Optional centralized version pinning. Referenced from `takumi.yaml` via `version-set.file`.

```yaml
version-set:
  name: release-2026Q2          # Display name for this version set.

  strategy: strict              # Valid: strict, prefer-latest, prefer-pinned.

  packages:                     # Dependency name → pinned version.
    react: "18.3.0"
    typescript: "5.4.0"
    node: "22.0.0"
    python: "3.12.0"
```

### Strategies

| Strategy | Meaning |
|----------|---------|
| `strict` | Exact versions required |
| `prefer-latest` | Use latest unless pinned |
| `prefer-pinned` | Prefer pinned, allow newer if compatible |

## Validation

Run `takumi validate` to check all configs. It verifies:

| Check | Severity |
|-------|----------|
| Empty workspace name | Error |
| Invalid AI agent | Error |
| Source missing URL | Error |
| Source missing path | Warning |
| Empty package name | Error |
| Invalid semver version | Warning |
| Null phase definition | Error |
| Phase with no commands | Warning |
| Runtime with no setup commands | Warning |
| Invalid version-set strategy | Error |
| Unresolved dependency references | Warning |
| Dependency cycles | Error |
