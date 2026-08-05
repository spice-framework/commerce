package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const validContract = `{
  "schema": 2,
  "core": {
    "module": "github.com/spice-framework/spice",
    "minimum": "v0.0.0-20260101000000-111111111111",
    "current": "v0.0.0-20260201000000-222222222222"
  },
  "toolchain": {
    "module": "github.com/spice-framework/toolchain",
    "minimum": "v0.0.0-20260301000000-333333333333",
    "current": "v0.0.0-20260401000000-444444444444",
    "tools": [
      "github.com/spice-framework/toolchain/cmd/spice",
      "github.com/spice-framework/toolchain/cmd/spice-annotation-core"
    ]
  }
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
			content: strings.Replace(validContract, `"schema": 2,`, `"schema": 2, "future": true,`, 1),
			wantErr: "unknown field",
		},
		{name: "trailing value", content: validContract + `{}`, wantErr: "trailing JSON values"},
		{
			name:    "unsupported schema",
			content: strings.Replace(validContract, `"schema": 2`, `"schema": 1`, 1),
			wantErr: "schema 1 is unsupported",
		},
		{
			name:    "whitespace",
			content: strings.Replace(validContract, `"minimum": "v`, `"minimum": " v`, 1),
			wantErr: "must not contain surrounding whitespace",
		},
		{
			name: "core-only current boundary",
			content: strings.Replace(
				validContract,
				`"current": "v0.0.0-20260401000000-444444444444"`,
				`"current": ""`,
				1,
			),
			wantErr: "explicit toolchain module, minimum, and current",
		},
		{
			name:    "wrong core module",
			content: strings.Replace(validContract, coreModulePath, "example.com/not-spice", 1),
			wantErr: "require " + coreModulePath,
		},
		{
			name:    "wrong toolchain module",
			content: strings.Replace(validContract, toolchainModulePath, "example.com/not-toolchain", 1),
			wantErr: "require " + toolchainModulePath,
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
				toolchainModulePath+"/cmd/unknown",
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
				if got.Core.Module != coreModulePath ||
					got.Toolchain.Module != toolchainModulePath ||
					len(got.Toolchain.Tools) != 2 {
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

func TestCompatibilityBoundariesCollapseTheInitialPublishedPair(t *testing.T) {
	t.Parallel()
	contract := compatibilityContract{
		Core: moduleCompatibility{Minimum: "core", Current: "core"},
		Toolchain: toolCompatibility{moduleCompatibility: moduleCompatibility{
			Minimum: "toolchain",
			Current: "toolchain",
		}},
	}
	got, err := contract.boundaries("all")
	if err != nil {
		t.Fatalf("boundaries(all) error = %v", err)
	}
	want := []compatibilityBoundary{{
		Name: "minimum/current", CoreVersion: "core", ToolchainVersion: "toolchain",
	}}
	if !slices.Equal(got, want) {
		t.Fatalf("boundaries(all) = %#v, want %#v", got, want)
	}
}

func TestCompatibilityBoundaries(t *testing.T) {
	t.Parallel()
	contract := compatibilityContract{
		Core: moduleCompatibility{Minimum: "core-minimum", Current: "core-current"},
		Toolchain: toolCompatibility{moduleCompatibility: moduleCompatibility{
			Minimum: "toolchain-minimum",
			Current: "toolchain-current",
		}},
	}
	tests := []struct {
		line    string
		want    []compatibilityBoundary
		wantErr string
	}{
		{
			line: "minimum",
			want: []compatibilityBoundary{{
				Name: "minimum", CoreVersion: "core-minimum", ToolchainVersion: "toolchain-minimum",
			}},
		},
		{
			line: "current",
			want: []compatibilityBoundary{{
				Name: "current", CoreVersion: "core-current", ToolchainVersion: "toolchain-current",
			}},
		},
		{
			line: "all",
			want: []compatibilityBoundary{
				{Name: "minimum", CoreVersion: "core-minimum", ToolchainVersion: "toolchain-minimum"},
				{Name: "current", CoreVersion: "core-current", ToolchainVersion: "toolchain-current"},
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

func TestCompatibilityPairMayAdvanceOneModuleIndependently(t *testing.T) {
	t.Parallel()
	base := compatibilityContract{
		Schema: compatibilityV2,
		Core: moduleCompatibility{
			Module:  "github.com/spice-framework/spice",
			Minimum: "v0.1.0",
			Current: "v0.2.0",
		},
		Toolchain: toolCompatibility{
			moduleCompatibility: moduleCompatibility{
				Module:  "github.com/spice-framework/toolchain",
				Minimum: "v0.3.0",
				Current: "v0.3.0",
			},
			Tools: []string{spiceToolPath, annotationToolPath},
		},
	}
	slices.Sort(base.Toolchain.Tools)
	if err := validateContract(base); err != nil {
		t.Fatalf("validateContract(core advance) error = %v", err)
	}
	base.Core.Current = base.Core.Minimum
	base.Toolchain.Current = "v0.4.0"
	if err := validateContract(base); err != nil {
		t.Fatalf("validateContract(toolchain advance) error = %v", err)
	}
}

func TestValidateModuleContract(t *testing.T) {
	t.Parallel()
	contract := compatibilityContract{
		Core: moduleCompatibility{
			Module:  coreModulePath,
			Minimum: "v0.1.0",
		},
		Toolchain: toolCompatibility{
			moduleCompatibility: moduleCompatibility{
				Module:  toolchainModulePath,
				Minimum: "v0.2.0",
			},
			Tools: []string{spiceToolPath, annotationToolPath},
		},
	}
	slices.Sort(contract.Toolchain.Tools)
	metadata := moduleMetadata{}
	metadata.Require = append(metadata.Require, []struct {
		Path     string
		Version  string
		Indirect bool
	}{
		{Path: coreModulePath, Version: contract.Core.Minimum},
		{Path: toolchainModulePath, Version: contract.Toolchain.Minimum, Indirect: true},
	}...)
	for _, path := range contract.Toolchain.Tools {
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
	wrongVersion.Require[1].Version = "v0.3.0"
	if err := validateModuleContract(contract, wrongVersion); err == nil || !strings.Contains(err.Error(), "toolchain minimum") {
		t.Fatalf("validateModuleContract(wrong version) error = %v", err)
	}

	missingToolchain := metadata
	missingToolchain.Require = slices.Clone(metadata.Require[:1])
	if err := validateModuleContract(contract, missingToolchain); err == nil || !strings.Contains(err.Error(), "must require "+toolchainModulePath) {
		t.Fatalf("validateModuleContract(missing toolchain) error = %v", err)
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
