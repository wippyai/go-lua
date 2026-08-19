package oracle

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/internal/testfixture"
)

// The architecture battery is the static half of the grounding kit. Every law
// below reads source, never a running analyzer, so it stays cheap enough to run
// on every package invocation and cannot be starved by a corpus budget.
//
// Two shapes appear here. A closed law names a direction that is already true
// and fails on the first counterexample. A ratchet law names a direction that
// is not yet true, freezes today's inventory of counterexamples, and fails both
// when the inventory grows and when a counterexample appears outside it. A
// ratchet inventory only shrinks: removing an entry is a deliberate edit that
// records the debt actually being paid.
//
// Test sources are scanned with their own inventories. A law that reads only
// production files states nothing about the direction the package's own tests
// pull, which is how a boundary erodes from the test side while the production
// law stays green.

const architectureBatteryModule = "github.com/wippyai/go-lua"

// architectureBatteryDomainModule is the live semantic-domain tree. Spine
// sources that import it are reaching into a domain, except the composition
// root which only seals the catalog.
const architectureBatteryDomainModule = architectureBatteryModule + "/domain"

// architectureBatteryCompositionRoot is the analyzer composition root: the one
// package that composes the artifact columns with the domain registrations and
// seals the catalog. It carries domain edges because composing them is its
// role, so a spine source naming it is reaching for the sealed catalog, not for
// a domain's semantics.
const architectureBatteryCompositionRoot = architectureBatteryDomainModule + "/composite"

// architectureBatteryRepositoryRoot locates the module root by walking up from
// this source file to the directory holding go.mod, so the battery is
// independent of both the working directory a test runs in and of how deep the
// kit sits in the tree.
func architectureBatteryRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("oracle source location unavailable")
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

// architectureBatterySource is one parsed Go file with the two facts every law
// here selects on: its module-relative slash path and whether it is a test.
type architectureBatterySource struct {
	path string
	test bool
	file *ast.File
}

// architectureBatteryImports lists the import paths of one source.
func (source architectureBatterySource) imports(t *testing.T) []string {
	t.Helper()
	paths := make([]string, 0, len(source.file.Imports))
	for _, imported := range source.file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("%s: unquote import %s: %v", source.path, imported.Path.Value, err)
		}
		paths = append(paths, value)
	}
	return paths
}

// architectureBatteryPackage is the module-relative directory of a source.
func (source architectureBatterySource) directory() string {
	return filepath.ToSlash(filepath.Dir(source.path))
}

// architectureBatteryWalk parses every Go file under one module-relative root.
// Directory names beginning with a dot are skipped: they are not part of the
// module and hold scratch material.
func architectureBatteryWalk(t *testing.T, root string, visit func(architectureBatterySource)) {
	t.Helper()
	repository := architectureBatteryRepositoryRoot(t)
	err := filepath.WalkDir(filepath.Join(repository, filepath.FromSlash(root)), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); len(name) > 1 && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		relative, relativeErr := filepath.Rel(repository, path)
		if relativeErr != nil {
			return relativeErr
		}
		visit(architectureBatterySource{path: filepath.ToSlash(relative), test: strings.HasSuffix(path, "_test.go"), file: parsed})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// architectureBatteryRatchet judges one inventory of known counterexamples.
// Found entries outside the inventory fail; entries whose recorded ceiling is
// exceeded fail; entries that have disappeared are reported so the inventory
// can be trimmed, because a ratchet that silently keeps paid debt stops
// measuring anything.
func architectureBatteryRatchet(t *testing.T, law string, inventory map[string]int, found map[string]int) {
	t.Helper()
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ceiling, listed := inventory[name]
		if !listed {
			t.Errorf("%s: %s is not in the frozen inventory; a new counterexample is a regression, not a list entry", law, name)
			continue
		}
		if found[name] > ceiling {
			t.Errorf("%s: %s grew from %d to %d; the inventory only shrinks", law, name, ceiling, found[name])
		}
	}
	paid := make([]string, 0, len(inventory))
	for name := range inventory {
		if _, still := found[name]; !still {
			paid = append(paid, name)
		}
	}
	if len(paid) != 0 {
		sort.Strings(paid)
		t.Logf("%s: inventory entries no longer present, remove them: %s", law, strings.Join(paid, ", "))
	}
}

