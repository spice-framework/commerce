package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const validContract = `{
  "schema": 1,
  "module": "github.com/spice-framework/spice",
  "minimum": "v0.0.0-20260101000000-111111111111",
  "current": "v0.0.0-20260201000000-222222222222",
  "tools": [
    "github.com/spice-framework/spice/cmd/spice",
    "github.com/spice-framework/spice/cmd/spice-annotation-core"
  ]
}`

func TestReadContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "valid", content: validContract},
		{
			name:    "unknown field",
			content: strings.Replace(validContract, `"schema": 1,`, `"schema": 1, "future": true,`, 1),
			wantErr: "unknown field",
		},
		{name: "trailing value", content: validContract + `{}`, wantErr: "trailing JSON values"},
		{
			name:    "unsupported schema",
			content: strings.Replace(validContract, `"schema": 1`, `"schema": 2`, 1),
			wantErr: "schema 2 is unsupported",
		},
		{
			name:    "whitespace",
			content: strings.Replace(validContract, `"minimum": "v`, `"minimum": " v`, 1),
			wantErr: "must not contain surrounding whitespace",
		},
		{
			name:    "equal versions",
			content: strings.Replace(validContract, "v0.0.0-20260201000000-222222222222", "v0.0.0-20260101000000-111111111111", 1),
			wantErr: "minimum and current versions must differ",
		},
		{
			name:    "wrong module",
			content: strings.Replace(validContract, spiceModulePath, "example.com/not-spice", 1),
			wantErr: "require " + spiceModulePath,
		},
		{
			name: "duplicate tool",
			content: strings.Replace(
				validContract,
				annotationToolPath,
				spiceToolPath,
				1,
			),
			wantErr: "is duplicated",
		},
		{
			name: "undeclared tool",
			content: strings.Replace(
				validContract,
				annotationToolPath,
				spiceModulePath+"/cmd/unknown",
				1,
			),
			wantErr: "require",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(root, compatibilityFile),
				[]byte(test.content),
				0o600,
			); err != nil {
				t.Fatalf("write contract fixture: %v", err)
			}
			got, err := readContract(root)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("readContract() error = %v", err)
				}
				if got.Module != spiceModulePath || len(got.Tools) != 2 {
					t.Fatalf("readContract() = %#v", got)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("readContract() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestCompatibilityBoundaries(t *testing.T) {
	t.Parallel()
	contract := compatibilityContract{Minimum: "minimum", Current: "current"}
	tests := []struct {
		line    string
		want    []compatibilityBoundary
		wantErr string
	}{
		{line: "minimum", want: []compatibilityBoundary{{Name: "minimum", Version: "minimum"}}},
		{line: "current", want: []compatibilityBoundary{{Name: "current", Version: "current"}}},
		{
			line: "all",
			want: []compatibilityBoundary{
				{Name: "minimum", Version: "minimum"},
				{Name: "current", Version: "current"},
			},
		},
		{line: "latest", wantErr: "minimum, current, or all"},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			t.Parallel()
			got, err := contract.boundaries(test.line)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("boundaries(%q) error = %v, want containing %q", test.line, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("boundaries(%q) error = %v", test.line, err)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("boundaries(%q) = %#v, want %#v", test.line, got, test.want)
			}
		})
	}
}

