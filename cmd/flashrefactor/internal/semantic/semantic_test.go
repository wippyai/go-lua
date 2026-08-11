package semantic

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"golang.org/x/tools/go/packages"
)

var goneMethod = cutplan.SymbolRef{Object: "example.com/semanticfixture/pkg/gone#type:T/method:F"}

func TestCollectBindsStructuredAuthorityAndExactObjectUses(t *testing.T) {
	root := testWorkspace(t)
	request := SymbolRequest{Object: goneMethod, Role: cutplan.ObjectSource, Impact: true}
	expectedDefinition := cutplan.Position{PackageIDs: []string{"example.com/semanticfixture/pkg/gone"}, Path: "pkg/gone/g.go", Offset: 40, Line: 5, Column: 10, Role: cutplan.SiteDeclaration}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), []SymbolRequest{request}, []string{"pkg/a/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Authority.Loader != packagesAuthority || snapshot.Authority.Go.Path == "" || len(snapshot.Authority.Go.SHA256) != 64 || snapshot.Authority.Go.Version == "" || len(snapshot.Authority.BuildEnvSHA256) != 64 || len(snapshot.Authority.ModuleGraphSHA256) != 64 {
		t.Fatalf("authority %#v", snapshot.Authority)
	}
	if snapshot.Toolchain.Resolver != packagesAuthority {
		t.Fatalf("unexpected resolver evidence %#v", snapshot.Toolchain)
	}
	if len(snapshot.Objects) != 1 || len(snapshot.Objects[0].References) != 1 {
		t.Fatalf("resolved object evidence %#v", snapshot.Objects)
	}
	if snapshot.Objects[0].Role != cutplan.ObjectSource || snapshot.Objects[0].Package != "gone" {
		t.Fatalf("source classification %#v", snapshot.Objects[0])
	}
	if !samePosition(snapshot.Objects[0].Definition, expectedDefinition) {
		t.Fatalf("definition mismatch: %#v", snapshot.Objects[0].Definition)
	}
	file, err := snapshot.Workspace.File("pkg/a/a.go")
	if err != nil || file.AST == nil || snapshot.Workspace.FileSet() == nil || !strings.Contains(string(file.Source), "gone.T") {
		t.Fatalf("workspace file=%#v err=%v", file, err)
	}
	pkg, err := snapshot.Workspace.PackageForFile(file)
	if err != nil || pkg.Info == nil || pkg.Types == nil {
		t.Fatalf("workspace package=%#v err=%v", pkg, err)
	}
	imported, err := snapshot.Workspace.ImportPkgName(file, "example.com/semanticfixture/pkg/gone")
	if err != nil || imported.Imported().Path() != "example.com/semanticfixture/pkg/gone" {
		t.Fatalf("workspace import=%v err=%v", imported, err)
	}
	object, err := snapshot.Workspace.Object(goneMethod)
	if err != nil || object.Name() != "F" {
		t.Fatalf("workspace object=%v err=%v", object, err)
	}
	fromWorkspace, err := snapshot.Workspace.resolve([]SymbolRequest{request})
	if err != nil || len(fromWorkspace) != len(snapshot.Objects) || fromWorkspace[0].Object != snapshot.Objects[0].Object || !samePosition(fromWorkspace[0].Definition, snapshot.Objects[0].Definition) || !samePositions(fromWorkspace[0].References, snapshot.Objects[0].References) {
		t.Fatalf("snapshot evidence is not derived from the same workspace: %#v err=%v", fromWorkspace, err)
	}
	if err := VerifyExpected(snapshot.Objects, fromWorkspace); err != nil {
		t.Fatal(err)
	}
	wrongPackage := append([]cutplan.ObjectEvidence(nil), fromWorkspace...)
	wrongPackage[0].Package = "other"
	if err := VerifyExpected(snapshot.Objects, wrongPackage); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("package replay drift accepted: %v", err)
	}
}

