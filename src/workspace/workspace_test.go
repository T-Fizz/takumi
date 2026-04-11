package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Detect ---

func TestDetect_FindsMarkerInCurrentDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	assert.Equal(t, root, Detect(root))
}

func TestDetect_FindsMarkerInParent(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	child := filepath.Join(root, "sub", "deep")
	require.NoError(t, os.MkdirAll(child, 0755))

	assert.Equal(t, root, Detect(child))
}

func TestDetect_ReturnsEmptyWhenNoMarker(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "", Detect(dir))
}

func TestDetect_IgnoresMarkerFile(t *testing.T) {
	// .takumi as a file (not a directory) should not match
	root := t.TempDir()
	f, err := os.Create(filepath.Join(root, MarkerDir))
	require.NoError(t, err)
	f.Close()

	assert.Equal(t, "", Detect(root))
}

func TestDetect_ImmediateDir(t *testing.T) {
	// Marker in the exact directory passed
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))
	assert.Equal(t, root, Detect(root))
}

// --- ScanPackages ---

func TestScanPackages_FindsPackages(t *testing.T) {
	root := setupTestWorkspace(t, map[string]string{
		"svc-a": "svc-a",
		"svc-b": "sub/svc-b",
	})

	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
	assert.Contains(t, pkgs, "svc-a")
	assert.Contains(t, pkgs, "svc-b")

	// Verify paths are correct
	assert.Equal(t, filepath.Join(root, "svc-a"), pkgs["svc-a"].Dir)
	assert.Equal(t, filepath.Join(root, "sub", "svc-b"), pkgs["svc-b"].Dir)
}

func TestScanPackages_RespectsIgnoreList(t *testing.T) {
	root := setupTestWorkspace(t, map[string]string{
		"keep":   "keep",
		"vendor": "vendor/vendored",
		"node":   "node_modules/pkg",
	})

	pkgs, _, err := ScanPackages(root, []string{"vendor/", "node_modules/"})
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "keep")
}

func TestScanPackages_IgnoresDirectoryByName(t *testing.T) {
	root := setupTestWorkspace(t, map[string]string{
		"good":   "good",
		"hidden": "nested/vendor/hidden",
	})

	pkgs, _, err := ScanPackages(root, []string{"vendor"})
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "good")
}

func TestScanPackages_SkipsTakumiDir(t *testing.T) {
	root := t.TempDir()
	takumiDir := filepath.Join(root, MarkerDir, "nested")
	require.NoError(t, os.MkdirAll(takumiDir, 0755))
	writePkgYAML(t, takumiDir, "should-skip")

	// Also add a real package
	svcDir := filepath.Join(root, "real-svc")
	require.NoError(t, os.Mkdir(svcDir, 0755))
	writePkgYAML(t, svcDir, "real-svc")

	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "real-svc")
}

func TestScanPackages_SkipsUnparseableFiles(t *testing.T) {
	root := t.TempDir()

	// Valid package
	goodDir := filepath.Join(root, "good")
	require.NoError(t, os.Mkdir(goodDir, 0755))
	writePkgYAML(t, goodDir, "good")

	// Invalid YAML
	badDir := filepath.Join(root, "bad")
	require.NoError(t, os.Mkdir(badDir, 0755))
	err := os.WriteFile(filepath.Join(badDir, PackageFile), []byte("{{broken}}"), 0644)
	require.NoError(t, err)

	pkgs, parseErrors, scanErr := ScanPackages(root, nil)
	require.NoError(t, scanErr)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "good")

	// Parse errors should be collected, not silently dropped
	require.Len(t, parseErrors, 1)
	assert.Equal(t, filepath.Join(badDir, PackageFile), parseErrors[0].Path)
	assert.Error(t, parseErrors[0].Err)
}

