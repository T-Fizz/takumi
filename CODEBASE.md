# Takumi (匠) — Codebase Outline

> Generated for manual pre-release review. Every exported symbol, function signature,
> and data flow is documented below. Internal helpers are included where they carry
> meaningful logic.

---

## Architecture Overview

```
cmd/takumi/main.go          Entry point — calls cli.Execute()
                              │
src/cli/                     Cobra command tree + TUI glue
  ├── root.go                Root command, osExit injection, workspace loading
  ├── init.go                `takumi init` — scaffold workspace + package
  ├── build.go               `takumi build` — phase execution + dry-run + caching
  ├── test.go                `takumi test` — delegates to runPhaseCommand("test")
  ├── run.go                 `takumi run <phase>` — arbitrary phase execution
  ├── graph.go               `takumi graph` — dependency DAG display
  ├── status.go              `takumi status` — workspace health dashboard
  ├── affected.go            `takumi affected` — git-diff → package mapping
  ├── env.go                 `takumi env setup/clean/list` — runtime envs
  ├── sync.go                `takumi sync` — git clone/pull tracked sources
  ├── checkout.go            `takumi checkout <url>` — clone + register
  ├── remove.go              `takumi remove <pkg>` — deregister + cleanup
  ├── validate.go            `takumi validate` — structural + cross-validation
  ├── versionset.go          `takumi version-set check` — pinned dep report
  ├── ai.go                  `takumi ai *` — skill context/diagnose/review/optimize/onboard
  ├── agent.go               AI agent selection (interactive + flag) + config
  ├── mcp.go                 `takumi mcp serve` — start MCP server
  └── docs.go                `takumi docs generate/hook` — auto-doc generation
                              │
src/config/                  YAML config parsing + validation
  ├── workspace.go           WorkspaceConfig (takumi.yaml)
  ├── package.go             PackageConfig (takumi-pkg.yaml)
  ├── versionset.go          VersionSetConfig (takumi-versions.yaml)
  └── validate.go            Structural validators + Finding type
                              │
src/workspace/               Workspace detection + package scanning
  └── workspace.go           Detect(), ScanPackages(), Load()
                              │
src/graph/                   Dependency DAG (Kahn's algorithm)
  └── graph.go               Graph type, Sort(), Flatten(), TransitiveDependents()
                              │
src/executor/                Phase execution engine
  └── executor.go            Run(), cache-aware execution, metrics, prefixWriter
                              │
src/cache/                   Content-addressed build caching
  └── cache.go               ComputeKey(), Store (Lookup/Write/Clean)
                              │
src/skills/                  AI skill template system
  ├── skills.go              LoadBuiltins(), LoadFromDir(), Render()
  └── builtin/*.yaml         6 embedded skill YAML files
                              │
src/mcp/                     MCP (Model Context Protocol) server
  ├── server.go              NewServer() — creates and configures MCPServer
  └── tools.go               7 tool definitions + handlers
                              │
src/ui/                      Terminal styling (charmbracelet)
  ├── styles.go              Colors, text styles, helper renderers
  └── logger.go              Global logger (charmbracelet/log)
```

### Data Flow: `takumi build`

```
CLI flags → loadWorkspace() → buildGraph() → graph.Sort()
  → for each level:
      → cache.ComputeKey() per package (SHA-256 of phase+config+sources+dep-keys)
      → cache.Store.Lookup() → hit? skip : runPhase()
      → on success → cache.Store.Write()
  → executor.RecordMetrics() (non-cached only)
  → print summary (passed/failed/cached/skipped)
```

### Data Flow: `takumi ai diagnose <pkg>`

```
CLI args → loadWorkspace() → read .takumi/logs/<pkg>.<phase>.log
  → findSkill("diagnose") from embedded YAML
  → collect context: git diff, dependency chain, env status
  → skills.Render(template, vars) → print rendered prompt
```

---

## Package: `config` — Configuration Parsing & Validation

### Types

