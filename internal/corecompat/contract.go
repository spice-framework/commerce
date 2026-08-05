package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	compatibilityFile   = "spice-compatibility.json"
	compatibilityV2     = 2
	repositoryModule    = "github.com/spice-framework/commerce"
	coreModulePath      = "github.com/spice-framework/spice"
	toolchainModulePath = "github.com/spice-framework/toolchain"
	spiceToolPath       = toolchainModulePath + "/cmd/spice"
	annotationToolPath  = toolchainModulePath + "/cmd/spice-annotation-core"
)

type compatibilityContract struct {
	Schema    int                 `json:"schema"`
	Core      moduleCompatibility `json:"core"`
	Toolchain toolCompatibility   `json:"toolchain"`
}

type moduleCompatibility struct {
	Module  string `json:"module"`
	Minimum string `json:"minimum"`
	Current string `json:"current"`
}

type toolCompatibility struct {
	moduleCompatibility
	Tools []string `json:"tools"`
}

type compatibilityBoundary struct {
	Name             string
	CoreVersion      string
	ToolchainVersion string
}

func readContract(root string) (compatibilityContract, error) {
	content, err := os.ReadFile(filepath.Join(root, compatibilityFile)) // #nosec G304 -- root and filename are repository-owned.
	if err != nil {
		return compatibilityContract{}, fmt.Errorf("read %s: %w", compatibilityFile, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var result compatibilityContract
	if err := decoder.Decode(&result); err != nil {
		return compatibilityContract{}, fmt.Errorf("decode %s: %w", compatibilityFile, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return compatibilityContract{}, fmt.Errorf("%s has trailing JSON values", compatibilityFile)
		}
		return compatibilityContract{}, fmt.Errorf("decode trailing %s content: %w", compatibilityFile, err)
	}
	if validationErr := validateContract(result); validationErr != nil {
		return compatibilityContract{}, validationErr
	}
	return result, nil
}

func validateContract(result compatibilityContract) error {
	if result.Schema != compatibilityV2 {
		return fmt.Errorf("%s schema %d is unsupported", compatibilityFile, result.Schema)
	}
	if err := validateModuleLine("core", result.Core); err != nil {
		return err
	}
	if err := validateModuleLine("toolchain", result.Toolchain.moduleCompatibility); err != nil {
		return err
	}
	if result.Core.Module != coreModulePath {
		return fmt.Errorf("%s core module is %q; require %s", compatibilityFile, result.Core.Module, coreModulePath)
	}
	if result.Toolchain.Module != toolchainModulePath {
		return fmt.Errorf(
			"%s toolchain module is %q; require %s",
			compatibilityFile,
			result.Toolchain.Module,
			toolchainModulePath,
		)
	}
	for _, value := range result.Toolchain.Tools {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("%s values must not contain surrounding whitespace", compatibilityFile)
		}
	}
	if len(result.Toolchain.Tools) == 0 {
		return fmt.Errorf("%s requires at least one tool", compatibilityFile)
	}
	seen := make(map[string]struct{}, len(result.Toolchain.Tools))
	for _, tool := range result.Toolchain.Tools {
		if tool == "" {
			return fmt.Errorf("%s tool paths must not be empty", compatibilityFile)
		}
		if _, exists := seen[tool]; exists {
			return fmt.Errorf("%s tool %q is duplicated", compatibilityFile, tool)
		}
		seen[tool] = struct{}{}
	}
	if !slices.IsSorted(result.Toolchain.Tools) {
		return fmt.Errorf("%s tool paths must be sorted", compatibilityFile)
	}
	requiredTools := []string{spiceToolPath, annotationToolPath}
	slices.Sort(requiredTools)
	if !slices.Equal(result.Toolchain.Tools, requiredTools) {
		return fmt.Errorf(
			"%s tools are %v; require %v",
			compatibilityFile,
			result.Toolchain.Tools,
			requiredTools,
		)
	}
	return nil
}

func validateModuleLine(name string, line moduleCompatibility) error {
	if line.Module == "" || line.Minimum == "" || line.Current == "" {
		return fmt.Errorf(
			"%s requires explicit %s module, minimum, and current values",
			compatibilityFile,
			name,
		)
	}
	for _, value := range []string{line.Module, line.Minimum, line.Current} {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("%s values must not contain surrounding whitespace", compatibilityFile)
		}
	}
	return nil
}

