package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempYAML(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))
	return path
}

func TestLoadPackageConfig_RejectsUnknownPhaseField(t *testing.T) {
	path := writeTempYAML(t, "takumi-pkg.yaml", `
package:
  name: foo
  version: 0.1.0
phases:
  build:
    commands: [echo hi]
    cacheable: false
`)
	_, err := LoadPackageConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "cacheable"`)
}

func TestLoadPackageConfig_SuggestsCloseMatch(t *testing.T) {
	path := writeTempYAML(t, "takumi-pkg.yaml", `
package:
  name: foo
  version: 0.1.0
phases:
  build:
    cmmands: [echo hi]
`)
	_, err := LoadPackageConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `did you mean "commands"`)
}

func TestLoadPackageConfig_RejectsUnknownTopLevel(t *testing.T) {
	path := writeTempYAML(t, "takumi-pkg.yaml", `
package:
  name: foo
  version: 0.1.0
extras: stuff
`)
	_, err := LoadPackageConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "extras"`)
}

func TestLoadPackageConfig_AcceptsValid(t *testing.T) {
	path := writeTempYAML(t, "takumi-pkg.yaml", `
package:
  name: foo
  version: 0.1.0
phases:
  build:
    pre: [echo pre]
    commands: [echo build]
    post: [echo post]
`)
	cfg, err := LoadPackageConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "foo", cfg.Package.Name)
}

func TestLoadWorkspaceConfig_RejectsUnknownField(t *testing.T) {
	path := writeTempYAML(t, "takumi.yaml", `
workspace:
  name: my-ws
  parralel: true
  settings:
    parallel: true
`)
	_, err := LoadWorkspaceConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "parralel"`)
}

func TestLoadVersionSetConfig_RejectsUnknownField(t *testing.T) {
	path := writeTempYAML(t, "takumi-versions.yaml", `
version-set:
  name: q1
  strategy: strict
  packages:
    react: 18.0.0
  garbage: yes
`)
	_, err := LoadVersionSetConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "garbage"`)
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"cacheable", "cache", 4},
		{"cmmands", "commands", 1},
		{"parralel", "parallel", 2},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, levenshtein(c.a, c.b), "levenshtein(%q, %q)", c.a, c.b)
	}
}

func TestNearestField(t *testing.T) {
	candidates := []string{"commands", "pre", "post"}
	assert.Equal(t, "commands", nearestField("cmmands", candidates))
	assert.Equal(t, "", nearestField("totallydifferent", candidates))
}

func TestKnownFieldsForType_FindsNestedStruct(t *testing.T) {
	var cfg PackageConfig
	got := knownFieldsForType(&cfg, "config.Phase")
	assert.ElementsMatch(t, []string{"pre", "commands", "post"}, got)
}