func TestValidateModuleContract(t *testing.T) {
	t.Parallel()
	contract := compatibilityContract{
		Module:  spiceModulePath,
		Minimum: "v0.1.0",
		Tools:   []string{spiceToolPath, annotationToolPath},
	}
	slices.Sort(contract.Tools)
	metadata := moduleMetadata{}
	metadata.Require = append(metadata.Require, struct {
		Path     string
		Version  string
		Indirect bool
	}{Path: spiceModulePath, Version: contract.Minimum})
	for _, path := range contract.Tools {
		metadata.Tool = append(metadata.Tool, struct{ Path string }{Path: path})
	}
	if err := validateModuleContract(contract, metadata); err != nil {
		t.Fatalf("validateModuleContract() error = %v", err)
	}

	indirect := metadata
	indirect.Require = slices.Clone(metadata.Require)
	indirect.Require[0].Indirect = true
	if err := validateModuleContract(contract, indirect); err == nil || !strings.Contains(err.Error(), "directly require") {
		t.Fatalf("validateModuleContract(indirect) error = %v", err)
	}

	wrongVersion := metadata
	wrongVersion.Require = slices.Clone(metadata.Require)
	wrongVersion.Require[0].Version = "v0.2.0"
	if err := validateModuleContract(contract, wrongVersion); err == nil || !strings.Contains(err.Error(), "compatibility minimum") {
		t.Fatalf("validateModuleContract(wrong version) error = %v", err)
	}

	missingTool := metadata
	missingTool.Tool = slices.Clone(metadata.Tool[:1])
	if err := validateModuleContract(contract, missingTool); err == nil || !strings.Contains(err.Error(), "compatibility tools") {
		t.Fatalf("validateModuleContract(missing tool) error = %v", err)
	}
}

func TestRepositoryStateTracksGeneratedAndVendorFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, directory := range []string{".git", ".spice", "internal/spicegen/commerce", "vendor/example.com/dependency"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(directory)), 0o750); err != nil {
			t.Fatalf("create fixture directory %q: %v", directory, err)
		}
	}
	fixtures := map[string]string{
		"main.go":                       "package main\n",
		".spice/commerce.manifest.json": "{}\n",
		"internal/spicegen/commerce/spice_assembly_gen.go": "package commerce\n",
		"vendor/example.com/dependency/value.go":           "package dependency\n",
		".git/index":                                       "ignored metadata",
	}
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture %q: %v", name, err)
		}
	}
	before, err := repositoryState(root)
	if err != nil {
		t.Fatalf("repositoryState() error = %v", err)
	}
	if _, exists := before[".git/index"]; exists {
		t.Fatal("repositoryState() included Git metadata")
	}
	for _, expected := range []string{
		".spice/commerce.manifest.json",
		"internal/spicegen/commerce/spice_assembly_gen.go",
		"vendor/example.com/dependency/value.go",
	} {
		if _, exists := before[expected]; !exists {
			t.Fatalf("repositoryState() omitted %q", expected)
		}
	}
	if writeErr := os.WriteFile(
		filepath.Join(root, "internal", "spicegen", "commerce", "spice_assembly_gen.go"),
		[]byte("package changed\n"),
		0o600,
	); writeErr != nil {
		t.Fatalf("modify generated fixture: %v", writeErr)
	}
	after, err := repositoryState(root)
	if err != nil {
		t.Fatalf("repositoryState() after modification error = %v", err)
	}
	if mapsEqual(before, after) {
		t.Fatal("repositoryState() did not detect generated modification")
	}
}

func TestCopyApplicationExcludesGitAndCommittedVendor(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	destination := t.TempDir()
	for _, directory := range []string{".git", "internal/spicegen/commerce", "vendor/example.com/dependency"} {
		if err := os.MkdirAll(filepath.Join(source, filepath.FromSlash(directory)), 0o750); err != nil {
			t.Fatalf("create source directory %q: %v", directory, err)
		}
	}
	fixtures := map[string]string{
		"go.mod": "module example.com/application\n",
		"internal/spicegen/commerce/spice_assembly_gen.go": "package commerce\n",
		"vendor/example.com/dependency/value.go":           "package dependency\n",
		".git/index":                                       "metadata",
	}
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatalf("write source fixture %q: %v", name, err)
		}
	}
	if err := copyApplication(source, destination); err != nil {
		t.Fatalf("copyApplication() error = %v", err)
	}
	for _, expected := range []string{"go.mod", "internal/spicegen/commerce/spice_assembly_gen.go"} {
		if _, err := os.Stat(filepath.Join(destination, filepath.FromSlash(expected))); err != nil {
			t.Fatalf("copied file %q: %v", expected, err)
		}
	}
	for _, excluded := range []string{".git", "vendor"} {
		if _, err := os.Stat(filepath.Join(destination, excluded)); !os.IsNotExist(err) {
			t.Fatalf("excluded directory %q stat error = %v, want not exist", excluded, err)
		}
	}
}
