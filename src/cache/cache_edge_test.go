package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStore_WriteOverwritesCorruptedEntry verifies that an entry corrupted on
// disk is cleanly overwritten by the next Write — no stale state lingers,
// no append errors. This pins the recovery contract that Lookup relies on.
func TestStore_WriteOverwritesCorruptedEntry(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)

	// Plant a corrupt entry file.
	require.NoError(t, os.MkdirAll(s.Dir, 0755))
	corruptPath := filepath.Join(s.Dir, "pkg.build.json")
	require.NoError(t, os.WriteFile(corruptPath, []byte("{not-json"), 0644))

	require.Nil(t, s.Lookup("pkg", "build"), "Lookup must return nil for corrupt JSON")

	// Write fresh entry — must succeed and overwrite cleanly.
	fresh := &Entry{Key: "abc123", Package: "pkg", Phase: "build", FileCount: 3}
	require.NoError(t, s.Write(fresh))

	got := s.Lookup("pkg", "build")
	require.NotNil(t, got, "after Write the entry must be readable")
	assert.Equal(t, "abc123", got.Key)
	assert.Equal(t, 3, got.FileCount)
}

// TestStore_WriteCreatesDirIfMissing verifies that Write creates .takumi/cache/
// when it doesn't exist (so first-time writes don't fail with ENOENT).
func TestStore_WriteCreatesDirIfMissing(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	// Note: NewStore does NOT create the dir.
	_, err := os.Stat(s.Dir)
	require.True(t, os.IsNotExist(err), "Store dir should not exist before first Write")

	require.NoError(t, s.Write(&Entry{Key: "k", Package: "p", Phase: "build"}))

	info, err := os.Stat(s.Dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestStore_Clean_RemovesAllEntriesAndDir verifies that Clean wipes every
// cached entry and the directory itself (subsequent Lookup returns nil).
func TestStore_Clean_RemovesAllEntriesAndDir(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	require.NoError(t, s.Write(&Entry{Key: "k1", Package: "a", Phase: "build"}))
	require.NoError(t, s.Write(&Entry{Key: "k2", Package: "b", Phase: "test"}))

	require.NoError(t, s.Clean())

	_, statErr := os.Stat(s.Dir)
	assert.True(t, os.IsNotExist(statErr), "cache dir must be gone after Clean")
	assert.Nil(t, s.Lookup("a", "build"))
	assert.Nil(t, s.Lookup("b", "test"))
}

// TestComputeKey_FollowsSymlinkedFile verifies that a symlink pointing at a
// file inside the package is hashed (we follow symlinks via filepath.Walk).
// This pins down current behavior so any future "skip symlinks" change is
// caught explicitly.
func TestComputeKey_FollowsSymlinkedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	pkgDir, configPath := setupPkg(t)
	target := filepath.Join(pkgDir, "main.go")
	link := filepath.Join(pkgDir, "alias.go")
	require.NoError(t, os.Symlink(target, link))

	keyWithLink, fileCount, err := ComputeKey(pkgDir, configPath, "build", nil, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, fileCount, 3, "symlinked file should be counted")

	// Removing the link must change the key (proving the symlink contributes).
	require.NoError(t, os.Remove(link))
	keyWithoutLink, _, err := ComputeKey(pkgDir, configPath, "build", nil, nil)
	require.NoError(t, err)
	assert.NotEqual(t, keyWithLink, keyWithoutLink, "removing the symlink must change the cache key")
}

// TestComputeKey_DanglingSymlink_DoesNotAbort verifies a broken symlink
// doesn't fail the whole hash — it's silently skipped (via the err-eating
// branch in hashDirectory). This preserves cache usability when an editor
// or VCS leaves stale links behind.
func TestComputeKey_DanglingSymlink_DoesNotAbort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	pkgDir, configPath := setupPkg(t)
	require.NoError(t, os.Symlink(filepath.Join(pkgDir, "does-not-exist.go"), filepath.Join(pkgDir, "broken.go")))

	key, fileCount, err := ComputeKey(pkgDir, configPath, "build", nil, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.GreaterOrEqual(t, fileCount, 2, "real files still hashed despite dangling link")
}
