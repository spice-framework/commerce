// Command qualitygate runs Commerce's repository-owned cross-platform checks.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	requiredGoVersion = "go1.26.5"
	modulePath        = "github.com/spice-framework/commerce"
	legacyModulePath  = "github.com/spice-framework/spice/examples/" + "commerce"
	minimumCoverage   = 85.0
	spiceTool         = "github.com/spice-framework/spice/cmd/spice"
)

var output = log.New(os.Stdout, "", 0)

func main() {
	os.Exit(execute()) // Entrypoint exception: propagate verification failure.
}

func execute() int {
	mode := flag.String(
		"mode",
		"verify",
		"verification mode: check, fmt, lint, security, smoke, test, or verify",
	)
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	root, err := repositoryRoot()
	if err == nil {
		err = run(ctx, root, *mode)
	}
	if err != nil {
		output.Printf("quality gate failed: %v", err)
		return 1
	}
	return 0
}

type verificationStep struct {
	name string
	run  func() error
}

func run(ctx context.Context, root, mode string) error {
	if err := checkGoVersion(); err != nil {
		return err
	}
	steps, err := stepsForMode(ctx, root, mode)
	if err != nil {
		return err
	}
	return runSequential(steps)
}

func stepsForMode(ctx context.Context, root, mode string) ([]verificationStep, error) {
	identity := verificationStep{"repository identity", func() error { return checkIdentity(root) }}
	formatting := verificationStep{"formatting", func() error { return format(ctx, root, false) }}
	modules := verificationStep{"module and vendor", func() error { return checkModule(ctx, root) }}
	vet := verificationStep{
		"go vet",
		func() error { return command(ctx, root, nil, "go", "vet", "./...") },
	}
	lintStep := verificationStep{"lint and nil safety", func() error { return lint(ctx, root) }}
	securityStep := verificationStep{"security", func() error { return security(ctx, root) }}
	testStep := verificationStep{"shuffled and race tests", func() error { return tests(ctx, root) }}
	coverageStep := verificationStep{"business coverage", func() error { return coverage(ctx, root) }}
	offlineStep := verificationStep{"offline vendor", func() error { return offline(ctx, root) }}
	spiceStep := verificationStep{"Spice application", func() error { return spiceApplication(ctx, root) }}

	switch mode {
	case "check":
		return []verificationStep{identity, formatting, modules, vet}, nil
	case "fmt":
		return []verificationStep{{"formatting", func() error { return format(ctx, root, true) }}}, nil
	case "lint":
		return []verificationStep{lintStep}, nil
	case "security":
		return []verificationStep{securityStep}, nil
	case "smoke":
		return []verificationStep{spiceStep}, nil
	case "test":
		return []verificationStep{testStep, coverageStep}, nil
	case "verify":
		return []verificationStep{
			identity,
			formatting,
			modules,
			vet,
			lintStep,
			securityStep,
			testStep,
			coverageStep,
			offlineStep,
			spiceStep,
		}, nil
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
}

func runSequential(steps []verificationStep) error {
	for _, step := range steps {
		started := time.Now()
		output.Printf("==> %s", step.name)
		if err := step.run(); err != nil {
			return fmt.Errorf(
				"%s (%s): %w",
				step.name,
				time.Since(started).Round(time.Millisecond),
				err,
			)
		}
		output.Printf("<== %s passed in %s", step.name, time.Since(started).Round(time.Millisecond))
	}
	output.Print("==> all verification passed")
	return nil
}

func checkGoVersion() error {
	if runtime.Version() != requiredGoVersion {
		return fmt.Errorf(
			"go version is %s; require exactly %s",
			runtime.Version(),
			requiredGoVersion,
		)
	}
	return nil
}

func checkIdentity(root string) error {
	opened, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open repository root: %w", err)
	}
	defer closeRoot(opened, root)
	goMod, err := opened.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("read go.mod: %w", err)
	}
	content := string(goMod)
	if !hasModuleDirective(content, modulePath) {
		return fmt.Errorf("go.mod does not declare canonical module %s", modulePath)
	}
	if strings.Contains(content, "\nreplace ") || strings.Contains(content, "\nreplace (") {
		return errors.New("committed Commerce go.mod must not contain replace directives")
	}
	return fs.WalkDir(opened.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != "." && slices.Contains(
			[]string{".git", "tools", "vendor"},
			entry.Name(),
		) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !identityFile(entry.Name()) {
			return nil
		}
		data, readErr := opened.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read identity file %q: %w", path, readErr)
		}
		if bytes.Contains(data, []byte(legacyModulePath)) {
			return fmt.Errorf("legacy monorepo module path remains in %s", path)
		}
		return nil
	})
}

func closeRoot(opened *os.Root, root string) {
	if err := opened.Close(); err != nil {
		output.Printf("warning: close root %q: %v", root, err)
	}
}

func hasModuleDirective(content, expected string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == "module "+expected {
			return true
		}
	}
	return false
}

