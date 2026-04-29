package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/workspace"
)

// setupTestWorkspace creates a temporary workspace with the given packages.
// It creates the .takumi/ marker directory, writes takumi.yaml, and for each
// package creates a directory with a takumi-pkg.yaml plus a dummy source file
// (so that cache key computation has something to hash).
func setupTestWorkspace(t *testing.T, packages map[string]*config.PackageConfig) *workspace.Info {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, workspace.MarkerDir), 0755)

	wsCfg := config.DefaultWorkspaceConfig("test")
	data, err := wsCfg.Marshal()
	require.NoError(t, err)
	os.WriteFile(filepath.Join(root, "takumi.yaml"), data, 0644)

	pkgs := make(map[string]*workspace.DiscoveredPkg)
	for name, cfg := range packages {
		pkgDir := filepath.Join(root, name)
		os.MkdirAll(pkgDir, 0755)
		cfgData, err := cfg.Marshal()
		require.NoError(t, err)
		os.WriteFile(filepath.Join(pkgDir, workspace.PackageFile), cfgData, 0644)
		// Write a dummy source file so cache key computation works.
		os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte("package main\n"), 0644)
		pkgs[name] = &workspace.DiscoveredPkg{Name: name, Dir: pkgDir, Config: cfg}
	}

	return &workspace.Info{Root: root, Config: wsCfg, Packages: pkgs}
}

// ---------------------------------------------------------------------------
// 1. TestRun_SinglePackage
// ---------------------------------------------------------------------------

func TestRun_SinglePackage(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"alpha": {
			Package: config.PackageMeta{Name: "alpha", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo hello"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "alpha", results[0].Package)
	assert.Equal(t, "build", results[0].Phase)
	assert.Equal(t, 0, results[0].ExitCode)
	assert.Nil(t, results[0].Error)
	assert.False(t, results[0].CacheHit)
}

// ---------------------------------------------------------------------------
// 2. TestRun_DependencyOrder
// ---------------------------------------------------------------------------

func TestRun_DependencyOrder(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"base": {
			Package: config.PackageMeta{Name: "base", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{
					fmt.Sprintf("echo base > %s", filepath.Join(t.TempDir(), "order.txt")),
				}},
			},
		},
		"app": {
			Package:      config.PackageMeta{Name: "app", Version: "1.0.0"},
			Dependencies: []string{"base"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo app"}},
			},
		},
	})

	// Use file timestamps to prove ordering: base writes a stamp file, app
	// writes a later one. We only need to verify the result array order,
	// which reflects level order.
	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 2)

	// base should come before app in results (it is at level 0, app at level 1)
	assert.Equal(t, "base", results[0].Package)
	assert.Equal(t, "app", results[1].Package)
}

// ---------------------------------------------------------------------------
// 3. TestRun_ParallelExecution
// ---------------------------------------------------------------------------