```go
// WorkspaceConfig — top-level takumi.yaml
type WorkspaceConfig struct {
    Workspace Workspace `yaml:"workspace"`
}

type Workspace struct {
    Name       string            `yaml:"name"`
    Ignore     []string          `yaml:"ignore,omitempty"`
    Sources    map[string]Source  `yaml:"sources,omitempty"`
    VersionSet VersionSetRef     `yaml:"version-set,omitempty"`
    Settings   WorkspaceSettings `yaml:"settings"`
    AI         WorkspaceAIRef    `yaml:"ai,omitempty"`
}

type Source struct {
    URL    string `yaml:"url"`
    Branch string `yaml:"branch"`
    Path   string `yaml:"path"`
}

type VersionSetRef struct {
    File string `yaml:"file,omitempty"`
}

type WorkspaceSettings struct {
    Parallel bool `yaml:"parallel"`
}

type WorkspaceAIRef struct {
    Instructions string `yaml:"instructions,omitempty"`
    Agent        string `yaml:"agent,omitempty"`
}
```

```go
// PackageConfig — takumi-pkg.yaml
type PackageConfig struct {
    Package      PackageMeta       `yaml:"package"`
    Dependencies []string          `yaml:"dependencies,omitempty"`
    Runtime      *Runtime          `yaml:"runtime,omitempty"`
    Phases       map[string]*Phase `yaml:"phases,omitempty"`
    AI           *PackageAI        `yaml:"ai,omitempty"`
}

type PackageMeta struct {
    Name    string `yaml:"name"`
    Version string `yaml:"version"`
}

type Runtime struct {
    Setup []string          `yaml:"setup,omitempty"`
    Env   map[string]string `yaml:"env,omitempty"`
}

type Phase struct {
    Pre      []string `yaml:"pre,omitempty"`
    Commands []string `yaml:"commands"`
    Post     []string `yaml:"post,omitempty"`
}

type PackageAI struct {
    Description string            `yaml:"description,omitempty"`
    Notes       []string          `yaml:"notes,omitempty"`
    Tasks       map[string]AITask `yaml:"tasks,omitempty"`
}

type AITask struct {
    Description string   `yaml:"description"`
    Steps       []string `yaml:"steps"`
}
```

```go
// VersionSetConfig — takumi-versions.yaml
type VersionSetConfig struct {
    VersionSet VersionSet `yaml:"version-set"`
}

type VersionSet struct {
    Name     string            `yaml:"name"`
    Packages map[string]string `yaml:"packages"`
    Strategy string            `yaml:"strategy"` // strict | prefer-latest | prefer-pinned
}
```

```go
// Validation
type Severity int  // SeverityError=0, SeverityWarning=1

type Finding struct {
    Severity Severity
    Field    string  // e.g. "workspace.name", "package.version"
    Message  string
}
```

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `LoadWorkspaceConfig(path string)` | File path to takumi.yaml | `(*WorkspaceConfig, error)` | Reads + YAML-parses workspace config. Error wraps with "reading workspace config" or "parsing workspace config". |
| `DefaultWorkspaceConfig(name string)` | Workspace name | `*WorkspaceConfig` | Returns config with name, default ignores (`vendor/`, `node_modules/`, `.git/`), parallel=true, empty sources map. |
| `(c *WorkspaceConfig) Marshal()` | — | `([]byte, error)` | Serializes to YAML bytes via yaml.Marshal. |
| `SaveWorkspaceConfig(path string, cfg *WorkspaceConfig)` | File path, config | `error` | Marshal + WriteFile. Error wraps "marshaling" or "writing workspace config". |
| `LoadPackageConfig(path string)` | File path to takumi-pkg.yaml | `(*PackageConfig, error)` | Reads + YAML-parses package config. |
| `DefaultPackageConfig(name string)` | Package name | `*PackageConfig` | Returns config with name, version "0.1.0", build+test phases with echo placeholders. |
| `(c *PackageConfig) Marshal()` | — | `([]byte, error)` | Serializes to YAML bytes. |
| `LoadVersionSetConfig(path string)` | File path to takumi-versions.yaml | `(*VersionSetConfig, error)` | Reads + YAML-parses version-set config. |
| `ValidateWorkspace(cfg *WorkspaceConfig)` | Parsed workspace config | `[]Finding` | Checks: empty name → error; invalid AI agent → error; source missing URL → error; source missing path → warning. Valid agents: claude, cursor, copilot, windsurf, cline, none. |
| `ValidatePackage(cfg *PackageConfig)` | Parsed package config | `[]Finding` | Checks: empty name → error; empty/invalid semver version → warning; null phase → error; empty commands → warning; runtime with no setup → warning. Semver regex: `^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$` |
| `ValidateVersionSet(cfg *VersionSetConfig)` | Parsed version-set config | `[]Finding` | Checks: empty name → warning; invalid strategy → error; empty packages → warning. Valid strategies: strict, prefer-latest, prefer-pinned. |
| `(f Finding) String()` | — | `string` | Formats as `"error: field — message"` or `"warning: message"`. Omits field if empty. |

