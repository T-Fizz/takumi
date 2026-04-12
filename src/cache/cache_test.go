package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPkg(t *testing.T) (pkgDir, configPath string) {
	t.Helper()
	dir := t.TempDir()
	pkgDir = filepath.Join(dir, "my-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "util.go"), []byte("package main\nfunc util() {}\n"), 0644)
	configPath = filepath.Join(pkgDir, "takumi-pkg.yaml")
	os.WriteFile(configPath, []byte("package:\n  name: my-pkg\n  version: 0.1.0\n"), 0644)
	return pkgDir, configPath
}

func TestComputeKey_Deterministic(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, n1, err := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	require.NoError(t, err)
	k2, n2, err := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, k1, k2)
	assert.Equal(t, n1, n2)
	assert.Equal(t, 3, n1) // main.go, util.go, takumi-pkg.yaml
}

func TestComputeKey_ChangesOnFileEdit(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte("package main\n// changed\n"), 0644)
	k2, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	assert.NotEqual(t, k1, k2)
}

func TestComputeKey_ChangesOnFileAdd(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, n1, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	os.WriteFile(filepath.Join(pkgDir, "new.go"), []byte("package main\n"), 0644)
	k2, n2, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	assert.NotEqual(t, k1, k2)
	assert.Equal(t, n1+1, n2)
}

func TestComputeKey_ChangesOnFileDelete(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	os.Remove(filepath.Join(pkgDir, "util.go"))
	k2, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	assert.NotEqual(t, k1, k2)
}

func TestComputeKey_ChangesOnConfigEdit(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	os.WriteFile(cfgPath, []byte("package:\n  name: my-pkg\n  version: 0.2.0\n"), 0644)
	k2, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	assert.NotEqual(t, k1, k2)
}

func TestComputeKey_ChangesOnPhase(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	k2, _, _ := ComputeKey(pkgDir, cfgPath, "test", nil, nil)
	assert.NotEqual(t, k1, k2)
}

func TestComputeKey_ChangesOnDepKey(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	deps1 := map[string]string{"lib": "aaa"}
	deps2 := map[string]string{"lib": "bbb"}
	k1, _, _ := ComputeKey(pkgDir, cfgPath, "build", deps1, nil)
	k2, _, _ := ComputeKey(pkgDir, cfgPath, "build", deps2, nil)
	assert.NotEqual(t, k1, k2)
}

func TestComputeKey_IgnoresSkippedDirs(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	// Create a vendor/ directory with a file
	vendorDir := filepath.Join(pkgDir, "vendor")
	os.MkdirAll(vendorDir, 0755)
	os.WriteFile(filepath.Join(vendorDir, "dep.go"), []byte("package dep\n"), 0644)
	k1, n1, _ := ComputeKey(pkgDir, cfgPath, "build", nil, []string{"vendor/"})
	// Key should not include vendor files
	k2, n2, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	assert.NotEqual(t, k1, k2, "ignoring vendor should produce different key")
	assert.Less(t, n1, n2, "ignored dir should reduce file count")
}

func TestComputeKey_IgnoresGitDir(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	gitDir := filepath.Join(pkgDir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)
	k1, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	// Remove .git and compare — should be the same since .git is always skipped
	os.RemoveAll(gitDir)
	k2, _, _ := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	assert.Equal(t, k1, k2, ".git should be ignored")
}

func TestStore_LookupMiss(t *testing.T) {
	store := NewStore(t.TempDir())
	assert.Nil(t, store.Lookup("pkg", "build"))
}

func TestStore_WriteAndLookup(t *testing.T) {
	store := NewStore(t.TempDir())
	entry := &Entry{
		Key:        "abc123",
		Package:    "my-pkg",
		Phase:      "build",
		Timestamp:  "2026-01-01T00:00:00Z",
		DurationMs: 150,
		FileCount:  10,
	}
	require.NoError(t, store.Write(entry))
	got := store.Lookup("my-pkg", "build")
	require.NotNil(t, got)
	assert.Equal(t, "abc123", got.Key)
	assert.Equal(t, int64(150), got.DurationMs)
}

func TestStore_Clean(t *testing.T) {
	store := NewStore(t.TempDir())
	entry := &Entry{Key: "abc", Package: "p", Phase: "build"}
	require.NoError(t, store.Write(entry))
	assert.NotNil(t, store.Lookup("p", "build"))
	require.NoError(t, store.Clean())
	assert.Nil(t, store.Lookup("p", "build"))
}

func TestStore_CorruptData(t *testing.T) {
	store := NewStore(t.TempDir())
	os.MkdirAll(store.Dir, 0755)
	os.WriteFile(store.entryPath("pkg", "build"), []byte("not json"), 0644)
	assert.Nil(t, store.Lookup("pkg", "build"), "corrupt entry should return nil")
}