func (contract compatibilityContract) boundaries(line string) ([]compatibilityBoundary, error) {
	switch line {
	case "minimum":
		return []compatibilityBoundary{{
			Name:             line,
			CoreVersion:      contract.Core.Minimum,
			ToolchainVersion: contract.Toolchain.Minimum,
		}}, nil
	case "current":
		return []compatibilityBoundary{{
			Name:             line,
			CoreVersion:      contract.Core.Current,
			ToolchainVersion: contract.Toolchain.Current,
		}}, nil
	case "all":
		minimum := compatibilityBoundary{
			Name:             "minimum",
			CoreVersion:      contract.Core.Minimum,
			ToolchainVersion: contract.Toolchain.Minimum,
		}
		current := compatibilityBoundary{
			Name:             "current",
			CoreVersion:      contract.Core.Current,
			ToolchainVersion: contract.Toolchain.Current,
		}
		if minimum.CoreVersion == current.CoreVersion &&
			minimum.ToolchainVersion == current.ToolchainVersion {
			minimum.Name = "minimum/current"
			return []compatibilityBoundary{minimum}, nil
		}
		return []compatibilityBoundary{minimum, current}, nil
	default:
		return nil, invalidLine(line)
	}
}

func run(ctx context.Context, root, line string) error {
	contract, err := readContract(root)
	if err != nil {
		return err
	}
	metadata, err := readModuleMetadata(ctx, root)
	if err != nil {
		return err
	}
	if validationErr := validateModuleContract(contract, metadata); validationErr != nil {
		return validationErr
	}
	boundaries, err := contract.boundaries(line)
	if err != nil {
		return err
	}
	for _, boundary := range boundaries {
		if boundaryErr := runBoundary(ctx, root, contract, boundary); boundaryErr != nil {
			return boundaryErr
		}
	}
	return nil
}

func runBoundary(
	ctx context.Context,
	root string,
	contract compatibilityContract,
	boundary compatibilityBoundary,
) (returnErr error) {
	before, err := repositoryState(root)
	if err != nil {
		return err
	}
	defer func() {
		after, stateErr := repositoryState(root)
		if stateErr != nil {
			returnErr = errors.Join(returnErr, stateErr)
			return
		}
		if !mapsEqual(before, after) {
			returnErr = errors.Join(returnErr, errors.New("compatibility verification modified repository contents"))
		}
	}()
	if prepareErr := prepareBoundary(ctx, root, contract, boundary); prepareErr != nil {
		return prepareErr
	}
	return verifyBoundary(ctx, root, contract, boundary)
}

type moduleMetadata struct {
	Require []struct {
		Path     string
		Version  string
		Indirect bool
	}
	Tool []struct {
		Path string
	}
}

func readModuleMetadata(ctx context.Context, root string) (moduleMetadata, error) {
	content, err := captureGo(ctx, root, offlineEnvironment(), "mod", "edit", "-json")
	if err != nil {
		return moduleMetadata{}, fmt.Errorf("read go.mod metadata: %w", err)
	}
	var result moduleMetadata
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return moduleMetadata{}, fmt.Errorf("decode go.mod metadata: %w", err)
	}
	return result, nil
}