---

## Package: `workspace` — Detection & Discovery

### Constants

```go
MarkerDir     = ".takumi"
WorkspaceFile = "takumi.yaml"
PackageFile   = "takumi-pkg.yaml"
VersionsFile  = "takumi-versions.yaml"
AIFile        = "takumi-ai.yaml"
```

### Types

```go
type Info struct {
    Root     string                    // Absolute path to workspace root
    Config   *config.WorkspaceConfig
    Packages map[string]*DiscoveredPkg
}

type DiscoveredPkg struct {
    Name   string
    Dir    string                    // Absolute path to pkg dir
    Config *config.PackageConfig
}
```

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `Detect(startDir string)` | Starting directory | `string` (empty on miss) | Walks up filesystem looking for `.takumi/` directory. Returns the parent dir (workspace root) or `""`. |
| `ScanPackages(root string, ignore []string)` | Workspace root, ignore patterns | `(map[string]*DiscoveredPkg, error)` | Recursive `filepath.Walk` from root. Skips `.takumi/` and ignored dirs. Finds all `takumi-pkg.yaml`, parses each, returns map keyed by package name. Silently skips inaccessible paths and unparseable configs. |
| `Load(startDir string)` | Starting directory | `(*Info, error)` | Calls Detect → LoadWorkspaceConfig → ScanPackages. Returns `nil, nil` if not in workspace (no error). |
| `shouldIgnore(root, path string, ignore []string)` | Root dir, candidate path, patterns | `bool` | Internal. Matches directory name directly or relative path prefix against ignore patterns (trailing `/` stripped). |

---

## Package: `graph` — Dependency DAG

### Types

```go
type Graph struct {
    nodes map[string][]string  // node → dependencies
}

type Level struct {
    Index    int      // 0-based
    Packages []string // parallelizable within level
}
```

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `New()` | — | `*Graph` | Creates empty graph with initialized map. |
| `(g *Graph) AddNode(name string, deps []string)` | Node name, dependency list | — | Registers node. External deps (not in graph) silently ignored during sort. |
| `(g *Graph) Nodes()` | — | `[]string` | All registered node names (arbitrary order). |
| `(g *Graph) DepsOf(name string)` | Node name | `[]string` | Direct dependencies of node, nil if not found. |
| `(g *Graph) Dependents(name string)` | Node name | `[]string` | All nodes that directly depend on given node. O(V*E) scan. |
| `(g *Graph) Sort()` | — | `([]Level, error)` | Kahn's algorithm topological sort. Returns levels (each level's packages have no interdependencies). Error if cycle detected — includes names of cycle-involved nodes. |
| `(g *Graph) Flatten()` | — | `([]string, error)` | Calls Sort(), concatenates all levels into single ordered slice. |
| `(g *Graph) TransitiveDependents(name string)` | Node name | `[]string` | BFS from node through Dependents(). Returns all downstream consumers (excludes the node itself). |

### Algorithm Detail: Kahn's Sort

1. Compute in-degree for each node (count of in-graph deps)
2. Seed queue with zero-degree nodes → Level 0
3. For each completed node, decrement in-degree of its dependents
4. Nodes hitting zero form the next level
5. If processed < total nodes → cycle detected

---

## Package: `executor` — Phase Execution Engine

### Types

