# Package Reference

Auto-generated from Go source via `takumi docs generate`.

Import path: `github.com/tfitz/takumi/src/<package>`.

## agent

**Files:** anthropic.go, config.go, http.go, openai.go, runner.go

```
Package agent provides a multi-turn LLM agent loop with tool calling. Supports
Anthropic and OpenAI providers.
type Config struct{ ... }
type ProviderConfig struct{ ... }
    func DetectProvider(providerName, modelOverride string) (*ProviderConfig, error)
type Result struct{ ... }
    func Run(ctx context.Context, pc *ProviderConfig, cfg *Config, userMessage string) (*Result, error)
type Tool struct{ ... }
```

---

## cache

**Files:** cache.go

```
Package cache provides content-addressed build caching for Takumi. Cache keys
are SHA-256 digests of source files, config, phase name, and dependency cache
keys. Keys are computed in topological order so that dependency changes cascade
automatically.
func ComputeKey(pkgDir, configPath, phase string, depKeys map[string]string, ignore []string) (string, int, error)
type Entry struct{ ... }
type Store struct{ ... }
    func NewStore(wsRoot string) *Store
```

---

## config

**Files:** package.go, validate.go, versionset.go, workspace.go

```
Package config handles parsing and validation of Takumi configuration files.
func SaveWorkspaceConfig(path string, cfg *WorkspaceConfig) error
type AITask struct{ ... }
type Finding struct{ ... }
    func ValidatePackage(cfg *PackageConfig) []Finding
    func ValidateVersionSet(cfg *VersionSetConfig) []Finding
    func ValidateWorkspace(cfg *WorkspaceConfig) []Finding
type PackageAI struct{ ... }
type PackageConfig struct{ ... }
    func DefaultPackageConfig(name string) *PackageConfig
    func LoadPackageConfig(path string) (*PackageConfig, error)
type PackageMeta struct{ ... }
type Phase struct{ ... }
type Runtime struct{ ... }
type Severity int
    const SeverityError Severity = iota ...
type Source struct{ ... }
type VersionSet struct{ ... }
type VersionSetConfig struct{ ... }
    func LoadVersionSetConfig(path string) (*VersionSetConfig, error)
type VersionSetRef struct{ ... }
type Workspace struct{ ... }
type WorkspaceAIRef struct{ ... }
type WorkspaceConfig struct{ ... }
    func DefaultWorkspaceConfig(name string) *WorkspaceConfig
    func LoadWorkspaceConfig(path string) (*WorkspaceConfig, error)
type WorkspaceSettings struct{ ... }
```

---

## executor

**Files:** executor.go

```
Package executor runs build phases for packages in dependency order, with
support for parallel execution within dependency levels.
func RecordMetrics(wsRoot string, results []Result) error
type MetricsEntry struct{ ... }
type MetricsFile struct{ ... }
type Result struct{ ... }
    func Run(ws *workspace.Info, opts RunOptions) ([]Result, error)
type RunOptions struct{ ... }
```

---

## graph

**Files:** graph.go

```
Package graph provides a dependency DAG with topological sort, cycle detection,
and parallel execution levels using Kahn's algorithm.
type Graph struct{ ... }
    func New() *Graph
type Level struct{ ... }
```

---

## mcp

**Files:** server.go, tools.go

```
Package mcp provides a Model Context Protocol server that exposes takumi
workspace operations as tools for AI agents.
func NewServer() *server.MCPServer
```

---

## skills

**Files:** skills.go

```
Package skills loads, renders, and manages AI skill templates.
func Render(prompt string, vars map[string]string) string
type LoadedSkill struct{ ... }
    func LoadBuiltins() ([]LoadedSkill, error)
    func LoadFromDir(dir string, source Source) ([]LoadedSkill, error)
type Skill struct{ ... }
type SkillFile struct{ ... }
type Source int
    const SourceBuiltin Source = iota ...
```

---

## ui

**Files:** logger.go, styles.go

```
Package ui provides styled terminal output for Takumi CLI commands.
var ColorPrimary = lipgloss.Color("#E8A87C") ...
var Bold = lipgloss.NewStyle().Bold(true).Foreground(ColorBright) ...
var Banner = ...(1) ...
var Log *log.Logger
func Bullet(msg string) string
func Check(msg string) string
func Command(cmd string) string
func Cross(msg string) string
func Divider() string
func FilePath(path string) string
func FormatCount(n int, singular, plural string) string
func Header() string
func SetVerbose(on bool)
func StepDone(msg string) string
func StepInfo(msg string) string
func Summary(title, body string) string
func Warn(msg string) string
```

---

## workspace

**Files:** git.go, workspace.go

```
Package workspace handles workspace detection and package discovery.
const MarkerDir = ".takumi" ...
func ChangedFiles(wsRoot, since string) ([]string, error)
func Detect(startDir string) string
func MapFilesToPackages(ws *Info, files []string) map[string]bool
type DiscoveredPkg struct{ ... }
type Info struct{ ... }
    func Load(startDir string) (*Info, error)
type ScanError struct{ ... }
    func ScanPackages(root string, ignore []string) (map[string]*DiscoveredPkg, []ScanError, error)
```

---