func TestRun_ParallelExecution(t *testing.T) {
	// Three independent packages — all at the same level.
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"svc-a": {
			Package: config.PackageMeta{Name: "svc-a", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo a"}},
			},
		},
		"svc-b": {
			Package: config.PackageMeta{Name: "svc-b", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo b"}},
			},
		},
		"svc-c": {
			Package: config.PackageMeta{Name: "svc-c", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo c"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// All should succeed.
	for _, r := range results {
		assert.Equal(t, 0, r.ExitCode, "package %s should succeed", r.Package)
		assert.Nil(t, r.Error)
	}
}

// ---------------------------------------------------------------------------
// 4. TestRun_FailureStopsExecution
// ---------------------------------------------------------------------------

func TestRun_FailureStopsExecution(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"base": {
			Package: config.PackageMeta{Name: "base", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"exit 1"}},
			},
		},
		"downstream": {
			Package:      config.PackageMeta{Name: "downstream", Version: "1.0.0"},
			Dependencies: []string{"base"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo should-not-run"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed")

	// Only base should have run (and failed).
	require.Len(t, results, 1)
	assert.Equal(t, "base", results[0].Package)
	assert.NotEqual(t, 0, results[0].ExitCode)
}

// ---------------------------------------------------------------------------
// 5. TestRun_NoPhase
// ---------------------------------------------------------------------------

func TestRun_NoPhase(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"nophase": {
			Package: config.PackageMeta{Name: "nophase", Version: "1.0.0"},
			// No phases defined at all.
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Should succeed silently — no error, exit code 0, no log file.
	assert.Equal(t, 0, results[0].ExitCode)
	assert.Nil(t, results[0].Error)
	assert.Empty(t, results[0].LogFile)
}

// ---------------------------------------------------------------------------
// 6. TestRun_TargetPackages
// ---------------------------------------------------------------------------

func TestRun_TargetPackages(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"included": {
			Package: config.PackageMeta{Name: "included", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo yes"}},
			},
		},
		"excluded": {
			Package: config.PackageMeta{Name: "excluded", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo no"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{
		Phase:    "build",
		Packages: []string{"included"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "included", results[0].Package)
}

// ---------------------------------------------------------------------------
// 7. TestRun_PreAndPostCommands
// ---------------------------------------------------------------------------

func TestRun_PreAndPostCommands(t *testing.T) {
	stampDir := t.TempDir()
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"ordered": {
			Package: config.PackageMeta{Name: "ordered", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {
					Pre:      []string{fmt.Sprintf("echo pre > %s/pre.txt", stampDir)},
					Commands: []string{fmt.Sprintf("echo cmd > %s/cmd.txt", stampDir)},
					Post:     []string{fmt.Sprintf("echo post > %s/post.txt", stampDir)},
				},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].ExitCode)

	// All three stamp files should exist.
	for _, name := range []string{"pre.txt", "cmd.txt", "post.txt"} {
		_, err := os.Stat(filepath.Join(stampDir, name))
		assert.NoError(t, err, "%s should have been created", name)
	}

	// Verify order via the log file: pre, cmd, post.
	logData, err := os.ReadFile(results[0].LogFile)
	require.NoError(t, err)
	log := string(logData)

	preIdx := strings.Index(log, "echo pre")
	cmdIdx := strings.Index(log, "echo cmd")
	postIdx := strings.Index(log, "echo post")
	assert.Greater(t, cmdIdx, preIdx, "cmd should run after pre")
	assert.Greater(t, postIdx, cmdIdx, "post should run after cmd")
}

// TestRun_PreFails_SkipsCommandsAndPost asserts that when a Pre command fails,
// the Commands and Post stages are not executed at all.
func TestRun_PreFails_SkipsCommandsAndPost(t *testing.T) {
	stampDir := t.TempDir()
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"prefails": {
			Package: config.PackageMeta{Name: "prefails", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {
					Pre:      []string{"exit 7"},
					Commands: []string{fmt.Sprintf("touch %s/cmd.txt", stampDir)},
					Post:     []string{fmt.Sprintf("touch %s/post.txt", stampDir)},
				},
			},
		},
	})

	results, _ := Run(ws, RunOptions{Phase: "build"})
	require.Len(t, results, 1)
	assert.Equal(t, 7, results[0].ExitCode, "exit code must come from the failing Pre command")

	_, cmdErr := os.Stat(filepath.Join(stampDir, "cmd.txt"))
	assert.True(t, os.IsNotExist(cmdErr), "Commands stage must not run when Pre fails")
	_, postErr := os.Stat(filepath.Join(stampDir, "post.txt"))
	assert.True(t, os.IsNotExist(postErr), "Post stage must not run when Pre fails")
}

// TestRun_PostFails_ReturnsPostExitCode asserts that when Commands succeed but
// Post fails, the result reflects the Post failure (not a silent success).
func TestRun_PostFails_ReturnsPostExitCode(t *testing.T) {
	stampDir := t.TempDir()
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"postfails": {
			Package: config.PackageMeta{Name: "postfails", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {
					Commands: []string{fmt.Sprintf("touch %s/cmd.txt", stampDir)},
					Post:     []string{"exit 9"},
				},
			},
		},
	})

	results, _ := Run(ws, RunOptions{Phase: "build"})
	require.Len(t, results, 1)
	assert.Equal(t, 9, results[0].ExitCode, "exit code must come from the failing Post command")

	_, cmdErr := os.Stat(filepath.Join(stampDir, "cmd.txt"))
	assert.NoError(t, cmdErr, "Commands stage must have run before Post failed")
}