func TestComputeKey_EmptyDepKeys(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	k1, _, err := ComputeKey(pkgDir, cfgPath, "build", map[string]string{}, nil)
	require.NoError(t, err)
	k2, _, err := ComputeKey(pkgDir, cfgPath, "build", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, k1, k2, "empty map and nil should produce same key")
}

func TestComputeKey_ConfigNotFound(t *testing.T) {
	pkgDir, _ := setupPkg(t)
	_, _, err := ComputeKey(pkgDir, "/nonexistent/config.yaml", "build", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hashing config")
}

func TestComputeKey_MultipleDeps(t *testing.T) {
	pkgDir, cfgPath := setupPkg(t)
	deps := map[string]string{"a": "key1", "b": "key2", "c": "key3"}
	k, _, err := ComputeKey(pkgDir, cfgPath, "build", deps, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, k)
	// Same deps in different map order should produce same key (sorted)
	deps2 := map[string]string{"c": "key3", "a": "key1", "b": "key2"}
	k2, _, err := ComputeKey(pkgDir, cfgPath, "build", deps2, nil)
	require.NoError(t, err)
	assert.Equal(t, k, k2, "key should be deterministic regardless of map order")
}

func TestStore_WriteCreatesDir(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	// Dir doesn't exist yet
	entry := &Entry{Key: "abc", Package: "p", Phase: "test"}
	require.NoError(t, store.Write(entry))
	// Should have created the dir
	assert.NotNil(t, store.Lookup("p", "test"))
}

func TestHashFile_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)
	h1, err := hashFile(path)
	require.NoError(t, err)
	h2, err := hashFile(path)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 64) // SHA-256 hex = 64 chars
}

func TestHashFile_NotFound(t *testing.T) {
	_, err := hashFile("/nonexistent/file")
	assert.Error(t, err)
}

func TestStore_Write_MkdirFails(t *testing.T) {
	// Place a regular file where the cache directory would be created
	root := t.TempDir()
	blocker := filepath.Join(root, ".takumi")
	os.WriteFile(blocker, []byte("I am a file, not a dir"), 0644)

	store := NewStore(root)
	entry := &Entry{Key: "abc", Package: "p", Phase: "build"}
	err := store.Write(entry)
	assert.Error(t, err, "Write should fail when MkdirAll cannot create cache dir")
}

func TestComputeKey_NonexistentPkgDir(t *testing.T) {
	// When pkgDir doesn't exist, hashDirectory returns 0 files (no error).
	// ComputeKey should still succeed with a valid key and 0 file count.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "takumi-pkg.yaml")
	os.WriteFile(cfgPath, []byte("package:\n  name: x\n  version: 0.1.0\n"), 0644)

	key, count, err := ComputeKey(filepath.Join(dir, "nonexistent"), cfgPath, "build", nil, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, key, "should produce a key even with empty source dir")
	assert.Equal(t, 0, count, "nonexistent dir should yield 0 files")
}

func TestHashDirectory_SkipsUnhashableFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readable.go"), []byte("package main"), 0644)

	// Create a file that can't be read (hashFile will fail on it)
	unreadable := filepath.Join(dir, "secret.go")
	os.WriteFile(unreadable, []byte("package main"), 0000)
	t.Cleanup(func() { os.Chmod(unreadable, 0644) })

	var buf strings.Builder
	n, err := hashDirectory(&buf, dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the readable file should be hashed")
	assert.Contains(t, buf.String(), "file:readable.go:")
	assert.NotContains(t, buf.String(), "secret.go")
}

func TestHashDirectory_WalkCallbackError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.go"), []byte("package main"), 0644)

	// Create inaccessible subdirectory — Walk calls callback with err != nil
	badDir := filepath.Join(dir, "noperm")
	os.Mkdir(badDir, 0000)
	t.Cleanup(func() { os.Chmod(badDir, 0755) })

	var buf strings.Builder
	n, err := hashDirectory(&buf, dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "should hash only the accessible file")
}

func TestHashFile_DirectoryFails(t *testing.T) {
	// os.Open succeeds on directories, but io.Copy fails (can't read a dir FD)
	dir := t.TempDir()
	_, err := hashFile(dir)
	assert.Error(t, err, "hashFile on a directory should fail during io.Copy")
}

func TestHashDirectory_SkipsTakumiDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "src.go"), []byte("package main"), 0644)
	takumiDir := filepath.Join(dir, ".takumi")
	os.MkdirAll(takumiDir, 0755)
	os.WriteFile(filepath.Join(takumiDir, "state.json"), []byte("{}"), 0644)

	var buf1 strings.Builder
	n1, _ := hashDirectory(&buf1, dir, nil)

	// Remove .takumi and hash again — should be the same
	os.RemoveAll(takumiDir)
	var buf2 strings.Builder
	n2, _ := hashDirectory(&buf2, dir, nil)

	assert.Equal(t, n1, n2, ".takumi dir should be skipped")
	assert.Equal(t, buf1.String(), buf2.String())
}