func TestPackagesLoaderRejectsPostLoadSemanticContextDrift(t *testing.T) {
	originalLoad, originalMeasure := loadPackages, measureSemanticToolchain
	t.Cleanup(func() {
		loadPackages, measureSemanticToolchain = originalLoad, originalMeasure
	})
	root := t.TempDir()
	file := filepath.Join(root, "p.go")
	writeTestFile(t, root, "p.go", "package p\n")
	metadata := &packages.Package{ID: "example.com/p", PkgPath: "example.com/p", Name: "p", GoFiles: []string{file}, CompiledGoFiles: []string{file}}
	loadPackages = func(_ *packages.Config, patterns ...string) ([]*packages.Package, error) {
		if len(patterns) == 1 && patterns[0] == "./..." {
			return []*packages.Package{metadata}, nil
		}
		return nil, nil
	}
	first := validFakeAuthority()
	second := first
	second.ModuleGraphSHA256 = strings.Repeat("d", 64)
	calls := 0
	measureSemanticToolchain = func(context.Context, string, []string, []string, []string) (ToolchainEvidence, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	_, err := packagesLoader{}.Load(context.Background(), LoadRequest{Root: root, Scratch: t.TempDir(), Environment: []string{"PATH=/bin"}, BuildFlags: []string{"-buildvcs=false", "-trimpath"}, Patterns: []string{"./..."}, scope: loadScope{Files: []string{filepath.Base(file)}}})
	if err == nil || !strings.Contains(err.Error(), "context changed") || calls != 2 {
		t.Fatalf("post-load semantic drift accepted: calls=%d err=%v", calls, err)
	}
}

func TestPackagesLoaderAcceptsStablePostLoadSemanticContext(t *testing.T) {
	originalLoad, originalMeasure := loadPackages, measureSemanticToolchain
	t.Cleanup(func() {
		loadPackages, measureSemanticToolchain = originalLoad, originalMeasure
	})
	root := t.TempDir()
	file := filepath.Join(root, "p.go")
	writeTestFile(t, root, "p.go", "package p\n")
	metadata := &packages.Package{ID: "example.com/p", PkgPath: "example.com/p", Name: "p", GoFiles: []string{file}, CompiledGoFiles: []string{file}}
	loadPackages = func(_ *packages.Config, patterns ...string) ([]*packages.Package, error) {
		if len(patterns) == 1 && patterns[0] == "./..." {
			return []*packages.Package{metadata}, nil
		}
		return nil, nil
	}
	authority := validFakeAuthority()
	calls := 0
	measureSemanticToolchain = func(context.Context, string, []string, []string, []string) (ToolchainEvidence, error) {
		calls++
		return authority, nil
	}
	result, err := packagesLoader{}.Load(context.Background(), LoadRequest{Root: root, Scratch: t.TempDir(), Environment: []string{"PATH=/bin"}, BuildFlags: []string{"-buildvcs=false", "-trimpath"}, Patterns: []string{"./..."}, scope: loadScope{Files: []string{filepath.Base(file)}}})
	if err != nil || calls != 2 || result.Toolchain != authority || len(result.WorkspaceFailures) != 1 {
		t.Fatalf("stable post-load semantic evidence: calls=%d result=%#v err=%v", calls, result, err)
	}
}

func TestMergeRejectsSemanticContextDrift(t *testing.T) {
	source := Snapshot{Workspace: &Workspace{}, Toolchain: cutplan.Toolchain{BuildEnvSHA256: strings.Repeat("a", 64), ModuleGraphSHA256: strings.Repeat("b", 64)}}
	target := source
	target.Toolchain.BuildEnvSHA256 = strings.Repeat("c", 64)
	if _, err := Merge(source, target, nil); err == nil || !strings.Contains(err.Error(), "authorities differ") {
		t.Fatalf("semantic build-context drift accepted: %v", err)
	}
}

func TestWorkspaceExcludesDependencySyntaxAndRejectsPartialVariant(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/rootscope\n\ngo 1.23.0\n")
	writeTestFile(t, root, "pkg/p/p.go", "package p\n\nimport \"fmt\"\n\nfunc Visible() string { return fmt.Sprint(1) }\n")
	writeTestFile(t, root, "pkg/p/p_test.go", "package p\n\nimport \"testing\"\n\nfunc TestLocal(t *testing.T) { if Visible() != \"1\" { t.Fatal(Visible()) } }\n")
	requests := []SymbolRequest{
		{Object: cutplan.SymbolRef{Object: "example.com/rootscope/pkg/p#package:Visible"}, Role: cutplan.ObjectSource},
		{Object: cutplan.SymbolRef{Object: "example.com/rootscope/pkg/p#package:TestLocal"}, Role: cutplan.ObjectSource},
	}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), requests, nil)
	if err != nil {
		t.Fatalf("in-root package referring to dependency did not resolve: %v", err)
	}
	for _, request := range requests {
		if _, err := snapshot.Workspace.Object(request.Object); err != nil {
			t.Fatalf("in-root declaration was not retained: %v", err)
		}
	}
	for _, object := range snapshot.Objects {
		for _, site := range append([]cutplan.Position{object.Definition}, object.References...) {
			if strings.HasPrefix(site.Path, "../") || filepath.IsAbs(site.Path) {
				t.Fatalf("outside-root evidence entered lock material: %#v", site)
			}
		}
	}
	for _, pkg := range snapshot.Workspace.Packages() {
		if pkg.Path == "fmt" {
			t.Fatal("dependency package syntax became workspace authority")
		}
	}

	set := token.NewFileSet()
	insidePath := filepath.Join(root, "pkg/mixed/inside.go")
	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "outside.go")
	writeTestFile(t, root, "pkg/mixed/inside.go", "package mixed\n")
	if err := os.WriteFile(outsidePath, []byte("package mixed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inside, err := parser.ParseFile(set, insidePath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	outside, err := parser.ParseFile(set, outsidePath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, variant := range map[string]*packages.Package{
		"mixed": {
			ID: "example.com/rootscope/pkg/mixed", PkgPath: "example.com/rootscope/pkg/mixed", Name: "mixed",
			Types: types.NewPackage("example.com/rootscope/pkg/mixed", "mixed"), TypesInfo: &types.Info{}, Syntax: []*ast.File{inside, outside},
		},
		"incomplete": {
			ID: "example.com/rootscope/pkg/incomplete", PkgPath: "example.com/rootscope/pkg/incomplete", Name: "incomplete",
			Types: types.NewPackage("example.com/rootscope/pkg/incomplete", "incomplete"), TypesInfo: &types.Info{}, Syntax: []*ast.File{inside, nil},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if packageInsideWorkspace(root, set, variant) {
				t.Fatal("partial package variant entered workspace")
			}
			workspace, buildErr := buildWorkspace(root, set, []*packages.Package{variant}, nil)
			if buildErr != nil {
				t.Fatalf("partial package should be rejected, not partially indexed: %v", buildErr)
			}
			if len(workspace.Packages()) != 0 || len(workspace.Files()) != 0 {
				t.Fatalf("partial package produced workspace authority: packages=%#v files=%#v", workspace.Packages(), workspace.Files())
			}
		})
	}
}