// architectureBatterySetRatchet is the ratchet over a plain name set.
func architectureBatterySetRatchet(t *testing.T, law string, inventory map[string]struct{}, found []string) {
	t.Helper()
	sort.Strings(found)
	for _, name := range found {
		if _, listed := inventory[name]; !listed {
			t.Errorf("%s: %s is not in the frozen inventory; a new counterexample is a regression, not a list entry", law, name)
		}
	}
	if len(found) > len(inventory) {
		t.Errorf("%s: %d counterexamples exceed the frozen inventory of %d", law, len(found), len(inventory))
	}
	present := make(map[string]struct{}, len(found))
	for _, name := range found {
		present[name] = struct{}{}
	}
	paid := make([]string, 0, len(inventory))
	for name := range inventory {
		if _, still := present[name]; !still {
			paid = append(paid, name)
		}
	}
	if len(paid) != 0 {
		sort.Strings(paid)
		t.Logf("%s: inventory entries no longer present, remove them: %s", law, strings.Join(paid, ", "))
	}
}

// architectureBatteryProgramDomainProduction freezes the production packages
// under analysis/program and analysis/schema that still reach into a semantic
// domain. Program and schema are the neutral compilation spine: a domain is a
// consumer of what they publish, never an input to how they publish it. The
// composition root that knows both worlds is domain/composite; this scan
// names it and excludes it rather than reading its position. Its subpackages
// are ordinary cross-domain relations and stay in scope.
var architectureBatteryProgramDomainProduction = map[string]int{}

// architectureBatteryProgramDomainTest is the same freeze for test sources.
// It is a separate inventory because a test-side dependency is a separate
// decision: production code may be clean while its tests keep the coupling
// alive and make the eventual production cut look larger than it is.
var architectureBatteryProgramDomainTest = map[string]int{
	"analysis/program/artifact":      1,
	"analysis/program/link":          1,
	"analysis/program/link/boundary": 3,
	"analysis/program/link/host":     2,
	"analysis/program/link/module":   1,
	"analysis/program/link/project":  2,
	"analysis/program/target":        2,
}

// TestArchitectureProgramAndSchemaDomainCouplingOnlyShrinks is the L13 law with
// its test-source hole closed. Production and test sources are scanned with
// their own frozen inventories, so neither side can absorb the other's growth.
func TestArchitectureProgramAndSchemaDomainCouplingOnlyShrinks(t *testing.T) {
	production := make(map[string]int)
	tests := make(map[string]int)
	for _, root := range []string{"analysis/program", "analysis/schema"} {
		architectureBatteryWalk(t, root, func(source architectureBatterySource) {
			coupled := false
			for _, imported := range source.imports(t) {
				if architectureBatteryDomainCompositionRoot(imported) {
					continue
				}
				if architectureBatteryDomainImport(imported) {
					coupled = true
					break
				}
			}
			if !coupled {
				return
			}
			if source.test {
				tests[source.directory()]++
				return
			}
			production[source.directory()]++
		})
	}
	architectureBatteryRatchet(t, "program/schema production imports no domain", architectureBatteryProgramDomainProduction, production)
	architectureBatteryRatchet(t, "program/schema test imports no domain", architectureBatteryProgramDomainTest, tests)
}

