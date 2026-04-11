// Package executor runs build phases for packages in dependency order,
// with support for parallel execution within dependency levels.
package executor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tfitz/takumi/src/cache"
	"github.com/tfitz/takumi/src/graph"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

// Result captures the outcome of running a phase for a package.
type Result struct {
	Package  string
	Phase    string
	ExitCode int
	Duration time.Duration
	Error    error
	LogFile  string
	CacheHit bool // true if this result was served from cache
}

// RunOptions controls execution behavior.
type RunOptions struct {
	Phase    string   // phase to run (build, test, or custom)
	Packages []string // specific packages (nil = all)
	Parallel bool     // run levels in parallel
	NoCache  bool     // skip cache lookup and force execution
}

// Run executes a phase across workspace packages in dependency order.
// When caching is enabled (the default), packages with unchanged source
// files, config, and dependencies are skipped automatically.
func Run(ws *workspace.Info, opts RunOptions) ([]Result, error) {
	// Build graph
	g := graph.New()
	for name, pkg := range ws.Packages {
		g.AddNode(name, pkg.Config.Dependencies)
	}

	levels, err := g.Sort()
	if err != nil {
		return nil, err
	}

	// Filter to requested packages if specified
	targetSet := make(map[string]bool)
	if len(opts.Packages) > 0 {
		for _, p := range opts.Packages {
			targetSet[p] = true
		}
	}

	store := cache.NewStore(ws.Root)
	cacheKeys := make(map[string]string) // pkgName → cache key (populated per level)

	var allResults []Result

	for _, level := range levels {
		// Filter packages in this level
		var levelPkgs []string
		for _, name := range level.Packages {
			if len(targetSet) > 0 && !targetSet[name] {
				continue
			}
			if _, exists := ws.Packages[name]; !exists {
				continue
			}
			levelPkgs = append(levelPkgs, name)
		}

		if len(levelPkgs) == 0 {
			continue
		}

		if opts.Parallel && len(levelPkgs) > 1 {
			results := runParallelCached(ws, levelPkgs, opts, store, cacheKeys)
			allResults = append(allResults, results...)
		} else {
			for _, name := range levelPkgs {
				result := runCached(ws, name, opts, store, cacheKeys)
				allResults = append(allResults, result)
			}
		}

		// Check for failures — stop if any package in this level failed
		for _, r := range allResults {
			if r.Error != nil || r.ExitCode != 0 {
				return allResults, fmt.Errorf("phase %q failed for package %q", opts.Phase, r.Package)
			}
		}
	}

	return allResults, nil
}

// runCached checks the cache before running a phase. On cache hit the phase
// is skipped; on miss the phase runs and a successful result is cached.
func runCached(ws *workspace.Info, pkgName string, opts RunOptions, store *cache.Store, cacheKeys map[string]string) Result {
	pkg := ws.Packages[pkgName]

	// Skip cache for packages that don't define this phase
	if pkg.Config.Phases[opts.Phase] == nil {
		return runPhase(ws, pkgName, opts.Phase)
	}

	// Build dependency keys from earlier levels (already populated)
	depKeys := make(map[string]string)
	for _, dep := range pkg.Config.Dependencies {
		if key, ok := cacheKeys[dep]; ok {
			depKeys[dep] = key
		}
	}

	configPath := filepath.Join(pkg.Dir, workspace.PackageFile)
	key, fileCount, err := cache.ComputeKey(pkg.Dir, configPath, opts.Phase, depKeys, ws.Config.Workspace.Ignore)
	if err != nil {
		// Can't compute key — run without caching
		return runPhase(ws, pkgName, opts.Phase)
	}
	cacheKeys[pkgName] = key

	// Check cache
	if !opts.NoCache {
		if entry := store.Lookup(pkgName, opts.Phase); entry != nil && entry.Key == key {
			return Result{
				Package:  pkgName,
				Phase:    opts.Phase,
				CacheHit: true,
			}
		}
	}

	// Cache miss — run the phase
	result := runPhase(ws, pkgName, opts.Phase)

	// On success, write cache entry
	if result.Error == nil && result.ExitCode == 0 {
		store.Write(&cache.Entry{
			Key:        key,
			Package:    pkgName,
			Phase:      opts.Phase,
			Timestamp:  time.Now().Format(time.RFC3339),
			DurationMs: result.Duration.Milliseconds(),
			FileCount:  fileCount,
		})
	}

	return result
}