```go
type Result struct {
    Package  string
    Phase    string
    ExitCode int
    Duration time.Duration
    Error    error
    LogFile  string
    CacheHit bool
}

type RunOptions struct {
    Phase    string
    Packages []string  // nil = all
    Parallel bool
    NoCache  bool
    Quiet    bool      // suppress terminal output (log files still written)
}

type MetricsEntry struct {
    Timestamp  string `json:"timestamp"`
    Phase      string `json:"phase"`
    Package    string `json:"package"`
    DurationMs int64  `json:"duration_ms"`
    ExitCode   int    `json:"exit_code"`
}

type MetricsFile struct {
    Runs []MetricsEntry `json:"runs"`
}
```

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `Run(ws *workspace.Info, opts RunOptions)` | Workspace, options | `([]Result, error)` | Main entry. Builds graph, sorts into levels, executes each level (serial or parallel per opts.Parallel). Stops on first failure. Returns all results + error if any phase failed. |
| `runCached(ws, pkgName, opts, store, cacheKeys)` | — | `Result` | Internal. Computes cache key from phase+config+sources+dep-keys. On cache hit returns `CacheHit=true` result. On miss runs phase + writes cache entry on success. |
| `runParallelCached(ws, packages, opts, store, cacheKeys)` | — | `[]Result` | Internal. Snapshots cacheKeys for safe concurrent reads, spawns goroutine per package, merges keys back after `sync.WaitGroup`. |
| `runCachedLocal(ws, pkgName, opts, store, localKeys)` | — | `Result` | Internal. Goroutine-safe version of runCached using local key map. |
| `runPhase(ws *workspace.Info, pkgName, phase string)` | Workspace, package, phase | `Result` | Executes pre→commands→post via `sh -c`. Creates log file at `.takumi/logs/<pkg>.<phase>.log`. Injects runtime env vars (with `{{env_dir}}` substitution). Tees output to log + prefixed stdout/stderr. |
| `RecordMetrics(wsRoot string, results []Result)` | Workspace root, results | `error` | Appends to `.takumi/metrics.json`. Reads existing file (resets on corrupt JSON), appends new entries, writes back indented JSON. |

### Internal: `prefixWriter`

```go
type prefixWriter struct {
    prefix string
    w      io.Writer
    atBOL  bool  // at beginning of line
}
```

Prepends `[pkg-name] ` prefix to each line of command output. Implements `io.Writer`.

---

## Package: `cache` — Content-Addressed Build Caching

### Types

```go
type Entry struct {
    Key        string `json:"key"`
    Package    string `json:"package"`
    Phase      string `json:"phase"`
    Timestamp  string `json:"timestamp"`
    DurationMs int64  `json:"duration_ms"`
    FileCount  int    `json:"file_count"`
}

type Store struct {
    Dir string  // .takumi/cache/
}
```

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `NewStore(wsRoot string)` | Workspace root | `*Store` | Creates store at `wsRoot/.takumi/cache/`. |
| `(s *Store) Lookup(pkg, phase string)` | Package name, phase | `*Entry` (nil on miss) | Reads `<pkg>.<phase>.json` from cache dir. Returns nil on any error or corrupt JSON. |
| `(s *Store) Write(entry *Entry)` | Cache entry | `error` | Creates cache dir if needed. Writes indented JSON to `<pkg>.<phase>.json`. |
| `(s *Store) Clean()` | — | `error` | `os.RemoveAll` on cache directory. |
| `ComputeKey(pkgDir, configPath, phase string, depKeys map[string]string, ignore []string)` | Package dir, config path, phase, dep cache keys, ignore patterns | `(key string, fileCount int, error)` | SHA-256 digest of: `phase:<name>` + `config:<hash>` + sorted `file:<relpath>:<hash>` + sorted `dep:<name>:<key>`. Returns hex-encoded 64-char key. |
| `hashFile(path string)` | File path | `(string, error)` | Internal. SHA-256 of file contents, hex-encoded. |
| `hashDirectory(w io.Writer, pkgDir string, ignore []string)` | Writer, dir, ignores | `(int, error)` | Internal. Walks dir, hashes all files (sorted by relative path), writes `file:<rel>:<hash>` lines to writer. Skips `.takumi/`, `.git/`, and ignored dirs. Returns file count. |

