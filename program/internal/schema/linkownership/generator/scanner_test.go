package generator

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	root := filepath.Dir(file)
	for i := 0; i < 5; i++ {
		root = filepath.Dir(root)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

func checkedBuildContext(t *testing.T) BuildContext {
	t.Helper()
	context, err := canonicalBuildContextChecked()
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func TestScanProductionLinkSurface(t *testing.T) {
	scan, err := Scan(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if !scan.ProductionOnly || scan.Root.PackagePath != linkImportPath {
		t.Fatalf("unexpected package scan: production=%v path=%q", scan.ProductionOnly, scan.Root.PackagePath)
	}
	if len(scan.Types.Declarations) == 0 || len(scan.Dependencies.ImportEdges) == 0 || callerPackageCount(scan.Uses, scan.Root.PackagePath) == 0 {
		t.Fatalf("incomplete production inventory: declarations=%d imports=%d callers=%d", len(scan.Types.Declarations), len(scan.Dependencies.ImportEdges), callerPackageCount(scan.Uses, scan.Root.PackagePath))
	}
	wantPackages := map[string]bool{
		linkImportPath:                     false,
		linkImportPath + "/keyspace":       false,
		linkImportPath + "/project":        false,
		linkImportPath + "/static":         false,
		linkImportPath + "/internal/radix": false,
	}
	for _, pkg := range scan.Sources.Packages {
		if _, wanted := wantPackages[pkg.Path]; wanted {
			wantPackages[pkg.Path] = true
		}
	}
	for path, found := range wantPackages {
		if !found {
			t.Fatalf("live Link-family package is absent: %s", path)
		}
	}
	foundReverseCompilerInput := false
	for _, source := range scan.Sources.ProductionSources {
		path := source.Path
		if path == "compiler/bytecode/bytecode.go" {
			foundReverseCompilerInput = true
		}
		if filepath.IsAbs(path) || strings.Contains(path, "__legacy/") || strings.Contains(path, "/analysis/test/") ||
			strings.HasPrefix(path, "analysis/test/") || strings.Contains(path, "/program/testfixture/") ||
			strings.HasPrefix(path, "program/testfixture/") || strings.HasSuffix(path, "_test.go") {
			t.Fatalf("non-live production input %q", path)
		}
	}
	if !foundReverseCompilerInput {
		t.Fatal("complete reverse import closure omitted compiler/bytecode/bytecode.go")
	}
	foundTypedSelection := false
	for _, use := range scan.Uses {
		if use.PackagePath == scan.Root.PackagePath || strings.HasPrefix(use.PackagePath, scan.Root.PackagePath+"/") {
			continue
		}
		if strings.Contains(use.PackagePath, "/__legacy/") || strings.HasPrefix(use.PackagePath, moduleImportPath+"/analysis/test") ||
			strings.HasPrefix(use.PackagePath, moduleImportPath+"/program/testfixture") {
			t.Fatalf("non-live typed caller use: %+v", use)
		}
		if use.Symbol == "" || use.Evidence == "" || use.SourceFile == "" || filepath.IsAbs(use.SourceFile) || use.Line <= 0 || use.Column <= 0 {
			t.Fatalf("malformed typed use: %+v", use)
		}
		if use.Evidence == "method-value" || use.Evidence == "method-expression" || use.Evidence == "field-selection" {
			foundTypedSelection = true
		}
	}
	if !foundTypedSelection {
		t.Fatal("typed Link selection evidence is absent")
	}
	if scan.Build.SourceDigest == [32]byte{} {
		t.Fatal("source digest is empty")
	}
	for _, source := range scan.Sources.ProductionSources {
		path := source.Path
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			t.Fatalf("non-production source path %q", path)
		}
	}
	linkSurfaceID := findSurfaceID(scan.Types.Surfaces, linkImportPath, typeSurfaceID(linkImportPath, "Link"))
	if linkSurfaceID == "" {
		t.Fatal("declared Link structural surface missing")
	}
	rootFields := 0
	for _, field := range scan.Types.Structure.Fields {
		if field.SurfaceID == linkSurfaceID {
			rootFields++
		}
	}
	if rootFields != 38 {
		t.Fatalf("Link structural root fields = %d, want 38", rootFields)
	}
	if got := declarationCount(scan.Types.Declarations, linkImportPath, "method", "Link"); got != 185 {
		t.Fatalf("Link declared method count = %d, want 185", got)
	}
	if got := declarationCount(scan.Types.Declarations, linkImportPath, "func", ""); got == 0 {
		t.Fatal("Link package function declarations are absent")
	}
	for _, declaration := range scan.Types.Declarations {
		if declaration.FactID == "" || declaration.SourceFile == "" || declaration.Line <= 0 || declaration.Column <= 0 {
			t.Fatalf("inexact declaration commitment: %+v", declaration)
		}
		if declaration.Kind == "method" && declaration.OwnerType == "Link" && declaration.Signature == "" {
			t.Fatalf("method signature commitment is empty: %+v", declaration)
		}
	}
	foundMap := false
	for _, container := range scan.Types.Structure.Maps {
		if strings.HasSuffix(container.Path, "moduleInitOutcomeOrdinalsByID") {
			foundMap = true
			if container.Key == "" || container.Value == "" || !strings.Contains(container.Value, "moduleInitOutcomeCoordinate") {
				t.Fatalf("map key/value structural evidence is incomplete: %+v", container)
			}
		}
	}
	if !foundMap {
		t.Fatal("Link module-init outcome index shape is absent")
	}
}

func TestScanRejectsImportCycles(t *testing.T) {
	err := validateImportGraph(map[string][]string{
		"a": {"b"},
		"b": {"a"},
	})
	if !errors.Is(err, ErrImportCycle) {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestRunFailsClosedWithoutManifest(t *testing.T) {
	scan, err := Scan(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = runWithScan(t.TempDir(), ModeInventory, false, scan)
	if !errors.Is(err, ErrManifestMissing) {
		t.Fatalf("missing manifest error = %v", err)
	}
}

func TestScanFamilyTypedEvidenceAndProductionSelection(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test\n\ngo 1.23\n")
	write("program/link/link.go", `package link

type Link struct { Value int }
func (*Link) Do() {}
func Pick[T any](value T) T { return value }
`)
	write("program/link/dead.go", `//go:build never
package link
func Dead() {}
`)
	write("program/link/link_test.go", `package link
func TestOnly() {}
`)
	write("caller/caller.go", `package caller
import l "example.test/program/link"
func Use(value *l.Link) {
	value.Do()
	_ = value.Do
	_ = (*l.Link).Do
	_ = l.Pick(1)
}

`)
	write("blank/blank.go", `package blank
import _ "example.test/program/link"
`)

	scan, err := scanFamily(root, "example.test/program/link")
	if err != nil {
		t.Fatal(err)
	}
	surfaceID := findSurfaceID(scan.Types.Surfaces, "example.test/program/link", typeSurfaceID("example.test/program/link", "Link"))
	fields := 0
	for _, field := range scan.Types.Structure.Fields {
		if field.SurfaceID == surfaceID {
			fields++
		}
	}
	if surfaceID == "" || fields != 1 {
		t.Fatalf("synthetic Link structural surface id=%q fields=%d", surfaceID, fields)
	}
	if got := declarationCount(scan.Types.Declarations, "example.test/program/link", "method", "Link"); got != 1 {
		t.Fatalf("synthetic Link declared method count = %d, want 1", got)
	}
	for _, source := range scan.Sources.ProductionSources {
		path := source.Path
		if strings.Contains(path, "dead.go") || strings.HasSuffix(path, "_test.go") {
			t.Fatalf("excluded source entered scan: %q", path)
		}
	}
	seenEvidence := map[string]bool{}
	for _, use := range scan.Uses {
		seenEvidence[use.Evidence] = true
	}
	for _, evidence := range []string{"method-value", "method-expression", "instance"} {
		if !seenEvidence[evidence] {
			t.Fatalf("synthetic typed evidence %q absent: %#v", evidence, scan.Uses)
		}
	}
	for _, use := range scan.Uses {
		if use.PackagePath == "example.test/blank" {
			t.Fatal("blank-import package became a typed caller")
		}
	}
}

func TestScanFamilyReverseClosureAliasChain(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test\n\ngo 1.23\n")
	write("program/link/link.go", `package link

type Link struct{ Value int }
func (*Link) Do() {}
`)
	write("bridge/bridge.go", `package bridge

import l "example.test/program/link"

type Direct = l.Link
type Chained = Direct
type Pointer = *l.Link
type Slice = []l.Link
type Map = map[l.Link]l.Link
type Anonymous = struct{ Value l.Link }
type Wrapped l.Link
`)
	write("consumer/consumer.go", `package consumer

import b "example.test/bridge"

func Use(value b.Chained) {
	value.Do()
	_ = b.Direct{}
	_ = b.Wrapped{}
}

func UseComposite(pointer b.Pointer, slice b.Slice, table b.Map, anonymous b.Anonymous) {
	pointer.Do()
	_ = slice
	_ = table
	_ = anonymous
}
`)

	first, err := scanFamily(root, "example.test/program/link")
	if err != nil {
		t.Fatal(err)
	}
	second, err := scanFamily(root, "example.test/program/link")
	if err != nil {
		t.Fatal(err)
	}
	if !sameProductionSources(first.Sources.ProductionSources, second.Sources.ProductionSources) {
		t.Fatalf("reverse closure is nondeterministic: first=%v second=%v", first.Sources.ProductionSources, second.Sources.ProductionSources)
	}
	for _, path := range []string{"bridge/bridge.go", "consumer/consumer.go"} {
		if !containsProductionSource(first.Sources.ProductionSources, path) {
			t.Fatalf("reverse-closure source %q is absent: %v", path, first.Sources.ProductionSources)
		}
	}
	var chained, chainedMethod, plainCompositeMethod, wrapped bool
	var compositeSpellings []string
	for _, use := range first.Uses {
		if use.FactID == "" || use.FactID != useSiteFactID(use) {
			t.Fatalf("use fact ID is not canonical: %+v", use)
		}
		if use.PackagePath == "example.test/consumer" && strings.HasSuffix(use.Symbol, ".Chained") {
			chained = true
			if use.TargetDeclID == "" || len(use.AliasChain) != 2 || use.Evidence != "alias-chain/use" {
				t.Fatalf("chained alias evidence is incomplete: %+v", use)
			}
		}
		if use.PackagePath == "example.test/consumer" && strings.HasSuffix(use.Symbol, ".Link.Do") {
			if len(use.AliasChain) == 2 {
				chainedMethod = true
				if use.Evidence != "alias-chain/method-value" {
					t.Fatalf("selection alias evidence is incomplete: %+v", use)
				}
			} else if len(use.AliasChain) == 0 {
				plainCompositeMethod = true
			}
		}
		if use.PackagePath == "example.test/consumer" && (strings.HasSuffix(use.Symbol, ".Pointer") || strings.HasSuffix(use.Symbol, ".Slice") || strings.HasSuffix(use.Symbol, ".Map") || strings.HasSuffix(use.Symbol, ".Anonymous")) {
			compositeSpellings = append(compositeSpellings, use.Symbol)
		}
		if use.PackagePath == "example.test/consumer" && strings.HasSuffix(use.Symbol, ".Wrapped") {
			wrapped = true
		}
	}
	if !chained {
		t.Fatalf("external chained alias use is absent: %v", first.Uses)
	}
	if !chainedMethod {
		t.Fatalf("external chained alias selection evidence is absent: %v", first.Uses)
	}
	if !plainCompositeMethod {
		t.Fatalf("concrete method selection through a composite alias is absent: %v", first.Uses)
	}
	if len(compositeSpellings) != 0 {
		t.Fatalf("composite aliases acquired singular Link declaration uses: %v", compositeSpellings)
	}
	if wrapped {
		t.Fatalf("defined wrapper was attributed to Link: %v", first.Uses)
	}
	if !sameUseSites(first.Uses, second.Uses) {
		t.Fatalf("alias-chain/fact IDs are nondeterministic:\nfirst=%v\nsecond=%v", first.Uses, second.Uses)
	}
}

func TestValidateProductionFilesRejectsOutOfRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.go")
	outside := filepath.Join(root, "..", "outside.go")
	if err := os.WriteFile(inside, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := validateProductionFiles(root, &packages.Package{PkgPath: "example.test/p", CompiledGoFiles: []string{inside, outside}})
	if err == nil || !errors.Is(err, ErrTypeCheck) || !strings.Contains(err.Error(), "out-of-root") {
		t.Fatalf("out-of-root production file was accepted: %v", err)
	}
}

func TestValidateProductionFilesRejectsExcludedCompiledFile(t *testing.T) {
	root := t.TempDir()
	excluded := filepath.Join(root, "__legacy", "old.go")
	if err := os.MkdirAll(filepath.Dir(excluded), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excluded, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := validateProductionFiles(root, &packages.Package{PkgPath: "example.test/p", CompiledGoFiles: []string{excluded}})
	if err == nil || !strings.Contains(err.Error(), "non-production compiled file") {
		t.Fatalf("excluded compiled source was accepted: %v", err)
	}
}

func TestIndexPackagesRejectsMalformedMetadata(t *testing.T) {
	nilImport := &packages.Package{PkgPath: "example.test/root", Imports: map[string]*packages.Package{"example.test/missing": nil}}
	if _, err := indexPackagesChecked([]*packages.Package{nilImport}); err == nil || !strings.Contains(err.Error(), "nil imported package") {
		t.Fatalf("nil imported package metadata was accepted: %v", err)
	}
	first := &packages.Package{PkgPath: "example.test/dup", Name: "dup", CompiledGoFiles: []string{"/one/dup.go"}}
	second := &packages.Package{PkgPath: "example.test/dup", Name: "dup", CompiledGoFiles: []string{"/two/dup.go"}}
	if _, err := indexPackagesChecked([]*packages.Package{first, second}); err == nil || !strings.Contains(err.Error(), "duplicate package metadata") {
		t.Fatalf("duplicate package metadata was accepted: %v", err)
	}
	childFirst := &packages.Package{PkgPath: "example.test/child", CompiledGoFiles: []string{"/one/child.go"}}
	childSecond := &packages.Package{PkgPath: "example.test/child", CompiledGoFiles: []string{"/two/child.go"}}
	rootFirst := &packages.Package{PkgPath: "example.test/root", Imports: map[string]*packages.Package{"example.test/child": childFirst}}
	rootSecond := &packages.Package{PkgPath: "example.test/root", Imports: map[string]*packages.Package{"example.test/child": childSecond}}
	if _, err := indexPackagesChecked([]*packages.Package{rootFirst, rootSecond}); err == nil || !strings.Contains(err.Error(), "duplicate package metadata") {
		t.Fatalf("duplicate imported package metadata was accepted: %v", err)
	}
	if err := validateIndexedImports(map[string]*packages.Package{"wrong-key": {PkgPath: "example.test/right"}}); err == nil || !strings.Contains(err.Error(), "does not match package path") {
		t.Fatalf("indexed package key mismatch was accepted: %v", err)
	}
	root := t.TempDir()
	linkFile := filepath.Join(root, "link.go")
	if err := os.WriteFile(linkFile, []byte("package link\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := &packages.Package{PkgPath: "example.test/program/link", CompiledGoFiles: []string{linkFile}, Imports: map[string]*packages.Package{"example.test/missing": {PkgPath: "example.test/missing"}}}
	if _, err := reverseImportClosure(root, link.PkgPath, map[string]*packages.Package{link.PkgPath: link}); err == nil || !strings.Contains(err.Error(), "absent from indexed metadata") {
		t.Fatalf("missing reverse-closure importer metadata was accepted: %v", err)
	}
}

func TestReverseClosureRejectsExcludedBridgeAndInRootSourceHole(t *testing.T) {
	root := t.TempDir()
	write := func(name string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	linkPath := moduleImportPath + "/program/link"
	linkFile := write("program/link/link.go")
	excludedPath := moduleImportPath + "/analysis/test"
	excludedFile := write("analysis/test/bridge.go")
	liveFile := write("live/consumer.go")
	link := &packages.Package{PkgPath: linkPath, Dir: filepath.Dir(linkFile), CompiledGoFiles: []string{linkFile}, Imports: map[string]*packages.Package{}}
	excluded := &packages.Package{PkgPath: excludedPath, Dir: filepath.Dir(excludedFile), CompiledGoFiles: []string{excludedFile}, Imports: map[string]*packages.Package{linkPath: link}}
	live := &packages.Package{PkgPath: moduleImportPath + "/live", Dir: filepath.Dir(liveFile), CompiledGoFiles: []string{liveFile}, Imports: map[string]*packages.Package{excludedPath: excluded}}
	_, err := reverseImportClosure(root, linkPath, map[string]*packages.Package{linkPath: link, excludedPath: excluded, live.PkgPath: live})
	if err == nil || !strings.Contains(err.Error(), "excluded importer") {
		t.Fatalf("excluded bridge was treated as transparent: %v", err)
	}
	inertPath := moduleImportPath + "/analysis/test/inert"
	inertFile := write("analysis/test/inert.go")
	inert := &packages.Package{PkgPath: inertPath, Dir: filepath.Dir(inertFile), CompiledGoFiles: []string{inertFile}, Imports: map[string]*packages.Package{linkPath: link}}
	inertSelected, err := reverseImportClosure(root, linkPath, map[string]*packages.Package{linkPath: link, inertPath: inert})
	if err != nil {
		t.Fatalf("inert excluded leaf was rejected without a production ancestor: %v", err)
	}
	if inertSelected[inertPath] != nil {
		t.Fatal("inert excluded leaf entered selected production closure")
	}
	dualFile := write("dual/consumer.go")
	dual := &packages.Package{PkgPath: moduleImportPath + "/dual", Dir: filepath.Dir(dualFile), CompiledGoFiles: []string{dualFile}, Imports: map[string]*packages.Package{linkPath: link, excludedPath: excluded}}
	_, err = reverseImportClosure(root, linkPath, map[string]*packages.Package{linkPath: link, excludedPath: excluded, dual.PkgPath: dual})
	if err == nil || !strings.Contains(err.Error(), "excluded importer") {
		t.Fatalf("clean plus excluded paths to one production consumer were accepted: %v", err)
	}
	unrelatedFile := write("analysis/test/unrelated.go")
	unrelated := &packages.Package{PkgPath: moduleImportPath + "/analysis/test/unrelated", Dir: filepath.Dir(unrelatedFile), CompiledGoFiles: []string{unrelatedFile}, Imports: map[string]*packages.Package{}}
	if _, err := reverseImportClosure(root, linkPath, map[string]*packages.Package{linkPath: link, unrelated.PkgPath: unrelated}); err != nil {
		t.Fatalf("unrelated excluded package was not ignored: %v", err)
	}

	holeFile := write("hole/placeholder.go")
	hole := &packages.Package{PkgPath: moduleImportPath + "/hole", Dir: filepath.Dir(holeFile), Imports: map[string]*packages.Package{linkPath: link}}
	_, err = reverseImportClosure(root, linkPath, map[string]*packages.Package{linkPath: link, hole.PkgPath: hole})
	if err == nil || !strings.Contains(err.Error(), "no production source files") {
		t.Fatalf("in-root no-source importer was omitted: %v", err)
	}
}

func TestValidateProductionFilesRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "escape.go")
	if err := os.WriteFile(target, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.go")
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
			t.Skipf("symlink creation is not permitted: %v", err)
		}
		t.Fatal(err)
	}
	_, err := validateProductionFiles(root, &packages.Package{PkgPath: "example.test/p", CompiledGoFiles: []string{link}})
	if err == nil || !strings.Contains(err.Error(), "out-of-root") {
		t.Fatalf("symlink escape was accepted: %v", err)
	}
	excludedTarget := filepath.Join(root, "analysis", "test", "hidden.go")
	if err := os.MkdirAll(filepath.Dir(excludedTarget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludedTarget, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	excludedLink := filepath.Join(root, "alias.go")
	if err := os.Symlink(excludedTarget, excludedLink); err != nil {
		if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
			t.Skipf("symlink creation is not permitted: %v", err)
		}
		t.Fatal(err)
	}
	_, err = validateProductionFiles(root, &packages.Package{PkgPath: "example.test/live", CompiledGoFiles: []string{excludedLink}})
	if err == nil || !strings.Contains(err.Error(), "excluded physical") {
		t.Fatalf("symlink to an in-root excluded source was accepted: %v", err)
	}
	// A metadata path and a typed path that resolve to the same inode are still
	// not interchangeable when their logical identities differ. This catches
	// a symlink-induced load mismatch before any typed evidence is attributed.
	physical := filepath.Join(root, "physical.go")
	logical := filepath.Join(root, "logical.go")
	if err := os.WriteFile(physical, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, logical); err != nil {
		if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
			t.Skipf("symlink creation is not permitted: %v", err)
		}
		t.Fatal(err)
	}
	metadata := productionFileSet{Files: []productionFilePair{{Logical: logical, Physical: physical}}}
	fileSet := productionFileSet{Files: []productionFilePair{{Logical: physical, Physical: physical}}}
	if sameFileSets(metadata, fileSet) {
		t.Fatal("symlink-induced metadata/typed mismatch was accepted")
	}
	permutedLeft := productionFileSet{Files: []productionFilePair{{Logical: "a.go", Physical: "one.go"}, {Logical: "b.go", Physical: "two.go"}}}
	permutedRight := productionFileSet{Files: []productionFilePair{{Logical: "a.go", Physical: "two.go"}, {Logical: "b.go", Physical: "one.go"}}}
	if sameFileSets(permutedLeft, permutedRight) {
		t.Fatal("logical/physical file pair permutation was accepted")
	}
}

func TestResolvedModulesRetainOriginalAndReplacementIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("a.test v1.0.0 h1:asum\nc.test v1.0.0 h1:csum\nb.test v2.0.0 h1:bsum\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	replacement := func(original string) *packages.Package {
		source := filepath.Join(root, strings.ReplaceAll(original, ".", "_")+".go")
		if err := os.WriteFile(source, []byte("package replacement\nconst Value = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return &packages.Package{PkgPath: original, CompiledGoFiles: []string{source}, Module: &packages.Module{Path: original, Version: "v1.0.0", GoMod: "/original/" + original + ".mod", Replace: &packages.Module{Path: "b.test", Version: "v2.0.0", GoMod: "/replacement/B.mod"}}}
	}
	modules, err := resolvedModules(root, map[string]*packages.Package{"a.test": replacement("a.test"), "c.test": replacement("c.test")})
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 2 {
		t.Fatalf("original modules collapsed through replacement: %+v", modules)
	}
	for _, module := range modules {
		if module.Path == "a.test" {
			if module.Sum != "h1:asum" || module.ResolvedPath != "b.test" || module.ResolvedSum != "h1:bsum" || module.ResolvedGoMod != "module:b.test@v2.0.0/go.mod" {
				t.Fatalf("A replacement identity incomplete: %+v", module)
			}
		}
		if module.Path == "c.test" && module.Sum != "h1:csum" {
			t.Fatalf("C original identity incomplete: %+v", module)
		}
	}
	reordered := []ModuleInfo{modules[1], modules[0]}
	context := checkedBuildContext(t)
	firstFingerprint, err := buildFingerprint([32]byte{1}, modules, context)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := buildFingerprint([32]byte{1}, reordered, context)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint != secondFingerprint {
		t.Fatal("module fingerprint depends on input order")
	}
}

func TestResolvedModulesFollowTerminalReplacementChainAndRejectAmbiguity(t *testing.T) {
	root := t.TempDir()
	replacementDir := filepath.Join(root, "replacement")
	if err := os.MkdirAll(replacementDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(replacementDir, "go.mod")
	source := filepath.Join(replacementDir, "value.go")
	if err := os.WriteFile(goMod, []byte("module replacement.test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package replacement\nconst Value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := &packages.Module{Path: replacementDir, GoMod: goMod, Dir: replacementDir}
	middle := &packages.Module{Path: "b.test", Version: "v2.0.0", GoMod: "/cache/b/go.mod", Replace: target}
	original := &packages.Module{Path: "a.test", Version: "v1.0.0", GoMod: "/cache/a/go.mod", Replace: middle}
	pkg := &packages.Package{PkgPath: "replacement.test", CompiledGoFiles: []string{source}, Module: original}
	modules, err := resolvedModules(root, map[string]*packages.Package{pkg.PkgPath: pkg})
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 1 || modules[0].ResolvedPath != "local/replacement" || modules[0].ResolvedContentDigest == "" {
		t.Fatalf("terminal local replacement was not committed: %+v", modules)
	}

	cycleA := &packages.Module{Path: "cycle-a.test", Version: "v1.0.0", GoMod: "/cache/a/one.mod"}
	cycleAAgain := &packages.Module{Path: "cycle-a.test", Version: "v1.0.0", GoMod: "/cache/a/two.mod"}
	cycleB := &packages.Module{Path: "cycle-b.test", Version: "v1.0.0", GoMod: "/cache/b/mod"}
	cycleA.Replace = cycleB
	cycleB.Replace = cycleAAgain
	cycleAAgain.Replace = cycleB
	cyclePkg := &packages.Package{PkgPath: "cycle-a.test/pkg", Module: cycleA}
	if _, err := resolvedModules(root, map[string]*packages.Package{cyclePkg.PkgPath: cyclePkg}); err == nil || !strings.Contains(err.Error(), "replacement cycle") {
		t.Fatalf("replacement cycle was accepted: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("ambiguous.test v1.0.0 h1:a\nb.test v1.0.0 h1:b\nc.test v1.0.0 h1:c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileB := filepath.Join(root, "b.go")
	fileC := filepath.Join(root, "c.go")
	if err := os.WriteFile(fileB, []byte("package b\nconst B = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileC, []byte("package c\nconst C = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ambiguousA := &packages.Module{Path: "ambiguous.test", Version: "v1.0.0", GoMod: "/cache/ambiguous/go.mod", Replace: &packages.Module{Path: "b.test", Version: "v1.0.0", GoMod: "/cache/b/go.mod"}}
	ambiguousC := &packages.Module{Path: "ambiguous.test", Version: "v1.0.0", GoMod: "/cache/ambiguous/go.mod", Replace: &packages.Module{Path: "c.test", Version: "v1.0.0", GoMod: "/cache/c/go.mod"}}
	if _, err := resolvedModules(root, map[string]*packages.Package{
		"ambiguous.test/b": {PkgPath: "ambiguous.test/b", CompiledGoFiles: []string{fileB}, Module: ambiguousA},
		"ambiguous.test/c": {PkgPath: "ambiguous.test/c", CompiledGoFiles: []string{fileC}, Module: ambiguousC},
	}); err == nil || !strings.Contains(err.Error(), "ambiguous replacement") {
		t.Fatalf("ambiguous replacement identities were accepted: %v", err)
	}
	sameTerminalA := &packages.Module{Path: "same.test", Version: "v1.0.0", GoMod: "/cache/same/one.mod", Replace: &packages.Module{Path: "b.test", Version: "v1.0.0", GoMod: "/cache/b/one.mod"}}
	sameTerminalB := &packages.Module{Path: "same.test", Version: "v1.0.0", GoMod: "/cache/same/two.mod", Replace: &packages.Module{Path: "b.test", Version: "v1.0.0", GoMod: "/cache/b/two.mod"}}
	sameModules, err := resolvedModules(root, map[string]*packages.Package{
		"same.test/one": {PkgPath: "same.test/one", CompiledGoFiles: []string{fileB}, Module: sameTerminalA},
		"same.test/two": {PkgPath: "same.test/two", CompiledGoFiles: []string{fileB}, Module: sameTerminalB},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sameModules) != 1 {
		t.Fatalf("equivalent terminal cache identities falsely conflicted: %+v", sameModules)
	}
}

func TestResolvedModulesIncludeConnectorAndMutationIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("example.external v1.0.0 h1:external\nexample.external v1.1.0 h1:external-new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "external.go")
	if err := os.WriteFile(source, []byte("package external\nconst Value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	connector := &packages.Package{PkgPath: "example.external/pkg", CompiledGoFiles: []string{source}, Module: &packages.Module{Path: "example.external", Version: "v1.0.0", GoMod: "/cache/module/example.external@v1.0.0/go.mod"}}
	first, err := resolvedModules(root, map[string]*packages.Package{"example.external/pkg": connector})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Path != "example.external" || first[0].GoMod != "module:example.external@v1.0.0/go.mod" {
		t.Fatalf("out-of-root connector module was not canonically retained: %+v", first)
	}
	if err := os.WriteFile(source, []byte("package external\nconst Value = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutatedBytes := &packages.Package{PkgPath: connector.PkgPath, CompiledGoFiles: []string{source}, Module: &packages.Module{Path: "example.external", Version: "v1.0.0", GoMod: "/cache/module/example.external@v1.0.0/go.mod"}}
	third, err := resolvedModules(root, map[string]*packages.Package{mutatedBytes.PkgPath: mutatedBytes})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ResolvedContentDigest == third[0].ResolvedContentDigest {
		t.Fatal("typed external module byte mutation did not change committed content identity")
	}
	mutated := &packages.Package{PkgPath: connector.PkgPath, CompiledGoFiles: []string{source}, Module: &packages.Module{Path: "example.external", Version: "v1.1.0", GoMod: "/different/cache/path/go.mod"}}
	second, err := resolvedModules(root, map[string]*packages.Package{mutated.PkgPath: mutated})
	if err != nil {
		t.Fatal(err)
	}
	context := checkedBuildContext(t)
	firstFingerprint, err := buildFingerprint([32]byte{7}, first, context)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := buildFingerprint([32]byte{7}, second, context)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint == secondFingerprint {
		t.Fatal("connector module mutation did not change the committed fingerprint")
	}
}

func TestLocalReplacementContentMutationChangesIdentity(t *testing.T) {
	root := t.TempDir()
	replacementDir := filepath.Join(root, "replacement")
	goMod := filepath.Join(replacementDir, "go.mod")
	source := filepath.Join(replacementDir, "value.go")
	if err := os.MkdirAll(replacementDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goMod, []byte("module replacement.test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package replacement\nconst Value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	module := &packages.Module{Path: "example.external", Version: "v1.0.0", GoMod: "/cache/example.external/go.mod", Replace: &packages.Module{Path: replacementDir, GoMod: goMod}}
	pkg := &packages.Package{PkgPath: "replacement.test", CompiledGoFiles: []string{source}, Module: module}
	first, err := resolvedModules(root, map[string]*packages.Package{pkg.PkgPath: pkg})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].ResolvedContentDigest == "" || !strings.HasPrefix(first[0].ResolvedPath, "local/") || first[0].ResolvedGoMod != "replacement/go.mod" {
		t.Fatalf("local replacement content identity incomplete: %+v", first)
	}
	if err := os.WriteFile(source, []byte("package replacement\nconst Value = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := resolvedModules(root, map[string]*packages.Package{pkg.PkgPath: pkg})
	if err != nil {
		t.Fatal(err)
	}
	context := checkedBuildContext(t)
	firstFingerprint, err := buildFingerprint([32]byte{8}, first, context)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := buildFingerprint([32]byte{8}, second, context)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ResolvedContentDigest == second[0].ResolvedContentDigest || firstFingerprint == secondFingerprint {
		t.Fatal("local replacement content mutation did not change the committed identity")
	}
}

func TestLocalReplacementDigestFiltersByTerminalModuleIdentity(t *testing.T) {
	root := t.TempDir()
	replacementDir := filepath.Join(root, "replacement")
	if err := os.MkdirAll(replacementDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(replacementDir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module replacement.test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(replacementDir, "live.go")
	if err := os.WriteFile(live, []byte("package replacement\nconst Live = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(root, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.go")
	if err := os.WriteFile(external, []byte("package external\nconst External = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := &packages.Module{Path: replacementDir, GoMod: goMod, Dir: replacementDir}
	byPath := map[string]*packages.Package{
		"replacement.test": {
			PkgPath:         "replacement.test",
			CompiledGoFiles: []string{live},
			Module:          &packages.Module{Path: "example.external", Version: "v1.0.0", Replace: target},
		},
		"example.test": {
			PkgPath:         "example.test",
			CompiledGoFiles: []string{mainFile},
			Module:          &packages.Module{Path: "example.test", Dir: root, GoMod: filepath.Join(root, "go.mod")},
		},
		"external.test/pkg": {
			PkgPath:         "external.test/pkg",
			CompiledGoFiles: []string{external},
			Module:          &packages.Module{Path: "external.test", Version: "v1.0.0", GoMod: "/cache/external/go.mod"},
		},
	}
	first, err := localModuleContentDigest(root, replacementDir, target, byPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainFile, []byte("package main\nconst Changed = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, []byte("package external\nconst External = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := localModuleContentDigest(root, replacementDir, target, byPath)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("non-terminal main/external module inputs changed local replacement identity")
	}
}

func TestResolvedModulesRejectUncommittedExternalAndFilesystemPaths(t *testing.T) {
	root := t.TempDir()
	external := &packages.Package{PkgPath: "external.test/pkg", Module: &packages.Module{Path: "external.test", Version: "v1.0.0", GoMod: "/cache/external/go.mod"}}
	if _, err := resolvedModules(root, map[string]*packages.Package{external.PkgPath: external}); err == nil || !strings.Contains(err.Error(), "typed package closure") {
		t.Fatalf("uncommitted external module was accepted: %v", err)
	}
	badOriginal := &packages.Package{PkgPath: "bad/pkg", Module: &packages.Module{Path: filepath.Join(root, "bad"), Version: "v1.0.0", GoMod: "/cache/bad/go.mod"}}
	if _, err := resolvedModules(root, map[string]*packages.Package{badOriginal.PkgPath: badOriginal}); err == nil || !strings.Contains(err.Error(), "original module path") {
		t.Fatalf("filesystem-shaped original module path was accepted: %v", err)
	}
	badResolved := &packages.Package{PkgPath: "bad/pkg", Module: &packages.Module{Path: "example.test", Version: "v1.0.0", GoMod: "/cache/example/go.mod", Replace: &packages.Module{Path: filepath.Join(root, "bad"), Version: "v1.1.0", GoMod: "/cache/bad/go.mod"}}}
	if _, err := resolvedModules(root, map[string]*packages.Package{badResolved.PkgPath: badResolved}); err == nil || !strings.Contains(err.Error(), "resolved module path") {
		t.Fatalf("filesystem-shaped resolved module path was accepted: %v", err)
	}
}

func TestCanonicalBuildContextIsPinnedAndFingerprinted(t *testing.T) {
	context := checkedBuildContext(t)
	if context.GOWORK != "off" || context.GOENV != "off" || context.GOTOOLCHAIN != "local" ||
		context.GOOS == "" || context.GOARCH == "" || (context.CGOEnabled != "0" && context.CGOEnabled != "1") || context.GOFLAGS != "" || context.BuildTags != "" || context.Toolchain == "" {
		t.Fatalf("build context is not explicit: %+v", context)
	}
	if filepath.Clean(context.path) != filepath.Clean(filepath.Dir(context.goExecutable)) || filepath.Clean(context.goRoot) != filepath.Clean(runtime.GOROOT()) {
		t.Fatalf("go tool binding is not pinned to runtime GOROOT/bin: %+v", context)
	}
	mutated := context
	mutated.BuildTags = "hostile_tag"
	firstFingerprint, err := buildFingerprint([32]byte{9}, nil, context)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := buildFingerprint([32]byte{9}, nil, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint == secondFingerprint {
		t.Fatal("build-context mutation was absent from the fingerprint")
	}
	for _, entry := range context.environment() {
		if strings.HasPrefix(entry, "GOWORK=") && entry != "GOWORK=off" || strings.HasPrefix(entry, "GOENV=") && entry != "GOENV=off" || strings.HasPrefix(entry, "GOTOOLCHAIN=") && entry != "GOTOOLCHAIN=local" {
			t.Fatalf("ambient build selection leaked into environment: %q", entry)
		}
	}
}

func TestLoadWorkspaceRejectsUnverifiedAmbientGo(t *testing.T) {
	context := checkedBuildContext(t)
	fakeGo := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyGoExecutablePath(context, fakeGo); err == nil || !strings.Contains(err.Error(), "not verified runtime executable") {
		t.Fatalf("unverified go executable was accepted: %v", err)
	}
	if err := verifyAmbientGoBinding(context); err != nil {
		t.Skipf("ambient test runner does not expose the verified go executable: %v", err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "program", "link"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "program", "link", "link.go"), []byte("package link\ntype Link struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := loadWorkspaceDetails(root, "example.test/program/link")
	if err != nil {
		t.Fatal(err)
	}
	driverOff := false
	for _, entry := range workspace.Context.environment() {
		if entry == "GOPACKAGESDRIVER=off" {
			driverOff = true
		}
	}
	if !driverOff {
		t.Fatal("workspace loader did not disable the external package driver")
	}
}

func TestLocalReplacementOutsideRootIsRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	goMod := filepath.Join(outside, "go.mod")
	if err := os.WriteFile(goMod, []byte("module outside.test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := &packages.Package{PkgPath: "outside.test", Module: &packages.Module{Path: "example.external", Version: "v1.0.0", Replace: &packages.Module{Path: outside, GoMod: goMod}}}
	if _, err := resolvedModules(root, map[string]*packages.Package{pkg.PkgPath: pkg}); err == nil || !strings.Contains(err.Error(), "outside the admitted root") {
		t.Fatalf("outside local replacement was accepted: %v", err)
	}
}

func TestValidateProductionFilesRejectsDuplicatePhysical(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "same.go")
	if err := os.WriteFile(file, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := validateProductionFiles(root, &packages.Package{PkgPath: "example.test/p", CompiledGoFiles: []string{file, file}})
	if err == nil || !strings.Contains(err.Error(), "duplicate physical") {
		t.Fatalf("duplicate physical compiled source was accepted: %v", err)
	}
}

func TestGenericMethodSelectorEvidenceDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test\n\ngo 1.23\n")
	write("program/link/link.go", `package link
type Link struct{ Value int }
type Box[T any] struct{ Value T }
func (b Box[T]) Do(value T) T { return value }
`)
	write("consumer/consumer.go", `package consumer
import l "example.test/program/link"
func Use(value l.Box[int]) int {
	method := value.Do
	return method(1)
}
`)
	scan, err := scanFamily(root, "example.test/program/link")
	if err != nil {
		t.Fatal(err)
	}
	byPosition := make(map[string]map[string]struct{})
	for _, use := range scan.Uses {
		if use.PackagePath != "example.test/consumer" || !strings.HasSuffix(use.Symbol, ".Box.Do") {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", use.SourceFile, use.Line, use.Column)
		if byPosition[key] == nil {
			byPosition[key] = make(map[string]struct{})
		}
		byPosition[key][use.Evidence] = struct{}{}
	}
	for position, evidence := range byPosition {
		if len(evidence) >= 2 {
			return
		}
		_ = position
	}
	t.Fatalf("generic method selector evidence was overwritten: %+v", scan.Uses)
}

func containsProductionSource(sources []ProductionSource, want string) bool {
	for _, source := range sources {
		if source.Path == want {
			return true
		}
	}
	return false
}

func sameProductionSources(left, right []ProductionSource) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func callerPackageCount(uses []UseSite, familyPath string) int {
	callers := make(map[string]struct{})
	for _, use := range uses {
		if use.PackagePath == familyPath || strings.HasPrefix(use.PackagePath, familyPath+"/") {
			continue
		}
		callers[use.PackagePath] = struct{}{}
	}
	return len(callers)
}

func findSurfaceID(surfaces []SurfaceInfo, packagePath, surfaceName string) string {
	for _, surface := range surfaces {
		if surface.PackagePath == packagePath && surface.Surface == surfaceName {
			return surface.FactID
		}
	}
	return ""
}

func sameUseSites(left, right []UseSite) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].PackagePath != right[index].PackagePath || left[index].SourceFile != right[index].SourceFile ||
			left[index].Line != right[index].Line || left[index].Column != right[index].Column || left[index].Symbol != right[index].Symbol ||
			left[index].Evidence != right[index].Evidence || left[index].Type != right[index].Type || left[index].TargetDeclID != right[index].TargetDeclID ||
			left[index].FactID != right[index].FactID || !sameAliasChain(left[index].AliasChain, right[index].AliasChain) {
			return false
		}
	}
	return true
}

func declarationCount(declarations []DeclarationInfo, packagePath, kind, owner string) int {
	count := 0
	for _, declaration := range declarations {
		if declaration.PackagePath == packagePath && declaration.Kind == kind && declaration.OwnerType == owner {
			count++
		}
	}
	return count
}

func TestProductionManifestDigestRejectsSymlinkedInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "program", "link", "link.go")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package link\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sources := []ProductionSource{{PackagePath: "example.test/program/link", Path: "program/link/link.go"}}
	if _, err := productionManifestDigest(root, sources); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "go.sum")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := productionManifestDigest(root, sources); err == nil {
		t.Fatal("symlinked optional go.sum was accepted")
	}
}