func TestWorkspaceStructurePreservesImportClauseAndExactAliasSpelling(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/importshape\n\ngo 1.23.0\n")
	writeTestFile(t, root, "implicit.go", "package importshape\n\nimport \"fmt\"\n\nfunc Implicit() { fmt.Print(1) }\n")
	writeTestFile(t, root, "explicit.go", "package importshape\n\nimport named \"fmt\"\n\nfunc Explicit() { named.Print(2) }\n")
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]cutplan.ImportRef{
		"explicit.go": {Path: "fmt", Name: "fmt", Alias: "named"},
		"implicit.go": {Path: "fmt", Name: "fmt", Alias: ""},
	}
	for _, file := range snapshot.Structure.Files {
		expected, ok := want[file.Path]
		if !ok {
			continue
		}
		if len(file.Imports) != 1 || file.Imports[0] != expected {
			t.Fatalf("import shape for %s = %#v, want %#v", file.Path, file.Imports, expected)
		}
		delete(want, file.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing structural import evidence: %#v", want)
	}
}

func TestCanonicalStructureOrdersFullImportIdentity(t *testing.T) {
	value := canonicalStructure(StructuralSnapshot{Files: []StructuralFile{{
		Path: "p.go", PackageID: "p", Imports: []cutplan.ImportRef{
			{Path: "example.com/p", Name: "z", Alias: ""},
			{Path: "example.com/p", Name: "a", Alias: ""},
		},
	}}})
	if got, want := value.Files[0].Imports[0].Name, "a"; got != want {
		t.Fatalf("first import name = %q, want %q", got, want)
	}
}

func TestCollectionRejectsWrongMissingAndMixedRoles(t *testing.T) {
	root := testWorkspace(t)
	session := testSession(t, root, fakeLoader{})
	defer session.Close()
	source := SymbolRequest{Object: goneMethod, Role: cutplan.ObjectSource}
	target := SymbolRequest{Object: goneMethod, Role: cutplan.ObjectTarget}
	missing := SymbolRequest{Object: goneMethod}
	for name, call := range map[string]func() error{
		"source-collection-target": func() error {
			_, err := testCollect(session, context.Background(), []SymbolRequest{target}, nil)
			return err
		},
		"target-collection-source": func() error {
			_, err := testCollectVirtual(session, context.Background(), []SymbolRequest{source}, nil, nil)
			return err
		},
		"missing-role": func() error {
			_, err := testCollect(session, context.Background(), []SymbolRequest{missing}, nil)
			return err
		},
		"mixed-roles": func() error {
			_, err := testCollect(session, context.Background(), []SymbolRequest{source, target}, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "role") {
				t.Fatalf("invalid role accepted: %v", err)
			}
		})
	}
}

func samePositions(left, right []cutplan.Position) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !samePosition(left[index], right[index]) {
			return false
		}
	}
	return true
}

