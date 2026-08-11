package gorewrite

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestMemberBindingPreservesImpureReceiverExactlyOnce(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
type Child struct { Count int }
type Owner struct { flow Child; Count int }
func next() Owner { return Owner{} }
func f() int { return next().Count }
`)
	owner := typeObject(t, info, "Owner")
	count := fieldObject(t, owner, "Count")
	plan := RoutePlan{Consumer: file, Members: []MemberBinding{{
		Form: MemberField, From: count, Target: FutureMember{Name: "Count"}, Via: []ReceiverStep{{Form: ReceiverField, Name: "flow"}},
	}}}
	if err := ApplyRoutePlan(file, fset, info, plan); err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	if !strings.Contains(got, "return next().flow.Count") {
		t.Fatalf("missing exact-once receiver route:\n%s", got)
	}
	if strings.Count(got, "next().flow.Count") != 1 || strings.Contains(got, "next().flow.flow") {
		t.Fatalf("impure receiver was duplicated or lost:\n%s", got)
	}
	recheckRendered(t, got)
}

func TestFutureTargetPackageAndFieldNeedNoCurrentTargetObject(t *testing.T) {
	packageFile, packageSet, packageInfo := checkedFile(t, `package p
import old "fmt"
func f() { old.Println("x") }
`)
	old := packageNameFor(t, packageInfo, packageFile, "fmt")
	if err := ApplyRoutePlan(packageFile, packageSet, packageInfo, RoutePlan{Consumer: packageFile,
		Imports: []ImportBinding{{Form: ImportReplace, From: old, Target: futurePackage("log", "log"), Alias: "logger"}},
		Members: []MemberBinding{{Form: MemberPackageSelector, From: objectInPackage(t, old.Imported(), "Println"), Target: FutureMember{Name: "Println"}, Package: old}},
	}); err != nil {
		t.Fatal(err)
	}
	packageText := render(t, packageSet, packageFile)
	if !strings.Contains(packageText, `logger "log"`) || !strings.Contains(packageText, `logger.Println("x")`) {
		t.Fatalf("future package route was not rendered:\n%s", packageText)
	}
	recheckRendered(t, packageText)

	fieldFile, fieldSet, fieldInfo := checkedFile(t, `package p
type Flow struct{}
type Owner struct { Count int }
func next() Owner { return Owner{} }
func f() int { return next().Count }
`)
	owner := typeObject(t, fieldInfo, "Owner")
	oldCount := fieldObject(t, owner, "Count")
	if err := RelocateNamedFields(fieldFile, fieldSet, FieldRelocation{Owner: "Owner", Child: "Flow", ChildField: "flow", Fields: setOf("Count")}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyRoutePlan(fieldFile, fieldSet, fieldInfo, RoutePlan{Consumer: fieldFile, Members: []MemberBinding{{
		Form: MemberField, From: oldCount, Target: FutureMember{Name: "Count"}, Via: []ReceiverStep{{Form: ReceiverField, Name: "flow"}},
	}}}); err != nil {
		t.Fatal(err)
	}
	fieldText := render(t, fieldSet, fieldFile)
	if !strings.Contains(fieldText, "next().flow.Count") {
		t.Fatalf("future field route was not rendered:\n%s", fieldText)
	}
	recheckRendered(t, fieldText)
}

func TestExplicitImportAddAndRemove(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
import old "fmt"
func obsolete() { old.Println("x") }
`)
	old := packageNameFor(t, info, file, "fmt")
	file.Decls = file.Decls[:1] // The declaration using old was moved by this cut.
	if err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Imports: []ImportBinding{
		{Form: ImportRemove, From: old},
		{Form: ImportAdd, Target: futurePackage("log", "log"), Alias: "logger"},
	}}); err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	if strings.Contains(got, `"fmt"`) || !strings.Contains(got, `logger "log"`) {
		t.Fatalf("explicit import add/remove was not exact:\n%s", got)
	}
	recheckRendered(t, got+"\nfunc f() { logger.Println(\"x\") }\n")
}