func identityFile(name string) bool {
	switch filepath.Ext(name) {
	case ".go", ".json", ".md", ".mod", ".sum", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func format(ctx context.Context, root string, write bool) error {
	files, err := goFiles(root)
	if err != nil {
		return err
	}
	goimports, err := toolPath(ctx, root, "goimports")
	if err != nil {
		return err
	}
	gofumpt, err := toolPath(ctx, root, "gofumpt")
	if err != nil {
		return err
	}
	if write {
		if err := fileBatches(ctx, root, goimports, "-w", files); err != nil {
			return err
		}
		return fileBatches(ctx, root, gofumpt, "-w", files)
	}
	if err := checkFormatted(ctx, root, goimports, files); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}
	if err := checkFormatted(ctx, root, gofumpt, files); err != nil {
		return fmt.Errorf("gofumpt: %w", err)
	}
	return nil
}

func goFiles(root string) ([]string, error) {
	var result []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != root && slices.Contains(
			[]string{".git", "tools", "vendor"},
			entry.Name(),
		) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			result = append(result, path)
		}
		return nil
	})
	slices.Sort(result)
	return result, err
}

func checkFormatted(ctx context.Context, root, executable string, files []string) error {
	var changed []string
	for _, batch := range batches(files, 64) {
		stdout, err := capture(ctx, root, nil, executable, append([]string{"-l"}, batch...)...)
		if err != nil {
			return err
		}
		changed = append(changed, strings.Fields(stdout)...)
	}
	if len(changed) != 0 {
		return fmt.Errorf("files require formatting: %s", strings.Join(changed, ", "))
	}
	return nil
}

func fileBatches(ctx context.Context, root, executable, option string, files []string) error {
	for _, batch := range batches(files, 64) {
		if err := command(ctx, root, nil, executable, append([]string{option}, batch...)...); err != nil {
			return err
		}
	}
	return nil
}

func batches(values []string, size int) [][]string {
	var result [][]string
	for len(values) != 0 {
		length := min(size, len(values))
		result = append(result, values[:length])
		values = values[length:]
	}
	return result
}

func checkModule(ctx context.Context, root string) error {
	if err := command(ctx, root, nil, "go", "mod", "tidy", "-diff"); err != nil {
		return err
	}
	if err := command(ctx, root, nil, "go", "-C", "tools", "mod", "tidy", "-diff"); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "spice-commerce-vendor-")
	if err != nil {
		return fmt.Errorf("create vendor comparison directory: %w", err)
	}
	defer removeTree(temporary)
	candidate := filepath.Join(temporary, "vendor")
	if vendorErr := command(ctx, root, nil, "go", "mod", "vendor", "-o", candidate); vendorErr != nil {
		return vendorErr
	}
	current, err := treeDigests(filepath.Join(root, "vendor"))
	if err != nil {
		return err
	}
	expected, err := treeDigests(candidate)
	if err != nil {
		return err
	}
	if !equalDigests(current, expected) {
		return errors.New("vendor differs from a fresh go mod vendor result")
	}
	return nil
}

func removeTree(path string) {
	if err := os.RemoveAll(path); err != nil {
		output.Printf("warning: remove temporary tree %q: %v", path, err)
	}
}

func treeDigests(root string) (map[string][sha256.Size]byte, error) {
	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open digest root %q: %w", root, err)
	}
	defer closeRoot(opened, root)
	result := make(map[string][sha256.Size]byte)
	err = fs.WalkDir(opened.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("vendor contains symbolic link %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		content, readErr := opened.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		result[filepath.ToSlash(path)] = sha256.Sum256(content)
		return nil
	})
	return result, err
}

