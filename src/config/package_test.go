package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPackageConfig_FullConfig(t *testing.T) {
	yaml := `
package:
  name: api-service
  version: 1.2.0

dependencies:
  - shared-utils
  - data-models

runtime:
  setup:
    - python3 -m venv {{env_dir}}
    - "{{env_dir}}/bin/pip install -r requirements.txt"
  env:
    PATH: "{{env_dir}}/bin:$PATH"
    VIRTUAL_ENV: "{{env_dir}}"

phases:
  build:
    pre:
      - protoc --python_out=. protos/*.proto
    commands:
      - python -m build
    post:
      - cp dist/* ../releases/
  test:
    commands:
      - pytest tests/ -v
  lint:
    commands:
      - ruff check .

ai:
  description: "REST API service"
  notes:
    - "Requires Postgres locally"
  tasks:
    add-endpoint:
      description: "Add a new API endpoint"
      steps:
        - "Create route handler in src/routes/"
        - "Add schema in src/schemas/"
`
	path := writeTempFile(t, "takumi-pkg.yaml", yaml)
	cfg, err := LoadPackageConfig(path)

	require.NoError(t, err)
	assert.Equal(t, "api-service", cfg.Package.Name)
	assert.Equal(t, "1.2.0", cfg.Package.Version)

	// Dependencies
	assert.Equal(t, []string{"shared-utils", "data-models"}, cfg.Dependencies)

	// Runtime
	require.NotNil(t, cfg.Runtime)
	assert.Len(t, cfg.Runtime.Setup, 2)
	assert.Contains(t, cfg.Runtime.Setup[0], "venv")
	assert.Equal(t, "{{env_dir}}/bin:$PATH", cfg.Runtime.Env["PATH"])
	assert.Equal(t, "{{env_dir}}", cfg.Runtime.Env["VIRTUAL_ENV"])

	// Phases
	require.Len(t, cfg.Phases, 3)

	build := cfg.Phases["build"]
	require.NotNil(t, build)
	assert.Len(t, build.Pre, 1)
	assert.Equal(t, "python -m build", build.Commands[0])
	assert.Len(t, build.Post, 1)

	test := cfg.Phases["test"]
	require.NotNil(t, test)
	assert.Equal(t, "pytest tests/ -v", test.Commands[0])
	assert.Empty(t, test.Pre)
	assert.Empty(t, test.Post)

	// AI
	require.NotNil(t, cfg.AI)
	assert.Equal(t, "REST API service", cfg.AI.Description)
	assert.Equal(t, []string{"Requires Postgres locally"}, cfg.AI.Notes)
	require.Contains(t, cfg.AI.Tasks, "add-endpoint")
	assert.Len(t, cfg.AI.Tasks["add-endpoint"].Steps, 2)
}

func TestLoadPackageConfig_MinimalConfig(t *testing.T) {
	yaml := `
package:
  name: simple-svc
  version: 1.0.0

phases:
  build:
    commands:
      - npm install
      - npx expo export
  test:
    commands:
      - npx jest --ci
`
	path := writeTempFile(t, "takumi-pkg.yaml", yaml)
	cfg, err := LoadPackageConfig(path)

	require.NoError(t, err)
	assert.Equal(t, "simple-svc", cfg.Package.Name)
	assert.Nil(t, cfg.Dependencies)
	assert.Nil(t, cfg.Runtime)
	assert.Nil(t, cfg.AI)
	assert.Len(t, cfg.Phases, 2)
}

func TestLoadPackageConfig_NoPhasesOrDeps(t *testing.T) {
	yaml := `
package:
  name: bare
  version: 0.0.1
`
	path := writeTempFile(t, "takumi-pkg.yaml", yaml)
	cfg, err := LoadPackageConfig(path)

	require.NoError(t, err)
	assert.Equal(t, "bare", cfg.Package.Name)
	assert.Equal(t, "0.0.1", cfg.Package.Version)
	assert.Nil(t, cfg.Phases)
	assert.Nil(t, cfg.Dependencies)
}

func TestLoadPackageConfig_FileNotFound(t *testing.T) {
	_, err := LoadPackageConfig("/nonexistent/takumi-pkg.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading package config")
}

func TestLoadPackageConfig_InvalidYAML(t *testing.T) {
	path := writeTempFile(t, "takumi-pkg.yaml", "{{invalid}}")
	_, err := LoadPackageConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing package config")
}

func TestDefaultPackageConfig(t *testing.T) {
	cfg := DefaultPackageConfig("my-svc")

	assert.Equal(t, "my-svc", cfg.Package.Name)
	assert.Equal(t, "0.1.0", cfg.Package.Version)
	assert.Nil(t, cfg.Dependencies)
	assert.Nil(t, cfg.Runtime)
	assert.Nil(t, cfg.AI)
	require.Contains(t, cfg.Phases, "build")
	require.Contains(t, cfg.Phases, "test")
	assert.Len(t, cfg.Phases["build"].Commands, 1)
	assert.Len(t, cfg.Phases["test"].Commands, 1)
}

func TestPackageConfig_MarshalRoundtrip(t *testing.T) {
	original := &PackageConfig{
		Package: PackageMeta{
			Name:    "roundtrip-pkg",
			Version: "2.0.0",
		},
		Dependencies: []string{"dep-a", "dep-b"},
		Runtime: &Runtime{
			Setup: []string{"setup-cmd"},
			Env:   map[string]string{"FOO": "bar"},
		},
		Phases: map[string]*Phase{
			"build": {
				Pre:      []string{"pre-cmd"},
				Commands: []string{"build-cmd"},
				Post:     []string{"post-cmd"},
			},
		},
	}

	data, err := original.Marshal()
	require.NoError(t, err)

	path := writeTempFile(t, "takumi-pkg.yaml", string(data))
	loaded, err := LoadPackageConfig(path)
	require.NoError(t, err)

	assert.Equal(t, original.Package.Name, loaded.Package.Name)
	assert.Equal(t, original.Package.Version, loaded.Package.Version)
	assert.Equal(t, original.Dependencies, loaded.Dependencies)
	assert.Equal(t, original.Runtime.Setup, loaded.Runtime.Setup)
	assert.Equal(t, original.Runtime.Env["FOO"], loaded.Runtime.Env["FOO"])
	assert.Equal(t, original.Phases["build"].Pre, loaded.Phases["build"].Pre)
	assert.Equal(t, original.Phases["build"].Commands, loaded.Phases["build"].Commands)
	assert.Equal(t, original.Phases["build"].Post, loaded.Phases["build"].Post)
}

func TestPackageConfig_MarshalOmitsEmptyFields(t *testing.T) {
	cfg := DefaultPackageConfig("sparse")
	data, err := cfg.Marshal()
	require.NoError(t, err)

	yaml := string(data)
	assert.NotContains(t, yaml, "dependencies")
	assert.NotContains(t, yaml, "runtime")
	assert.NotContains(t, yaml, "ai")
}