func TestExplicitImportAddSupportsUnresolvedMovedDependency(t *testing.T) {
	file, fset := parsedFile(t, `package p
func moved() { logger.Println("x") }
`)
	if err := ApplyRoutePlan(file, fset, nil, RoutePlan{Consumer: file, Imports: []ImportBinding{{
		Form: ImportAdd, Target: futurePackage("log", "log"), Alias: "logger",
	}}}); err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	if !strings.Contains(got, `logger "log"`) || !strings.Contains(got, `logger.Println("x")`) {
		t.Fatalf("unresolved moved dependency was not made explicit:\n%s", got)
	}
	recheckRendered(t, got)
}

func TestCanonicalImportSpellingSeparatesRawAliasFromEffectiveQualifier(t *testing.T) {
	const path = "example.invalid/platform/contracts"
	t.Run("implicit uses target package name but emits no alias", func(t *testing.T) {
		file, fset := parsedFile(t, `package p
func moved() { contract.Open() }
`)
		err := ApplyRoutePlan(file, fset, nil, RoutePlan{Consumer: file, Imports: []ImportBinding{{
			Form: ImportAdd, Target: futurePackage(path, "contract"),
		}}})
		if err != nil {
			t.Fatal(err)
		}
		got := render(t, fset, file)
		if !strings.Contains(got, `"example.invalid/platform/contracts"`) || strings.Contains(got, `contract "example.invalid/platform/contracts"`) || !strings.Contains(got, "contract.Open()") {
			t.Fatalf("implicit import was not spelled canonically:\n%s", got)
		}
	})
	t.Run("explicit retains exact alias", func(t *testing.T) {
		file, fset := parsedFile(t, `package p
func moved() { api.Open() }
`)
		err := ApplyRoutePlan(file, fset, nil, RoutePlan{Consumer: file, Imports: []ImportBinding{{
			Form: ImportAdd, Target: futurePackage(path, "contract"), Alias: "api",
		}}})
		if err != nil {
			t.Fatal(err)
		}
		got := render(t, fset, file)
		if !strings.Contains(got, `api "example.invalid/platform/contracts"`) || !strings.Contains(got, "api.Open()") {
			t.Fatalf("explicit import alias was not retained:\n%s", got)
		}
	})
}

func TestCanonicalImportSpellingReplaceUsesImplicitName(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
import old "fmt"
func f() { old.Println("x") }
`)
	old := packageNameFor(t, info, file, "fmt")
	err := ApplyRoutePlan(file, fset, info, RoutePlan{
		Consumer: file,
		Imports:  []ImportBinding{{Form: ImportReplace, From: old, Target: futurePackage("example.invalid/platform/contracts", "contract")}},
		Members:  []MemberBinding{{Form: MemberPackageSelector, From: objectInPackage(t, old.Imported(), "Println"), Target: FutureMember{Name: "Println"}, Package: old}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	if !strings.Contains(got, `"example.invalid/platform/contracts"`) || strings.Contains(got, `contract "example.invalid/platform/contracts"`) || !strings.Contains(got, `contract.Println("x")`) {
		t.Fatalf("implicit replacement was not rendered canonically:\n%s", got)
	}
}

func TestMemberBindingDirectViewAndMethodCall(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
type Child struct{}
func (Child) M() {}
type Owner struct { core Child }
func (Owner) M() {}
func (Owner) view() Child { return Child{} }
func next() Owner { return Owner{} }
func f() { next().M() }
`)
	owner := typeObject(t, info, "Owner")
	oldMethod := methodObject(t, owner, "M")
	plan := RoutePlan{Consumer: file, Members: []MemberBinding{{
		Form: MemberDirectMethodCall, From: oldMethod, Target: FutureMember{Name: "M"}, Via: []ReceiverStep{{Form: ReceiverDirectView, Name: "view"}},
	}}}
	if err := ApplyRoutePlan(file, fset, info, plan); err != nil {
		t.Fatal(err)
	}
	if got := render(t, fset, file); !strings.Contains(got, "next().view().M()") {
		t.Fatalf("missing direct-view method route:\n%s", got)
	} else {
		recheckRendered(t, got)
	}
}