### Cache Key Composition

```
phase:build
config:<sha256 of takumi-pkg.yaml>
file:main.go:<sha256>
file:util.go:<sha256>
dep:lib-a:<lib-a's cache key>
dep:lib-b:<lib-b's cache key>
```

Keys are computed in topological order so dependency changes cascade.

---

## Package: `skills` — AI Skill Templates

### Types

```go
type Skill struct {
    Name         string   `yaml:"name"`
    Description  string   `yaml:"description"`
    AutoContext  []string `yaml:"auto_context,omitempty"`
    Prompt       string   `yaml:"prompt"`
    OutputFormat string   `yaml:"output_format,omitempty"`
    MaxTokens    int      `yaml:"max_tokens,omitempty"`
}

type SkillFile struct {
    Skill Skill `yaml:"skill"`
}

type Source int  // SourceBuiltin=0, SourceWorkspace=1, SourcePackage=2

type LoadedSkill struct {
    Skill
    Source Source
    Path   string  // empty for embedded
}
```

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `LoadBuiltins()` | — | `([]LoadedSkill, error)` | Reads all `.yaml` from `go:embed builtin/` FS. Parses each as SkillFile. All returned with `Source=SourceBuiltin`, `Path=""`. |
| `LoadFromDir(dir string, source Source)` | Directory path, source tag | `([]LoadedSkill, error)` | Reads `.yaml` files from filesystem directory. Silently skips: dirs, non-YAML, unreadable files, invalid YAML, empty names. Returns nil (not error) for nonexistent directory. |
| `Render(prompt string, vars map[string]string)` | Template string, variable map | `string` | Simple `{{key}}` → value substitution via `strings.ReplaceAll`. Unmatched placeholders left as-is. |

### Built-in Skills (6 total)

| Skill | Purpose |
|-------|---------|
| `operator` | Workspace operation instructions for AI assistants |
| `diagnose` | Triage build/test failures — uses `{{package_name}}`, `{{error_output}}`, etc. |
| `review` | Summarize workspace changes for code review |
| `optimize` | Analyze build performance from metrics |
| `onboard` | Generate workspace briefing for new developers |
| `doc-writer` | Generate enhanced documentation |

---

## Package: `mcp` — MCP Server

Exposes Takumi workspace operations as Model Context Protocol tools over stdio, enabling AI agents to operate the workspace directly.

### Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `NewServer()` | — | `*server.MCPServer` | Creates MCP server ("takumi", "0.1.0"), registers all 7 tools, returns configured server. |

### Tools (7 total)

| Tool | Parameters | Description |
|------|-----------|-------------|
| `takumi_status` | — | Workspace health dashboard: packages, deps, phases, sources, recent builds, AI agent |
| `takumi_build` | `packages?`, `affected?`, `no_cache?` | Build packages in dependency order. Supports affected-only and cache bypass. |
| `takumi_test` | `packages?`, `affected?`, `no_cache?` | Run test phase. Same options as build. |
| `takumi_diagnose` | `package` (required) | Read most recent build/test log for a package. Returns log contents for failure triage. |
| `takumi_affected` | `since?` | List packages affected by file changes + transitive dependents. Default: working tree changes. |
| `takumi_validate` | — | Validate all config files: structural checks, unresolved deps, cycle detection, version-set. |
| `takumi_graph` | — | Return dependency graph with parallel level annotations. |

### Design Decisions

- **File-based output:** Build/test logs are written to `.takumi/logs/`. Tool results return a summary + file path, not inline log content. Reduces token consumption for AI agents.
- **Serialized execution:** Server uses `WorkerPoolSize(1)` to serialize tool calls. Prevents concurrent builds from interfering.
- **Quiet mode:** All tool handlers set `executor.RunOptions.Quiet = true`, routing terminal output to `io.Discard` so it doesn't corrupt the stdio JSON-RPC transport.
- **Package alias:** Uses `gomcp "github.com/mark3labs/mcp-go/mcp"` to avoid collision with the `package mcp` declaration.

### Internal Helpers

