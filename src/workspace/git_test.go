package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ChangedFiles ---

func TestChangedFiles_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := ChangedFiles(dir, "HEAD")
	assert.Error(t, err)
}

func TestChangedFiles_CleanRepo(t *testing.T) {
	dir := setupGitRepo(t)
	files, err := ChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestChangedFiles_WithChanges(t *testing.T) {
	dir := setupGitRepo(t)

	// Create and stage a new file so it shows in git diff HEAD
	newFile := filepath.Join(dir, "svc-a", "new.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package svc\n"), 0644))
	require.NoError(t, exec.Command("git", "-C", dir, "add", newFile).Run())

	files, err := ChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.Contains(t, files, "svc-a/new.go")
}

func TestChangedFiles_FallbackOnBadRef(t *testing.T) {
	dir := setupGitRepo(t)

	// Invalid ref triggers fallback to working tree diff (which is clean)
	files, err := ChangedFiles(dir, "nonexistent-ref-xyz")
	require.NoError(t, err)
	assert.Empty(t, files)
}

// --- MapFilesToPackages ---

func TestMapFilesToPackages_RelativePath(t *testing.T) {
	root := t.TempDir()
	ws := &Info{
		Root: root,
		Packages: map[string]*DiscoveredPkg{
			"svc-a": {Name: "svc-a", Dir: filepath.Join(root, "svc-a")},
			"svc-b": {Name: "svc-b", Dir: filepath.Join(root, "svc-b")},
		},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc-a"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc-b"), 0755))

	affected := MapFilesToPackages(ws, []string{"svc-a/main.go"})
	assert.True(t, affected["svc-a"])
	assert.False(t, affected["svc-b"])
}

func TestMapFilesToPackages_AbsolutePath(t *testing.T) {
	root := t.TempDir()
	ws := &Info{
		Root: root,
		Packages: map[string]*DiscoveredPkg{
			"svc-a": {Name: "svc-a", Dir: filepath.Join(root, "svc-a")},
		},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc-a"), 0755))

	affected := MapFilesToPackages(ws, []string{filepath.Join(root, "svc-a", "main.go")})
	assert.True(t, affected["svc-a"])
}

func TestMapFilesToPackages_NoMatch(t *testing.T) {
	root := t.TempDir()
	ws := &Info{
		Root: root,
		Packages: map[string]*DiscoveredPkg{
			"svc-a": {Name: "svc-a", Dir: filepath.Join(root, "svc-a")},
		},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc-a"), 0755))

	affected := MapFilesToPackages(ws, []string{"README.md"})
	assert.Empty(t, affected)
}

func TestMapFilesToPackages_EmptyFiles(t *testing.T) {
	root := t.TempDir()
	ws := &Info{
		Root: root,
		Packages: map[string]*DiscoveredPkg{
			"svc-a": {Name: "svc-a", Dir: filepath.Join(root, "svc-a")},
		},
	}

	affected := MapFilesToPackages(ws, nil)
	assert.Empty(t, affected)
}

func TestMapFilesToPackages_MultiplePackages(t *testing.T) {
	root := t.TempDir()
	ws := &Info{
		Root: root,
		Packages: map[string]*DiscoveredPkg{
			"svc-a": {Name: "svc-a", Dir: filepath.Join(root, "svc-a")},
			"svc-b": {Name: "svc-b", Dir: filepath.Join(root, "svc-b")},
		},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc-a"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc-b"), 0755))

	affected := MapFilesToPackages(ws, []string{"svc-a/main.go", "svc-b/handler.go"})
	assert.True(t, affected["svc-a"])
	assert.True(t, affected["svc-b"])
}

// --- helpers ---

// setupGitRepo creates a temp dir with a git repo containing two packages.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Init git repo
	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		require.NoError(t, exec.Command(args[0], args[1:]...).Run())
	}

	// Create package directories with files
	for _, pkg := range []string{"svc-a", "svc-b"} {
		pkgDir := filepath.Join(dir, pkg)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(pkgDir, "main.go"),
			[]byte("package "+pkg+"\n"),
			0644,
		))
	}

	// Initial commit
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())

	return dir
}