func TestWorkspaceRejectsMissingAndAmbiguousObjectsAndFiles(t *testing.T) {
	root := testWorkspace(t)
	filePath := filepath.Join(root, "pkg/a/a.go")
	workspace := &Workspace{
		root:      root,
		files:     []WorkspaceFile{{Path: "pkg/a/a.go", PackageID: "one"}, {Path: "pkg/a/a.go", PackageID: "two"}},
		fileIndex: map[string][]int{"pkg/a/a.go": {0, 1}},
		objects: map[string][]workspaceObject{
			"example.com/a#type:T/field:F": {{object: nil}, {object: nil}},
		},
	}
	if _, err := workspace.File(filePath); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous file accepted: %v", err)
	}
	if _, err := workspace.File(filepath.Join(root, "missing.go")); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err := workspace.Object(cutplan.SymbolRef{Object: "missing#type:T/field:F"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing object accepted: %v", err)
	}
	if _, err := workspace.Object(cutplan.SymbolRef{Object: "example.com/a#type:T/field:F"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous object accepted: %v", err)
	}
}

func TestWorkspaceNormalizesBaseInternalAndExternalTestVariants(t *testing.T) {
	root := testVariantWorkspace(t)
	method := cutplan.SymbolRef{Object: "example.com/variants/pkg/p#type:T/method:M"}
	request := SymbolRequest{Object: method, Role: cutplan.ObjectSource}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), []SymbolRequest{request}, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := snapshot.Workspace.File("pkg/p/p.go")
	if err != nil || base.PackageID != "example.com/variants/pkg/p" {
		t.Fatalf("base variant=%#v err=%v", base, err)
	}
	internal, err := snapshot.Workspace.File("pkg/p/p_test.go")
	if err != nil || internal.PackageID == base.PackageID {
		t.Fatalf("internal test variant=%#v err=%v", internal, err)
	}
	external, err := snapshot.Workspace.File("pkg/p/external_test.go")
	if err != nil || external.PackagePath != "example.com/variants/pkg/p_test" {
		t.Fatalf("external test variant=%#v err=%v", external, err)
	}
	selected, err := snapshot.Workspace.canonicalObject(method)
	if err != nil || selected.packageID != base.PackageID {
		t.Fatalf("ordinary declaration did not select the base variant: %#v err=%v", selected, err)
	}
	testOnly := cutplan.SymbolRef{Object: "example.com/variants/pkg/p#package:localOnly"}
	selected, err = snapshot.Workspace.canonicalObject(testOnly)
	if err != nil || selected.packageID != internal.PackageID {
		t.Fatalf("test declaration did not select its exact test variant: %#v err=%v", selected, err)
	}
	sites := snapshot.Objects[0].References
	if len(sites) != 3 {
		t.Fatalf("variant evidence lost or duplicated: %#v", sites)
	}
	selectors := 0
	for _, site := range sites {
		if len(site.PackageIDs) == 0 || site.Offset < 0 || site.Line < 1 || site.Column < 1 {
			t.Fatalf("site does not bind a complete generated identity: %#v", site)
		}
		switch site.Role {
		case cutplan.SiteSelector:
			selectors++
		default:
			t.Fatalf("method site has wrong structural role: %#v", site)
		}
	}
	if snapshot.Objects[0].Definition.Role != cutplan.SiteDeclaration || len(snapshot.Objects[0].Definition.PackageIDs) != 2 || selectors != 3 {
		t.Fatalf("variant role/identity coverage is incomplete: %#v", sites)
	}
	residue, err := snapshot.Workspace.ObjectResidue(method, []string{"pkg/p/p.go"})
	if err != nil || len(residue.Sites) != 2 || len(residue.Sites[0].PackageIDs) != 2 || len(residue.Sites[1].PackageIDs) != 2 {
		t.Fatalf("variant-aware residue did not retain both package variants: %#v err=%v", residue, err)
	}
}

func TestWorkspaceObjectForFileProjectsExactConsumerVariant(t *testing.T) {
	root := testVariantWorkspace(t)
	ref := cutplan.SymbolRef{Object: "example.com/variants/pkg/p#package:T"}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), []SymbolRequest{{Object: ref, Role: cutplan.ObjectSource}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := snapshot.Workspace.Object(ref)
	if err != nil {
		t.Fatal(err)
	}
	local, err := snapshot.Workspace.File("pkg/p/p.go")
	if err != nil {
		t.Fatal(err)
	}
	localObject, err := snapshot.Workspace.ObjectForFile(ref, local)
	if err != nil {
		t.Fatal(err)
	}
	if localObject != canonical {
		t.Fatalf("base-file projection = %#v, want canonical %#v", localObject, canonical)
	}
	external, err := snapshot.Workspace.File("pkg/p/external_test.go")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := snapshot.Workspace.ObjectForFile(ref, external)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := snapshot.Workspace.ImportPkgName(external, "example.com/variants/pkg/p")
	if err != nil {
		t.Fatal(err)
	}
	want := imported.Imported().Scope().Lookup("T")
	if projected != want {
		t.Fatalf("external projection = %#v, want exact imported object %#v", projected, want)
	}
	if projected == canonical {
		t.Fatal("external-test projection reused the canonical object instead of its exact imported package variant")
	}
}

func TestWorkspaceObjectForFileRejectsMissingAndAmbiguousVisibility(t *testing.T) {
	ref := cutplan.SymbolRef{Object: "example.com/projected#package:T"}
	visiblePackage := types.NewPackage("example.com/projected", "projected")
	imported := types.NewPkgName(token.NoPos, nil, "projected", visiblePackage)
	file := WorkspaceFile{Path: "consumer/use.go", PackageID: "consumer", PackagePath: "example.com/consumer"}
	workspace := &Workspace{
		files:     []WorkspaceFile{file},
		fileIndex: map[string][]int{file.Path: {0}},
		objects: map[string][]workspaceObject{
			ref.Object: {
				{object: types.NewVar(token.NoPos, visiblePackage, "T", types.Typ[types.Int]), packageID: "producer-a"},
				{object: types.NewVar(token.NoPos, visiblePackage, "T", types.Typ[types.Int]), packageID: "producer-b"},
			},
		},
		imports: map[string][]resolvedImport{
			workspaceImportKey(file.Path, file.PackageID): {{object: imported}},
		},
	}
	if _, err := workspace.ObjectForFile(cutplan.SymbolRef{Object: "missing#package:T"}, file); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing projection accepted: %v", err)
	}
	if _, err := workspace.ObjectForFile(ref, file); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous projection accepted: %v", err)
	}
	fabricated := file
	fabricated.Path = "other/fabricated.go"
	if _, err := workspace.ObjectForFile(ref, fabricated); err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("fabricated projection accepted: %v", err)
	}
}

