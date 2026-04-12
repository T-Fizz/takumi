// Package cache provides content-addressed build caching for Takumi.
// Cache keys are SHA-256 digests of source files, config, phase name,
// and dependency cache keys. Keys are computed in topological order
// so that dependency changes cascade automatically.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry records the cache state for one (package, phase) pair.
type Entry struct {
	Key        string `json:"key"`
	Package    string `json:"package"`
	Phase      string `json:"phase"`
	Timestamp  string `json:"timestamp"`
	DurationMs int64  `json:"duration_ms"`
	FileCount  int    `json:"file_count"`
}

// Store manages the .takumi/cache/ directory.
type Store struct {
	Dir string
}

// NewStore creates a cache store at wsRoot/.takumi/cache/.
func NewStore(wsRoot string) *Store {
	return &Store{Dir: filepath.Join(wsRoot, ".takumi", "cache")}
}

// Lookup reads the stored cache entry for (pkg, phase).
// Returns nil on miss, corrupt data, or any read error.
func (s *Store) Lookup(pkg, phase string) *Entry {
	data, err := os.ReadFile(s.entryPath(pkg, phase))
	if err != nil {
		return nil
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}
	return &entry
}

// Write persists a cache entry after a successful phase run.
func (s *Store) Write(entry *Entry) error {
	if err := os.MkdirAll(s.Dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.entryPath(entry.Package, entry.Phase), data, 0644)
}

// Clean removes all cached entries.
func (s *Store) Clean() error {
	return os.RemoveAll(s.Dir)
}

func (s *Store) entryPath(pkg, phase string) string {
	return filepath.Join(s.Dir, fmt.Sprintf("%s.%s.json", pkg, phase))
}

// ComputeKey computes the cache key for a (package, phase) pair.
// depKeys maps dependency package names to their cache keys for this phase.
// ignore is the workspace ignore list used to skip directories during hashing.
// Returns the hex-encoded SHA-256 key and the number of files hashed.
func ComputeKey(pkgDir, configPath, phase string, depKeys map[string]string, ignore []string) (string, int, error) {
	h := sha256.New()

	// 1. Phase name
	fmt.Fprintf(h, "phase:%s\n", phase)

	// 2. Config file hash
	cfgHash, err := hashFile(configPath)
	if err != nil {
		return "", 0, fmt.Errorf("hashing config: %w", err)
	}
	fmt.Fprintf(h, "config:%s\n", cfgHash)

	// 3. Source file hashes (sorted by relative path)
	fileCount, err := hashDirectory(h, pkgDir, ignore)
	if err != nil {
		return "", 0, fmt.Errorf("hashing sources: %w", err)
	}

	// 4. Dependency cache keys (sorted for determinism)
	depNames := make([]string, 0, len(depKeys))
	for name := range depKeys {
		depNames = append(depNames, name)
	}
	sort.Strings(depNames)
	for _, name := range depNames {
		fmt.Fprintf(h, "dep:%s:%s\n", name, depKeys[name])
	}

	return hex.EncodeToString(h.Sum(nil)), fileCount, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashDirectory(w io.Writer, pkgDir string, ignore []string) (int, error) {
	type entry struct {
		rel  string
		hash string
	}
	var entries []entry

	err := filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".takumi" || name == ".git" {
				return filepath.SkipDir
			}
			for _, pat := range ignore {
				if name == strings.TrimSuffix(pat, "/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, _ := filepath.Rel(pkgDir, path)
		fh, err := hashFile(path)
		if err != nil {
			return nil // skip unhashable files
		}
		entries = append(entries, entry{rel: rel, hash: fh})
		return nil
	})
	if err != nil {
		return 0, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	for _, e := range entries {
		fmt.Fprintf(w, "file:%s:%s\n", e.rel, e.hash)
	}
	return len(entries), nil
}
