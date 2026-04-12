package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
)

// realPath resolves symlinks (macOS /var → /private/var).
func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

// withFakeExit replaces osExit with a recorder for the duration of the test.
func withFakeExit(t *testing.T) *int {
	t.Helper()
	exitCode := -1
	original := osExit
	osExit = func(code int) { exitCode = code }
	t.Cleanup(func() { osExit = original })
	return &exitCode
}

// --- Execute ---

func TestExecute_Success(t *testing.T) {
	exitCode := withFakeExit(t)
	rootCmd.SetArgs([]string{"--help"})
	Execute()
	assert.Equal(t, -1, *exitCode, "os.Exit should not be called on success")
}

func TestExecute_Error(t *testing.T) {
	exitCode := withFakeExit(t)
	rootCmd.SetArgs([]string{"nonexistent-command"})
	Execute()
	assert.Equal(t, 1, *exitCode, "os.Exit(1) should be called on error")
}

// --- loadWorkspace ---

func TestLoadWorkspace_Success(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, dir, ws.Root)
	assert.Equal(t, "test-ws", ws.Config.Workspace.Name)
}

func TestLoadWorkspace_NotInWorkspace(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "not in a Takumi workspace")
}

func TestLoadWorkspace_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte("{{bad}}"), 0644))
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "loading workspace")
}

func TestLoadWorkspace_FromSubdirectory(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	subDir := filepath.Join(dir, "deep", "nested")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	chdirClean(t, subDir)

	ws, err := loadWorkspace()
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, dir, ws.Root)
}

// --- requireWorkspace ---

func TestRequireWorkspace_Success(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	exitCode := withFakeExit(t)
	ws := requireWorkspace()
	assert.NotNil(t, ws)
	assert.Equal(t, -1, *exitCode, "os.Exit should not be called on success")
}

func TestRequireWorkspace_ExitsOnError(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)

	exitCode := withFakeExit(t)
	_ = requireWorkspace()
	assert.Equal(t, 1, *exitCode, "os.Exit(1) should be called when not in workspace")
}

// --- edge cases: deleted cwd ---

func TestLoadWorkspace_DeletedCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot delete cwd on Windows")
	}

	dir := t.TempDir()
	doomed := filepath.Join(dir, "doomed")
	require.NoError(t, os.Mkdir(doomed, 0755))
	chdirClean(t, doomed)
	require.NoError(t, os.Remove(doomed))

	// On macOS, Getwd returns a stale path (no error), so this hits the
	// "not in workspace" branch. On Linux, Getwd may actually fail.
	ws, err := loadWorkspace()
	assert.Error(t, err)
	assert.Nil(t, ws)
}

// --- helpers ---

// setupWorkspace creates a minimal .takumi/ + takumi.yaml in dir.
func setupWorkspace(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
}