// ---------------------------------------------------------------------------
// 8. TestRun_RuntimeEnvInjection
// ---------------------------------------------------------------------------

func TestRun_RuntimeEnvInjection(t *testing.T) {
	stampDir := t.TempDir()
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"envpkg": {
			Package: config.PackageMeta{Name: "envpkg", Version: "1.0.0"},
			Runtime: &config.Runtime{
				Env: map[string]string{
					"MY_CUSTOM_VAR": "takumi_test_value",
				},
			},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{
					fmt.Sprintf("echo $MY_CUSTOM_VAR > %s/env_out.txt", stampDir),
				}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].ExitCode)

	out, err := os.ReadFile(filepath.Join(stampDir, "env_out.txt"))
	require.NoError(t, err)
	assert.Equal(t, "takumi_test_value", strings.TrimSpace(string(out)))
}

// ---------------------------------------------------------------------------
// 9. TestRun_LogCapture
// ---------------------------------------------------------------------------

func TestRun_LogCapture(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"logpkg": {
			Package: config.PackageMeta{Name: "logpkg", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo log-test-output"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)

	logPath := results[0].LogFile
	assert.NotEmpty(t, logPath)
	assert.FileExists(t, logPath)

	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	logStr := string(logData)

	// Check log header
	assert.Contains(t, logStr, "# takumi build logpkg")
	assert.Contains(t, logStr, "# started:")
	assert.Contains(t, logStr, "# cwd:")

	// Check command and output are captured
	assert.Contains(t, logStr, "$ echo log-test-output")
	assert.Contains(t, logStr, "log-test-output")

	// Check footer
	assert.Contains(t, logStr, "# exit code: 0")
	assert.Contains(t, logStr, "# duration:")
}

// ---------------------------------------------------------------------------
// 10. TestRun_CacheHit
// ---------------------------------------------------------------------------

func TestRun_CacheHit(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"cached": {
			Package: config.PackageMeta{Name: "cached", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo run"}},
			},
		},
	})

	// First run — cache miss
	results1, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results1, 1)
	assert.False(t, results1[0].CacheHit, "first run should be a cache miss")

	// Second run — cache hit (nothing changed)
	results2, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results2, 1)
	assert.True(t, results2[0].CacheHit, "second run should be a cache hit")
}

// ---------------------------------------------------------------------------
// 11. TestRun_CacheMissOnChange
// ---------------------------------------------------------------------------

func TestRun_CacheMissOnChange(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"mutable": {
			Package: config.PackageMeta{Name: "mutable", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo built"}},
			},
		},
	})

	// First run — cache miss
	results1, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	assert.False(t, results1[0].CacheHit)

	// Modify a source file in the package directory
	srcFile := filepath.Join(ws.Packages["mutable"].Dir, "main.go")
	os.WriteFile(srcFile, []byte("package main\n// changed\n"), 0644)

	// Third run — should be a cache miss because the source file changed
	results2, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results2, 1)
	assert.False(t, results2[0].CacheHit, "should be cache miss after file change")
}

// ---------------------------------------------------------------------------
// 12. TestRun_NoCache
// ---------------------------------------------------------------------------

func TestRun_NoCache(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"nocache": {
			Package: config.PackageMeta{Name: "nocache", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo forced"}},
			},
		},
	})

	// First run
	results1, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	assert.False(t, results1[0].CacheHit)

	// Second run with NoCache — should NOT be a cache hit
	results2, err := Run(ws, RunOptions{Phase: "build", NoCache: true})
	require.NoError(t, err)
	require.Len(t, results2, 1)
	assert.False(t, results2[0].CacheHit, "NoCache should bypass cache")
}

