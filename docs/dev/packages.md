# Package Reference

Go package API reference for Takumi's internal libraries. Import path: `github.com/tfitz/takumi/src/<package>`.

## config

Parses and validates the three YAML config files.

### Types

```go
// WorkspaceConfig represents takumi.yaml
type WorkspaceConfig struct {
    Workspace Workspace
}

type Workspace struct {
    Name       string
    Ignore     []string
    Sources    map[string]Source
    VersionSet VersionSetRef
    Settings   WorkspaceSettings
    AI         WorkspaceAIRef
}

type Source struct {
    URL    string
    Branch string
    Path   string
}

type WorkspaceSettings struct {
    Parallel bool  // default: true
}

type WorkspaceAIRef struct {
    Agent        string  // claude, cursor, copilot, windsurf, cline, none
    Instructions string  // path to takumi-ai.yaml
}
```

```go
// PackageConfig represents takumi-pkg.yaml
type PackageConfig struct {
    Package      PackageMeta
    Dependencies []string
    Runtime      *Runtime
    Phases       map[string]*Phase
    AI           *PackageAI
}

type PackageMeta struct {
    Name    string
    Version string
}

type Phase struct {
    Pre      []string
    Commands []string
    Post     []string
}

type Runtime struct {
    Setup []string
    Env   map[string]string
}

type PackageAI struct {
    Description string
    Notes       []string
    Tasks       map[string]AITask
}

type AITask struct {
    Description string
    Steps       []string
}
```

```go
// VersionSetConfig represents takumi-versions.yaml
type VersionSetConfig struct {
    VersionSet VersionSet
}

type VersionSet struct {
    Name     string
    Strategy string            // strict, prefer-latest, prefer-pinned
    Packages map[string]string // dependency → version
}
```

### Functions

```go
// LoadWorkspace reads and parses takumi.yaml from the given directory.
func LoadWorkspace(dir string) (*WorkspaceConfig, error)

// LoadPackage reads and parses takumi-pkg.yaml from the given directory.
func LoadPackage(dir string) (*PackageConfig, error)

// LoadVersionSet reads and parses the version set file.
func LoadVersionSet(path string) (*VersionSetConfig, error)

// Validate checks a workspace config for errors and warnings.
func (w *WorkspaceConfig) Validate() []ValidationResult

// Validate checks a package config for errors and warnings.
func (p *PackageConfig) Validate() []ValidationResult
```

## workspace

Detects and loads the workspace.

### Types

```go
type Info struct {
    Root     string                      // absolute path to workspace root
    Config   *config.WorkspaceConfig
    Packages map[string]*DiscoveredPkg   // name → package
}

type DiscoveredPkg struct {
    Name   string
    Dir    string                        // absolute path to package directory
    Config *config.PackageConfig
}
```

### Functions

```go
// Detect walks up from cwd looking for .takumi/ marker.
// Returns the workspace root path or an error.
func Detect() (string, error)

// Load detects the workspace and loads all configs + discovered packages.
func Load(cwd string) (*Info, error)
```

### Constants

```go
const (
    MarkerDir   = ".takumi"
    ConfigFile  = "takumi.yaml"
    PackageFile = "takumi-pkg.yaml"
    AIFile      = "takumi-ai.yaml"
)
```

## graph

Dependency DAG with topological sort.

### Types

```go
type Graph struct {
    // unexported: nodes map[string][]string
}

type Level struct {
    Index    int
    Packages []string
}
```

### Functions

```go
// New creates an empty graph.
func New() *Graph

// AddNode registers a package and its dependencies.
func (g *Graph) AddNode(name string, deps []string)

// Sort performs topological sort using Kahn's algorithm.
// Returns levels (groups of independent packages) or an error if cycles exist.
func (g *Graph) Sort() ([]Level, error)

// Flatten returns a flat build order (level 0 first, then level 1, etc.).
func (g *Graph) Flatten() ([]string, error)

// TransitiveDependents returns all packages that transitively depend on the given package.
func (g *Graph) TransitiveDependents(name string) []string
```

## cache

Content-addressed build cache.

### Types

```go
type Entry struct {
    Key        string  // SHA-256 hex digest
    Package    string
    Phase      string
    Timestamp  string
    DurationMs int64
    FileCount  int
}

type Store struct {
    Dir string  // .takumi/cache/
}
```

### Functions

```go
// NewStore creates a cache store at the given directory.
func NewStore(dir string) *Store

// ComputeKey hashes phase name + config + source files + dependency keys.
func (s *Store) ComputeKey(pkg, phase string, configHash string, sourceHashes []string, depKeys []string) string

// Lookup checks if a valid cache entry exists for the given key.
func (s *Store) Lookup(pkg, phase, key string) (*Entry, bool)

// Store writes a cache entry after a successful build.
func (s *Store) Store(entry *Entry) error

// Clear removes all cache entries.
func (s *Store) Clear() error
```

## executor

Phase execution with parallelism and logging.

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
    Packages []string
    Parallel bool
    NoCache  bool
}
```

### Functions

```go
// Run executes a phase for the given packages in dependency order.
func Run(ws *workspace.Info, opts RunOptions) ([]Result, error)

// RecordMetrics appends build results to .takumi/metrics.json.
func RecordMetrics(metricsFile string, results []Result) error
```

## skills

AI skill template loading and rendering.

### Types

```go
type Source int

const (
    SourceBuiltin   Source = iota  // embedded in binary
    SourceWorkspace                // .takumi/skills/
    SourcePackage                  // package-level ai.tasks
)

type Skill struct {
    Name         string
    Description  string
    AutoContext  []string
    Prompt       string
    OutputFormat string
    MaxTokens    int
}

type LoadedSkill struct {
    Skill
    Source Source
    Path   string
}
```

### Functions

```go
// LoadBuiltins reads all embedded YAML skill templates.
func LoadBuiltins() ([]LoadedSkill, error)

// LoadFromDir reads YAML skill files from a directory.
func LoadFromDir(dir string, source Source) ([]LoadedSkill, error)

// Render replaces {{key}} placeholders in the prompt with values from vars.
func Render(prompt string, vars map[string]string) string
```
