# Takumi Config Reference

Auto-generated from config structures.

## takumi.yaml (Workspace Config)

```yaml
workspace:
  name: <string>          # Workspace name
  ignore:                  # Directories to skip during package scan
    - vendor/
  sources:                 # Tracked git repos
    <name>:
      url: <string>        # Git clone URL
      branch: <string>     # Branch to track
      path: <string>       # Local path
  version-set:
    file: <string>         # Path to takumi-versions.yaml
  settings:
    parallel: <bool>       # Enable parallel builds (default: true)
  ai:
    agent: <string>        # AI agent (claude, cursor, copilot, windsurf, cline, kiro)
    instructions: <string> # Path to takumi-ai.yaml
```

## takumi-pkg.yaml (Package Config)

```yaml
package:
  name: <string>           # Package name
  version: <string>        # Semver version
dependencies:              # Other takumi packages this depends on
  - <package-name>
runtime:                   # Optional: isolated runtime environment
  setup:                   # Commands to create the env
    - <command>
  env:                     # Env vars injected into all commands
    KEY: VALUE
phases:                    # Build phases
  build:
    pre: [<command>]
    commands: [<command>]
    post: [<command>]
```