func TestMovedMethodValueAndExpressionRejectWithoutMutation(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
type Child struct{}
func (Child) M() {}
type Owner struct { core Child }
func (Owner) M() {}
func f(v Owner) { _ = v.M; Owner.M(v) }
`)
	owner := typeObject(t, info, "Owner")
	before := render(t, fset, file)
	err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Members: []MemberBinding{{
		Form: MemberDirectMethodCall, From: methodObject(t, owner, "M"), Target: FutureMember{Name: "M"},
		Via: []ReceiverStep{{Form: ReceiverField, Name: "core"}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Fatalf("expected method-value bridge rejection, got %v", err)
	}
	if got := render(t, fset, file); got != before {
		t.Fatalf("failed preflight mutated source:\n%s", got)
	}
}

func TestObjectBoundImportAndPackageMemberRoute(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
import old "fmt"
func f() { old.Println("x") }
`)
	old := packageNameFor(t, info, file, "fmt")
	oldPrintln := objectInPackage(t, old.Imported(), "Println")
	plan := RoutePlan{Consumer: file,
		Imports: []ImportBinding{{Form: ImportReplace, From: old, Target: futurePackage("log", "log"), Alias: "logger"}},
		Members: []MemberBinding{{Form: MemberPackageSelector, From: oldPrintln, Target: FutureMember{Name: "Println"}, Package: old}},
	}
	if err := ApplyRoutePlan(file, fset, info, plan); err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	for _, want := range []string{`logger "log"`, `logger.Println("x")`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	recheckRendered(t, got)
}

func TestImportRouteRejectsTargetAliasShadowing(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
import old "fmt"
func f() { logger := 1; _ = logger; old.Println("x") }
`)
	old := packageNameFor(t, info, file, "fmt")
	err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Imports: []ImportBinding{{Form: ImportReplace, From: old, Target: futurePackage("log", "log"), Alias: "logger"}}})
	if err == nil || !strings.Contains(err.Error(), "shadowed") {
		t.Fatalf("expected alias shadow rejection, got %v", err)
	}
}

func TestImportRouteDeduplicatesExistingResolvedTarget(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
import (
	old "fmt"
	logger "log"
)
func f() { old.Println("x"); logger.Println("y") }
`)
	old := packageNameFor(t, info, file, "fmt")
	if err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Imports: []ImportBinding{{Form: ImportReplace, From: old, Target: futurePackage("log", "log"), Alias: "logger"}}, Members: []MemberBinding{{
		Form: MemberPackageSelector, From: objectInPackage(t, old.Imported(), "Println"), Target: FutureMember{Name: "Println"}, Package: old,
	}}}); err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	if strings.Contains(got, `"fmt"`) || strings.Count(got, `"log"`) != 1 || strings.Count(got, "logger.Println") != 2 {
		t.Fatalf("target import was not deduplicated:\n%s", got)
	}
	recheckRendered(t, got)
}