func validateModuleContract(contract compatibilityContract, metadata moduleMetadata) error {
	for _, line := range []struct {
		name          string
		data          moduleCompatibility
		requireDirect bool
	}{
		{name: "core", data: contract.Core, requireDirect: true},
		{name: "toolchain", data: contract.Toolchain.moduleCompatibility},
	} {
		minimum := requirementVersion(metadata, line.data.Module, line.requireDirect)
		if minimum == "" {
			if line.requireDirect {
				return fmt.Errorf("go.mod must directly require %s at an exact version", line.data.Module)
			}
			return fmt.Errorf("go.mod must require %s at an exact version", line.data.Module)
		}
		if minimum != line.data.Minimum {
			return fmt.Errorf(
				"go.mod directly requires %s at %s; compatibility %s minimum is %s",
				line.data.Module,
				minimum,
				line.name,
				line.data.Minimum,
			)
		}
	}
	declaredTools := make([]string, 0, len(metadata.Tool))
	for _, tool := range metadata.Tool {
		declaredTools = append(declaredTools, tool.Path)
	}
	slices.Sort(declaredTools)
	if !slices.Equal(declaredTools, contract.Toolchain.Tools) {
		return fmt.Errorf(
			"go.mod tools are %v; compatibility tools are %v",
			declaredTools,
			contract.Toolchain.Tools,
		)
	}
	return nil
}

func requirementVersion(metadata moduleMetadata, module string, requireDirect bool) string {
	for _, requirement := range metadata.Require {
		if requirement.Path == module && (!requireDirect || !requirement.Indirect) {
			return requirement.Version
		}
	}
	return ""
}

func prepareBoundary(
	ctx context.Context,
	root string,
	contract compatibilityContract,
	boundary compatibilityBoundary,
) error {
	for _, selected := range []struct {
		name    string
		module  string
		version string
	}{
		{name: "core", module: contract.Core.Module, version: boundary.CoreVersion},
		{
			name:    "toolchain",
			module:  contract.Toolchain.Module,
			version: boundary.ToolchainVersion,
		},
	} {
		if err := resolveExactModule(ctx, root, boundary.Name, selected.name, selected.module, selected.version); err != nil {
			return err
		}
	}
	modfile, cleanup, err := alternateModfile(ctx, root, contract, boundary)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := goCommand(ctx, root, onlineEnvironment(), "mod", "download", "-modfile="+modfile); err != nil {
		return fmt.Errorf("prepare %s Spice graph: %w", boundary.Name, err)
	}
	return nil
}

func resolveExactModule(ctx context.Context, root, boundaryName, role, module, version string) error {
	content, err := captureGo(
		ctx,
		root,
		onlineEnvironment(),
		"list",
		"-mod=mod",
		"-m",
		"-json",
		module+"@"+version,
	)
	if err != nil {
		return fmt.Errorf("resolve %s %s version: %w", boundaryName, role, err)
	}
	var resolved struct {
		Path    string
		Version string
	}
	if decodeErr := json.Unmarshal([]byte(content), &resolved); decodeErr != nil {
		return fmt.Errorf("decode resolved %s %s version: %w", boundaryName, role, decodeErr)
	}
	if resolved.Path != module || resolved.Version != version {
		return fmt.Errorf(
			"%s %s version resolved as %s@%s; require exactly %s@%s",
			boundaryName,
			role,
			resolved.Path,
			resolved.Version,
			module,
			version,
		)
	}
	return nil
}

