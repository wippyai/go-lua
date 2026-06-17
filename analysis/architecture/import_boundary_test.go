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
			name:     "engine state tree stays below syntax type check and lua",
			patterns: []string{modulePath + "/analysis/engine/state/..."},
			banned: []string{
				modulePath + "/__old",
				modulePath + "/analysis/check",
				modulePath + "/analysis/ir/cfg",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
				"go/ast",
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
		{
			name: "type aware value bridge packages stay below engine lua check and compiler",
			patterns: []string{
				modulePath + "/analysis/domain/value/refinement",
				modulePath + "/analysis/domain/value/typevalue",
				modulePath + "/analysis/domain/value/variant",
			},
			banned: []string{
				modulePath + "/analysis/engine",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
				modulePath + "/compiler",
			},
		},
		{
			name:     "channel select type schema stays domain engine check lua and compiler free",
			patterns: []string{modulePath + "/analysis/type/channelselect"},
			banned: []string{
				modulePath + "/analysis/domain",
				modulePath + "/analysis/engine",
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "module signature schema stays engine check lua and compiler free",
			patterns: []string{modulePath + "/analysis/module/signature"},
			banned: []string{
				modulePath + "/analysis/engine",
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
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

func TestRequiredSemanticSurfacesExist(t *testing.T) {
	required := []string{
		modulePath + "/analysis/domain/value/axis/assertion",
		modulePath + "/analysis/domain/value/axis/escape",
		modulePath + "/analysis/domain/value/axis/evidence",
		modulePath + "/analysis/domain/value/axis/identity",
		modulePath + "/analysis/domain/value/axis/presence",
		modulePath + "/analysis/domain/value/axis/runtimekind",
		modulePath + "/analysis/domain/value/axis/typewitness",
		modulePath + "/analysis/domain/value/axis/variantorigin",
		modulePath + "/analysis/domain/placement",
		modulePath + "/analysis/domain/effect",
		modulePath + "/analysis/domain/effect/capability",
		modulePath + "/analysis/domain/effect/control",
		modulePath + "/analysis/domain/effect/dispatch",
		modulePath + "/analysis/domain/effect/iteration",
		modulePath + "/analysis/domain/effect/mutation",
		modulePath + "/analysis/domain/effect/ownership",
		modulePath + "/analysis/domain/effect/postcondition",
		modulePath + "/analysis/domain/effect/returns",
	}

	for _, pkg := range required {
		if got := productionPackages(t, pkg); len(got) != 1 {
			t.Fatalf("required package %s resolved to %d packages", pkg, len(got))
		}
	}
}

func TestValueAxisLeafDirectImportBoundaries(t *testing.T) {
	baseAllowed := allowSet(
		modulePath+"/analysis/domain/value/axis",
		modulePath+"/analysis/internal/hash",
	)
	typeWitnessAllowed := copyAllowSet(baseAllowed,
		modulePath+"/analysis/type/literal",
		modulePath+"/analysis/type/refinement",
		modulePath+"/analysis/type/typ",
		modulePath+"/analysis/type/unwrap",
	)

	for _, pkg := range productionPackages(t, modulePath+"/analysis/domain/value/axis/...") {
		if pkg.ImportPath == modulePath+"/analysis/domain/value/axis" {
			continue
		}
		allowed := baseAllowed
		switch pkg.ImportPath {
		case modulePath + "/analysis/domain/value/axis/typewitness":
			allowed = typeWitnessAllowed
		}
		assertModuleImportsAllowed(t, pkg.ImportPath, pkg.Imports, allowed)
	}
}

func TestPlacementDomainDirectImportBoundary(t *testing.T) {
	allowed := allowSet(
		modulePath+"/analysis/domain/lattice",
		modulePath+"/analysis/internal/hash",
	)
	assertModuleImportsAllowed(
		t,
		modulePath+"/analysis/domain/placement",
		productionImports(t, modulePath+"/analysis/domain/placement"),
		allowed,
	)
}

func TestDomainValuePackageDirectImportBoundaries(t *testing.T) {
	t.Run("product imports only the presence axis leaf", func(t *testing.T) {
		for _, imp := range productionImports(t, modulePath+"/analysis/domain/value/product") {
			if strings.HasPrefix(imp, modulePath+"/analysis/domain/value/axis/") &&
				imp != modulePath+"/analysis/domain/value/axis/presence" {
				t.Fatalf("product imports non-core axis leaf %q", imp)
			}
			for _, banned := range []string{
				modulePath + "/analysis/domain/effect",
				modulePath + "/analysis/domain/value/refinement",
				modulePath + "/analysis/domain/value/typevalue",
				modulePath + "/analysis/domain/value/variant",
				modulePath + "/analysis/engine",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			} {
				if forbiddenImport(imp, banned, false) {
					t.Fatalf("product imports forbidden dependency %q", imp)
				}
			}
		}
	})

	t.Run("variant stays independent from value products and axes", func(t *testing.T) {
		for _, dep := range productionDeps(t, modulePath+"/analysis/domain/value/variant/...") {
			for _, banned := range []string{
				modulePath + "/analysis/domain/value/axis",
				modulePath + "/analysis/domain/value/product",
			} {
				if forbiddenImport(dep, banned, false) {
					t.Fatalf("variant imports forbidden dependency %q", dep)
				}
			}
		}
	})

	t.Run("typevalue imports only approved axis leaves", func(t *testing.T) {
		allowedLeaves := allowSet(
			modulePath+"/analysis/domain/value/axis/evidence",
			modulePath+"/analysis/domain/value/axis/presence",
			modulePath+"/analysis/domain/value/axis/runtimekind",
			modulePath+"/analysis/domain/value/axis/typewitness",
			modulePath+"/analysis/domain/value/axis/variantorigin",
		)
		assertValuePackageAxisImports(t, modulePath+"/analysis/domain/value/typevalue", allowedLeaves)
	})

	t.Run("refinement imports only proof axis leaves", func(t *testing.T) {
		allowedLeaves := allowSet(
			modulePath+"/analysis/domain/value/axis/typewitness",
			modulePath+"/analysis/domain/value/axis/variantorigin",
		)
		assertValuePackageAxisImports(t, modulePath+"/analysis/domain/value/refinement", allowedLeaves)
	})
}

func TestEffectPackagesStayValueAndEngineFree(t *testing.T) {
	for _, dep := range productionDeps(t, modulePath+"/analysis/domain/effect/...") {
		for _, banned := range []string{
			modulePath + "/analysis/check",
			modulePath + "/analysis/domain/value",
			modulePath + "/analysis/engine",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("effect packages import forbidden dependency %q", dep)
			}
		}
	}
}

func TestEngineStateCompositionImportBoundaries(t *testing.T) {
	allowed := allowSet(
		modulePath+"/analysis/engine/dynamicindex",
		modulePath+"/analysis/engine/state/channelselectfact",
		modulePath+"/analysis/engine/state/effectdelta",
		modulePath+"/analysis/engine/state/heapidentity",
		modulePath+"/analysis/engine/state/lenbound",
		modulePath+"/analysis/engine/state/numbound",
		modulePath+"/analysis/engine/state/pathevidence",
	)
	for _, imp := range productionImports(t, modulePath+"/analysis/engine/state") {
		if strings.HasPrefix(imp, modulePath+"/analysis/engine/") {
			if _, ok := allowed[imp]; !ok {
				t.Fatalf("state root imports non-composition engine dependency %q", imp)
			}
		}
	}
}

func TestEngineStateLeafDirectImportBoundaries(t *testing.T) {
	leafAllowed := map[string]map[string]struct{}{
		modulePath + "/analysis/engine/state/channelselectfact": {},
		modulePath + "/analysis/engine/state/effectdelta":       {},
		modulePath + "/analysis/engine/state/heapidentity": copyAllowSet(nil,
			modulePath+"/analysis/engine/dynamicindex",
		),
		modulePath + "/analysis/engine/state/internal/floor": {},
		modulePath + "/analysis/engine/state/lenbound": copyAllowSet(nil,
			modulePath+"/analysis/engine/state/internal/floor",
		),
		modulePath + "/analysis/engine/state/numbound": copyAllowSet(nil,
			modulePath+"/analysis/engine/state/internal/floor",
		),
		modulePath + "/analysis/engine/state/pathevidence": {},
	}

	for leaf, allowed := range leafAllowed {
		for _, imp := range productionImports(t, leaf) {
			if !strings.HasPrefix(imp, modulePath+"/analysis/engine/") {
				continue
			}
			if _, ok := allowed[imp]; !ok {
				t.Fatalf("%s imports non-leaf engine dependency %q", leaf, imp)
			}
		}
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
			name: "sourcevalue does not directly import any analysis/type packages",
			pkg:  modulePath + "/analysis/engine/sourcevalue",
			banned: []string{
				modulePath + "/analysis/type",
			},
		},
		{
			name: "factapply does not directly import call outcome policy or type refinement internals",
			pkg:  modulePath + "/analysis/engine/factapply",
			banned: []string{
				modulePath + "/analysis/engine/calloutcome",
				modulePath + "/analysis/type/subtype",
				modulePath + "/analysis/type/access",
				modulePath + "/analysis/type/unwrap",
			},
		},
		{
			name: "calloutcome does not directly import factapply after payload extraction",
			pkg:  modulePath + "/analysis/engine/calloutcome",
			banned: []string{
				modulePath + "/analysis/engine/factapply",
			},
		},
		{
			name: "callpayload remains a neutral outcome payload package",
			pkg:  modulePath + "/analysis/engine/callpayload",
			banned: []string{
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/engine/calloutcome",
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name: "placementplan stays projection-only",
			pkg:  modulePath + "/analysis/check/placementplan",
			banned: []string{
				modulePath + "/analysis/check/checktest",
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/exportmanifest",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			},
		},
		{
			name: "value refinement remains below engine and check layers",
			pkg:  modulePath + "/analysis/domain/value/refinement",
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine",
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

func allowSet(imports ...string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(imports))
	for _, imp := range imports {
		allowed[imp] = struct{}{}
	}
	return allowed
}

func copyAllowSet(base map[string]struct{}, imports ...string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(base)+len(imports))
	for imp := range base {
		allowed[imp] = struct{}{}
	}
	for _, imp := range imports {
		allowed[imp] = struct{}{}
	}
	return allowed
}

func assertModuleImportsAllowed(t *testing.T, pkg string, imports []string, allowed map[string]struct{}) {
	t.Helper()

	for _, imp := range imports {
		if !strings.HasPrefix(imp, modulePath+"/") {
			continue
		}
		if _, ok := allowed[imp]; !ok {
			t.Fatalf("%s imports forbidden dependency %q", pkg, imp)
		}
	}
}

func assertValuePackageAxisImports(t *testing.T, pkg string, allowedLeaves map[string]struct{}) {
	t.Helper()

	for _, imp := range productionImports(t, pkg) {
		if !strings.HasPrefix(imp, modulePath+"/analysis/domain/value/axis/") {
			continue
		}
		if _, ok := allowedLeaves[imp]; !ok {
			t.Fatalf("%s imports unapproved axis leaf %q", pkg, imp)
		}
	}
}

func forbiddenImport(dep, banned string, exactly bool) bool {
	if exactly {
		return dep == banned
	}
	return dep == banned || strings.HasPrefix(dep, banned+"/")
}