| Function | Description |
|----------|-------------|
| `loadWorkspace()` | `os.Getwd()` → `workspace.Load(cwd)`, returns error if no workspace found |
| `newGraph(ws)` | Builds `graph.Graph` from workspace packages |
| `handlePhase(phase)` | Shared handler for build/test — parses packages, affected, no_cache params |
| `gitChangedFiles(wsRoot, since)` | Duplicated from cli/affected.go to avoid import cycle |
| `mapFilesToPackages(ws, files)` | Maps changed files to affected packages |
| `sortedPackageNames(ws)` | Returns alphabetically sorted package names |
| `sortedKeys(m)` | Returns sorted keys from a `map[string]string` |
| `capitalize(s)` | Uppercases first character |

---

## Package: `ui` — Terminal Styling

### Colors (Takumi Brand Palette)

| Constant | Hex | Usage |
|----------|-----|-------|
| `ColorPrimary` | `#E8A87C` | Warm amber — brand color |
| `ColorSecondary` | `#95DAC1` | Sage green — section headers, commands |
| `ColorAccent` | `#C49BBB` | Soft purple — bullets, accents |
| `ColorSuccess` | `#73D2A0` | Mint green — pass/check |
| `ColorWarning` | `#F4C95D` | Golden yellow — warnings |
| `ColorError` | `#E76F51` | Terracotta — errors/failures |
| `ColorMuted` | `#7C7C7C` | Gray — secondary info |
| `ColorBright` | `#FAFAFA` | Near-white — bold text |

### Style Variables

`Bold`, `Muted`, `Primary`, `Success`, `Warning`, `Error`, `Accent`, `Banner`, `SectionHeader`, `BulletStyle`, `FilePathStyle`, `CommandStyle`, `BoxStyle`

### Helper Functions

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `Check(msg)` | String | String | `✓` + msg (success colored) |
| `Cross(msg)` | String | String | `✗` + msg (error colored) |
| `Warn(msg)` | String | String | `!` + msg (warning colored) |
| `Bullet(msg)` | String | String | `→` + msg (accent colored) |
| `FilePath(path)` | String | String | Italic primary-colored path |
| `Command(cmd)` | String | String | Bold secondary-colored command |
| `Header()` | — | String | `匠 Takumi` banner |
| `StepDone(msg)` | String | String | Indented check mark |
| `StepInfo(msg)` | String | String | Indented bullet |
| `Summary(title, body)` | Strings | String | Rounded-border box with bold title |
| `Divider()` | — | String | Gray horizontal line |
| `FormatCount(n, singular, plural)` | int, strings | String | `"1 package"` or `"3 packages"` |

### Logger (`logger.go`)

```go
var Log *log.Logger  // Global, initialized in init()

func SetVerbose(on bool)  // Debug vs Info level
func takumiLogStyles()    // Internal — maps log levels to Takumi palette
```

---

## Package: `cli` — Command Tree

### Root (`root.go`)

```go
var rootCmd *cobra.Command   // "takumi" — top-level
var osExit = os.Exit         // Injected in tests

func Execute()                                    // Runs rootCmd.Execute(), exits on error
func loadWorkspace() (*workspace.Info, error)      // Detect + Load from cwd
func requireWorkspace() *workspace.Info            // loadWorkspace or exit(1) with styled error
```

### Init (`init.go`)

```go
// Flags: --root <name>, --agent <name>

func runInit(cmd, args) error
  // --root mode: create project dir → initWorkspace → initPackageInDir
  // Standard mode: determine target dir → Detect workspace → initWorkspace if needed → initPackageInDir

func resolveAgent(cmd) (*AgentType, error)
  // --agent flag → AgentByName, or interactive menu via promptAgentSelection

func initPackageInDir(targetDir, pkgName, wsRoot string, isSubdir bool) error
  // Creates takumi-pkg.yaml with DefaultPackageConfig. Errors if already exists.

func initWorkspace(root, name string, agent *AgentType) error
  // Creates: .takumi/ + subdirs (envs, logs, skills, skills/_builtin)
  // Writes: takumi.yaml, .takumi/TAKUMI.md, agent config file
```

### Agent (`agent.go`)