// ---------------------------------------------------------------------------
// 13. TestRecordMetrics_WritesJSON
// ---------------------------------------------------------------------------

func TestRecordMetrics_WritesJSON(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, workspace.MarkerDir), 0755)

	results := []Result{
		{
			Package:  "pkg-a",
			Phase:    "build",
			ExitCode: 0,
			Duration: 250 * time.Millisecond,
		},
	}

	err := RecordMetrics(root, results)
	require.NoError(t, err)

	metricsPath := filepath.Join(root, workspace.MarkerDir, "metrics.json")
	assert.FileExists(t, metricsPath)

	data, err := os.ReadFile(metricsPath)
	require.NoError(t, err)

	var metrics MetricsFile
	err = json.Unmarshal(data, &metrics)
	require.NoError(t, err)
	require.Len(t, metrics.Runs, 1)

	assert.Equal(t, "pkg-a", metrics.Runs[0].Package)
	assert.Equal(t, "build", metrics.Runs[0].Phase)
	assert.Equal(t, 0, metrics.Runs[0].ExitCode)
	assert.Equal(t, int64(250), metrics.Runs[0].DurationMs)
	assert.NotEmpty(t, metrics.Runs[0].Timestamp)
}

// ---------------------------------------------------------------------------
// 14. TestRecordMetrics_AppendsToExisting
// ---------------------------------------------------------------------------

func TestRecordMetrics_AppendsToExisting(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, workspace.MarkerDir), 0755)

	// Write first batch
	err := RecordMetrics(root, []Result{
		{Package: "first", Phase: "build", Duration: 100 * time.Millisecond},
	})
	require.NoError(t, err)

	// Write second batch
	err = RecordMetrics(root, []Result{
		{Package: "second", Phase: "test", Duration: 200 * time.Millisecond},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, workspace.MarkerDir, "metrics.json"))
	require.NoError(t, err)

	var metrics MetricsFile
	err = json.Unmarshal(data, &metrics)
	require.NoError(t, err)
	require.Len(t, metrics.Runs, 2, "should have two entries after two writes")

	assert.Equal(t, "first", metrics.Runs[0].Package)
	assert.Equal(t, "second", metrics.Runs[1].Package)
}

// ---------------------------------------------------------------------------
// 15. TestRecordMetrics_HandlesCorruptFile
// ---------------------------------------------------------------------------

func TestRecordMetrics_HandlesCorruptFile(t *testing.T) {
	root := t.TempDir()
	markerDir := filepath.Join(root, workspace.MarkerDir)
	os.MkdirAll(markerDir, 0755)

	// Write corrupt JSON
	metricsPath := filepath.Join(markerDir, "metrics.json")
	os.WriteFile(metricsPath, []byte("{corrupt json!!!"), 0644)

	// RecordMetrics should reset and write fresh data
	err := RecordMetrics(root, []Result{
		{Package: "recovered", Phase: "build", Duration: 50 * time.Millisecond},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(metricsPath)
	require.NoError(t, err)

	var metrics MetricsFile
	err = json.Unmarshal(data, &metrics)
	require.NoError(t, err)
	require.Len(t, metrics.Runs, 1, "corrupt file should be reset")
	assert.Equal(t, "recovered", metrics.Runs[0].Package)
}

// ---------------------------------------------------------------------------
// 16. TestPrefixWriter
// ---------------------------------------------------------------------------

func TestPrefixWriter(t *testing.T) {
	t.Run("single line", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[test] ", w: &buf, atBOL: true}
		pw.Write([]byte("hello\n"))
		assert.Equal(t, "[test] hello\n", buf.String())
	})

	t.Run("multiple lines", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[p] ", w: &buf, atBOL: true}
		pw.Write([]byte("line1\nline2\n"))
		assert.Equal(t, "[p] line1\n[p] line2\n", buf.String())
	})

	t.Run("no trailing newline", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[x] ", w: &buf, atBOL: true}
		pw.Write([]byte("partial"))
		assert.Equal(t, "[x] partial", buf.String())
	})

	t.Run("empty input", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[e] ", w: &buf, atBOL: true}
		n, err := pw.Write([]byte{})
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Empty(t, buf.String())
	})

	t.Run("incremental writes", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[i] ", w: &buf, atBOL: true}
		pw.Write([]byte("first"))
		pw.Write([]byte(" continued\n"))
		pw.Write([]byte("second\n"))
		assert.Equal(t, "[i] first continued\n[i] second\n", buf.String())
	})

	t.Run("returns correct byte count", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[c] ", w: &buf, atBOL: true}
		data := []byte("hello\nworld\n")
		n, err := pw.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n, "Write should return number of input bytes consumed")
	})

	t.Run("only newlines", func(t *testing.T) {
		var buf bytes.Buffer
		pw := &prefixWriter{prefix: "[n] ", w: &buf, atBOL: true}
		pw.Write([]byte("\n\n"))
		// First byte is newline with atBOL=true, so prefix is written then newline.
		// Second byte is newline with atBOL=true again.
		assert.Equal(t, "[n] \n[n] \n", buf.String())
	})
}