// TestArchitectureDomainCouplingNamesLivePackages keeps the L13 scan pointed
// at packages that exist. The composition root and one ordinary domain are
// enough: a renamed or relocated tree fails here instead of going silent.
func TestArchitectureDomainCouplingNamesLivePackages(t *testing.T) {
	repository := architectureBatteryRepositoryRoot(t)
	for _, rel := range []string{"domain/composite", "domain/type", "domain/value"} {
		if _, err := os.Stat(filepath.Join(repository, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("domain coupling scan names %s: %v", rel, err)
		}
	}
}

func architectureBatteryDomainCompositionRoot(imported string) bool {
	return imported == architectureBatteryCompositionRoot || strings.HasPrefix(imported, architectureBatteryCompositionRoot+"/")
}

func architectureBatteryDomainImport(imported string) bool {
	return imported == architectureBatteryDomainModule || strings.HasPrefix(imported, architectureBatteryDomainModule+"/")
}

// TestArchitectureIdentityDependsOnNothing is a closed law. Identity is the
// bottom of the analysis stack: content identity, mounts, and lexical
// coordinates are the vocabulary everything else is expressed in, so it may
// depend on the standard library alone. This holds for its own tests too, or
// the package would grow a dependency the moment a test needed one.
func TestArchitectureIdentityDependsOnNothing(t *testing.T) {
	architectureBatteryWalk(t, "analysis/identity", func(source architectureBatterySource) {
		for _, imported := range source.imports(t) {
			if architectureBatteryStandardLibrary(imported) {
				continue
			}
			t.Errorf("analysis/identity source %s imports %s; identity is standard-library only", source.path, imported)
		}
	})
}

// TestArchitectureSnapshotDependsOnIdentityOnly is a closed law. A snapshot is
// an identity-addressed column store: it may name what it stores rows about and
// nothing about what those rows mean.
func TestArchitectureSnapshotDependsOnIdentityOnly(t *testing.T) {
	architectureBatteryWalk(t, "analysis/snapshot", func(source architectureBatterySource) {
		for _, imported := range source.imports(t) {
			if architectureBatteryStandardLibrary(imported) || imported == architectureBatteryModule+"/analysis/identity" {
				continue
			}
			t.Errorf("analysis/snapshot source %s imports %s; snapshot may depend on analysis/identity and the standard library only", source.path, imported)
		}
	})
}

// architectureBatteryRootVMProduction is the L11 debt: root VM production files
// that still reach into the analysis tree. The runtime type surface was never
// cut from the analyzer's type domain, so these files import it directly. They
// are listed rather than exempted: an unlisted root file reaching into analysis
// is new debt and fails.
var architectureBatteryRootVMProduction = map[string]int{
	"ltype.go":            5,
	"ltype_validate.go":   3,
	"typeinfo_runtime.go": 2,
}

// architectureBatteryRootVMTest is the same debt on the test side of the root
// package.
var architectureBatteryRootVMTest = map[string]int{
	"compile_options_test.go":   3,
	"ltype_adversarial_test.go": 4,
	"ltype_bench_test.go":       4,
	"ltype_edge_test.go":        4,
	"ltype_fuzz_test.go":        5,
	"ltype_test.go":             4,
	"ltype_validate_test.go":    4,
	"typeinfo_runtime_test.go":  4,
}

// TestArchitectureRootVMAnalysisDebtIsListedAndOnlyShrinks tracks the runtime
// package's dependency on the analyzer instead of hiding it. Only the root
// package's own files are scanned; subdirectories are their own components.
func TestArchitectureRootVMAnalysisDebtIsListedAndOnlyShrinks(t *testing.T) {
	repository := architectureBatteryRepositoryRoot(t)
	entries, err := os.ReadDir(repository)
	if err != nil {
		t.Fatal(err)
	}
	production := make(map[string]int)
	tests := make(map[string]int)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(repository, entry.Name()), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		source := architectureBatterySource{path: entry.Name(), test: strings.HasSuffix(entry.Name(), "_test.go"), file: parsed}
		for _, imported := range source.imports(t) {
			if !strings.HasPrefix(imported, architectureBatteryModule+"/analysis") {
				continue
			}
			if source.test {
				tests[entry.Name()]++
				continue
			}
			production[entry.Name()]++
		}
	}
	architectureBatteryRatchet(t, "root VM production imports analysis", architectureBatteryRootVMProduction, production)
	architectureBatteryRatchet(t, "root VM test imports analysis", architectureBatteryRootVMTest, tests)
}

// architectureBatteryEnumerationPattern names the enumeration vocabulary: an
// API that hands a caller correlated rows, tuples, or products instead of a
// question it can answer.
var architectureBatteryEnumerationPattern = regexp.MustCompile(`(Rows?|Tuple|Product)`)