func TestScanPackages_CollectsColonSpaceYAMLParseError(t *testing.T) {
	// Regression test: a YAML plain scalar containing ": " (colon-space) gets
	// silently misparsed by the YAML decoder, causing struct unmarshalling to fail.
	// ScanPackages must surface these as parse errors, not silently drop them.
	root := t.TempDir()

	// Valid package
	goodDir := filepath.Join(root, "good")
	require.NoError(t, os.Mkdir(goodDir, 0755))
	writePkgYAML(t, goodDir, "good")

	// Package with unquoted colon-space in a command value
	badDir := filepath.Join(root, "bad-colon")
	require.NoError(t, os.Mkdir(badDir, 0755))
	badYAML := "package:\n  name: bad-colon\n  version: 0.1.0\nphases:\n  build:\n    commands:\n      - echo greeter: syntax OK\n"
	require.NoError(t, os.WriteFile(filepath.Join(badDir, PackageFile), []byte(badYAML), 0644))

	pkgs, parseErrors, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "good")

	require.Len(t, parseErrors, 1)
	assert.Equal(t, filepath.Join(badDir, PackageFile), parseErrors[0].Path)
	assert.Error(t, parseErrors[0].Err)
}

func TestScanPackages_EmptyWorkspace(t *testing.T) {
	root := t.TempDir()
	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

func TestScanPackages_RootPackage(t *testing.T) {
	root := t.TempDir()
	writePkgYAML(t, root, "root-pkg")

	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "root-pkg")
	assert.Equal(t, root, pkgs["root-pkg"].Dir)
}

func TestScanPackages_SkipsNonPackageFiles(t *testing.T) {
	root := t.TempDir()
	// Write a random YAML file that isn't takumi-pkg.yaml
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.yaml"), []byte("key: val"), 0644))
	writePkgYAML(t, root, "real")

	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "real")
}

func TestScanPackages_SkipsInaccessibleDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}

	root := t.TempDir()

	// Accessible package
	goodDir := filepath.Join(root, "good")
	require.NoError(t, os.Mkdir(goodDir, 0755))
	writePkgYAML(t, goodDir, "good")

	// Inaccessible directory — walk callback gets err != nil
	badDir := filepath.Join(root, "noperm")
	require.NoError(t, os.Mkdir(badDir, 0000))
	t.Cleanup(func() { os.Chmod(badDir, 0755) })

	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Contains(t, pkgs, "good")
}

func TestScanPackages_PackageConfigFields(t *testing.T) {
	root := t.TempDir()
	writePkgYAML(t, root, "my-pkg")

	pkgs, _, err := ScanPackages(root, nil)
	require.NoError(t, err)

	pkg := pkgs["my-pkg"]
	require.NotNil(t, pkg)
	assert.Equal(t, "my-pkg", pkg.Name)
	assert.Equal(t, root, pkg.Dir)
	assert.Equal(t, "my-pkg", pkg.Config.Package.Name)
	assert.Equal(t, "0.1.0", pkg.Config.Package.Version)
}

// --- Load ---

func TestLoad_FullWorkspace(t *testing.T) {
	root := t.TempDir()

	// Create marker
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	// Write workspace config
	wsYAML := `workspace:
  name: test-ws
  ignore:
    - vendor/
`
	require.NoError(t, os.WriteFile(filepath.Join(root, WorkspaceFile), []byte(wsYAML), 0644))

	// Write packages
	svcDir := filepath.Join(root, "svc")
	require.NoError(t, os.Mkdir(svcDir, 0755))
	writePkgYAML(t, svcDir, "svc")

	vendorDir := filepath.Join(root, "vendor", "ignored")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	writePkgYAML(t, vendorDir, "ignored")

	ws, err := Load(root)
	require.NoError(t, err)
	require.NotNil(t, ws)

	assert.Equal(t, root, ws.Root)
	assert.Equal(t, "test-ws", ws.Config.Workspace.Name)
	assert.Len(t, ws.Packages, 1)
	assert.Contains(t, ws.Packages, "svc")
}

func TestLoad_NotInWorkspace(t *testing.T) {
	dir := t.TempDir()
	ws, err := Load(dir)
	require.NoError(t, err)
	assert.Nil(t, ws)
}

func TestLoad_FromSubdirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	wsYAML := `workspace:
  name: sub-test
`
	require.NoError(t, os.WriteFile(filepath.Join(root, WorkspaceFile), []byte(wsYAML), 0644))

	subDir := filepath.Join(root, "deep", "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	ws, err := Load(subDir)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, root, ws.Root)
}

func TestLoad_InvalidWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	// Write invalid YAML as workspace config
	require.NoError(t, os.WriteFile(filepath.Join(root, WorkspaceFile), []byte("{{bad}}"), 0644))

	ws, err := Load(root)
	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "parsing workspace config")
}

func TestLoad_MissingWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	// Create marker but no takumi.yaml
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	ws, err := Load(root)
	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "reading workspace config")
}