func TestSymbolRefCoversPackageScopeObjects(t *testing.T) {
	root := testObjectWorkspace(t)
	requests := []SymbolRequest{
		{Object: cutplan.SymbolRef{Object: "example.com/objects/pkg/o#package:Named"}, Role: cutplan.ObjectSource},
		{Object: cutplan.SymbolRef{Object: "example.com/objects/pkg/o#package:Helper"}, Role: cutplan.ObjectSource},
		{Object: cutplan.SymbolRef{Object: "example.com/objects/pkg/o#package:Value"}, Role: cutplan.ObjectSource},
		{Object: cutplan.SymbolRef{Object: "example.com/objects/pkg/o#package:Limit"}, Role: cutplan.ObjectSource},
	}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), requests, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != len(requests) {
		t.Fatalf("package scope objects %#v", snapshot.Objects)
	}
}

func TestCollectClassifiesOrdinaryUsesWithByteOffsets(t *testing.T) {
	root := testObjectWorkspace(t)
	request := SymbolRequest{Object: cutplan.SymbolRef{Object: "example.com/objects/pkg/o#package:Helper"}, Role: cutplan.ObjectSource}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), []SymbolRequest{request}, nil)
	if err != nil {
		t.Fatal(err)
	}
	declaration := &snapshot.Objects[0].Definition
	var use *cutplan.Position
	for index := range snapshot.Objects[0].References {
		site := &snapshot.Objects[0].References[index]
		switch site.Role {
		case cutplan.SiteUse:
			use = site
		}
	}
	if declaration == nil || use == nil || len(declaration.PackageIDs) == 0 || len(use.PackageIDs) == 0 || declaration.Offset >= use.Offset {
		t.Fatalf("ordinary use is not an ordered, variant-bound semantic site: %#v", snapshot.Objects[0].References)
	}
}