```go
type AgentType struct {
    Name     string  // "claude", "cursor", "copilot", "windsurf", "cline", "none"
    Label    string  // Display name
    FilePath string  // Config file relative to workspace root
}

var SupportedAgents []AgentType  // 6 entries

func AgentByName(name string) *AgentType      // Lookup by name, nil if not found
func agentNames() string                       // "claude, cursor, copilot, ..."
var promptAgentSelection func() (*AgentType, error)  // Interactive huh.Select menu (mockable)
func setupAgentConfig(wsRoot string, agent *AgentType) error  // Creates/appends include line
func writeTakumiMD(wsRoot, wsName string) error               // Writes .takumi/TAKUMI.md
func takumiMDContent(wsName string) string                    // Returns TAKUMI.md template
```

### Build / Test / Run (`build.go`, `test.go`, `run.go`)

```go
// build.go — Flags: --affected, --no-cache, --dry-run
func runBuild(cmd, args) error       // → runPhaseCommand(cmd, args, "build")
func runBuildClean(cmd, args) error   // RemoveAll build/ + cache.Clean()

func runPhaseCommand(cmd, args []string, phase string) error
  // Core flow: loadWorkspace → handle --affected (git diff + transitive deps)
  // → handle --dry-run (printDryRun) → executor.Run() → RecordMetrics → print summary

func printDryRun(ws, packages []string, phase string, noCache bool) error
  // Builds graph, sorts levels, computes cache keys, shows execution plan:
  // Per package: "cached" or "will run (N commands) — reason"
  // Lists pre:/cmd:/post: commands + env vars for uncached packages

func printCmdGroup(label string, cmds []string)
  // Prints "label: command" for each command

// test.go — Flags: --affected, --no-cache, --dry-run
// Delegates to runPhaseCommand(cmd, args, "test")

// run.go — Flags: --affected, --no-cache, --dry-run
// Args: phase is args[0], packages are args[1:]
// Delegates to runPhaseCommand(cmd, args[1:], args[0])
```

### Graph (`graph.go`)

```go
func runGraph(cmd, args) error        // Loads workspace, sorts graph, prints ASCII levels
func buildGraph(ws) *graph.Graph      // Constructs Graph from workspace packages
func contains(list []string, item string) bool
```

### Status (`status.go`)

```go
func runStatus(cmd, args) error
  // Dashboard: packages (name+version+deps+phases+runtime), tracked sources,
  // environment status, recent builds (last 5 from metrics.json), AI agent
```

### Affected (`affected.go`)

```go
func runAffected(cmd, args) error
  // Git diff → mapFilesToPackages → TransitiveDependents → print direct + downstream

func gitChangedFiles(wsRoot, since string) ([]string, error)
  // `git diff --name-only <since>`, falls back to working tree diff

func mapFilesToPackages(ws, files []string) map[string]bool
  // Maps file paths to packages by checking filepath.Rel against each package dir
```

### Env (`env.go`)

```go
func runEnvSetup(cmd, args) error   // Runs runtime.setup commands with {{env_dir}} substitution
func runEnvClean(cmd, args) error   // RemoveAll .takumi/envs/<pkg>
func runEnvList(cmd, args) error    // Shows ready/not-set-up status per runtime package
```

### Sync (`sync.go`)

```go
func runSync(cmd, args) error       // For each tracked source: clone if missing, pull if present
func joinParts(parts []string) string  // "a, b, c" or "nothing to do"
```

### Checkout (`checkout.go`)

```go
func runCheckout(cmd, args) error
  // git clone → scan for packages → register in workspace sources → save config

func repoNameFromURL(url string) string     // "https://...org/my-repo.git" → "my-repo"
func detectGitBranch(dir string) string     // git rev-parse --abbrev-ref HEAD, default "main"
```

### Remove (`remove.go`)

```go
func runRemove(cmd, args) error
  // Remove from sources → clean env dir → optionally delete from disk → save config
```

### Validate (`validate.go`)

