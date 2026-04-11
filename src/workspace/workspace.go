// Package workspace handles workspace detection and package discovery.
package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tfitz/takumi/src/config"
)

const (
	MarkerDir     = ".takumi"
	WorkspaceFile = "takumi.yaml"
	PackageFile   = "takumi-pkg.yaml"
	VersionsFile  = "takumi-versions.yaml"
	AIFile        = "takumi-ai.yaml"
)

// Info holds the resolved workspace state.
type Info struct {
	Root     string                    // Absolute path to workspace root (parent of .takumi/)
	Config   *config.WorkspaceConfig   // Parsed takumi.yaml
	Packages map[string]*DiscoveredPkg // name → discovered package
}

// DiscoveredPkg is a package found during recursive scanning.
type DiscoveredPkg struct {
	Name   string
	Dir    string // Absolute path to directory containing takumi-pkg.yaml
	Config *config.PackageConfig
}

// Detect walks up from startDir looking for the .takumi/ marker directory.
// Returns the workspace root (the directory containing .takumi/) or empty string if not found.
func Detect(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}

	for {
		marker := filepath.Join(dir, MarkerDir)
		if info, err := os.Stat(marker); err == nil && info.IsDir() {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return ""
		}
		dir = parent
	}
}

// ScanPackages recursively finds all takumi-pkg.yaml files under root,
// respecting the ignore list from the workspace config.
func ScanPackages(root string, ignore []string) (map[string]*DiscoveredPkg, error) {
	pkgs := make(map[string]*DiscoveredPkg)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}

		if info.IsDir() {
			name := info.Name()
			// Always skip the .takumi directory itself
			if name == MarkerDir {
				return filepath.SkipDir
			}
			// Skip ignored directories
			if shouldIgnore(root, path, ignore) {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() != PackageFile {
			return nil
		}

		cfg, err := config.LoadPackageConfig(path)
		if err != nil {
			return nil // skip unparseable package files
		}

		pkgs[cfg.Package.Name] = &DiscoveredPkg{
			Name:   cfg.Package.Name,
			Dir:    filepath.Dir(path),
			Config: cfg,
		}
		return nil
	})

	return pkgs, err
}

// shouldIgnore checks if a directory path matches any entry in the ignore list.
func shouldIgnore(root, path string, ignore []string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	for _, pattern := range ignore {
		// Normalize: strip trailing slash for comparison
		pattern = strings.TrimSuffix(pattern, "/")

		// Match directory name directly
		if filepath.Base(path) == pattern {
			return true
		}

		// Match relative path prefix
		if rel == pattern || strings.HasPrefix(rel, pattern+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Load detects the workspace from startDir and loads its config + packages.
func Load(startDir string) (*Info, error) {
	root := Detect(startDir)
	if root == "" {
		return nil, nil // not in a workspace
	}

	cfgPath := filepath.Join(root, WorkspaceFile)
	cfg, err := config.LoadWorkspaceConfig(cfgPath)
	if err != nil {
		return nil, err
	}

	pkgs, err := ScanPackages(root, cfg.Workspace.Ignore)
	if err != nil {
		return nil, err
	}

	return &Info{
		Root:     root,
		Config:   cfg,
		Packages: pkgs,
	}, nil
}