// runParallelCached runs cache-aware execution for a set of packages concurrently.
// All packages in a level read from cacheKeys (populated by earlier levels)
// and their own keys are merged back after all goroutines complete.
func runParallelCached(ws *workspace.Info, packages []string, opts RunOptions, store *cache.Store, cacheKeys map[string]string) []Result {
	type resultWithKey struct {
		result Result
		key    string
	}
	entries := make([]resultWithKey, len(packages))
	var wg sync.WaitGroup

	// Snapshot cacheKeys for safe concurrent reads (earlier levels only)
	snapshot := make(map[string]string, len(cacheKeys))
	for k, v := range cacheKeys {
		snapshot[k] = v
	}

	for i, name := range packages {
		wg.Add(1)
		go func(idx int, pkgName string) {
			defer wg.Done()
			// Use a local copy that won't write to shared map
			localKeys := make(map[string]string, len(snapshot))
			for k, v := range snapshot {
				localKeys[k] = v
			}
			entries[idx] = resultWithKey{
				result: runCachedLocal(ws, pkgName, opts, store, localKeys),
				key:    localKeys[pkgName],
			}
		}(i, name)
	}
	wg.Wait()

	// Merge keys back into shared map
	results := make([]Result, len(packages))
	for i, e := range entries {
		results[i] = e.result
		if e.key != "" {
			cacheKeys[packages[i]] = e.key
		}
	}
	return results
}

// runCachedLocal is the same as runCached but writes to a local key map
// instead of the shared one, making it safe for concurrent use.
func runCachedLocal(ws *workspace.Info, pkgName string, opts RunOptions, store *cache.Store, localKeys map[string]string) Result {
	pkg := ws.Packages[pkgName]

	if pkg.Config.Phases[opts.Phase] == nil {
		return runPhase(ws, pkgName, opts.Phase)
	}

	depKeys := make(map[string]string)
	for _, dep := range pkg.Config.Dependencies {
		if key, ok := localKeys[dep]; ok {
			depKeys[dep] = key
		}
	}

	configPath := filepath.Join(pkg.Dir, workspace.PackageFile)
	key, fileCount, err := cache.ComputeKey(pkg.Dir, configPath, opts.Phase, depKeys, ws.Config.Workspace.Ignore)
	if err != nil {
		return runPhase(ws, pkgName, opts.Phase)
	}
	localKeys[pkgName] = key

	if !opts.NoCache {
		if entry := store.Lookup(pkgName, opts.Phase); entry != nil && entry.Key == key {
			return Result{
				Package:  pkgName,
				Phase:    opts.Phase,
				CacheHit: true,
			}
		}
	}

	result := runPhase(ws, pkgName, opts.Phase)

	if result.Error == nil && result.ExitCode == 0 {
		store.Write(&cache.Entry{
			Key:        key,
			Package:    pkgName,
			Phase:      opts.Phase,
			Timestamp:  time.Now().Format(time.RFC3339),
			DurationMs: result.Duration.Milliseconds(),
			FileCount:  fileCount,
		})
	}

	return result
}