```go
func runValidate(cmd, args) error
  // 1. ValidateWorkspace(cfg)
  // 2. ValidatePackage(cfg) for each package
  // 3. ValidateVersionSet if configured (checks file existence + parse + validate)
  // 4. Cross-validate: unresolved dependency warnings
  // 5. Cross-validate: cycle detection via graph.Sort()
  // Returns error if any errors found

func printFindings(label string, findings []Finding, errors, warnings int) (int, int)
  // Prints styled findings with check/cross/warn icons. Returns updated counts.
```

### Version Set (`versionset.go`)

```go
func runVersionSetCheck(cmd, args) error
  // Loads version-set file, prints strategy + all pinned deps (sorted)
```

### AI (`ai.go`)

```go
// Commands: ai context, ai diagnose <pkg>, ai review, ai optimize, ai onboard
// Commands: ai skill list, ai skill show <name>, ai skill run <name>

func runAIContext(cmd, args) error    // Regenerate .takumi/TAKUMI.md + agent config
func runAIDiagnose(cmd, args) error   // Load log → render diagnose skill with context vars
func runAIReview(cmd, args) error     // Git diff → render review skill
func runAIOptimize(cmd, args) error   // Metrics + graph → render optimize skill
func runAIOnboard(cmd, args) error    // All configs + graph → render onboard skill
func runAISkillList(cmd, args) error  // List all skills with source labels
func runAISkillShow(cmd, args) error  // Print skill template + metadata
func runAISkillRun(cmd, args) error   // Render + print skill (delegates known skills)

func loadAllSkills() ([]LoadedSkill, error)  // Currently just LoadBuiltins()
func findSkill(name string) *LoadedSkill     // Linear search through all skills
func envStatus(ws, pkgName string) string    // "no runtime defined" | "not set up" | "ready (path)"
func gitDiffOutput(wsRoot string) string     // git diff output or "(git diff unavailable)"
```

### Docs (`docs.go`)

```go
func runDocsGenerate(cmd, args) error
  // Generates 4 markdown files in docs/user/:
  //   commands.md — from Cobra command tree
  //   skills-reference.md — from built-in skills
  //   config-reference.md — annotated YAML schemas
  //   packages.md — table from workspace scan
  // Optional --ai flag runs doc-writer skill

func writeCommandDocs(buf *strings.Builder, cmd *cobra.Command, prefix string)
  // Recursive Cobra tree walker, writes markdown for each runnable command

func runDocsHookInstall(cmd, args) error  // Writes .git/hooks/pre-commit
func runDocsHookRemove(cmd, args) error   // Removes pre-commit hook
```

---

## Entry Point

```go
// cmd/takumi/main.go
package main

import "github.com/tfitz/takumi/src/cli"

func main() {
    cli.Execute()
}
```

---

## Dependencies (go.mod)

| Dependency | Version | Purpose |
|------------|---------|---------|
| `cobra` | v1.10.2 | CLI command framework |
| `pflag` | v1.0.9 | CLI flag parsing (cobra dep) |
| `yaml.v3` | v3.0.1 | YAML config parsing |
| `charmbracelet/lipgloss` | v1.1.0 | Terminal styling |
| `charmbracelet/log` | v1.0.0 | Styled logging |
| `charmbracelet/huh` | v1.0.0 | Interactive prompts (agent selection) |
| `mark3labs/mcp-go` | v0.47.1 | MCP server SDK (stdio transport) |
| `testify` | v1.11.1 | Test assertions (dev only) |

All dependencies are MIT or compatible licensed.

---

## Test Coverage Summary

| Package | Coverage | Notes |
|---------|----------|-------|
| `cache` | 95.5% | |
| `cli` | 94.8% | ~130 tests in coverage_test.go |
| `config` | 98.6% | |
| `executor` | 95.4% | 23 tests |
| `graph` | 100% | |
| `mcp` | 96.7% | 57+ unit tests + 3 E2E simulation tests |
| `skills` | 100% | |
| `ui` | 100% | |
| `workspace` | 96.2% | |
| **Total** | **~97%** | Remaining gaps are OS-level unreachable error paths |

---

## File Count

- **Source files:** 32 `.go` files (non-test)
- **Test files:** 21 `_test.go` files
- **Skill YAMLs:** 6 embedded templates
- **Total Go LoC:** ~4,900 (source) + ~10,000 (tests)