func equalDigests(left, right map[string][sha256.Size]byte) bool {
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

func lint(ctx context.Context, root string) error {
	golangci, err := toolPath(ctx, root, "golangci-lint")
	if err != nil {
		return err
	}
	if lintErr := command(ctx, root, nil, golangci, "run", "--timeout=10m"); lintErr != nil {
		return lintErr
	}
	nilaway, err := toolPath(ctx, root, "nilaway")
	if err != nil {
		return err
	}
	return command(ctx, root, nil, nilaway, "-include-pkgs="+modulePath, "./...")
}

func security(ctx context.Context, root string) error {
	gosec, err := toolPath(ctx, root, "gosec")
	if err != nil {
		return err
	}
	if securityErr := command(ctx, root, nil, gosec, "-quiet", "-exclude-generated", "./..."); securityErr != nil {
		return securityErr
	}
	govulncheck, err := toolPath(ctx, root, "govulncheck")
	if err != nil {
		return err
	}
	return command(ctx, root, nil, govulncheck, "./...")
}

func tests(ctx context.Context, root string) error {
	if err := command(ctx, root, nil, "go", "test", "-shuffle=on", "-count=1", "./..."); err != nil {
		return err
	}
	return command(ctx, root, nil, "go", "test", "-race", "-count=1", "./...")
}

func coverage(ctx context.Context, root string) (returnErr error) {
	profile, err := os.CreateTemp("", "spice-commerce-coverage-*.out")
	if err != nil {
		return fmt.Errorf("create coverage profile: %w", err)
	}
	profilePath := profile.Name()
	if closeErr := profile.Close(); closeErr != nil {
		return fmt.Errorf("close coverage profile: %w", closeErr)
	}
	defer func() {
		returnErr = errors.Join(returnErr, os.Remove(profilePath))
	}()
	packages := []string{"inventory", "notifications", "orders", "payments", "platform", "storage"}
	covered := make([]string, len(packages))
	tests := make([]string, len(packages))
	for index, name := range packages {
		covered[index] = modulePath + "/" + name
		tests[index] = "./" + name
	}
	arguments := []string{
		"test",
		"-covermode=atomic",
		"-coverpkg=" + strings.Join(covered, ","),
		"-coverprofile=" + profilePath,
	}
	arguments = append(arguments, tests...)
	if coverageErr := command(ctx, root, nil, "go", arguments...); coverageErr != nil {
		return coverageErr
	}
	stdout, err := capture(ctx, root, nil, "go", "tool", "cover", "-func="+profilePath)
	if err != nil {
		return err
	}
	percentage, err := parseTotalCoverage(stdout)
	if err != nil {
		return err
	}
	output.Printf("business coverage %.1f%% (minimum %.1f%%)", percentage, minimumCoverage)
	if percentage < minimumCoverage {
		return fmt.Errorf("business coverage %.1f%% is below %.1f%%", percentage, minimumCoverage)
	}
	return nil
}

func parseTotalCoverage(report string) (float64, error) {
	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return 0, errors.New("coverage report is empty")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) == 0 || !strings.HasSuffix(fields[len(fields)-1], "%") {
		return 0, errors.New("coverage report has no total percentage")
	}
	value := strings.TrimSuffix(fields[len(fields)-1], "%")
	percentage, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse coverage percentage %q: %w", value, err)
	}
	return percentage, nil
}

func offline(ctx context.Context, root string) error {
	environment := map[string]string{"GOFLAGS": "-mod=vendor"}
	if err := command(ctx, root, environment, "go", "test", "-count=1", "./..."); err != nil {
		return err
	}
	return command(ctx, root, environment, "go", "build", "-trimpath", "./...")
}

func spiceApplication(ctx context.Context, root string) error {
	environment := map[string]string{"GOFLAGS": "-mod=vendor"}
	commands := [][]string{
		{"verify", "."},
		{"generate", "--check", "--target", "Commerce", "."},
		{"build", "--target", "Commerce", "."},
		{"run", "--target", "Commerce", ".", "--", "-check"},
	}
	for _, arguments := range commands {
		if err := command(
			ctx,
			root,
			environment,
			"go",
			append([]string{"tool", spiceTool}, arguments...)...,
		); err != nil {
			return err
		}
	}
	return nil
}

func toolPath(ctx context.Context, root, name string) (string, error) {
	stdout, err := capture(ctx, root, nil, "go", "tool", "-C", "tools", "-n", name)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(stdout)
	if path == "" {
		return "", fmt.Errorf("resolve tool %q: empty path", name)
	}
	return path, nil
}

func repositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		opened, err := os.OpenRoot(current)
		if err != nil {
			return "", fmt.Errorf("open repository candidate %q: %w", current, err)
		}
		content, readErr := opened.ReadFile("go.mod")
		closeErr := opened.Close()
		if closeErr != nil {
			return "", fmt.Errorf("close repository candidate %q: %w", current, closeErr)
		}
		if readErr == nil && bytes.Contains(content, []byte("module "+modulePath)) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("find Commerce repository root: go.mod not found")
		}
		current = parent
	}
}

func command(
	ctx context.Context,
	directory string,
	environment map[string]string,
	executable string,
	arguments ...string,
) error {
	// #nosec G204,G702 -- executable and arguments are fixed repository-owned values.
	cmd := exec.CommandContext(ctx, executable, arguments...)
	cmd.Dir = directory
	cmd.Env = mergedEnvironment(environment)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", executable, strings.Join(arguments, " "), err)
	}
	return nil
}

func capture(
	ctx context.Context,
	directory string,
	environment map[string]string,
	executable string,
	arguments ...string,
) (string, error) {
	// #nosec G204,G702 -- executable and arguments are fixed repository-owned values.
	cmd := exec.CommandContext(ctx, executable, arguments...)
	cmd.Dir = directory
	cmd.Env = mergedEnvironment(environment)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(
			"%s %s: %w\n%s",
			executable,
			strings.Join(arguments, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return stdout.String(), nil
}

func mergedEnvironment(overrides map[string]string) []string {
	values := map[string]string{
		"GOWORK":      "off",
		"GOPROXY":     "off",
		"GOFLAGS":     "",
		"GOTOOLCHAIN": "local",
	}
	maps.Copy(values, overrides)
	result := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if _, replaced := values[strings.ToUpper(key)]; !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	slices.Sort(result)
	return result
}
