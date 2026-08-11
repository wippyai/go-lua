package semantic

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"golang.org/x/tools/go/packages"
)

func TestTypedImpactPatternsKeepUnexportedFieldsLocal(t *testing.T) {
	root := t.TempDir()
	program := metadataFixture(root, "example.com/program", nil)
	link := metadataFixture(root, "example.com/link", nil)
	consumer := metadataFixture(root, "example.com/consumer", map[string]*packages.Package{"example.com/link": link})
	patterns, err := typedImpactPatterns(root, []*packages.Package{program, link, consumer}, []SymbolRequest{
		{Object: cutplan.SymbolRef{Object: "example.com/program#type:Program/field:state"}, Role: cutplan.ObjectSource, Impact: true},
		{Object: cutplan.SymbolRef{Object: "example.com/link#type:Link/field:next"}, Role: cutplan.ObjectSource, Impact: true},
	}, loadScope{})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/link", "example.com/program"}) {
		t.Fatalf("unexported containment frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsIncludeExportedReverseImporters(t *testing.T) {
	root := t.TempDir()
	owner := metadataFixture(root, "example.com/owner", nil)
	client := metadataFixture(root, "example.com/client", map[string]*packages.Package{"example.com/owner": owner})
	leaf := metadataFixture(root, "example.com/leaf", map[string]*packages.Package{"example.com/client": client})
	patterns, err := typedImpactPatterns(root, []*packages.Package{owner, client, leaf}, []SymbolRequest{{Object: cutplan.SymbolRef{Object: "example.com/owner#package:Flow"}, Role: cutplan.ObjectSource, Impact: true}}, loadScope{})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/client", "example.com/leaf", "example.com/owner"}) {
		t.Fatalf("exported impact frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsExpandTargetWrittenOwners(t *testing.T) {
	root := t.TempDir()
	owner := metadataFixture(root, "example.com/owner", nil)
	consumer := metadataFixture(root, "example.com/consumer", map[string]*packages.Package{"example.com/owner": owner})
	path := filepath.Base(owner.CompiledGoFiles[0])
	patterns, err := typedImpactPatterns(root, []*packages.Package{owner, consumer}, nil, loadScope{Files: []string{path}, ExpandFileOwners: true})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/consumer", "example.com/owner"}) {
		t.Fatalf("target written owner frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsCanonicalizeTestVariantsByForTest(t *testing.T) {
	root := t.TempDir()
	dependency := metadataFixture(root, "example.com/dep", nil)
	internal := metadataFixture(root, "example.com/p", map[string]*packages.Package{"example.com/dep": dependency})
	internal.ForTest = "example.com/p"
	external := metadataFixture(root, "example.com/p_test", map[string]*packages.Package{"example.com/dep": dependency})
	external.ForTest = "example.com/p"
	patterns, err := typedImpactPatterns(root, []*packages.Package{dependency, internal, external}, []SymbolRequest{{Object: cutplan.SymbolRef{Object: "example.com/dep#package:Flow"}, Role: cutplan.ObjectSource, Impact: true}}, loadScope{})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/dep", "example.com/p"}) {
		t.Fatalf("ForTest frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsSeedEveryRequestOwnerAndRepeat(t *testing.T) {
	root := t.TempDir()
	left := metadataFixture(root, "example.com/left", nil)
	right := metadataFixture(root, "example.com/right", nil)
	requests := []SymbolRequest{
		{Object: cutplan.SymbolRef{Object: "example.com/right#type:R/field:value"}, Role: cutplan.ObjectSource},
		{Object: cutplan.SymbolRef{Object: "example.com/left#type:L/field:value"}, Role: cutplan.ObjectSource},
	}
	first, err := typedImpactPatterns(root, []*packages.Package{left, right}, requests, loadScope{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := typedImpactPatterns(root, []*packages.Package{right, left}, requests, loadScope{})
	if err != nil || !reflect.DeepEqual(first, []string{"example.com/left", "example.com/right"}) || !reflect.DeepEqual(first, second) {
		t.Fatalf("owner seed first=%#v second=%#v err=%v", first, second, err)
	}
}

func TestPackagesLoaderUsesMetadataDotDotOnly(t *testing.T) {
	originalLoad, originalMeasure := loadPackages, measureSemanticToolchain
	t.Cleanup(func() { loadPackages, measureSemanticToolchain = originalLoad, originalMeasure })
	root := t.TempDir()
	pkg := metadataFixture(root, "example.com/p", nil)
	var calls [][]string
	loadPackages = func(_ *packages.Config, patterns ...string) ([]*packages.Package, error) {
		calls = append(calls, append([]string(nil), patterns...))
		if len(calls) == 1 {
			return []*packages.Package{pkg}, nil
		}
		return nil, nil
	}
	authority := validFakeAuthority()
	measureSemanticToolchain = func(context.Context, string, []string, []string, []string) (ToolchainEvidence, error) {
		return authority, nil
	}
	_, err := packagesLoader{}.Load(context.Background(), LoadRequest{Root: root, Scratch: t.TempDir(), Environment: []string{"PATH=/bin"}, BuildFlags: []string{"-buildvcs=false", "-trimpath"}, Patterns: []string{"./..."}, scope: loadScope{Files: []string{filepath.Base(pkg.CompiledGoFiles[0])}}})
	if err != nil || !reflect.DeepEqual(calls, [][]string{{"./..."}, {"example.com/p"}}) || strings.Contains(strings.Join(calls[1], "\x00"), "./...") {
		t.Fatalf("metadata/typed package calls=%#v err=%v", calls, err)
	}
}

func TestCollectionInputsKeepSourceAndTargetStatesSeparate(t *testing.T) {
	retired := cutplan.SymbolRef{Object: "example.com/p#package:Gone"}
	intent := cutplan.Intent{Schema: cutplan.Version, Name: "retire", Operations: []cutplan.Operation{{
		ID: "retire", Authority: cutplan.Authority{From: "p", To: "none"},
		Edits:     []cutplan.Edit{{Kind: cutplan.EditRetire, Retire: &cutplan.Retire{Source: "p/gone.go", Symbols: []cutplan.SymbolRef{retired}}}},
		Footprint: cutplan.Footprint{Read: []string{"p/gone.go"}, Write: []string{"p/gone.go"}},
		Verify:    cutplan.Verification{Laws: []cutplan.Law{{ID: "retired", Package: "./p", Test: "TestRetired"}}, Gates: []cutplan.Gate{cutplan.GateImportDAG}},
	}}}
	source, sourceScope, err := collectionInputs(intent, cutplan.ObjectSource)
	if err != nil || len(source) != 1 || !source[0].Impact || !reflect.DeepEqual(sourceScope, loadScope{Files: []string{"p/gone.go"}}) {
		t.Fatalf("source collection inputs=%#v scope=%#v err=%v", source, sourceScope, err)
	}
	target, targetScope, err := collectionInputs(intent, cutplan.ObjectTarget)
	if err != nil || len(target) != 0 || !reflect.DeepEqual(targetScope, loadScope{Files: []string{"p/gone.go"}, ExpandFileOwners: true, RemovedSurfaceOwners: []string{"example.com/p"}}) {
		t.Fatalf("target collection inputs=%#v scope=%#v err=%v", target, targetScope, err)
	}
}

func TestCollectionInputsSeedCompleteGoFootprintWithoutObjectOwnership(t *testing.T) {
	from := cutplan.SymbolRef{Object: "example.com/a#package:Old"}
	to := cutplan.SymbolRef{Object: "example.com/b#package:New"}
	intent := cutplan.Intent{Schema: cutplan.Version, Name: "complete-footprint", Operations: []cutplan.Operation{{
		ID: "move", Authority: cutplan.Authority{From: "a", To: "b"},
		Edits: []cutplan.Edit{
			{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{
				Source: "pkg/a/a.go", Destination: cutplan.Destination{Path: "pkg/b/b.go", Package: "b"},
				Subjects: []cutplan.Relocation{{From: from, To: to}},
			}},
			{Kind: cutplan.EditGenerate, Generate: &cutplan.Generate{Provider: "copy", Inputs: []string{"pkg/a/a.go"}, Destination: "generated.txt"}},
		},
		Bindings:  []cutplan.Binding{{Consumer: "pkg/use/use.go", From: from, To: to, Form: cutplan.BindingDirect}},
		Footprint: cutplan.Footprint{Read: []string{"pkg/a/a.go", "pkg/b/b.go", "pkg/use/use.go"}, Write: []string{"generated.txt", "pkg/a/a.go", "pkg/b/b.go", "pkg/use/use.go"}},
		Verify: cutplan.Verification{
			Laws:  []cutplan.Law{{ID: "move", Package: "./pkg/a", Test: "TestMove"}},
			Gates: []cutplan.Gate{cutplan.GateImportDAG},
		},
	}}}
	source, sourceScope, err := collectionInputs(intent, cutplan.ObjectSource)
	if err != nil || len(source) != 1 || !reflect.DeepEqual(sourceScope, loadScope{Files: []string{"pkg/a/a.go", "pkg/b/b.go", "pkg/use/use.go"}}) {
		t.Fatalf("source collection inputs=%#v scope=%#v err=%v", source, sourceScope, err)
	}
	target, targetScope, err := collectionInputs(intent, cutplan.ObjectTarget)
	if err != nil || len(target) != 1 || !reflect.DeepEqual(targetScope, loadScope{Files: []string{"pkg/a/a.go", "pkg/b/b.go", "pkg/use/use.go"}, ExpandFileOwners: true, RemovedSurfaceOwners: []string{"example.com/a"}}) {
		t.Fatalf("target collection inputs=%#v scope=%#v err=%v", target, targetScope, err)
	}
}

func TestMaterializeScopeUsesConcreteRoleState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "p", "kept.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scope := loadScope{Files: []string{"p/gone.go", "p/kept.go"}, ExpandFileOwners: true, RemovedSurfaceOwners: []string{"example.com/p"}}
	target, err := materializeScope(root, cutplan.ObjectTarget, scope)
	if err != nil || !reflect.DeepEqual(target, loadScope{Files: []string{"p/kept.go"}, ExpandFileOwners: true, RemovedSurfaceOwners: []string{"example.com/p"}}) {
		t.Fatalf("target scope=%#v err=%v", target, err)
	}
	if _, err := materializeScope(root, cutplan.ObjectSource, loadScope{Files: []string{"p/gone.go"}}); err == nil || !strings.Contains(err.Error(), "gone.go") {
		t.Fatalf("source missing path accepted: %v", err)
	}
}

func TestTypedImpactPatternsKeepRetiredPackageAfterDeletedFile(t *testing.T) {
	root := t.TempDir()
	pkg := metadataPackageInDirectory(t, root, "example.com/p", "p", "remaining.go")
	patterns, err := typedImpactPatterns(root, []*packages.Package{pkg}, nil, loadScope{RemovedSurfaceOwners: []string{"example.com/p"}})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/p"}) {
		t.Fatalf("retire target frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsAcceptGenerateNewFileInExistingPackage(t *testing.T) {
	root := t.TempDir()
	pkg := metadataPackageInDirectory(t, root, "example.com/p", "p", "existing.go")
	generated := filepath.Join(root, "p", "generated.go")
	if err := os.WriteFile(generated, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg.GoFiles = append(pkg.GoFiles, generated)
	pkg.CompiledGoFiles = append(pkg.CompiledGoFiles, generated)
	patterns, err := typedImpactPatterns(root, []*packages.Package{pkg}, nil, loadScope{Files: []string{"p/generated.go"}, ExpandFileOwners: true})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/p"}) {
		t.Fatalf("new file in existing package frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsRejectGenerateNewPackage(t *testing.T) {
	root := t.TempDir()
	pkg := metadataPackageInDirectory(t, root, "example.com/p", "p", "existing.go")
	if _, err := typedImpactPatterns(root, []*packages.Package{pkg}, nil, loadScope{Files: []string{"newpkg/generated.go"}, ExpandFileOwners: true}); err == nil || !strings.Contains(err.Error(), "unique affected package owner") {
		t.Fatalf("new package accepted: %v", err)
	}
}

func TestTypedImpactPatternsRejectRetireFinalFile(t *testing.T) {
	root := t.TempDir()
	if _, err := typedImpactPatterns(root, nil, nil, loadScope{RemovedSurfaceOwners: []string{"example.com/p"}}); err == nil || !strings.Contains(err.Error(), "produced no typed package roots") {
		t.Fatalf("final-file retirement accepted: %v", err)
	}
}

func TestTypedImpactPatternsKeepGenerateSourceBeforeNewFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "p", "source.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg := &packages.Package{ID: "example.com/p", PkgPath: "example.com/p", Name: "p", GoFiles: []string{file}, CompiledGoFiles: []string{file}}
	patterns, err := typedImpactPatterns(root, []*packages.Package{pkg}, nil, loadScope{Files: []string{"p/source.go"}})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/p"}) {
		t.Fatalf("generate source frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsExpandRemovedExportedOwner(t *testing.T) {
	root := t.TempDir()
	owner := metadataFixture(root, "example.com/owner", nil)
	consumer := metadataFixture(root, "example.com/consumer", map[string]*packages.Package{"example.com/owner": owner})
	patterns, err := typedImpactPatterns(root, []*packages.Package{owner, consumer}, nil, loadScope{RemovedSurfaceOwners: []string{"example.com/owner"}})
	if err != nil || !reflect.DeepEqual(patterns, []string{"example.com/consumer", "example.com/owner"}) {
		t.Fatalf("opposite-role exported frontier=%#v err=%v", patterns, err)
	}
}

func TestTypedImpactPatternsSkipsMissingRemovedOwner(t *testing.T) {
	root := t.TempDir()
	pkg := metadataFixture(root, "example.com/p", nil)
	patterns, err := typedImpactPatterns(root, []*packages.Package{pkg}, nil, loadScope{RemovedSurfaceOwners: []string{"example.com/missing"}})
	if err == nil || !strings.Contains(err.Error(), "produced no typed package roots") || patterns != nil {
		t.Fatalf("missing removed owner=%#v err=%v", patterns, err)
	}
}

func metadataFixture(root, path string, imports map[string]*packages.Package) *packages.Package {
	file := filepath.Join(root, strings.ReplaceAll(path, "/", "_")+".go")
	return &packages.Package{ID: path, PkgPath: path, Name: "p", GoFiles: []string{file}, CompiledGoFiles: []string{file}, Imports: imports}
}

func metadataPackageInDirectory(t *testing.T, root, path, directory, name string) *packages.Package {
	t.Helper()
	file := filepath.Join(root, directory, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &packages.Package{ID: path, PkgPath: path, Name: "p", GoFiles: []string{file}, CompiledGoFiles: []string{file}}
}