// architectureBatteryEngineEnumeration freezes today's enumeration-shaped
// exported names in the engine tree. The engine answers queries; it does not
// publish correlated collections for a caller to re-correlate. This inventory
// is name-shaped rather than signature-shaped on purpose: it is total over the
// vocabulary today and tightens to a signature judgment at the cut, when the
// result component owns the projection surface.
var architectureBatteryEngineEnumeration = map[string]struct{}{
	// PublishRow/WithdrawRow are the W1 write-capability verbs ratified at the
	// Snapshot.Query design (one gated write door per column); recorded, not grown.
	"analysis/engine:PublishRow":                                       {},
	"analysis/engine:WithdrawRow":                                      {},
	"analysis/engine/internal/carrier/product:NewRows":                 {},
	"analysis/engine/internal/carrier/product:Rows":                    {},
	"analysis/engine/internal/carrier/product:SourceRows":              {},
	"analysis/engine/internal/carrier:ObservationRow":                  {},
	"analysis/engine/internal/carrier:ObservationWork.ResolveRow":      {},
	"analysis/engine/internal/carrier:ObservationWork.Row":             {},
	"analysis/engine/internal/equation:ActivationMemberRowLocator":     {},
	"analysis/engine/internal/equation:Batch.TargetInputRows":          {},
	"analysis/engine/internal/equation:Batch.TargetMetadataRows":       {},
	"analysis/engine/internal/equation:Batch.TargetRows":               {},
	"analysis/engine/internal/equation:PointRowLocator":                {},
	"analysis/engine/internal/equation:QueryRowLocator":                {},
	"analysis/engine/internal/equation:Relation.Rows":                  {},
	"analysis/engine/internal/equation:RuleMemberRowLocator":           {},
	"analysis/engine/internal/equation:Topology.ActivationMemberRow":   {},
	"analysis/engine/internal/equation:Topology.PointRow":              {},
	"analysis/engine/internal/equation:Topology.PointRowCount":         {},
	"analysis/engine/internal/equation:Topology.QueryRow":              {},
	"analysis/engine/internal/equation:Topology.QueryRowCount":         {},
	"analysis/engine/internal/equation:Topology.RuleMemberRow":         {},
	"analysis/engine/internal/equation:Topology.RuleMemberRowCount":    {},
	"analysis/engine:Product":                                          {},
	"analysis/engine:Row":                                              {},
	"analysis/engine:SolveDiagnosticRow":                               {},
}

// TestArchitectureEngineEnumerationVocabularyOnlyShrinks is the no-enumeration
// API ratchet.
func TestArchitectureEngineEnumerationVocabularyOnlyShrinks(t *testing.T) {
	found := architectureBatteryExportedNames(t, "analysis/engine", architectureBatteryEnumerationPattern.MatchString)
	architectureBatterySetRatchet(t, "engine publishes no new enumeration API", architectureBatteryEngineEnumeration, found)
}

// architectureBatteryReceiptVocabulary freezes every exported type under
// analysis whose name carries the receipt vocabulary. A receipt is a closed
// construction proof; the count of distinct receipt types is the count of
// construction boundaries the analyzer still proves separately, so it is the
// direct measure of how far the boundary consolidation has come.
var architectureBatteryReceiptVocabulary = map[string]struct{}{}

// TestArchitectureReceiptVocabularyOnlyShrinks is the receipt tripwire. It
// covers exported types only: a receipt is a type, and constructors and methods
// follow the type they belong to.
func TestArchitectureReceiptVocabularyOnlyShrinks(t *testing.T) {
	found := architectureBatteryExportedTypes(t, "analysis", func(name string) bool {
		return strings.Contains(name, "Receipt")
	})
	architectureBatterySetRatchet(t, "no new receipt-vocabulary type", architectureBatteryReceiptVocabulary, found)
}

// architectureBatteryExportedNames collects qualified exported declarations
// under one root whose name satisfies select. Methods are qualified by their
// receiver so two identically named methods stay distinct entries.
func architectureBatteryExportedNames(t *testing.T, root string, selects func(string) bool) []string {
	t.Helper()
	found := make(map[string]struct{})
	architectureBatteryWalk(t, root, func(source architectureBatterySource) {
		if source.test {
			return
		}
		directory := source.directory()
		for _, decl := range source.file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if !typed.Name.IsExported() || !selects(typed.Name.Name) {
					continue
				}
				qualified := typed.Name.Name
				if typed.Recv != nil && len(typed.Recv.List) == 1 {
					qualified = architectureBatteryReceiverName(typed.Recv.List[0].Type) + "." + qualified
				}
				found[directory+":"+qualified] = struct{}{}
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, spec := range typed.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !typeSpec.Name.IsExported() || !selects(typeSpec.Name.Name) {
						continue
					}
					found[directory+":"+typeSpec.Name.Name] = struct{}{}
				}
			}
		}
	})
	return architectureBatteryKeys(found)
}

