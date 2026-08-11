package semantic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var semanticBuildKeys = []string{
	"CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_ENABLED", "CGO_LDFLAGS",
	"CC", "CXX", "GO111MODULE", "GOARCH", "GOCOMPILER", "GOEXPERIMENT",
	"GOFLAGS", "GOOS", "GOTOOLCHAIN",
}

func semanticToolchain(ctx context.Context, root string, environment, flags, patterns []string) (ToolchainEvidence, error) {
	goTool, err := executableIdentity(ctx, "go", environment)
	if err != nil {
		return ToolchainEvidence{}, err
	}
	work, err := selectedWorkIdentity(ctx, goTool.Path, root, environment)
	if err != nil {
		return ToolchainEvidence{}, err
	}
	environmentHash, err := semanticBuildDigest(ctx, goTool.Path, root, environment, flags, patterns, work)
	if err != nil {
		return ToolchainEvidence{}, err
	}
	modules, err := semanticModuleGraphDigest(ctx, goTool.Path, root, environment, work)
	if err != nil {
		return ToolchainEvidence{}, err
	}
	return ToolchainEvidence{Go: goTool, Loader: packagesAuthority, BuildEnvSHA256: environmentHash, ModuleGraphSHA256: modules}, nil
}

func executableIdentity(ctx context.Context, name string, environment []string) (ExecutableIdentity, error) {
	path, err := executableFromEnvironment(name, environment)
	if err != nil {
		return ExecutableIdentity{}, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("resolve %s: %w", name, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("read %s: %w", name, err)
	}
	digest := sha256.Sum256(data)
	command := exec.CommandContext(ctx, path, "version")
	command.Env = cloneStrings(environment)
	stdout, err := command.Output()
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("%s version: %w", name, err)
	}
	version := strings.TrimSpace(string(stdout))
	if version == "" || strings.Contains(version, "\n") {
		return ExecutableIdentity{}, fmt.Errorf("%s version returned non-single-line output", name)
	}
	return ExecutableIdentity{Path: path, SHA256: hex.EncodeToString(digest[:]), Version: version}, nil
}

func executableFromEnvironment(name string, environment []string) (string, error) {
	path := semanticEnvironmentValues(environment)["PATH"]
	if path == "" {
		return "", fmt.Errorf("locate %s: frozen environment has no PATH", name)
	}
	for _, directory := range filepath.SplitList(path) {
		candidate := filepath.Join(directory, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("locate %s in frozen PATH", name)
}

func selectedWorkIdentity(ctx context.Context, goExecutable, root string, environment []string) (string, error) {
	command := exec.CommandContext(ctx, goExecutable, "env", "GOWORK")
	command.Dir, command.Env = root, cloneStrings(environment)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read selected Go workfile: %w", err)
	}
	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return "none", nil
	}
	if selected == "off" {
		return "off", nil
	}
	path, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return "", fmt.Errorf("resolve selected Go workfile: %w", err)
	}
	if !pathInsideRoot(root, path) {
		return "", fmt.Errorf("selected Go workfile escapes semantic root: %s", selected)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read selected Go workfile: %w", err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func semanticBuildDigest(ctx context.Context, goExecutable, root string, environment, flags, patterns []string, work string) (string, error) {
	arguments := append([]string{"env", "-json"}, semanticBuildKeys...)
	command := exec.CommandContext(ctx, goExecutable, arguments...)
	command.Dir, command.Env = root, cloneStrings(environment)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read effective Go build configuration: %w", err)
	}
	values := map[string]string{}
	if err := json.Unmarshal(output, &values); err != nil {
		return "", fmt.Errorf("decode effective Go build configuration: %w", err)
	}
	lines := make([]string, 0, len(semanticBuildKeys)+len(flags)+len(patterns)+4)
	for _, key := range semanticBuildKeys {
		value, exists := values[key]
		if !exists {
			return "", fmt.Errorf("effective Go build configuration omits %s", key)
		}
		lines = append(lines, key+"="+value)
	}
	lines = append(lines, "GOPACKAGESDRIVER=off", "GOWORK="+work, "tests=true", "mode=need-name,files,compiled-files,syntax,types,types-info,module,imports")
	for _, flag := range flags {
		lines = append(lines, "build-flag="+flag)
	}
	for _, pattern := range patterns {
		lines = append(lines, "pattern="+pattern)
	}
	return digestSemanticLines(lines), nil
}

func semanticModuleGraphDigest(ctx context.Context, goExecutable, root string, environment []string, work string) (string, error) {
	command := exec.CommandContext(ctx, goExecutable, "list", "-m", "-json", "all")
	command.Dir, command.Env = root, cloneStrings(environment)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read effective Go module graph: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	modules := make([]map[string]any, 0)
	for {
		module := map[string]any{}
		err := decoder.Decode(&module)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode effective Go module graph: %w", err)
		}
		if len(module) == 0 {
			return "", fmt.Errorf("effective Go module graph contains an empty module")
		}
		if err := normalizeSemanticModule(root, module); err != nil {
			return "", err
		}
		modules = append(modules, module)
	}
	if len(modules) == 0 {
		return "", fmt.Errorf("effective Go module graph is empty")
	}
	payload, err := json.Marshal(struct {
		Modules []map[string]any
		Work    string
	}{Modules: modules, Work: work})
	if err != nil {
		return "", fmt.Errorf("canonicalize effective Go module graph: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeSemanticModule(root string, module map[string]any) error {
	if main, _ := module["Main"].(bool); main {
		if directory, _ := module["Dir"].(string); directory != "" && !pathInsideRoot(root, directory) {
			return fmt.Errorf("selected main module escapes semantic root: %s", directory)
		}
	}
	if replacement, ok := module["Replace"].(map[string]any); ok {
		if directory, _ := replacement["Dir"].(string); directory != "" {
			version, versioned := replacement["Version"].(string)
			if (!versioned || version == "") && !pathInsideRoot(root, directory) {
				return fmt.Errorf("local Go module replacement escapes semantic root: %s", directory)
			}
		}
		if err := normalizeSemanticModule(root, replacement); err != nil {
			return err
		}
	}
	delete(module, "Dir")
	delete(module, "GoMod")
	return nil
}

func digestSemanticLines(values []string) string {
	copyValues := cloneStrings(values)
	sort.Strings(copyValues)
	sum := sha256.Sum256([]byte(strings.Join(copyValues, "\n") + "\n"))
	return hex.EncodeToString(sum[:])
}