func verifyBoundary(
	ctx context.Context,
	root string,
	contract compatibilityContract,
	boundary compatibilityBoundary,
) error {
	modfile, cleanup, err := alternateModfile(ctx, root, contract, boundary)
	if err != nil {
		return err
	}
	defer cleanup()
	environment := offlineModfileEnvironment(modfile)
	for _, selected := range []struct {
		name    string
		module  string
		version string
	}{
		{name: "core", module: contract.Core.Module, version: boundary.CoreVersion},
		{
			name:    "toolchain",
			module:  contract.Toolchain.Module,
			version: boundary.ToolchainVersion,
		},
	} {
		if selectedErr := verifySelectedModule(ctx, root, environment, modfile, boundary.Name, selected.name, selected.module, selected.version); selectedErr != nil {
			return selectedErr
		}
	}
	if toolErr := verifyToolModules(ctx, root, environment, modfile, contract, boundary); toolErr != nil {
		return toolErr
	}
	packages, err := productPackages(ctx, root, environment, modfile)
	if err != nil {
		return err
	}
	output.Printf(
		"==> Commerce with %s Spice core %s and toolchain %s across %d product packages",
		boundary.Name,
		boundary.CoreVersion,
		boundary.ToolchainVersion,
		len(packages),
	)
	vetArguments := []string{"vet", "-mod=mod", "-modfile=" + modfile}
	vetArguments = append(vetArguments, packages...)
	if vetErr := goCommand(ctx, root, environment, vetArguments...); vetErr != nil {
		return vetErr
	}
	testArguments := []string{
		"test",
		"-mod=mod",
		"-modfile=" + modfile,
		"-race",
		"-shuffle=on",
		"-count=1",
	}
	testArguments = append(testArguments, packages...)
	if testErr := goCommand(ctx, root, environment, testArguments...); testErr != nil {
		return testErr
	}
	mirror, cleanupMirror, err := applicationMirror(ctx, root, modfile)
	if err != nil {
		return err
	}
	defer cleanupMirror()
	toolEnvironment := vendoredOfflineEnvironment()
	for _, arguments := range [][]string{
		{"verify", "."},
		{"generate", "--check", "--target", "Commerce", "."},
		{"build", "--target", "Commerce", "."},
	} {
		toolArguments := []string{"tool", spiceToolPath}
		toolArguments = append(toolArguments, arguments...)
		if err := goCommand(ctx, mirror, toolEnvironment, toolArguments...); err != nil {
			return err
		}
	}
	output.Printf(
		"<== Commerce %s Spice compatibility passed at core %s and toolchain %s",
		boundary.Name,
		boundary.CoreVersion,
		boundary.ToolchainVersion,
	)
	return nil
}

func verifySelectedModule(
	ctx context.Context,
	root string,
	environment []string,
	modfile string,
	boundaryName string,
	role string,
	module string,
	version string,
) error {
	selected, err := captureGo(
		ctx,
		root,
		environment,
		"list",
		"-mod=mod",
		"-modfile="+modfile,
		"-m",
		"-f={{.Version}}",
		module,
	)
	if err != nil {
		return fmt.Errorf("resolve %s %s MVS graph: %w", boundaryName, role, err)
	}
	if strings.TrimSpace(selected) != version {
		return fmt.Errorf(
			"%s MVS graph selected %s %q; require exactly %q",
			boundaryName,
			role,
			strings.TrimSpace(selected),
			version,
		)
	}
	return nil
}

func verifyToolModules(
	ctx context.Context,
	root string,
	environment []string,
	modfile string,
	contract compatibilityContract,
	boundary compatibilityBoundary,
) error {
	for _, tool := range contract.Toolchain.Tools {
		content, err := captureGo(
			ctx,
			root,
			environment,
			"list",
			"-mod=mod",
			"-modfile="+modfile,
			"-f={{.Name}} {{.Module.Path}} {{.Module.Version}}",
			tool,
		)
		if err != nil {
			return fmt.Errorf("resolve tool %s: %w", tool, err)
		}
		fields := strings.Fields(content)
		if len(fields) != 3 ||
			fields[0] != "main" ||
			fields[1] != contract.Toolchain.Module ||
			fields[2] != boundary.ToolchainVersion {
			return fmt.Errorf(
				"tool %s resolved as %q; require main package from %s@%s",
				tool,
				strings.TrimSpace(content),
				contract.Toolchain.Module,
				boundary.ToolchainVersion,
			)
		}
	}
	return nil
}

func productPackages(ctx context.Context, root string, environment []string, modfile string) ([]string, error) {
	content, err := captureGo(
		ctx,
		root,
		environment,
		"list",
		"-mod=mod",
		"-modfile="+modfile,
		"-f={{.ImportPath}}",
		"./...",
	)
	if err != nil {
		return nil, fmt.Errorf("list Commerce product packages: %w", err)
	}
	tooling := map[string]struct{}{
		repositoryModule + "/internal/corecompat":  {},
		repositoryModule + "/internal/qualitygate": {},
	}
	var result []string
	for candidate := range strings.FieldsSeq(content) {
		if _, excluded := tooling[candidate]; !excluded {
			result = append(result, candidate)
		}
	}
	slices.Sort(result)
	if len(result) == 0 {
		return nil, errors.New("compatibility graph contains no Commerce product packages")
	}
	return result, nil
}

