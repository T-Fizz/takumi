package workspace

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedFiles returns files changed since the given git ref.
// Falls back to working tree diff if the ref comparison fails.
func ChangedFiles(wsRoot, since string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", since)
	cmd.Dir = wsRoot
	out, err := cmd.Output()
	if err != nil {
		// Fall back to working tree changes
		cmd2 := exec.Command("git", "diff", "--name-only")
		cmd2.Dir = wsRoot
		out, err = cmd2.Output()
		if err != nil {
			return nil, err
		}
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// MapFilesToPackages determines which packages contain the given files.
// File paths may be relative to the workspace root or absolute.
func MapFilesToPackages(ws *Info, files []string) map[string]bool {
	affected := make(map[string]bool)
	for _, file := range files {
		absPath := file
		if !filepath.IsAbs(file) {
			absPath = filepath.Join(ws.Root, file)
		}
		for name, pkg := range ws.Packages {
			rel, err := filepath.Rel(pkg.Dir, absPath)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(rel, "..") {
				affected[name] = true
			}
		}
	}
	return affected
}