func TestCollectAllowsUnusedDeclarationWithoutDuplicatingItAsReference(t *testing.T) {
	root := testObjectWorkspace(t)
	request := SymbolRequest{Object: cutplan.SymbolRef{Object: "example.com/objects/pkg/o#package:Named"}, Role: cutplan.ObjectSource}
	session := testSession(t, root, nil)
	defer session.Close()
	snapshot, err := testCollect(session, context.Background(), []SymbolRequest{request}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || snapshot.Objects[0].Definition.Role != cutplan.SiteDeclaration || len(snapshot.Objects[0].Definition.PackageIDs) != 1 || len(snapshot.Objects[0].References) != 0 {
		t.Fatalf("unused declaration evidence should have one definition and no references: %#v", snapshot.Objects)
	}
	if err := ValidateEvidence([]SymbolRequest{request}, snapshot.Objects); err != nil {
		t.Fatal(err)
	}
}

func TestSymbolRefRejectsBuiltinWithoutPackage(t *testing.T) {
	if _, err := symbolRef(types.Universe.Lookup("len"), ""); err == nil {
		t.Fatal("builtin without package accepted as a cutplan object")
	}
}

func TestCollectFailsClosedOnStructuredWorkspaceFailure(t *testing.T) {
	root := testWorkspace(t)
	session := testSession(t, root, fakeLoader{result: LoadResult{WorkspaceFailures: []string{"module graph unavailable"}}})
	defer session.Close()
	_, err := testCollect(session, context.Background(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "workspace load failed") {
		t.Fatalf("workspace failure accepted: %v", err)
	}
}

func TestCollectRejectsUnrecognizedOrIncompleteAuthority(t *testing.T) {
	root := testWorkspace(t)
	for name, loader := range map[string]Loader{
		"unknown":    fakeLoader{authority: ToolchainEvidence{Loader: "gopls"}},
		"incomplete": fakeLoader{authority: ToolchainEvidence{Loader: packagesAuthority, Go: ExecutableIdentity{Path: "/go", Version: "go version"}}},
	} {
		t.Run(name, func(t *testing.T) {
			session := testSession(t, root, loader)
			defer session.Close()
			if _, err := testCollect(session, context.Background(), nil, nil); err == nil {
				t.Fatal("invalid authority accepted")
			}
		})
	}
}

func TestCollectVirtualRetiredPackageCannotUseDiskPreimage(t *testing.T) {
	root := testWorkspace(t)
	session := testSession(t, root, nil)
	defer session.Close()
	_, err := testCollectVirtual(session, context.Background(), nil, nil, []VirtualFile{{Path: "pkg/gone/g.go", Delete: true}})
	if err == nil {
		t.Fatal("caller import of retired package was satisfied by on-disk preimage")
	}
}

func TestCollectVirtualUsesShadowForPostState(t *testing.T) {
	root := testWorkspace(t)
	request := SymbolRequest{Object: goneMethod, Role: cutplan.ObjectTarget}
	session := testSession(t, root, nil)
	defer session.Close()
	post, err := testCollectVirtual(session, context.Background(), []SymbolRequest{request}, nil, []VirtualFile{{Path: "pkg/gone/g.go", Content: []byte("package gone\n\ntype T struct{}\n\nfunc (T) F() { /* virtual */ }\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(post.Objects) != 1 || post.Objects[0].Object != goneMethod {
		t.Fatalf("overlay post-state %#v", post.Objects)
	}
	if post.Objects[0].Role != cutplan.ObjectTarget || post.Objects[0].Package != "gone" {
		t.Fatalf("target classification %#v", post.Objects[0])
	}
}

func TestVirtualWorkspaceUsesCompleteShadowForChangeWithoutDeletion(t *testing.T) {
	root := testWorkspace(t)
	session := testSession(t, root, nil)
	defer session.Close()
	shadow, overlay, cleanup, err := session.virtualWorkspace([]VirtualFile{{
		Path: "pkg/gone/g.go", Content: []byte("package gone\n\n// shadow\ntype T struct{}\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if shadow == root || overlay != nil {
		t.Fatalf("changed post-state did not receive a complete isolated shadow: root=%q overlay=%#v", shadow, overlay)
	}
	data, err := os.ReadFile(filepath.Join(shadow, "pkg/gone/g.go"))
	if err != nil || !strings.Contains(string(data), "shadow") {
		t.Fatalf("shadow change not materialized: %q err=%v", data, err)
	}
	preimage, err := os.ReadFile(filepath.Join(root, "pkg/gone/g.go"))
	if err != nil || strings.Contains(string(preimage), "shadow") {
		t.Fatalf("shadow wrote source preimage: %q err=%v", preimage, err)
	}
}

func TestShadowPostStateNeverMutatesHardlinkedPreimage(t *testing.T) {
	root := testWorkspace(t)
	writeTestFile(t, root, "pkg/unused/u.go", "package unused\n")
	original, err := os.ReadFile(filepath.Join(root, "pkg/gone/g.go"))
	if err != nil {
		t.Fatal(err)
	}
	session := testSession(t, root, nil)
	defer session.Close()
	shadow, overlay, cleanup, err := session.virtualWorkspace([]VirtualFile{
		{Path: "pkg/unused/u.go", Delete: true},
		{Path: "pkg/gone/g.go", Content: []byte("package gone\n\ntype T struct{}\n\n// shadow only\nfunc (T) F() {}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if overlay != nil {
		t.Fatalf("complete shadow unexpectedly returned overlay: %#v", overlay)
	}
	if _, err := os.Stat(filepath.Join(shadow, "pkg/unused/u.go")); !os.IsNotExist(err) {
		t.Fatalf("deleted shadow file survived: %v", err)
	}
	shadowed, err := os.ReadFile(filepath.Join(shadow, "pkg/gone/g.go"))
	if err != nil || !strings.Contains(string(shadowed), "shadow only") {
		t.Fatalf("shadow rewrite missing: %q err=%v", shadowed, err)
	}
	current, err := os.ReadFile(filepath.Join(root, "pkg/gone/g.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatal("virtual post-state changed the source preimage")
	}
}

func TestVerifyDiagnosticDeltaRequiresExactApproval(t *testing.T) {
	before := []Diagnostic{{Position: cutplan.Position{Path: "a.go", Line: 1, Column: 1}, Kind: "type", Message: "old"}}
	after := []Diagnostic{{Position: cutplan.Position{Path: "b.go", Line: 2, Column: 3}, Kind: "type", Message: "new"}}
	if _, err := VerifyDiagnosticDelta(before, after, nil, nil); err == nil || !strings.Contains(err.Error(), "unapproved") {
		t.Fatalf("diagnostic delta accepted: %v", err)
	}
	delta, err := VerifyDiagnosticDelta(before, after, after, before)
	if err != nil || len(delta.Added) != 1 || len(delta.Removed) != 1 {
		t.Fatalf("diagnostic delta=%#v err=%v", delta, err)
	}
}

func TestVirtualWorkspaceRejectsSymlinkEscape(t *testing.T) {
	root := testWorkspace(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	session := testSession(t, root, nil)
	defer session.Close()
	_, err := testCollectVirtual(session, context.Background(), nil, nil, []VirtualFile{{Path: "escape/p.go", Content: []byte("package escape\n")}})
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("symlink escape accepted: %v", err)
	}
}

func TestCollectVirtualBuildsCompleteShadowForNewNestedPackage(t *testing.T) {
	root := testWorkspace(t)
	newMethod := cutplan.SymbolRef{Object: "example.com/semanticfixture/pkg/new/deep#type:T/method:F"}
	session := testSession(t, root, nil)
	defer session.Close()
	post, err := testCollectVirtual(session, context.Background(), []SymbolRequest{{Object: newMethod, Role: cutplan.ObjectTarget}}, nil, []VirtualFile{{
		Path: "pkg/new/deep/d.go", Content: []byte("package deep\n\ntype T struct{}\n\nfunc (T) F() {}\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(post.Objects) != 1 || post.Objects[0].Object != newMethod {
		t.Fatalf("new package was not discovered in full shadow: %#v", post.Objects)
	}
	foundPackage, foundFile := false, false
	for _, pkg := range post.Structure.Packages {
		foundPackage = foundPackage || pkg.Path == "example.com/semanticfixture/pkg/new/deep"
	}
	for _, file := range post.Structure.Files {
		foundFile = foundFile || file.Path == "pkg/new/deep/d.go"
	}
	if !foundPackage || !foundFile {
		t.Fatalf("post structural snapshot omitted new package: %#v", post.Structure)
	}
	if _, err := os.Stat(filepath.Join(root, "pkg/new/deep/d.go")); !os.IsNotExist(err) {
		t.Fatalf("virtual new file escaped disposable shadow: %v", err)
	}
}

func TestVirtualWorkspaceRejectsHelperAndCachePaths(t *testing.T) {
	root := testWorkspace(t)
	session := testSession(t, root, nil)
	defer session.Close()
	for _, path := range []string{".flashrefactor/lock.go", ".gocache/noise.go", ".idea/noise.go"} {
		_, err := testCollectVirtual(session, context.Background(), nil, nil, []VirtualFile{{Path: path, Content: []byte("package noise\n")}})
		if err == nil || !strings.Contains(err.Error(), "outside semantic shadow") {
			t.Fatalf("excluded path accepted %s: %v", path, err)
		}
	}
}

func TestRequestsMergeAndStructuredResidueUseOneCutplanDenominator(t *testing.T) {
	root := testWorkspace(t)
	newMethod := cutplan.SymbolRef{Object: "example.com/semanticfixture/pkg/new#type:T/method:F"}
	intent := relocationIntent(goneMethod, newMethod)
	sourceRequests, err := Requests(intent, cutplan.ObjectSource)
	if err != nil || len(sourceRequests) != 1 || sourceRequests[0].Object != goneMethod {
		t.Fatalf("source requests=%#v err=%v", sourceRequests, err)
	}
	targetRequests, err := Requests(intent, cutplan.ObjectTarget)
	if err != nil || len(targetRequests) != 1 || targetRequests[0].Object != newMethod {
		t.Fatalf("target requests=%#v err=%v", targetRequests, err)
	}
	session := testSession(t, root, nil)
	defer session.Close()
	source, err := testCollect(session, context.Background(), sourceRequests, nil)
	if err != nil {
		t.Fatal(err)
	}
	postFiles := []VirtualFile{
		{Path: "pkg/gone/g.go", Content: []byte("package gone\n\ntype T struct{}\n")},
		{Path: "pkg/new/n.go", Content: []byte("package new\n\ntype T struct{}\n\nfunc (T) F() {}\n")},
		{Path: "pkg/a/a.go", Content: []byte("package a\n\nimport \"example.com/semanticfixture/pkg/new\"\n\nfunc Call() { new.T{}.F() }\n")},
	}
	target, err := testCollectVirtual(session, context.Background(), targetRequests, nil, postFiles)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := cutplan.ResolutionRequirements(intent)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Merge(source, target, requirements)
	if err != nil || len(merged.Objects) != 2 || len(merged.Requirements) != 2 {
		t.Fatalf("merged=%#v err=%v", merged, err)
	}
	wrongTarget := target
	wrongTarget.Objects = append([]cutplan.ObjectEvidence(nil), target.Objects...)
	wrongTarget.Objects[0].Package = "wrong"
	if _, err := Merge(source, wrongTarget, requirements); err == nil || !strings.Contains(err.Error(), "package does not match") {
		t.Fatalf("merge accepted target outside cutplan destination: %v", err)
	}
	residues, err := target.Residues([]ResidueQuery{{Object: goneMethod, Paths: []string{"pkg/a/a.go", "pkg/gone/g.go"}}})
	if err != nil || len(residues) != 1 || len(residues[0].Sites) != 0 {
		t.Fatalf("old semantic residue survived: %#v err=%v", residues, err)
	}
	if _, err := target.Residues([]ResidueQuery{{Object: goneMethod, Paths: []string{"pkg/a/a.go", "pkg/a/a.go"}}}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate structured residue path accepted: %v", err)
	}
}

func TestStructuredResidueReportsResolvedUseNotText(t *testing.T) {
	root := testWorkspace(t)
	session := testSession(t, root, nil)
	defer session.Close()
	newMethod := cutplan.SymbolRef{Object: "example.com/semanticfixture/pkg/new#type:T/method:F"}
	snapshot, err := session.Collect(context.Background(), relocationIntent(goneMethod, newMethod), nil)
	if err != nil {
		t.Fatal(err)
	}
	residues, err := snapshot.Residues([]ResidueQuery{{Object: goneMethod, Paths: []string{"pkg/a/a.go"}}})
	if err != nil || len(residues) != 1 || len(residues[0].Sites) != 1 || residues[0].Sites[0].Path != "pkg/a/a.go" {
		t.Fatalf("structured residue=%#v err=%v", residues, err)
	}
}

func relocationIntent(from, to cutplan.SymbolRef) cutplan.Intent {
	return cutplan.Intent{Schema: cutplan.Version, Name: "move-method", Operations: []cutplan.Operation{{
		ID: "move", Authority: cutplan.Authority{From: "gone", To: "new"},
		Edits: []cutplan.Edit{{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{
			Source: "pkg/gone/g.go", Destination: cutplan.Destination{Path: "pkg/new/n.go", Package: "new"},
			Subjects: []cutplan.Relocation{{From: from, To: to}},
		}}},
		Bindings:  []cutplan.Binding{{Consumer: "pkg/a/a.go", From: from, To: to, Form: cutplan.BindingDirect}},
		Footprint: cutplan.Footprint{Read: []string{"pkg/a/a.go", "pkg/gone/g.go"}, Write: []string{"pkg/a/a.go", "pkg/gone/g.go", "pkg/new/n.go"}},
		Verify: cutplan.Verification{
			Laws:  []cutplan.Law{{ID: "move", Package: "./pkg/gone", Test: "TestMove"}},
			Gates: []cutplan.Gate{cutplan.GateDiagnostics, cutplan.GateImportDAG, cutplan.GateResidue},
		},
	}}}
}

type fakeLoader struct {
	authority ToolchainEvidence
	result    LoadResult
	err       error
}

func (loader fakeLoader) Load(context.Context, LoadRequest) (LoadResult, error) {
	result := loader.result
	if result.Toolchain.Loader == "" {
		result.Toolchain = loader.authority
	}
	if result.Toolchain.Loader == "" {
		result.Toolchain = validFakeAuthority()
	}
	return result, loader.err
}

func validFakeAuthority() ToolchainEvidence {
	return ToolchainEvidence{Loader: packagesAuthority, Go: ExecutableIdentity{Path: "/fake/go", SHA256: strings.Repeat("a", 64), Version: "go version go1.23"}, BuildEnvSHA256: strings.Repeat("b", 64), ModuleGraphSHA256: strings.Repeat("c", 64)}
}

// Test collectors exercise the lower-level request validation directly. The
// production boundary is Intent-only; tests supply explicit workspace seeds
// only when intentionally collecting no object denominator.
func testCollect(session *Session, ctx context.Context, requests []SymbolRequest, diagnostics []string) (Snapshot, error) {
	return session.collect(ctx, requests, loadScope{Files: testSeedPaths(session, requests)}, diagnostics, nil, cutplan.ObjectSource)
}

func testCollectVirtual(session *Session, ctx context.Context, requests []SymbolRequest, diagnostics []string, files []VirtualFile) (Snapshot, error) {
	return session.collect(ctx, requests, loadScope{Files: testSeedPaths(session, requests), ExpandFileOwners: true}, diagnostics, files, cutplan.ObjectTarget)
}

func testSeedPaths(session *Session, requests []SymbolRequest) []string {
	if len(requests) != 0 || session == nil {
		return nil
	}
	result := make([]string, 0)
	_ = filepath.WalkDir(session.root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".go") {
			relative, relativeErr := filepath.Rel(session.root, path)
			if relativeErr == nil {
				result = append(result, relative)
			}
		}
		return nil
	})
	sort.Strings(result)
	return result
}

func testWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/semanticfixture\n\ngo 1.23.0\n")
	writeTestFile(t, root, "pkg/gone/g.go", "package gone\n\ntype T struct{}\n\nfunc (T) F() {}\n")
	writeTestFile(t, root, "pkg/a/a.go", "package a\n\nimport \"example.com/semanticfixture/pkg/gone\"\n\nfunc Call() { gone.T{}.F() }\n")
	return root
}

func testVariantWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/variants\n\ngo 1.23.0\n")
	writeTestFile(t, root, "pkg/p/p.go", "package p\n\ntype T struct{}\n\nfunc (T) M() {}\n\nfunc Use() { T{}.M() }\n")
	writeTestFile(t, root, "pkg/p/p_test.go", "package p\n\nfunc localOnly() {}\n\nfunc testInternal() { T{}.M() }\n")
	writeTestFile(t, root, "pkg/p/external_test.go", "package p_test\n\nimport \"example.com/variants/pkg/p\"\n\nfunc testExternal() { p.T{}.M() }\n")
	return root
}

func testObjectWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/objects\n\ngo 1.23.0\n")
	writeTestFile(t, root, "pkg/o/o.go", "package o\n\ntype Named struct{}\n\nfunc Helper() {}\n\nfunc Caller() { Helper() }\n\nvar Value = 1\n\nconst Limit = 2\n")
	return root
}

func writeTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testSession(t *testing.T, root string, loader Loader) *Session {
	t.Helper()
	session, err := NewSession(Config{Root: root, Flashrefactor: "test", CacheParent: root, Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