// ---------------------------------------------------------------------------
// Additional edge-case tests
// ---------------------------------------------------------------------------

func TestRun_DependencyOrder_ParallelLevels(t *testing.T) {
	// Verify that with parallel enabled, independent packages at the same
	// level all succeed, and the dependent package at level 1 runs after.
	stampDir := t.TempDir()

	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"lib-a": {
			Package: config.PackageMeta{Name: "lib-a", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{
					fmt.Sprintf("echo a > %s/a.txt", stampDir),
				}},
			},
		},
		"lib-b": {
			Package: config.PackageMeta{Name: "lib-b", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{
					fmt.Sprintf("echo b > %s/b.txt", stampDir),
				}},
			},
		},
		"app": {
			Package:      config.PackageMeta{Name: "app", Version: "1.0.0"},
			Dependencies: []string{"lib-a", "lib-b"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{
					fmt.Sprintf("echo app > %s/app.txt", stampDir),
				}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// All should succeed
	for _, r := range results {
		assert.Equal(t, 0, r.ExitCode, "package %s should succeed", r.Package)
	}

	// app must be the last result (level 1)
	assert.Equal(t, "app", results[2].Package)

	// All stamp files should exist
	for _, name := range []string{"a.txt", "b.txt", "app.txt"} {
		assert.FileExists(t, filepath.Join(stampDir, name))
	}
}

func TestRun_FailedCommandCreatesLogWithExitCode(t *testing.T) {
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"faillog": {
			Package: config.PackageMeta{Name: "faillog", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"exit 42"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.Error(t, err)
	require.Len(t, results, 1)

	// Log should capture the non-zero exit code
	logData, err := os.ReadFile(results[0].LogFile)
	require.NoError(t, err)
	assert.Contains(t, string(logData), "# exit code: 42")
}

func TestRun_RuntimeEnvDirSubstitution(t *testing.T) {
	stampDir := t.TempDir()
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"envdir": {
			Package: config.PackageMeta{Name: "envdir", Version: "1.0.0"},
			Runtime: &config.Runtime{
				Env: map[string]string{
					"MY_ENV_DIR": "{{env_dir}}/bin",
				},
			},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{
					fmt.Sprintf("echo $MY_ENV_DIR > %s/envdir_out.txt", stampDir),
				}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].ExitCode)

	out, err := os.ReadFile(filepath.Join(stampDir, "envdir_out.txt"))
	require.NoError(t, err)

	expected := filepath.Join(ws.Root, workspace.MarkerDir, "envs", "envdir", "bin")
	assert.Equal(t, expected, strings.TrimSpace(string(out)))
}

func TestRun_ParallelCacheHit(t *testing.T) {
	pkgA := config.DefaultPackageConfig("a")
	pkgA.Phases = map[string]*config.Phase{
		"build": {Commands: []string{"echo a"}},
	}
	pkgB := config.DefaultPackageConfig("b")
	pkgB.Phases = map[string]*config.Phase{
		"build": {Commands: []string{"echo b"}},
	}
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{"a": pkgA, "b": pkgB})

	// First run — populates cache
	results1, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	assert.Len(t, results1, 2)
	for _, r := range results1 {
		assert.False(t, r.CacheHit, "first run should not be cached")
	}

	// Second run — should be cache hits
	results2, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	assert.Len(t, results2, 2)
	for _, r := range results2 {
		assert.True(t, r.CacheHit, "second run of %s should be cached", r.Package)
	}
}

func TestRun_ParallelNoPhase(t *testing.T) {
	pkgA := config.DefaultPackageConfig("a")
	pkgA.Phases = nil
	pkgB := config.DefaultPackageConfig("b")
	pkgB.Phases = nil
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{"a": pkgA, "b": pkgB})

	results, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	for _, r := range results {
		assert.False(t, r.CacheHit)
		assert.Equal(t, 0, r.ExitCode)
	}
}

