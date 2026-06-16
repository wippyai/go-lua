package architecture

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/wippyai/go-lua"

type listedPackage struct {
	ImportPath string
	Name       string
	Imports    []string
}

func TestLowerLayerImportBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		banned   []string
	}{
		{
			name:     "ir stays below lua check and lua parser ast",
			patterns: []string{modulePath + "/analysis/ir/..."},
			banned: []string{
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
				modulePath + "/compiler/ast",
				modulePath + "/compiler/parse",
			},
		},
		{
			name:     "cfgbuild stays before semantics transferfacts and check",
			patterns: []string{modulePath + "/analysis/lua/cfgbuild"},
			banned: []string{
				modulePath + "/analysis/lua/semantics",
				modulePath + "/analysis/lua/transferfacts",
				modulePath + "/analysis/check",
			},
		},
		{
			name:     "semantics stays before transferfacts and check",
			patterns: []string{modulePath + "/analysis/lua/semantics"},
			banned: []string{
				modulePath + "/analysis/lua/transferfacts",
				modulePath + "/analysis/check",
			},
		},
		{
			name:     "transferfacts stays before check fixpoint",
			patterns: []string{modulePath + "/analysis/lua/transferfacts"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/check/fixpoint",
			},
		},
		{
			name:     "lua moduleidentity stays below check and engine",
			patterns: []string{modulePath + "/analysis/lua/moduleidentity"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine",
			},
		},
		{
			name:     "engine signature effect lowering stays lua and check free",
			patterns: []string{modulePath + "/analysis/engine/effectlowering"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine sourcevalue stays read-model only",
			patterns: []string{modulePath + "/analysis/engine/sourcevalue"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine visibility stays generic",
			patterns: []string{modulePath + "/analysis/engine/visibility"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factflow",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine callboundary stays boundary schema only",
			patterns: []string{modulePath + "/analysis/engine/callboundary"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/engine/factflow",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			},
		},
		{
			name:     "check body stays below fixpoint owners",
			patterns: []string{modulePath + "/analysis/check/body"},
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/fixpoint",
			},
		},
		{
			name:     "engine factflow stays syntax check type and state independent",
			patterns: []string{modulePath + "/analysis/engine/factflow"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/state",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
			},
		},
		{
			name:     "engine callproducer stays projection only",
			patterns: []string{modulePath + "/analysis/engine/callproducer"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/engine/state",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine transfer stays generic",
			patterns: []string{modulePath + "/analysis/engine/transfer"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
			},
		},
		{
			name:     "engine solve stays generic",
			patterns: []string{modulePath + "/analysis/engine/solve"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
			},
		},
		{
			name:     "engine state stays below check and lua",
			patterns: []string{modulePath + "/analysis/engine/state"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.patterns...) {
				for _, banned := range tt.banned {
					if forbiddenImport(dep, banned, false) {
						t.Fatalf("%s imports forbidden dependency %q", strings.Join(tt.patterns, " "), dep)
					}
				}
			}
		})
	}
}

func TestLowLevelLeafImportBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		banned   []string
	}{
		{
			name:     "type stays independent from domain values and paths",
			patterns: []string{modulePath + "/analysis/type/..."},
			banned: []string{
				modulePath + "/analysis/domain/value",
				modulePath + "/analysis/domain/path",
			},
		},
		{
			name:     "domain path stays independent from type and domain values",
			patterns: []string{modulePath + "/analysis/domain/path/..."},
			banned: []string{
				modulePath + "/analysis/type",
				modulePath + "/analysis/domain/value",
			},
		},
		{
			name: "domain value axis and product stay below type lua and check",
			patterns: []string{
				modulePath + "/analysis/domain/value/axis",
				modulePath + "/analysis/domain/value/product",
			},
			banned: []string{
				modulePath + "/analysis/type",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.patterns...) {
				for _, banned := range tt.banned {
					if forbiddenImport(dep, banned, false) {
						t.Fatalf("%s imports forbidden dependency %q", strings.Join(tt.patterns, " "), dep)
					}
				}
			}
		})
	}
}

func TestCheckSplitDirectImportBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		banned  []string
		exactly bool
	}{
		{
			name: "program does not import public check facade exactly",
			pkg:  modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check",
			},
			exactly: true,
		},
		{
			name: "program does not import diagnostics or fixture harnesses",
			pkg:  modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/checktest",
			},
		},
		{
			name: "sourcevalue does not directly import type semantics",
			pkg:  modulePath + "/analysis/engine/sourcevalue",
			banned: []string{
				modulePath + "/analysis/type",
			},
		},
		{
			name: "factapply does not directly import type access semantics",
			pkg:  modulePath + "/analysis/engine/factapply",
			banned: []string{
				modulePath + "/analysis/type/access",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, imp := range productionImports(t, tt.pkg) {
				for _, banned := range tt.banned {
					if forbiddenImport(imp, banned, tt.exactly) {
						t.Fatalf("%s imports forbidden dependency %q", tt.pkg, imp)
					}
				}
			}
		})
	}
}

func TestCheckCorePackagesDoNotImportDiagnosticsOrFixpoint(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		banned  []string
	}{
		{
			name:    "body stays below fixpoint and diagnostics",
			pattern: modulePath + "/analysis/check/body",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/fixpoint",
			},
		},
		{
			name:    "program stays below diagnostics",
			pattern: modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
			},
		},
		{
			name:    "summary stays below diagnostics",
			pattern: modulePath + "/analysis/check/fixpoint/summary",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.pattern) {
				for _, banned := range tt.banned {
					if forbiddenImport(dep, banned, false) {
						t.Fatalf("%s imports forbidden dependency %q", tt.pattern, dep)
					}
				}
			}
		})
	}
}

func TestActiveCheckTreeHasNoPipelinePackages(t *testing.T) {
	for _, pkg := range productionPackages(t, modulePath+"/analysis/check/...") {
		if pkg.Name == "pipeline" {
			t.Fatalf("%s uses forbidden package name %q", pkg.ImportPath, pkg.Name)
		}
	}
}

func TestLuaProductionPackagesDoNotImportEngineReadBoundaries(t *testing.T) {
	banned := []string{
		modulePath + "/analysis/engine/sourcevalue",
		modulePath + "/analysis/engine/state",
		modulePath + "/analysis/engine/visibility",
	}
	visibilityAdapter := modulePath + "/analysis/lua/visibilityfacts"

	for _, pkg := range productionPackages(t, modulePath+"/analysis/lua/...") {
		for _, imp := range pkg.Imports {
			for _, bannedImport := range banned {
				if pkg.ImportPath == visibilityAdapter && bannedImport == modulePath+"/analysis/engine/visibility" {
					continue
				}
				if forbiddenImport(imp, bannedImport, false) {
					t.Fatalf("%s imports forbidden dependency %q", pkg.ImportPath, imp)
				}
			}
		}
	}
}

func productionPackages(t *testing.T, patterns ...string) []listedPackage {
	t.Helper()

	args := append([]string{"list", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []listedPackage
	for {
		var pkg listedPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func productionDeps(t *testing.T, patterns ...string) []string {
	t.Helper()

	args := append([]string{"list", "-deps", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var deps []string
	for {
		var pkg listedPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		deps = append(deps, pkg.ImportPath)
	}
	return deps
}

func productionImports(t *testing.T, pattern string) []string {
	t.Helper()

	args := []string{"list", "-json", pattern}
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	var pkg listedPackage
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}
	return pkg.Imports
}

func forbiddenImport(dep, banned string, exactly bool) bool {
	if exactly {
		return dep == banned
	}
	return dep == banned || strings.HasPrefix(dep, banned+"/")
}