func TestImportRouteCanNormalizeAliasOfSameResolvedPackage(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
import old "fmt"
func f() { old.Println("x") }
`)
	old := packageNameFor(t, info, file, "fmt")
	if err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Imports: []ImportBinding{{Form: ImportReplace, From: old, Target: futurePackage("fmt", "fmt"), Alias: "fmt"}}}); err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	if !strings.Contains(got, `fmt "fmt"`) || !strings.Contains(got, `fmt.Println("x")`) {
		t.Fatalf("same-package alias normalization failed:\n%s", got)
	}
	recheckRendered(t, got)
}

func TestInterfaceMethodBindingRejectsRatherThanBuildingBridge(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
type Child struct{}
func (Child) M() {}
type Owner struct { core Child }
type I interface { M() }
func f(v I) { v.M() }
`)
	before := render(t, fset, file)
	err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Members: []MemberBinding{{
		Form: MemberDirectMethodCall, From: interfaceMethod(t, typeObject(t, info, "I"), "M"), Target: FutureMember{Name: "M"},
		Via: []ReceiverStep{{Form: ReceiverField, Name: "core"}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Fatalf("expected interface bridge rejection, got %v", err)
	}
	if got := render(t, fset, file); got != before {
		t.Fatalf("interface rejection mutated source:\n%s", got)
	}
}

func TestRoutePlanRejectsWrongConsumerWithoutMutation(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
type Child struct { Count int }
type Owner struct { flow Child; Count int }
func f(v Owner) int { return v.Count }
`)
	other, _ := parsedFile(t, "package p\n")
	owner := typeObject(t, info, "Owner")
	before := render(t, fset, file)
	err := ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: other, Members: []MemberBinding{{
		Form: MemberField, From: fieldObject(t, owner, "Count"), Target: FutureMember{Name: "Count"}, Via: []ReceiverStep{{Form: ReceiverField, Name: "flow"}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "consumer") {
		t.Fatalf("expected exact-consumer rejection, got %v", err)
	}
	if got := render(t, fset, file); got != before {
		t.Fatalf("wrong consumer mutated source:\n%s", got)
	}
}

func TestClassifyMethodForms(t *testing.T) {
	file, _, info := checkedFile(t, `package p
type S struct { X int }
func (S) M() {}
type I interface { M() }
func f(s S, i I) { _ = s.M; s.M(); S.M(s); _ = i.M; i.M(); _ = s.X }
`)
	parents := parentMap(file)
	seen := map[SelectorClass]int{}
	ast.Inspect(file, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok {
			seen[ClassifySelector(info, selector, parents[selector])]++
		}
		return true
	})
	for _, class := range []SelectorClass{FieldSelection, MethodInvocation, MethodValue, MethodExpression, InterfaceMethod} {
		if seen[class] == 0 {
			t.Fatalf("missing classification %s: %#v", class, seen)
		}
	}
}

func TestRelocateNamedFieldsRewritesKeyedLiteral(t *testing.T) {
	file, fset := parsedFile(t, `package p
type Flow struct{}
type Link struct { A int; X int; Y string; Z bool }
func f() Link { return Link{A: 1, X: 2, Y: "y", Z: true} }
`)
	err := RelocateNamedFields(file, fset, FieldRelocation{Owner: "Link", Child: "Flow", ChildField: "flow", Fields: setOf("X", "Y")})
	if err != nil {
		t.Fatal(err)
	}
	got := render(t, fset, file)
	for _, want := range []string{"flow Flow", "flow: Flow{X: 2, Y: \"y\"}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRelocateNamedFieldsRejectsInterleavedLiteral(t *testing.T) {
	file, fset := parsedFile(t, `package p
type Flow struct{}
type Link struct { A int; X int; B int; Y int }
func f() Link { return Link{A: 1, X: 2, B: 3, Y: 4} }
`)
	err := RelocateNamedFields(file, fset, FieldRelocation{Owner: "Link", Child: "Flow", ChildField: "flow", Fields: setOf("X", "Y")})
	if err == nil || !strings.Contains(err.Error(), "interleaved") {
		t.Fatalf("expected evaluation-order rejection, got %v", err)
	}
}

func TestCheckNamedFieldRelocationLiteralsIsPure(t *testing.T) {
	file, fset := parsedFile(t, `package p
type Link struct { X int; Y int }
func f() Link { return Link{X: 1, Y: 2} }
`)
	before := render(t, fset, file)
	if err := CheckNamedFieldRelocationLiterals(file, fset, FieldRelocation{Owner: "Link", Child: "State", ChildField: "state", Fields: setOf("X", "Y")}); err != nil {
		t.Fatal(err)
	}
	if after := render(t, fset, file); after != before {
		t.Fatalf("literal preflight mutated source:\n%s", after)
	}
}

func TestExtractDeclarationsMovesCommentsAndPeerExternalTest(t *testing.T) {
	source, sourceSet := parsedFile(t, `//go:build linux

package p

//go:noinline
// TestMove is intentionally documented.
func TestMove(t *T) { // body comment
}
`)
	destination, _ := parsedFile(t, `//go:build linux

package p_test
`)
	result, err := ExtractDeclarations(source, destination, DeclarationSelector{Tests: setOf("TestMove")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Moved) != 1 {
		t.Fatalf("moved %d declarations, want 1", len(result.Moved))
	}
	got := render(t, sourceSet, destination)
	for _, want := range []string{"go:noinline", "TestMove is intentionally documented", "body comment", "func TestMove"} {
		if !strings.Contains(got, want) {
			t.Fatalf("moved comment %q missing:\n%s", want, got)
		}
	}
}

func TestExtractDeclarationsRejectsBuildConstraintDivergence(t *testing.T) {
	source, _ := parsedFile(t, "//go:build linux\n\npackage p\nfunc TestMove(t *T) {}\n")
	destination, _ := parsedFile(t, "//go:build darwin\n\npackage p_test\n")
	err := mustExtract(source, destination, DeclarationSelector{Tests: setOf("TestMove")})
	if err == nil || !strings.Contains(err.Error(), "constraints differ") {
		t.Fatalf("expected build constraint rejection, got %v", err)
	}
}

func TestExtractDeclarationsRejectsDetachedComment(t *testing.T) {
	source, _ := parsedFile(t, `package p

// This comment has no declaration owner.

func TestMove(t *T) {}
`)
	destination, _ := parsedFile(t, "package p_test\n")
	err := mustExtract(source, destination, DeclarationSelector{Tests: setOf("TestMove")})
	if err == nil || !strings.Contains(err.Error(), "detached comment") {
		t.Fatalf("expected detached-comment rejection, got %v", err)
	}
}

func TestExtractDeclarationsRejectsTrailingDetachedComment(t *testing.T) {
	source, _ := parsedFile(t, `package p
func TestMove(t *T) {}

// This comment has no declaration owner.
`)
	destination, _ := parsedFile(t, "package p_test\n")
	err := mustExtract(source, destination, DeclarationSelector{Tests: setOf("TestMove")})
	if err == nil || !strings.Contains(err.Error(), "detached comment") {
		t.Fatalf("expected trailing detached-comment rejection, got %v", err)
	}
}

func TestHazardsDistinguishDiagnosticStringsAndRejectAuthority(t *testing.T) {
	file, fset, info := checkedFile(t, `// Code generated by x. DO NOT EDIT.
package p
import "reflect"
func f(v any) { panic("Count moved"); reflect.ValueOf(v).FieldByName("Count") }
`)
	hazards := FindHazards(file, fset, info)
	var diagnostic, generated, authority bool
	for _, hazard := range hazards {
		diagnostic = diagnostic || (hazard.Kind == "diagnostic-string" && hazard.Detail == "Count moved" && !hazard.Authority)
		generated = generated || (hazard.Kind == "generated-source" && hazard.Authority)
		authority = authority || (hazard.Kind == "reflection-string" && hazard.Detail == "Count" && hazard.Authority)
	}
	if !diagnostic || !generated || !authority {
		t.Fatalf("unexpected hazards %#v", hazards)
	}
	if err := RelocateNamedFields(file, fset, FieldRelocation{Owner: "X", Child: "Y", ChildField: "child", Fields: setOf("A")}); err == nil {
		t.Fatal("generated/reflection authority was not rejected")
	}
}

func TestTypedHazardsIgnoreOrdinarySelectorMethodNames(t *testing.T) {
	file, fset, info := checkedFile(t, `package p
type ordinary struct{}
func (ordinary) Call() {}
func (ordinary) Set() {}
func (ordinary) Interface() {}
func (ordinary) FieldByName(string) {}
func f(value ordinary) {
	value.Call()
	value.Set()
	value.Interface()
	value.FieldByName("Count")
}
`)
	for _, hazard := range FindHazards(file, fset, info) {
		if hazard.Authority {
			t.Fatalf("ordinary selector method became authority: %#v", hazard)
		}
	}
}

func TestAuthorityHazardsRejectLinknameAndCgo(t *testing.T) {
	linkname, linkSet := parsedFile(t, `package p
//go:linkname local example.com/other.remote
func local()
`)
	if err := RelocateNamedFields(linkname, linkSet, FieldRelocation{Owner: "X", Child: "Y", ChildField: "child", Fields: setOf("A")}); err == nil || !strings.Contains(err.Error(), "go-linkname") {
		t.Fatalf("expected go:linkname rejection, got %v", err)
	}
	cgo, cgoSet := parsedFile(t, "package p\nimport \"C\"\n")
	if err := RelocateNamedFields(cgo, cgoSet, FieldRelocation{Owner: "X", Child: "Y", ChildField: "child", Fields: setOf("A")}); err == nil || !strings.Contains(err.Error(), "cgo-import") {
		t.Fatalf("expected cgo rejection, got %v", err)
	}
}

func mustExtract(source, destination *ast.File, selector DeclarationSelector) error {
	_, err := ExtractDeclarations(source, destination, selector)
	return err
}

func parsedFile(t *testing.T, source string) (*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "input.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return file, fset
}

func checkedFile(t *testing.T, source string) (*ast.File, *token.FileSet, *types.Info) {
	t.Helper()
	file, fset := parsedFile(t, source)
	info := &types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Scopes:     make(map[ast.Node]*types.Scope),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Types:      make(map[ast.Expr]types.TypeAndValue),
	}
	config := types.Config{Importer: importer.Default()}
	if _, err := config.Check("example/p", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	return file, fset, info
}

func typeObject(t *testing.T, info *types.Info, name string) *types.TypeName {
	t.Helper()
	for ident, object := range info.Defs {
		if ident.Name == name {
			if named, ok := object.(*types.TypeName); ok {
				return named
			}
		}
	}
	t.Fatalf("type %s not found", name)
	return nil
}

func fieldObject(t *testing.T, owner *types.TypeName, name string) *types.Var {
	t.Helper()
	structure, ok := owner.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("%s is not a struct", owner.Name())
	}
	for index := 0; index < structure.NumFields(); index++ {
		field := structure.Field(index)
		if field.Name() == name {
			return field
		}
	}
	t.Fatalf("field %s.%s not found", owner.Name(), name)
	return nil
}

func methodObject(t *testing.T, owner *types.TypeName, name string) *types.Func {
	t.Helper()
	named, ok := owner.Type().(*types.Named)
	if !ok {
		t.Fatalf("%s is not named", owner.Name())
	}
	for index := 0; index < named.NumMethods(); index++ {
		method := named.Method(index)
		if method.Name() == name {
			return method
		}
	}
	t.Fatalf("method %s.%s not found", owner.Name(), name)
	return nil
}

func packageNameFor(t *testing.T, info *types.Info, file *ast.File, path string) *types.PkgName {
	t.Helper()
	for _, spec := range file.Imports {
		name := importPackageName(info, spec)
		if name != nil && name.Imported().Path() == path {
			return name
		}
	}
	t.Fatalf("package import %s not found", path)
	return nil
}

func objectInPackage(t *testing.T, pkg *types.Package, name string) types.Object {
	t.Helper()
	object := pkg.Scope().Lookup(name)
	if object == nil {
		t.Fatalf("package object %s.%s not found", pkg.Path(), name)
	}
	return object
}

func interfaceMethod(t *testing.T, owner *types.TypeName, name string) *types.Func {
	t.Helper()
	interfaceType, ok := owner.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("%s is not an interface", owner.Name())
	}
	interfaceType.Complete()
	for index := 0; index < interfaceType.NumMethods(); index++ {
		method := interfaceType.Method(index)
		if method.Name() == name {
			return method
		}
	}
	t.Fatalf("interface method %s.%s not found", owner.Name(), name)
	return nil
}

func recheckRendered(t *testing.T, source string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "rewritten.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&types.Config{Importer: importer.Default()}).Check("example/rechecked", fset, []*ast.File{file}, nil); err != nil {
		t.Fatalf("rewritten source does not typecheck: %v\n%s", err, source)
	}
}

func render(t *testing.T, fset *token.FileSet, file *ast.File) string {
	t.Helper()
	var output bytes.Buffer
	if err := format.Node(&output, fset, file); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func setOf(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

func futurePackage(path, name string) FuturePackage {
	return FuturePackage{Path: path, Name: name}
}