func TestRun_ParallelNoCache(t *testing.T) {
	pkgA := config.DefaultPackageConfig("a")
	pkgA.Phases = map[string]*config.Phase{
		"build": {Commands: []string{"echo a"}},
	}
	pkgB := config.DefaultPackageConfig("b")
	pkgB.Phases = map[string]*config.Phase{
		"build": {Commands: []string{"echo b"}},
	}
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{"a": pkgA, "b": pkgB})

	// First run
	Run(ws, RunOptions{Phase: "build", Parallel: true})

	// Second run with NoCache — should not be cache hits
	results, err := Run(ws, RunOptions{Phase: "build", Parallel: true, NoCache: true})
	require.NoError(t, err)
	for _, r := range results {
		assert.False(t, r.CacheHit, "%s should not be cached with NoCache", r.Package)
	}
}

func TestRun_EmptyLevelSkipped(t *testing.T) {
	// "base" at level 0, "app" at level 1.
	// Targeting only "app" means level 0 is empty → skipped.
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"base": {
			Package: config.PackageMeta{Name: "base", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo base"}},
			},
		},
		"app": {
			Package:      config.PackageMeta{Name: "app", Version: "1.0.0"},
			Dependencies: []string{"base"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo app"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{
		Phase:    "build",
		Packages: []string{"app"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "app", results[0].Package)
	assert.Equal(t, 0, results[0].ExitCode)
}

func TestRun_ComputeKeyErrorFallback(t *testing.T) {
	// Remove config file after workspace load so ComputeKey fails.
	// The phase should still run (without caching).
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"broken": {
			Package: config.PackageMeta{Name: "broken", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo works"}},
			},
		},
	})
	os.Remove(filepath.Join(ws.Packages["broken"].Dir, workspace.PackageFile))

	results, err := Run(ws, RunOptions{Phase: "build"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].ExitCode)
	assert.False(t, results[0].CacheHit, "should run uncached when ComputeKey fails")
}

func TestRun_ParallelComputeKeyError(t *testing.T) {
	// Same as above but in parallel — exercises runCachedLocal error path.
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"good": {
			Package: config.PackageMeta{Name: "good", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo good"}},
			},
		},
		"broken": {
			Package: config.PackageMeta{Name: "broken", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo broken"}},
			},
		},
	})
	os.Remove(filepath.Join(ws.Packages["broken"].Dir, workspace.PackageFile))

	results, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, 0, r.ExitCode, "%s should succeed", r.Package)
	}
}