// runPhase executes all commands for a single phase of a single package.
func runPhase(ws *workspace.Info, pkgName, phase string) Result {
	pkg := ws.Packages[pkgName]
	start := time.Now()

	result := Result{
		Package: pkgName,
		Phase:   phase,
	}

	phaseConfig := pkg.Config.Phases[phase]
	if phaseConfig == nil {
		// No phase defined — skip silently
		result.Duration = time.Since(start)
		return result
	}

	// Prepare log file
	logDir := filepath.Join(ws.Root, workspace.MarkerDir, "logs")
	os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.%s.log", pkgName, phase))
	logFile, err := os.Create(logPath)
	if err != nil {
		result.Error = fmt.Errorf("creating log file: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	defer logFile.Close()
	result.LogFile = logPath

	// Write header to log
	fmt.Fprintf(logFile, "# takumi %s %s\n", phase, pkgName)
	fmt.Fprintf(logFile, "# started: %s\n", start.Format(time.RFC3339))
	fmt.Fprintf(logFile, "# cwd: %s\n\n", pkg.Dir)

	// Build env vars
	env := os.Environ()
	if pkg.Config.Runtime != nil {
		envDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs", pkgName)
		for k, v := range pkg.Config.Runtime.Env {
			// Substitute {{env_dir}}
			v = strings.ReplaceAll(v, "{{env_dir}}", envDir)
			env = append(env, k+"="+v)
		}
	}

	// Run commands: pre → commands → post
	allCmds := make([]string, 0)
	allCmds = append(allCmds, phaseConfig.Pre...)
	allCmds = append(allCmds, phaseConfig.Commands...)
	allCmds = append(allCmds, phaseConfig.Post...)

	for _, cmdStr := range allCmds {
		fmt.Fprintf(logFile, "$ %s\n", cmdStr)

		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Dir = pkg.Dir
		cmd.Env = env

		// Tee stdout/stderr to both log file and prefixed terminal output
		prefix := ui.Muted.Render(fmt.Sprintf("[%s] ", pkgName))
		cmd.Stdout = io.MultiWriter(logFile, &prefixWriter{prefix: prefix, w: os.Stdout})
		cmd.Stderr = io.MultiWriter(logFile, &prefixWriter{prefix: prefix, w: os.Stderr})

		if err := cmd.Run(); err != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
			result.Error = err
			result.Duration = time.Since(start)

			fmt.Fprintf(logFile, "\n# exit code: %d\n", result.ExitCode)
			fmt.Fprintf(logFile, "# duration: %s\n", result.Duration)
			return result
		}
		fmt.Fprintln(logFile)
	}

	result.Duration = time.Since(start)
	fmt.Fprintf(logFile, "# exit code: 0\n")
	fmt.Fprintf(logFile, "# duration: %s\n", result.Duration)
	return result
}

// prefixWriter prepends a prefix to each line written.
type prefixWriter struct {
	prefix string
	w      io.Writer
	atBOL  bool // at beginning of line
}

func (pw *prefixWriter) Write(p []byte) (n int, err error) {
	for _, b := range p {
		if pw.atBOL || !pw.atBOL && b != '\n' {
			if pw.atBOL {
				_, err = fmt.Fprint(pw.w, pw.prefix)
				if err != nil {
					return n, err
				}
			}
			pw.atBOL = false
		}
		_, err = pw.w.Write([]byte{b})
		if err != nil {
			return n, err
		}
		n++
		if b == '\n' {
			pw.atBOL = true
		}
	}
	return n, nil
}

// MetricsEntry is a single build telemetry record.
type MetricsEntry struct {
	Timestamp  string `json:"timestamp"`
	Phase      string `json:"phase"`
	Package    string `json:"package"`
	DurationMs int64  `json:"duration_ms"`
	ExitCode   int    `json:"exit_code"`
}

// MetricsFile is the top-level .takumi/metrics.json structure.
type MetricsFile struct {
	Runs []MetricsEntry `json:"runs"`
}

// RecordMetrics appends results to .takumi/metrics.json.
func RecordMetrics(wsRoot string, results []Result) error {
	metricsPath := filepath.Join(wsRoot, workspace.MarkerDir, "metrics.json")

	var metrics MetricsFile
	if data, err := os.ReadFile(metricsPath); err == nil {
		if err := json.Unmarshal(data, &metrics); err != nil {
			metrics = MetricsFile{} // reset on corrupt data
		}
	}

	for _, r := range results {
		metrics.Runs = append(metrics.Runs, MetricsEntry{
			Timestamp:  time.Now().Format(time.RFC3339),
			Phase:      r.Phase,
			Package:    r.Package,
			DurationMs: r.Duration.Milliseconds(),
			ExitCode:   r.ExitCode,
		})
	}

	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metricsPath, data, 0644)
}