func repositoryState(root string) (map[string][sha256.Size]byte, error) {
	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open compatibility repository root: %w", err)
	}
	defer func() {
		if closeErr := opened.Close(); closeErr != nil {
			output.Printf("warning: close compatibility repository root %q: %v", root, closeErr)
		}
	}()
	result := make(map[string][sha256.Size]byte)
	err = fs.WalkDir(opened.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		content, readErr := opened.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read compatibility state %q: %w", path, readErr)
		}
		result[filepath.ToSlash(path)] = sha256.Sum256(content)
		return nil
	})
	return result, err
}

func mapsEqual(left, right map[string][sha256.Size]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for path, digest := range left {
		if right[path] != digest {
			return false
		}
	}
	return true
}

func applicationMirror(ctx context.Context, root, modfile string) (string, func(), error) {
	mirror, err := os.MkdirTemp("", "spice-commerce-compat-application-*")
	if err != nil {
		return "", nil, fmt.Errorf("create compatibility application mirror: %w", err)
	}
	cleanup := func() {
		// #nosec G703 -- mirror is the exact path returned by os.MkdirTemp above.
		if removeErr := os.RemoveAll(mirror); removeErr != nil {
			output.Printf("warning: remove compatibility application mirror %q: %v", mirror, removeErr)
		}
	}
	if copyErr := copyApplication(root, mirror); copyErr != nil {
		cleanup()
		return "", nil, copyErr
	}
	modContent, err := os.ReadFile(modfile) // #nosec G304 -- modfile is created and owned by this process.
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("read alternate modfile: %w", err)
	}
	sumfile := strings.TrimSuffix(modfile, ".mod") + ".sum"
	sumContent, err := os.ReadFile(sumfile) // #nosec G304 -- sumfile is paired with the owned modfile.
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("read alternate sumfile: %w", err)
	}
	mirrorRoot, err := os.OpenRoot(mirror)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open compatibility application mirror: %w", err)
	}
	if err := mirrorRoot.WriteFile("go.mod", modContent, 0o600); err != nil {
		closeErr := mirrorRoot.Close()
		cleanup()
		return "", nil, errors.Join(fmt.Errorf("write mirror go.mod: %w", err), closeErr)
	}
	if err := mirrorRoot.WriteFile("go.sum", sumContent, 0o600); err != nil {
		closeErr := mirrorRoot.Close()
		cleanup()
		return "", nil, errors.Join(fmt.Errorf("write mirror go.sum: %w", err), closeErr)
	}
	if err := mirrorRoot.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close compatibility application mirror: %w", err)
	}
	if err := goCommand(ctx, mirror, offlineEnvironment(), "mod", "vendor"); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("materialize compatibility vendor: %w", err)
	}
	return mirror, cleanup, nil
}

func copyApplication(source, destination string) error {
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return fmt.Errorf("open source application root: %w", err)
	}
	defer func() {
		if closeErr := sourceRoot.Close(); closeErr != nil {
			output.Printf("warning: close source application root %q: %v", source, closeErr)
		}
	}()
	destinationRoot, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("open destination application root: %w", err)
	}
	defer func() {
		if closeErr := destinationRoot.Close(); closeErr != nil {
			output.Printf("warning: close destination application root %q: %v", destination, closeErr)
		}
	}()
	return fs.WalkDir(sourceRoot.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			if path == "." {
				return nil
			}
			if err := destinationRoot.MkdirAll(path, 0o750); err != nil {
				return fmt.Errorf("create mirror directory %q: %w", path, err)
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("application contains unsupported non-regular file %q", path)
		}
		content, readErr := sourceRoot.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read application file %q: %w", path, readErr)
		}
		if writeErr := destinationRoot.WriteFile(path, content, 0o600); writeErr != nil {
			return fmt.Errorf("write mirror file %q: %w", path, writeErr)
		}
		return nil
	})
}

