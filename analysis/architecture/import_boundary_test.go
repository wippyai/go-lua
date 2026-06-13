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
			name:     "engine signature effect lowering stays lua and check free",
			patterns: []string{modulePath + "/analysis/engine/factapply/effectlowering"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "check body stays below fixpoint owners",
			patterns: []string{modulePath + "/analysis/check/body"},
			banned: []string{
				modulePath + "/analysis/check/fixpoint",
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
			name: "program does not import public check facade",
			pkg:  modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check",
			},
			exactly: true,
		},
		{
			name: "public check facade does not import query solver directly",
			pkg:  modulePath + "/analysis/check",
			banned: []string{
				modulePath + "/analysis/check/fixpoint/query",
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

func TestLuaProductionPackagesDoNotImportEngineReadBoundaries(t *testing.T) {
	banned := []string{
		modulePath + "/analysis/engine/sourcevalue",
		modulePath + "/analysis/engine/state",
		modulePath + "/analysis/engine/visibility",
	}

	for _, pkg := range productionPackages(t, modulePath+"/analysis/lua/...") {
		for _, imp := range pkg.Imports {
			for _, bannedImport := range banned {
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