func TestRun_ParallelWithDepsFromEarlierLevel(t *testing.T) {
	// base at level 0, app1+app2 at level 1 (parallel).
	// Exercises the snapshot/depKeys path in runParallelCached.
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{
		"base": {
			Package: config.PackageMeta{Name: "base", Version: "1.0.0"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo base"}},
			},
		},
		"app1": {
			Package:      config.PackageMeta{Name: "app1", Version: "1.0.0"},
			Dependencies: []string{"base"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo app1"}},
			},
		},
		"app2": {
			Package:      config.PackageMeta{Name: "app2", Version: "1.0.0"},
			Dependencies: []string{"base"},
			Phases: map[string]*config.Phase{
				"build": {Commands: []string{"echo app2"}},
			},
		},
	})

	results, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	require.NoError(t, err)
	require.Len(t, results, 3)
	for _, r := range results {
		assert.Equal(t, 0, r.ExitCode, "%s should succeed", r.Package)
	}
	// base must come first (level 0), app1/app2 at level 1
	assert.Equal(t, "base", results[0].Package)
}

// errWriter is an io.Writer that fails after a configured number of bytes.
type errWriter struct {
	failAfter int
	written   int
}

func (w *errWriter) Write(p []byte) (int, error) {
	if w.written >= w.failAfter {
		return 0, fmt.Errorf("write failed")
	}
	w.written += len(p)
	return len(p), nil
}

func TestPrefixWriter_ErrorOnPrefixWrite(t *testing.T) {
	w := &errWriter{failAfter: 0}
	pw := &prefixWriter{prefix: "[p] ", w: w, atBOL: true}
	_, err := pw.Write([]byte("hello"))
	assert.Error(t, err, "should propagate error from prefix write")
}

func TestPrefixWriter_ErrorOnByteWrite(t *testing.T) {
	w := &errWriter{failAfter: 4} // prefix "[p] " is 4 bytes
	pw := &prefixWriter{prefix: "[p] ", w: w, atBOL: true}
	_, err := pw.Write([]byte("hello"))
	assert.Error(t, err, "should propagate error from byte write")
}

func TestRecordMetrics_WriteError(t *testing.T) {
	root := t.TempDir()
	markerDir := filepath.Join(root, workspace.MarkerDir)
	os.MkdirAll(markerDir, 0755)

	// Make the marker directory read-only so WriteFile fails
	os.Chmod(markerDir, 0555)
	t.Cleanup(func() { os.Chmod(markerDir, 0755) })

	err := RecordMetrics(root, []Result{
		{Package: "p", Phase: "build", Duration: 100 * time.Millisecond},
	})
	assert.Error(t, err, "should fail when metrics file cannot be written")
}

// TestRun_LogFileCreationFails_RecordsError verifies that when the log path
// cannot be created (e.g., logs/ exists as a regular file), the package result
// surfaces a "creating log file" error and skips command execution entirely.
func TestRun_LogFileCreationFails_RecordsError(t *testing.T) {
	stampDir := t.TempDir()
	pkgCfg := config.DefaultPackageConfig("blocked")
	pkgCfg.Phases = map[string]*config.Phase{
		"build": {Commands: []string{fmt.Sprintf("touch %s/should-not-exist.txt", stampDir)}},
	}
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{"blocked": pkgCfg})

	// Plant a regular file where the logs directory should go.
	logsPath := filepath.Join(ws.Root, ".takumi", "logs")
	require.NoError(t, os.WriteFile(logsPath, []byte("not a dir"), 0644))

	results, _ := Run(ws, RunOptions{Phase: "build"})
	require.Len(t, results, 1)
	require.Error(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "creating log file")

	// Critical: the command MUST NOT have run.
	_, statErr := os.Stat(filepath.Join(stampDir, "should-not-exist.txt"))
	assert.True(t, os.IsNotExist(statErr), "command must not run when log file cannot be created")
}

func TestRun_ParallelFailure(t *testing.T) {
	pkgA := config.DefaultPackageConfig("a")
	pkgA.Phases = map[string]*config.Phase{
		"build": {Commands: []string{"exit 1"}},
	}
	pkgB := config.DefaultPackageConfig("b")
	pkgB.Phases = map[string]*config.Phase{
		"build": {Commands: []string{"echo b"}},
	}
	ws := setupTestWorkspace(t, map[string]*config.PackageConfig{"a": pkgA, "b": pkgB})

	_, err := Run(ws, RunOptions{Phase: "build", Parallel: true})
	assert.Error(t, err)
}