func alternateModfile(
	ctx context.Context,
	root string,
	contract compatibilityContract,
	boundary compatibilityBoundary,
) (string, func(), error) {
	productMod, err := os.ReadFile(filepath.Join(root, "go.mod")) // #nosec G304 -- root is repository-owned.
	if err != nil {
		return "", nil, fmt.Errorf("read product go.mod: %w", err)
	}
	productSum, err := os.ReadFile(filepath.Join(root, "go.sum")) // #nosec G304 -- root is repository-owned.
	if err != nil {
		return "", nil, fmt.Errorf("read product go.sum: %w", err)
	}
	file, err := os.CreateTemp("", "spice-commerce-compat-*.mod")
	if err != nil {
		return "", nil, fmt.Errorf("create compatibility modfile: %w", err)
	}
	modfile := file.Name()
	sumfile := strings.TrimSuffix(modfile, ".mod") + ".sum"
	cleanup := func() {
		for _, path := range []string{modfile, sumfile} {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				output.Printf("warning: remove compatibility file %q: %v", path, removeErr)
			}
		}
	}
	if _, err := file.Write(productMod); err != nil {
		closeErr := file.Close()
		cleanup()
		return "", nil, errors.Join(fmt.Errorf("write compatibility modfile: %w", err), closeErr)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close compatibility modfile: %w", err)
	}
	// #nosec G703 -- sumfile is derived only from the path returned by os.CreateTemp above.
	if err := os.WriteFile(sumfile, productSum, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write compatibility sumfile: %w", err)
	}
	arguments := []string{
		"mod",
		"edit",
		"-modfile=" + modfile,
		"-require=" + contract.Core.Module + "@" + boundary.CoreVersion,
		"-require=" + contract.Toolchain.Module + "@" + boundary.ToolchainVersion,
	}
	if err := goCommand(ctx, root, offlineEnvironment(), arguments...); err != nil {
		cleanup()
		return "", nil, err
	}
	return modfile, cleanup, nil
}

func repositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		content, readErr := os.ReadFile(filepath.Join(current, "go.mod")) // #nosec G304 -- candidates are bounded ancestors.
		if readErr == nil && bytes.Contains(content, []byte("module "+repositoryModule)) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("find Commerce repository root: go.mod not found")
		}
		current = parent
	}
}

func goCommand(ctx context.Context, directory string, environment []string, arguments ...string) error {
	// #nosec G204,G702 -- arguments and module versions are repository-owned values.
	cmd := exec.CommandContext(ctx, "go", arguments...)
	cmd.Dir = directory
	cmd.Env = environment
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s: %w", strings.Join(arguments, " "), err)
	}
	return nil
}

func captureGo(
	ctx context.Context,
	directory string,
	environment []string,
	arguments ...string,
) (string, error) {
	// #nosec G204,G702 -- arguments and module versions are repository-owned values.
	cmd := exec.CommandContext(ctx, "go", arguments...)
	cmd.Dir = directory
	cmd.Env = environment
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go %s: %w\n%s", strings.Join(arguments, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func offlineEnvironment() []string {
	return environment(map[string]string{
		"GOFLAGS":     "-mod=mod",
		"GOPROXY":     "off",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
}

func offlineModfileEnvironment(modfile string) []string {
	return environment(map[string]string{
		"GOFLAGS":     "-mod=mod -modfile=" + modfile,
		"GOPROXY":     "off",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
}

func vendoredOfflineEnvironment() []string {
	return environment(map[string]string{
		"GOFLAGS":     "-mod=vendor",
		"GOPROXY":     "off",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
}

func onlineEnvironment() []string {
	return environment(map[string]string{
		"GOFLAGS":     "",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
}

func environment(overrides map[string]string) []string {
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[strings.ToUpper(key)]; !replaced {
				result = append(result, entry)
			}
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	slices.Sort(result)
	return result
}