// architectureBatteryExportedTypes collects qualified exported type names only.
func architectureBatteryExportedTypes(t *testing.T, root string, selects func(string) bool) []string {
	t.Helper()
	found := make(map[string]struct{})
	architectureBatteryWalk(t, root, func(source architectureBatterySource) {
		if source.test {
			return
		}
		directory := source.directory()
		for _, decl := range source.file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Name.IsExported() || !selects(typeSpec.Name.Name) {
					continue
				}
				found[directory+":"+typeSpec.Name.Name] = struct{}{}
			}
		}
	})
	return architectureBatteryKeys(found)
}

func architectureBatteryKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func architectureBatteryReceiverName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return architectureBatteryReceiverName(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return architectureBatteryReceiverName(typed.X)
	case *ast.IndexListExpr:
		return architectureBatteryReceiverName(typed.X)
	}
	return "unknown"
}

// architectureBatteryStandardLibrary reports whether an import path names a
// standard-library package. A module path always carries a dot in its first
// element; a standard-library path never does.
func architectureBatteryStandardLibrary(path string) bool {
	head := path
	if index := strings.IndexByte(path, '/'); index >= 0 {
		head = path[:index]
	}
	return !strings.Contains(head, ".")
}

// TestArchitectureBatteryRatchetJudgesGrowthAndUnlistedEntries proves the
// ratchet itself, so a battery that silently stopped judging cannot pass. The
// diagnostics/result allowlist is deliberately absent from this battery: the
// result component that would own it does not exist yet, and a placeholder
// inventory would freeze the current diagnostic_report.go consumer set as if it
// were the intended surface.
func TestArchitectureBatteryRatchetJudgesGrowthAndUnlistedEntries(t *testing.T) {
	inventory := map[string]int{"listed": 2}
	cases := []struct {
		name  string
		found map[string]int
		fails bool
	}{
		{name: "within ceiling", found: map[string]int{"listed": 1}, fails: false},
		{name: "at ceiling", found: map[string]int{"listed": 2}, fails: false},
		{name: "grew", found: map[string]int{"listed": 3}, fails: true},
		{name: "unlisted", found: map[string]int{"listed": 1, "fresh": 1}, fails: true},
		{name: "paid", found: map[string]int{}, fails: false},
	}
	for _, testCase := range cases {
		probe := &testing.T{}
		func() {
			defer func() { recover() }()
			architectureBatteryRatchet(probe, "probe", inventory, testCase.found)
		}()
		if probe.Failed() != testCase.fails {
			t.Errorf("ratchet %s: failed=%t, want %t", testCase.name, probe.Failed(), testCase.fails)
		}
	}
	set := map[string]struct{}{"a": {}, "b": {}}
	setCases := []struct {
		name  string
		found []string
		fails bool
	}{
		{name: "subset", found: []string{"a"}, fails: false},
		{name: "equal", found: []string{"a", "b"}, fails: false},
		{name: "unlisted", found: []string{"a", "c"}, fails: true},
	}
	for _, testCase := range setCases {
		probe := &testing.T{}
		func() {
			defer func() { recover() }()
			architectureBatterySetRatchet(probe, "probe", set, testCase.found)
		}()
		if probe.Failed() != testCase.fails {
			t.Errorf("set ratchet %s: failed=%t, want %t", testCase.name, probe.Failed(), testCase.fails)
		}
	}
	if !architectureBatteryStandardLibrary("go/ast") || architectureBatteryStandardLibrary(architectureBatteryModule+"/analysis") {
		t.Fatal("standard-library classification is wrong")
	}
	if architectureBatteryReceiverName(&ast.StarExpr{X: ast.NewIdent("Topology")}) != "Topology" {
		t.Fatal("receiver qualification is wrong")
	}
}