func TestLoad_ReturnsDiscoveredPackages(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, MarkerDir), 0755))

	wsYAML := "workspace:\n  name: multi\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, WorkspaceFile), []byte(wsYAML), 0644))

	// Create two packages
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, name)
		require.NoError(t, os.Mkdir(dir, 0755))
		writePkgYAML(t, dir, name)
	}

	ws, err := Load(root)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Len(t, ws.Packages, 2)
	assert.Contains(t, ws.Packages, "alpha")
	assert.Contains(t, ws.Packages, "beta")
}

// --- shouldIgnore ---

func TestShouldIgnore_ExactDirectoryName(t *testing.T) {
	assert.True(t, shouldIgnore("/root", "/root/vendor", []string{"vendor"}))
	assert.True(t, shouldIgnore("/root", "/root/sub/vendor", []string{"vendor"}))
}

func TestShouldIgnore_WithTrailingSlash(t *testing.T) {
	assert.True(t, shouldIgnore("/root", "/root/vendor", []string{"vendor/"}))
}

func TestShouldIgnore_RelativePathPrefix(t *testing.T) {
	assert.True(t, shouldIgnore("/root", "/root/archived", []string{"archived"}))
}

func TestShouldIgnore_NestedRelativePath(t *testing.T) {
	// Match a relative path that is a prefix of a deeper path
	assert.True(t, shouldIgnore("/root", "/root/archived/deep", []string{"archived"}))
}

func TestShouldIgnore_NoMatch(t *testing.T) {
	assert.False(t, shouldIgnore("/root", "/root/src", []string{"vendor", "node_modules"}))
}

func TestShouldIgnore_EmptyIgnoreList(t *testing.T) {
	assert.False(t, shouldIgnore("/root", "/root/anything", nil))
}

func TestShouldIgnore_MultiplePatterns(t *testing.T) {
	ignore := []string{"vendor", "node_modules", ".git"}
	assert.True(t, shouldIgnore("/root", "/root/vendor", ignore))
	assert.True(t, shouldIgnore("/root", "/root/node_modules", ignore))
	assert.True(t, shouldIgnore("/root", "/root/.git", ignore))
	assert.False(t, shouldIgnore("/root", "/root/src", ignore))
}

func TestShouldIgnore_PartialNameNoMatch(t *testing.T) {
	// "vendor-extra" should not match "vendor"
	assert.False(t, shouldIgnore("/root", "/root/vendor-extra", []string{"vendor"}))
}

// --- Detect edge cases ---

func TestDetect_RelativePathWithDeletedCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot delete cwd on Windows")
	}

	dir := t.TempDir()
	doomed := filepath.Join(dir, "doomed")
	require.NoError(t, os.Mkdir(doomed, 0755))
	require.NoError(t, os.Chdir(doomed))
	require.NoError(t, os.Remove(doomed))
	t.Cleanup(func() { os.Chdir(dir) })

	// Relative path + deleted cwd → filepath.Abs fails
	result := Detect(".")
	assert.Equal(t, "", result)
}

// --- shouldIgnore edge cases ---

func TestShouldIgnore_ExactRelativePathMatch(t *testing.T) {
	// Multi-segment pattern: Base won't match, but rel == pattern should
	assert.True(t, shouldIgnore("/root", "/root/third_party/vendored", []string{"third_party/vendored"}))
}

func TestShouldIgnore_RelativePathPrefixOfDeeper(t *testing.T) {
	// Multi-segment pattern matching a deeper path via HasPrefix
	assert.True(t, shouldIgnore("/root", "/root/third_party/vendored/sub", []string{"third_party/vendored"}))
}

// --- helpers ---

// setupTestWorkspace creates a temp dir with packages at specified relative paths.
// pkgMap: name → relative dir path.
func setupTestWorkspace(t *testing.T, pkgMap map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, relDir := range pkgMap {
		dir := filepath.Join(root, relDir)
		require.NoError(t, os.MkdirAll(dir, 0755))
		writePkgYAML(t, dir, name)
	}
	return root
}

func writePkgYAML(t *testing.T, dir, name string) {
	t.Helper()
	yaml := "package:\n  name: " + name + "\n  version: 0.1.0\n"
	err := os.WriteFile(filepath.Join(dir, PackageFile), []byte(yaml), 0644)
	require.NoError(t, err)
}
