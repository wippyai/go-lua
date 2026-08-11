package render

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

func TestCanonicalHazardsUnionsPathSupportByHazardIdentity(t *testing.T) {
	got := canonicalHazards([]cutplanHazard{
		{code: "go-diagnostic-string", severity: "warning", detail: "same", path: "program/link/a.go"},
		{code: "go-diagnostic-string", severity: "warning", detail: "same", path: "program/link/b.go"},
		{code: "go-diagnostic-string", severity: "warning", detail: "same", path: "program/link/a.go"},
		{code: "go-diagnostic-string", severity: "warning", detail: "other", path: "program/link/c.go"},
		{code: "go-diagnostic-string", severity: "error", detail: "same", path: "program/link/d.go"},
	})
	want := []cutplan.Hazard{
		{Code: "go-diagnostic-string", Severity: "error", Detail: "same", Paths: []string{"program/link/d.go"}},
		{Code: "go-diagnostic-string", Severity: "warning", Detail: "other", Paths: []string{"program/link/c.go"}},
		{Code: "go-diagnostic-string", Severity: "warning", Detail: "same", Paths: []string{"program/link/a.go", "program/link/b.go"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical hazards = %#v, want %#v", got, want)
	}
}

func TestCompileContainmentFieldCutIsDetachedAndDeterministic(t *testing.T) {
	root := renderModule(t, map[string]string{
		"pkg/link/link.go": `package link

type Link struct { N int }

func Read( x Link ) int { return x.N + x.N }
func Make() Link { return Link{N: 1} }
`,
		"pkg/flow/flow.go": "package flow\n",
	})
	oldField := symbol("example.com/renderfixture/pkg/link#type:Link/field:N")
	newField := symbol("example.com/renderfixture/pkg/flow#type:Flow/field:N")
	parent := symbol("example.com/renderfixture/pkg/link#package:Link")
	child := symbol("example.com/renderfixture/pkg/flow#package:Flow")
	through := symbol("example.com/renderfixture/pkg/link#type:Link/field:flow")
	snapshot := collectSnapshot(t, root, oldField)
	before, err := snapshot.Workspace.File("pkg/link/link.go")
	if err != nil {
		t.Fatal(err)
	}
	intent := singleOperationIntent("containment", cutplan.Operation{
		ID: "link-flow", Authority: cutplan.Authority{From: "link", To: "flow"},
		Edits: []cutplan.Edit{{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{
			Source: "pkg/link/link.go", Destination: cutplan.Destination{Path: "pkg/flow/flow.go", Package: "flow"},
			Subjects:    []cutplan.Relocation{{From: oldField, To: newField}},
			Containment: &cutplan.Containment{Parent: parent, Child: child, Through: through},
		}}},
		Bindings: []cutplan.Binding{{Consumer: "pkg/link/link.go", From: oldField, To: newField, Form: cutplan.BindingField,
			Receiver: []cutplan.ReceiverPathStep{{Kind: cutplan.ReceiverField, Object: through}}}},
		Imports:   []cutplan.Import{{Consumer: "pkg/link/link.go", To: &cutplan.ImportRef{Path: "example.com/renderfixture/pkg/flow", Name: "flow", Alias: ""}, Symbols: []cutplan.SymbolRef{newField}}},
		Footprint: cutplan.Footprint{Read: []string{"pkg/link/link.go"}, Write: []string{"pkg/link/link.go", "pkg/flow/flow.go"}},
		Verify:    verification(oldField, "pkg/link/link.go"),
	})
	first, err := Compile(Input{Intent: intent, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compile(Input{Intent: intent, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("render is nondeterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	link := outputText(t, first, "pkg/link/link.go")
	flow := outputText(t, first, "pkg/flow/flow.go")
	for _, want := range []string{
		`"example.com/renderfixture/pkg/flow"`,
		"flow flow.Flow", "x.flow.N + x.flow.N", "flow: flow.Flow{N: 1}",
	} {
		if !strings.Contains(link, want) {
			t.Fatalf("link output lacks %q:\n%s", want, link)
		}
	}
	if strings.Contains(link, `flow "example.com/renderfixture/pkg/flow"`) {
		t.Fatalf("implicit target import gained an explicit alias:\n%s", link)
	}
	if !strings.Contains(flow, "type Flow struct") || !strings.Contains(flow, "N int") {
		t.Fatalf("flow output lost moved field:\n%s", flow)
	}
	witness := assertWitnesses(t, first, oldField, newField, map[cutplan.SiteRole]int{
		cutplan.SiteDeclaration: 1,
		cutplan.SiteSelector:    2,
		cutplan.SiteUse:         1,
	})
	for _, site := range witness.Sites {
		wantPath := "pkg/link/link.go"
		if site.Source.Role == cutplan.SiteDeclaration {
			wantPath = "pkg/flow/flow.go"
		}
		if site.Target.Path != wantPath {
			t.Fatalf("containment witness target path = %q, want %q: %#v", site.Target.Path, wantPath, site)
		}
	}
	if !strings.Contains(link, "func Read(x Link) int") {
		t.Fatalf("formatting drift did not preserve structural witnesses:\n%s", link)
	}
	after, err := snapshot.Workspace.File("pkg/link/link.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(before.Source) != string(after.Source) || !strings.Contains(string(after.Source), "x.N") {
		t.Fatalf("renderer mutated pre-cut workspace:\nbefore=%s\nafter=%s", before.Source, after.Source)
	}
}

func TestCompileMovesTopLevelCrossPackageIncludingTest(t *testing.T) {
	root := renderModule(t, map[string]string{
		"pkg/a/a.go": `package a

import "testing"

type T struct{}
const C = 1
var V = 2
func F() int { return C + V }
func TestMoved(t *testing.T) { if F() != 3 { t.Fatal(F()) } }
`,
		"pkg/b/b.go": "package b\n",
	})
	from := []cutplan.SymbolRef{
		symbol("example.com/renderfixture/pkg/a#package:T"),
		symbol("example.com/renderfixture/pkg/a#package:C"),
		symbol("example.com/renderfixture/pkg/a#package:V"),
		symbol("example.com/renderfixture/pkg/a#package:F"),
		symbol("example.com/renderfixture/pkg/a#package:TestMoved"),
	}
	to := []cutplan.SymbolRef{
		symbol("example.com/renderfixture/pkg/b#package:T"),
		symbol("example.com/renderfixture/pkg/b#package:C"),
		symbol("example.com/renderfixture/pkg/b#package:V"),
		symbol("example.com/renderfixture/pkg/b#package:F"),
		symbol("example.com/renderfixture/pkg/b#package:TestMoved"),
	}
	subjects := make([]cutplan.Relocation, len(from))
	for index := range subjects {
		subjects[index] = cutplan.Relocation{From: from[index], To: to[index]}
	}
	intent := singleOperationIntent("package-move", cutplan.Operation{
		ID: "move", Authority: cutplan.Authority{From: "a", To: "b"},
		Edits: []cutplan.Edit{{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{
			Source: "pkg/a/a.go", Destination: cutplan.Destination{Path: "pkg/b/b.go", Package: "b"}, Subjects: subjects,
		}}},
		Imports: []cutplan.Import{
			{Consumer: "pkg/a/a.go", From: &cutplan.ImportRef{Path: "testing", Name: "testing", Alias: ""}, Symbols: []cutplan.SymbolRef{to[4]}},
			{Consumer: "pkg/b/b.go", To: &cutplan.ImportRef{Path: "testing", Name: "testing", Alias: ""}, Symbols: []cutplan.SymbolRef{to[4]}},
		},
		Footprint: cutplan.Footprint{Read: []string{"pkg/a/a.go", "pkg/b/b.go"}, Write: []string{"pkg/a/a.go", "pkg/b/b.go"}},
		Verify:    verification(from[0], "pkg/a/a.go"),
	})
	snapshot := collectIntentSnapshot(t, root, intent)
	output, err := Compile(Input{Intent: intent, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if got := outputText(t, output, "pkg/a/a.go"); strings.TrimSpace(got) != "package a" {
		t.Fatalf("old package was not cleanly emptied:\n%s", got)
	}
	moved := outputText(t, output, "pkg/b/b.go")
	for _, want := range []string{"type T struct{}", "const C = 1", "var V = 2", "func F() int", "func TestMoved(t *testing.T)"} {
		if !strings.Contains(moved, want) {
			t.Fatalf("moved package lacks %q:\n%s", want, moved)
		}
	}
	if !strings.Contains(moved, `"testing"`) {
		t.Fatalf("moved test lacks declared future import:\n%s", moved)
	}
	if strings.Contains(moved, `testing "testing"`) {
		t.Fatalf("implicit target test import gained an explicit alias:\n%s", moved)
	}
	assertWitnesses(t, output, from[3], to[3], map[cutplan.SiteRole]int{
		cutplan.SiteDeclaration: 1,
		cutplan.SiteUse:         2,
	})
}

func TestCompileRetireGeneratorAndImportReplace(t *testing.T) {
	t.Skip("v3 rejects synthetic remap/import routes that lack a declared relocation")
	root := renderModule(t, map[string]string{
		"dead.go":        "package fixture\n\nfunc Dead() {}\n",
		"in.go":          "package fixture\n\nconst Input = 1\n",
		"pkg/old/old.go": "package old\n\nfunc F() int { return 1 }\n",
		"pkg/new/new.go": "package newer\n\nfunc F() int { return 2 }\n",
		"pkg/use/use.go": `package use

import old "example.com/renderfixture/pkg/old"

func Use() int { return old.F() }
`,
	})
	dead := symbol("example.com/renderfixture#package:Dead")
	oldF := symbol("example.com/renderfixture/pkg/old#package:F")
	newF := symbol("example.com/renderfixture/pkg/new#package:F")
	intent := singleOperationIntent("retire-route", cutplan.Operation{
		ID: "retire-route", Authority: cutplan.Authority{From: "old", To: "new"},
		Edits: []cutplan.Edit{
			{Kind: cutplan.EditRetire, Retire: &cutplan.Retire{Source: "dead.go", Symbols: []cutplan.SymbolRef{dead}}},
			{Kind: cutplan.EditGenerate, Generate: &cutplan.Generate{Provider: "copy", Inputs: []string{"in.go"}, Destination: "generated.txt"}},
		},
		Bindings:  []cutplan.Binding{{Consumer: "pkg/use/use.go", From: oldF, To: newF, Form: cutplan.BindingPackageSelector}},
		Imports:   []cutplan.Import{{Consumer: "pkg/use/use.go", From: &cutplan.ImportRef{Path: "example.com/renderfixture/pkg/old", Name: "old", Alias: "old"}, To: &cutplan.ImportRef{Path: "example.com/renderfixture/pkg/new", Name: "newer", Alias: "newer"}, Symbols: []cutplan.SymbolRef{newF}}},
		Footprint: cutplan.Footprint{Read: []string{"dead.go", "in.go", "pkg/use/use.go"}, Write: []string{"dead.go", "generated.txt", "pkg/use/use.go"}},
		Verify:    verification(dead, "dead.go"),
	})
	snapshot := collectIntentSnapshot(t, root, intent)
	registry, err := generate.NewRegistry([]generate.Provider{{Name: "copy", Identity: "copy-v1", Render: func(request generate.Request) ([]byte, error) {
		return append([]byte("generated:"), request.Inputs[0].Bytes...), nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	output, err := Compile(Input{Intent: intent, Snapshot: snapshot, Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if !deletedOutput(output, "dead.go") {
		t.Fatalf("retired source did not become an explicit deletion: %#v", output.Files)
	}
	use := outputText(t, output, "pkg/use/use.go")
	if !strings.Contains(use, `newer "example.com/renderfixture/pkg/new"`) || !strings.Contains(use, "newer.F()") {
		t.Fatalf("replace route failed:\n%s", use)
	}
	if got := outputText(t, output, "generated.txt"); !strings.HasPrefix(got, "generated:package fixture") {
		t.Fatalf("registered generator output = %q", got)
	}
	if !reflect.DeepEqual(output.Providers, []cutplan.ProviderEvidence{{Name: "copy", Identity: "copy-v1"}}) {
		t.Fatalf("provider evidence %#v", output.Providers)
	}
}

func TestImportFromInfoUsesExactSourceAliasSpelling(t *testing.T) {
	root := renderModule(t, map[string]string{
		"implicit.go": `package fixture

import "fmt"

func Implicit() { fmt.Print("implicit") }
`,
		"explicit.go": `package fixture

import named "fmt"

func Explicit() { named.Print("explicit") }
`,
	})
	snapshot := collectSnapshot(t, root)
	state, err := newRenderState(snapshot.Workspace, []string{"implicit.go", "explicit.go"}, generate.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	implicit, _, _, err := state.existingFile("implicit.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importFromInfo(implicit, cutplan.ImportRef{Path: "fmt", Name: "fmt", Alias: ""}); err != nil {
		t.Fatalf("implicit source spelling was not routable: %v", err)
	}
	if _, err := importFromInfo(implicit, cutplan.ImportRef{Path: "fmt", Name: "fmt", Alias: "fmt"}); err == nil {
		t.Fatal("derived package name was accepted as an implicit source alias")
	}
	explicit, _, _, err := state.existingFile("explicit.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importFromInfo(explicit, cutplan.ImportRef{Path: "fmt", Name: "fmt", Alias: "named"}); err != nil {
		t.Fatalf("explicit source spelling was not routable: %v", err)
	}
	if _, err := importFromInfo(explicit, cutplan.ImportRef{Path: "fmt", Name: "fmt", Alias: ""}); err == nil {
		t.Fatal("implicit spelling was accepted for an explicit source alias")
	}
}

func TestDetachedCloneKeepsImplicitImportForOrdinaryAndExternalConsumer(t *testing.T) {
	const corePath = "example.com/renderfixture/pkg/core"
	root := renderModule(t, map[string]string{
		"pkg/core/exports.go": "package core\n\ntype Token struct{}\n",
		"pkg/consumer/use.go": `package consumer

import "example.com/renderfixture/pkg/core"

func Use(token core.Token) { _ = token }
`,
		"pkg/core/external_test.go": `package core_test

import (
	"testing"

	"example.com/renderfixture/pkg/core"
)

func TestExternal(t *testing.T) { _ = core.Token{} }
`,
	})
	from := symbol(corePath + "#package:Token")
	snapshot := collectSnapshot(t, root, from)
	state, err := newRenderState(snapshot.Workspace, []string{"pkg/consumer/use.go", "pkg/core/external_test.go"}, generate.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pkg/consumer/use.go", "pkg/core/external_test.go"} {
		file, _, _, err := state.existingFile(path)
		if err != nil {
			t.Fatalf("clone %s: %v", path, err)
		}
		var imported *types.PkgName
		for _, spec := range file.file.Imports {
			if strings.Trim(spec.Path.Value, "\"") != corePath {
				continue
			}
			var ok bool
			imported, ok = file.info.Implicits[spec].(*types.PkgName)
			if !ok || imported == nil {
				t.Fatalf("detached %s lost implicit import typing for %s", path, corePath)
			}
			break
		}
		if imported == nil {
			t.Fatalf("detached %s lost source import %s", path, corePath)
		}
		if imported.Imported() == nil || imported.Imported().Path() != corePath {
			t.Fatalf("detached %s import is not %s: %#v", path, corePath, imported)
		}
		projected, err := snapshot.Workspace.ObjectForFile(from, file.origin)
		if err != nil {
			t.Fatalf("detached %s cannot project the source object: %v", path, err)
		}
		selected, err := state.importForPackageSelector(file, projected.Pkg())
		if err != nil {
			t.Fatalf("detached %s cannot project source package into consumer variant: %v", path, err)
		}
		if selected != imported {
			t.Fatalf("detached %s selected a different import: got %#v want %#v", path, selected, imported)
		}
	}
}

func TestCloneWithInfoPreservesNestedSelectorOccurrences(t *testing.T) {
	root := renderModule(t, map[string]string{
		"pkg/core/exports.go": `package core

type Token struct { Name string }
var DefaultToken = Token{Name: "default"}
`,
		"pkg/consumer/use.go": `package consumer

import "example.com/renderfixture/pkg/core"

func Use() string { return core.DefaultToken.Name }
`,
	})
	snapshot := collectSnapshot(t, root, symbol("example.com/renderfixture/pkg/core#package:DefaultToken"))
	source, err := snapshot.Workspace.File("pkg/consumer/use.go")
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := snapshot.Workspace.PackageForFile(source)
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := cloneWithInfo(source, snapshot.Workspace.FileSet(), pkg.Info)
	if err != nil {
		t.Fatal(err)
	}
	originalOuter, originalInner := nestedSelectorPair(t, source.AST)
	copiedOuter, copiedInner := nestedSelectorPair(t, cloned.file)
	if originalOuter.Pos() != originalInner.Pos() {
		t.Fatalf("test requires equal nested selector positions: outer=%d inner=%d", originalOuter.Pos(), originalInner.Pos())
	}
	originalSelection := pkg.Info.Selections[originalOuter]
	if originalSelection == nil || pkg.Info.Selections[originalInner] != nil {
		t.Fatalf("source selector classification is not nested package/field shape: outer=%#v inner=%#v", originalSelection, pkg.Info.Selections[originalInner])
	}
	if cloned.info.Selections[copiedOuter] != originalSelection {
		t.Fatalf("clone lost outer field selection: got %#v want %#v", cloned.info.Selections[copiedOuter], originalSelection)
	}
	if cloned.info.Selections[copiedInner] != nil {
		t.Fatalf("clone assigned field selection to inner package selector: %#v", cloned.info.Selections[copiedInner])
	}
	if got, want := cloned.info.Uses[copiedInner.Sel], pkg.Info.Uses[originalInner.Sel]; got != want || got == nil {
		t.Fatalf("clone inner package member = %#v, want %#v", got, want)
	}
}

func TestCloneWithInfoPreservesNestedFieldSelectionsAtOnePosition(t *testing.T) {
	source, sourceSet, info := nestedFieldCloneFixture(t)
	cloned, err := cloneWithInfo(source, sourceSet, info)
	if err != nil {
		t.Fatal(err)
	}
	originalOuter, originalInner := nestedSelectorPair(t, source.AST)
	copiedOuter, copiedInner := nestedSelectorPair(t, cloned.file)
	if originalOuter.Pos() != originalInner.Pos() {
		t.Fatalf("test requires equal nested selector positions: outer=%d inner=%d", originalOuter.Pos(), originalInner.Pos())
	}
	originalOuterSelection, originalInnerSelection := info.Selections[originalOuter], info.Selections[originalInner]
	if originalOuterSelection == nil || originalInnerSelection == nil || originalOuterSelection.Kind() != types.FieldVal || originalInnerSelection.Kind() != types.FieldVal {
		t.Fatalf("source selectors are not nested field selections: outer=%#v inner=%#v", originalOuterSelection, originalInnerSelection)
	}
	if cloned.info.Selections[copiedOuter] != originalOuterSelection || cloned.info.Selections[copiedInner] != originalInnerSelection {
		t.Fatalf("clone merged equal-position nested selections: outer=%#v inner=%#v", cloned.info.Selections[copiedOuter], cloned.info.Selections[copiedInner])
	}
}

func TestCloneWithInfoTransfersOnlyLocalMapDenominators(t *testing.T) {
	const sourceText = "package sample\n\nfunc Value() int { return data.Field }\n"
	const foreignText = "package foreign\n\nfunc Value() int { return data.Field }\n"
	sourceSet := token.NewFileSet()
	sourceAST, err := parser.ParseFile(sourceSet, "sample.go", sourceText, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	foreignSet := token.NewFileSet()
	foreignAST, err := parser.ParseFile(foreignSet, "foreign.go", foreignText, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	sourceFunc, sourceSelector := firstFunctionAndSelector(t, sourceAST)
	foreignFunc, foreignSelector := firstFunctionAndSelector(t, foreignAST)
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{
			sourceSelector: {}, foreignSelector: {},
		},
		Defs: map[*ast.Ident]types.Object{
			sourceFunc.Name: nil, foreignFunc.Name: nil,
		},
		Uses: map[*ast.Ident]types.Object{
			sourceSelector.Sel: nil, foreignSelector.Sel: nil,
		},
		Implicits: map[ast.Node]types.Object{
			sourceFunc: nil, foreignFunc: nil,
		},
		Selections: map[*ast.SelectorExpr]*types.Selection{
			sourceSelector: nil, foreignSelector: nil,
		},
		Scopes: map[ast.Node]*types.Scope{
			sourceFunc: nil, foreignFunc: nil,
		},
		Instances: map[*ast.Ident]types.Instance{
			sourceFunc.Name: {}, foreignFunc.Name: {},
		},
		FileVersions: map[*ast.File]string{
			sourceAST: "go1.25", foreignAST: "go1.24",
		},
		InitOrder: []*types.Initializer{{Rhs: sourceSelector}},
	}
	source := semantic.WorkspaceFile{Path: "sample.go", PackageID: "example.com/sample", PackagePath: "example.com/sample", AST: sourceAST, Source: []byte(sourceText)}
	cloned, err := cloneWithInfo(source, sourceSet, info)
	if err != nil {
		t.Fatal(err)
	}
	copiedFunc, copiedSelector := firstFunctionAndSelector(t, cloned.file)
	assertMapEntry(t, "Types", cloned.info.Types, ast.Expr(copiedSelector))
	assertMapEntry(t, "Defs", cloned.info.Defs, copiedFunc.Name)
	assertMapEntry(t, "Uses", cloned.info.Uses, copiedSelector.Sel)
	assertMapEntry(t, "Implicits", cloned.info.Implicits, ast.Node(copiedFunc))
	assertMapEntry(t, "Selections", cloned.info.Selections, copiedSelector)
	assertMapEntry(t, "Scopes", cloned.info.Scopes, ast.Node(copiedFunc))
	assertMapEntry(t, "Instances", cloned.info.Instances, copiedFunc.Name)
	assertMapEntry(t, "FileVersions", cloned.info.FileVersions, cloned.file)
	for _, denominator := range []int{
		len(cloned.info.Types), len(cloned.info.Defs), len(cloned.info.Uses), len(cloned.info.Implicits),
		len(cloned.info.Selections), len(cloned.info.Scopes), len(cloned.info.Instances), len(cloned.info.FileVersions),
	} {
		if denominator != 1 {
			t.Fatalf("local map denominator = %d, want 1; cross-file node leaked", denominator)
		}
	}
	if got := cloned.info.FileVersions[cloned.file]; got != "go1.25" {
		t.Fatalf("local file version = %q, want go1.25", got)
	}
	if cloned.info.InitOrder != nil {
		t.Fatalf("file-local clone retained package-wide InitOrder: %#v", cloned.info.InitOrder)
	}
}

func TestCloneWithInfoRejectsAlteredSpan(t *testing.T) {
	const sourceText = "package sample\n\nfunc Value() int { return 1 }\n"
	const alteredText = "package sample\n\nfunc Value() int { return 10 }\n"
	sourceSet := token.NewFileSet()
	source, err := parser.ParseFile(sourceSet, "sample.go", sourceText, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	alteredSet := token.NewFileSet()
	altered, err := parser.ParseFile(alteredSet, "sample.go", alteredText, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloneFileInfo(source, altered, sourceSet, alteredSet, &types.Info{}); err == nil || !strings.Contains(err.Error(), "span differs") {
		t.Fatalf("altered detached span accepted: %v", err)
	}
}

func BenchmarkCloneWithInfoNestedSelectors(b *testing.B) {
	source, sourceSet, info := nestedFieldCloneFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cloned, err := cloneWithInfo(source, sourceSet, info)
		if err != nil {
			b.Fatal(err)
		}
		if len(cloned.info.Selections) != 2 {
			b.Fatalf("selection denominator = %d, want 2", len(cloned.info.Selections))
		}
	}
}

func nestedFieldCloneFixture(tb testing.TB) (semantic.WorkspaceFile, *token.FileSet, *types.Info) {
	tb.Helper()
	const text = `package nested

type Leaf struct { Value int }
type Middle struct { Leaf Leaf }
var Root Middle
func Read() int { return Root.Leaf.Value }
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "nested.go", text, parser.ParseComments)
	if err != nil {
		tb.Fatal(err)
	}
	info := &types.Info{
		Types:        map[ast.Expr]types.TypeAndValue{},
		Defs:         map[*ast.Ident]types.Object{},
		Uses:         map[*ast.Ident]types.Object{},
		Implicits:    map[ast.Node]types.Object{},
		Selections:   map[*ast.SelectorExpr]*types.Selection{},
		Scopes:       map[ast.Node]*types.Scope{},
		Instances:    map[*ast.Ident]types.Instance{},
		FileVersions: map[*ast.File]string{},
	}
	if _, err := (&types.Config{}).Check("example.com/nested", set, []*ast.File{file}, info); err != nil {
		tb.Fatal(err)
	}
	return semantic.WorkspaceFile{Path: "nested.go", PackageID: "example.com/nested", PackagePath: "example.com/nested", AST: file, Source: []byte(text)}, set, info
}

func firstFunctionAndSelector(tb testing.TB, file *ast.File) (*ast.FuncDecl, *ast.SelectorExpr) {
	tb.Helper()
	var function *ast.FuncDecl
	for _, declaration := range file.Decls {
		if candidate, ok := declaration.(*ast.FuncDecl); ok {
			function = candidate
			break
		}
	}
	if function == nil {
		tb.Fatal("fixture has no function")
	}
	var selector *ast.SelectorExpr
	ast.Inspect(function, func(node ast.Node) bool {
		if selector == nil {
			selector, _ = node.(*ast.SelectorExpr)
		}
		return selector == nil
	})
	if selector == nil {
		tb.Fatal("fixture has no selector")
	}
	return function, selector
}

func assertMapEntry[K comparable, V any](tb testing.TB, name string, values map[K]V, key K) {
	tb.Helper()
	if _, exists := values[key]; !exists {
		tb.Fatalf("%s lost explicit local map entry", name)
	}
}

func nestedSelectorPair(t *testing.T, file *ast.File) (*ast.SelectorExpr, *ast.SelectorExpr) {
	t.Helper()
	var outer, inner *ast.SelectorExpr
	ast.Inspect(file, func(node ast.Node) bool {
		candidate, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		child, ok := candidate.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if outer != nil {
			t.Fatal("fixture has multiple nested selector pairs")
		}
		outer, inner = candidate, child
		return true
	})
	if outer == nil || inner == nil {
		t.Fatal("fixture has no nested selector pair")
	}
	return outer, inner
}

func TestCompileRoutesAllMovedObjectsThroughOrdinaryAndExternalConsumers(t *testing.T) {
	const (
		corePath = "example.com/renderfixture/pkg/core"
		flowPath = "example.com/renderfixture/pkg/flow"
	)
	root := renderModule(t, map[string]string{
		"pkg/core/exports.go": `package core

type Token struct { Name string }
const DefaultName = "default"
var DefaultToken = Token{Name: DefaultName}
func UseToken(token Token) string { return token.Name }
`,
		"pkg/flow/flow.go": "package flow\n",
		"pkg/consumer/use.go": `package consumer

import "example.com/renderfixture/pkg/core"

func Use(token core.Token) string {
	return core.UseToken(token) + core.DefaultToken.Name + core.DefaultName
}
`,
		"pkg/core/external_test.go": `package core_test

import (
	"testing"

	"example.com/renderfixture/pkg/core"
)

func TestExternal(t *testing.T) {
	token := core.Token{Name: core.DefaultName}
	if got := core.UseToken(token); got != core.DefaultName { t.Fatal(got) }
	if core.DefaultToken.Name != core.DefaultName { t.Fatal(core.DefaultToken) }
}
`,
	})
	from := []cutplan.SymbolRef{
		symbol(corePath + "#package:Token"),
		symbol(corePath + "#package:DefaultName"),
		symbol(corePath + "#package:DefaultToken"),
		symbol(corePath + "#package:UseToken"),
	}
	to := []cutplan.SymbolRef{
		symbol(flowPath + "#package:Token"),
		symbol(flowPath + "#package:DefaultName"),
		symbol(flowPath + "#package:DefaultToken"),
		symbol(flowPath + "#package:UseToken"),
	}
	snapshot := collectSnapshot(t, root, from...)
	subjects := make([]cutplan.Relocation, len(from))
	for index := range from {
		subjects[index] = cutplan.Relocation{From: from[index], To: to[index]}
	}
	bindings := make([]cutplan.Binding, 0, len(from)*2)
	for _, consumer := range []string{"pkg/consumer/use.go", "pkg/core/external_test.go"} {
		for index := range from {
			bindings = append(bindings, cutplan.Binding{Consumer: consumer, From: from[index], To: to[index], Form: cutplan.BindingPackageSelector})
		}
	}
	flowImport := cutplan.ImportRef{Path: flowPath, Name: "flow", Alias: ""}
	coreImport := cutplan.ImportRef{Path: corePath, Name: "core", Alias: ""}
	intent := singleOperationIntent("all-object-variant-routes", cutplan.Operation{
		ID: "core-flow", Authority: cutplan.Authority{From: "core", To: "flow"},
		Edits: []cutplan.Edit{{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{
			Source: "pkg/core/exports.go", Destination: cutplan.Destination{Path: "pkg/flow/flow.go", Package: "flow"}, Subjects: subjects,
		}}},
		Bindings: bindings,
		Imports: []cutplan.Import{
			{Consumer: "pkg/consumer/use.go", From: &coreImport, To: &flowImport, Symbols: to},
			{Consumer: "pkg/core/external_test.go", From: &coreImport, To: &flowImport, Symbols: to},
		},
		Footprint: cutplan.Footprint{
			Read:  []string{"pkg/core/exports.go", "pkg/consumer/use.go", "pkg/core/external_test.go", "pkg/flow/flow.go"},
			Write: []string{"pkg/core/exports.go", "pkg/consumer/use.go", "pkg/core/external_test.go", "pkg/flow/flow.go"},
		},
		Verify: verification(from[0], "pkg/core/exports.go"),
	})
	canonical, err := cutplan.CanonicalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := indexRequirements(canonical)
	if err != nil {
		t.Fatal(err)
	}
	detached, err := newRenderState(snapshot.Workspace, cutplan.WritePaths(canonical), generate.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pkg/consumer/use.go", "pkg/core/external_test.go"} {
		file, _, _, err := detached.existingFile(path)
		if err != nil {
			t.Fatal(err)
		}
		routes := routeSet{}
		for _, binding := range canonical.Operations[0].Bindings {
			if binding.Consumer == path {
				routes.bindings = append(routes.bindings, binding)
			}
		}
		for _, route := range canonical.Operations[0].Imports {
			if route.Consumer == path {
				routes.imports = append(routes.imports, route)
			}
		}
		plan, err := detached.routePlanForFile(requirements, file, routes)
		if err != nil {
			t.Fatalf("plan routes for %s: %v", path, err)
		}
		if len(plan.Members) != len(from) {
			t.Fatalf("plan member denominator in %s = %d, want %d", path, len(plan.Members), len(from))
		}
		for _, ref := range from {
			object, err := detached.bindingSource(requirements, file, ref)
			if err != nil {
				t.Fatalf("project %s in %s: %v", ref.Object, path, err)
			}
			uses := 0
			for _, resolved := range file.info.Uses {
				if resolved == object {
					uses++
				}
			}
			if uses == 0 {
				t.Fatalf("projected %s has no exact detached use in %s", ref.Object, path)
			}
		}
		for _, binding := range plan.Members {
			selectors := 0
			ast.Inspect(file.file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				object := file.info.Uses[selector.Sel]
				if selection := file.info.Selections[selector]; selection != nil {
					object = selection.Obj()
				}
				if object == binding.From {
					selectors++
				}
				return true
			})
			if selectors == 0 {
				t.Fatalf("planned source %s has no exact detached selector in %s", binding.From.Name(), path)
			}
		}
	}
	output, err := Compile(Input{Intent: intent, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pkg/consumer/use.go", "pkg/core/external_test.go"} {
		text := outputText(t, output, path)
		if !strings.Contains(text, `"example.com/renderfixture/pkg/flow"`) || strings.Contains(text, `flow "example.com/renderfixture/pkg/flow"`) {
			t.Fatalf("%s did not retain exact implicit replacement import:\n%s", path, text)
		}
		for _, want := range []string{"flow.Token", "flow.DefaultName", "flow.DefaultToken", "flow.UseToken"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s lacks routed %s:\n%s", path, want, text)
			}
		}
	}
	moved := outputText(t, output, "pkg/flow/flow.go")
	for _, want := range []string{"type Token", "const DefaultName", "var DefaultToken", "func UseToken"} {
		if !strings.Contains(moved, want) {
			t.Fatalf("flow output lacks moved %s:\n%s", want, moved)
		}
	}
}

func TestCompileRejectsMethodValueInsteadOfCreatingBridge(t *testing.T) {
	t.Skip("v3 rejects synthetic method routes before renderer classification")
	root := renderModule(t, map[string]string{
		"a.go": `package fixture

type T struct{}
func (T) M() {}
func Use(t T) { _ = t.M }
`,
		"b.go": "package fixture\n\nfunc Unused() {}\n",
	})
	snapshot := collectSnapshot(t, root)
	method := symbol("example.com/renderfixture#type:T/method:M")
	newMethod := symbol("example.com/renderfixture#type:T/method:N")
	intent := singleOperationIntent("method-value", cutplan.Operation{
		ID: "retire", Authority: cutplan.Authority{From: "a", To: "b"},
		Edits:     []cutplan.Edit{{Kind: cutplan.EditRetire, Retire: &cutplan.Retire{Source: "b.go", Symbols: []cutplan.SymbolRef{symbol("example.com/renderfixture#package:Unused")}}}},
		Bindings:  []cutplan.Binding{{Consumer: "a.go", From: method, To: newMethod, Form: cutplan.BindingMethodCall, Receiver: []cutplan.ReceiverPathStep{{Kind: cutplan.ReceiverDirectView, Object: newMethod}}}},
		Footprint: cutplan.Footprint{Read: []string{"a.go", "b.go"}, Write: []string{"a.go", "b.go"}},
		Verify:    verification(symbol("example.com/renderfixture#package:Unused"), "b.go"),
	})
	if _, err := Compile(Input{Intent: intent, Snapshot: snapshot}); err == nil || !strings.Contains(err.Error(), "method-value") {
		t.Fatalf("method value route was accepted: %v", err)
	}
}

func TestWitnessRejectsReplacedDetachedPointer(t *testing.T) {
	root := renderModule(t, map[string]string{
		"a.go": "package fixture\n\nfunc F() int { return 1 }\n",
	})
	from := symbol("example.com/renderfixture#package:F")
	to := symbol("example.com/renderfixture#package:G")
	snapshot := collectSnapshot(t, root, from)
	state, err := newRenderState(snapshot.Workspace, []string{"a.go"}, generate.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	file, source, pkg, err := state.existingFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	compiler := &compiler{state: state, snapshot: snapshot}
	captured, err := compiler.captureRelocation(cutplan.Relocation{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	compiler.witnesses = captured
	replacement, err := cloneWithInfo(source, snapshot.Workspace.FileSet(), pkg.Info)
	if err != nil {
		t.Fatal(err)
	}
	state.files[file.path] = replacement
	if err := compiler.materializeWitnesses(); err == nil || !strings.Contains(err.Error(), "removed or replaced") {
		t.Fatalf("replacement pointer was accepted: %v", err)
	}
}

func TestWitnessRejectsAmbiguousDetachedPointer(t *testing.T) {
	root := renderModule(t, map[string]string{
		"a.go": "package fixture\n\nfunc F() int { return 1 }\n",
	})
	from := symbol("example.com/renderfixture#package:F")
	to := symbol("example.com/renderfixture#package:G")
	snapshot := collectSnapshot(t, root, from)
	state, err := newRenderState(snapshot.Workspace, []string{"a.go"}, generate.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	file, _, _, err := state.existingFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	compiler := &compiler{state: state, snapshot: snapshot}
	captured, err := compiler.captureRelocation(cutplan.Relocation{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) == 0 {
		t.Fatal("capture did not retain a declaration pointer")
	}
	compiler.witnesses = captured
	file.file.Decls = append(file.file.Decls, &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
		&ast.ValueSpec{Names: []*ast.Ident{{Name: "_"}}, Values: []ast.Expr{captured[0].ident}},
	}})
	if err := compiler.materializeWitnesses(); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous pointer was accepted: %v", err)
	}
}

func singleOperationIntent(name string, operation cutplan.Operation) cutplan.Intent {
	return cutplan.Intent{Schema: cutplan.Version, Name: name, Operations: []cutplan.Operation{operation}}
}

func verification(absence cutplan.SymbolRef, path string) cutplan.Verification {
	return cutplan.Verification{
		Laws:  []cutplan.Law{{ID: "render", Package: "./fixture", Test: "TestRender"}},
		Gates: []cutplan.Gate{cutplan.GateDiagnostics},
	}
}

func symbol(value string) cutplan.SymbolRef { return cutplan.SymbolRef{Object: value} }

func renderModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeRenderFile(t, root, "go.mod", "module example.com/renderfixture\n\ngo 1.25\n")
	for path, value := range files {
		writeRenderFile(t, root, path, value)
	}
	return root
}

func writeRenderFile(t *testing.T, root, path, contents string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func collectSnapshot(t *testing.T, root string, source ...cutplan.SymbolRef) semantic.Snapshot {
	t.Helper()
	return collectIntentSnapshot(t, root, snapshotFixtureIntent(t, root, source))
}

func collectIntentSnapshot(t *testing.T, root string, intent cutplan.Intent) semantic.Snapshot {
	t.Helper()
	session, err := semantic.NewSession(semantic.Config{Root: root, Flashrefactor: "render-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := session.Close(); closeErr != nil {
			t.Errorf("close semantic session: %v", closeErr)
		}
	})
	snapshot, err := session.Collect(context.Background(), intent, nil)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func snapshotFixtureIntent(t *testing.T, root string, source []cutplan.SymbolRef) cutplan.Intent {
	t.Helper()
	if len(source) == 0 {
		source = []cutplan.SymbolRef{firstFixtureSymbol(t, root)}
	}
	byPath := map[string][]cutplan.SymbolRef{}
	for _, object := range source {
		path := fixtureSymbolPath(t, root, object)
		byPath[path] = append(byPath[path], object)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	operations := make([]cutplan.Operation, 0, len(paths))
	for index, path := range paths {
		symbols := byPath[path]
		operations = append(operations, cutplan.Operation{
			ID: "snapshot-" + strconv.Itoa(index), Authority: cutplan.Authority{From: "fixture", To: "retired"},
			Edits:     []cutplan.Edit{{Kind: cutplan.EditRetire, Retire: &cutplan.Retire{Source: path, Symbols: symbols}}},
			Footprint: cutplan.Footprint{Read: []string{path}, Write: []string{path}},
			Verify: cutplan.Verification{
				Laws:  []cutplan.Law{{ID: "empty", Package: "./fixture", Test: "TestRender"}},
				Gates: []cutplan.Gate{cutplan.GateImportDAG},
			},
		})
	}
	intent := cutplan.Intent{Schema: cutplan.Version, Name: "snapshot", Operations: operations}
	if err := cutplan.ValidateIntent(intent); err != nil {
		t.Fatalf("snapshot fixture intent: %v", err)
	}
	return intent
}

func fixtureSymbolPath(t *testing.T, root string, object cutplan.SymbolRef) string {
	t.Helper()
	terminal := object.Object[strings.LastIndex(object.Object, ":")+1:]
	var match string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || match != "" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr == nil && strings.Contains(string(data), terminal) {
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr == nil {
				match = filepath.ToSlash(relative)
			}
		}
		return nil
	})
	if match == "" {
		t.Fatalf("fixture source path for %s", object.Object)
	}
	return match
}

func firstFixtureSymbol(t *testing.T, root string) cutplan.SymbolRef {
	t.Helper()
	moduleData, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module := ""
	for _, line := range strings.Split(string(moduleData), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "module" {
			module = fields[1]
		}
	}
	if module == "" {
		t.Fatal("fixture go.mod lacks module")
	}
	var result cutplan.SymbolRef
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || result.Object != "" {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return nil
		}
		for _, declaration := range file.Decls {
			var name string
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				name = value.Name.Name
			case *ast.GenDecl:
				if len(value.Specs) != 0 {
					switch spec := value.Specs[0].(type) {
					case *ast.TypeSpec:
						name = spec.Name.Name
					case *ast.ValueSpec:
						if len(spec.Names) != 0 {
							name = spec.Names[0].Name
						}
					}
				}
			}
			if name != "" {
				relative, relativeErr := filepath.Rel(root, filepath.Dir(path))
				if relativeErr == nil {
					packagePath := module
					if relative != "." {
						packagePath += "/" + filepath.ToSlash(relative)
					}
					result = cutplan.SymbolRef{Object: packagePath + "#package:" + name}
				}
				return nil
			}
		}
		return nil
	})
	if result.Object == "" {
		t.Fatal("fixture has no package declaration")
	}
	return result
}

func assertWitnesses(t *testing.T, output Output, from, to cutplan.SymbolRef, want map[cutplan.SiteRole]int) RouteWitness {
	t.Helper()
	var found *RouteWitness
	for index := range output.Witnesses {
		candidate := &output.Witnesses[index]
		if candidate.From == from && candidate.To == to {
			if found != nil {
				t.Fatalf("duplicate witness group for %s -> %s", from.Object, to.Object)
			}
			found = candidate
		}
	}
	if found == nil {
		t.Fatalf("missing witness group for %s -> %s: %#v", from.Object, to.Object, output.Witnesses)
	}
	got := make(map[cutplan.SiteRole]int, len(found.Sites))
	seen := map[PhysicalWitnessSite]bool{}
	for _, site := range found.Sites {
		if seen[site.Source] {
			t.Fatalf("duplicate physical source witness: %#v", site.Source)
		}
		seen[site.Source] = true
		if site.Source.Role != site.Target.Role {
			t.Fatalf("role drift in witness: %#v", site)
		}
		if site.Target.Name != targetSymbolName(t, to) {
			t.Fatalf("target name mismatch in witness: %#v", site)
		}
		got[site.Source.Role]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("witness roles = %#v, want %#v", got, want)
	}
	return *found
}

func targetSymbolName(t *testing.T, ref cutplan.SymbolRef) string {
	t.Helper()
	name, err := targetName(ref)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func outputText(t *testing.T, output Output, path string) string {
	t.Helper()
	for _, file := range output.Files {
		if file.Path == path {
			if file.Delete {
				t.Fatalf("%s is deleted", path)
			}
			return string(file.Content)
		}
	}
	t.Fatalf("missing output %s in %#v", path, output.Files)
	return ""
}

func deletedOutput(output Output, path string) bool {
	for _, file := range output.Files {
		if file.Path == path {
			return file.Delete
		}
	}
	return false
}
