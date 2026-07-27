package moduleidentity_test

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestExactRequireCallRecognizesOnlyGlobalRequireStringCalls(t *testing.T) {
	stmts := parseChunk(t, `
		local json = require("json")
		local require = function(name) return name end
		local shadowed = require("shadowed")
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})

	first := stmts[0].(*ast.LocalAssignStmt)
	modulePath, ok := moduleidentity.ExactRequireCall(bindings, first.Exprs[0])
	if !ok || modulePath != "json" {
		t.Fatalf("ExactRequireCall(global require) = %q/%v, want json/true", modulePath, ok)
	}

	shadowed := stmts[2].(*ast.LocalAssignStmt)
	if modulePath, ok := moduleidentity.ExactRequireCall(bindings, shadowed.Exprs[0]); ok {
		t.Fatalf("ExactRequireCall(shadowed require) = %q/true, want false", modulePath)
	}
}

func TestProjectionSharesAliasAndSignatureIdentity(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local json = require("json")
		local value = json.decode("{}")
	`)

	modulePath, ok := projection.ModulePathForAlias("json")
	if !ok || modulePath != "json" {
		t.Fatalf("ModulePathForAlias(json) = %q/%v, want json/true", modulePath, ok)
	}

	var got string
	for _, point := range graph.RPO() {
		call, ok := facts.CallSiteView(point)
		if !ok || call.CalleePathRef().IsEmpty() {
			continue
		}
		name, ok := projection.SignatureName(point, call.CalleePathRef())
		if !ok {
			continue
		}
		got = name
	}
	if got != "json.decode" {
		t.Fatalf("SignatureName = %q, want json.decode", got)
	}
}

func TestProjectionUsesStaticIntMemberSignatureIdentity(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local runtime = require("runtime")
		local value = runtime[1]("payload")
	`)

	got := onlySignatureName(t, projection, graph, facts)
	if got != "runtime[1]" {
		t.Fatalf("SignatureName(static int member) = %q, want runtime[1]", got)
	}
}

func TestProjectionResolvesLocalModuleRootAliasStaticIntMember(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local runtime = require("runtime")
		local alias = runtime
		local value = alias[1]("payload")
	`)

	got := onlySignatureName(t, projection, graph, facts)
	if got != "runtime[1]" {
		t.Fatalf("SignatureName(alias static int member) = %q, want runtime[1]", got)
	}
}

func TestProjectionResolvesLocalSignatureAlias(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local runtime = require("runtime")
		local store = runtime.store
		store({})
	`)

	got := onlySignatureName(t, projection, graph, facts)
	if got != "runtime.store" {
		t.Fatalf("SignatureName(local alias) = %q, want runtime.store", got)
	}
}

func TestProjectionResolvesAssignedMemberSignatureAlias(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local runtime = require("runtime")
		local provider = {}
		provider.store = runtime.store
		provider.store({})
	`)

	got := onlySignatureName(t, projection, graph, facts)
	if got != "runtime.store" {
		t.Fatalf("SignatureName(member alias) = %q, want runtime.store", got)
	}
}

func TestProjectionInvalidatesReassignedSignatureAlias(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local runtime = require("runtime")
		local store = runtime.store
		store = function() end
		store({})
	`)

	if got := onlySignatureName(t, projection, graph, facts); got != "" {
		t.Fatalf("SignatureName(reassigned local alias) = %q, want none", got)
	}
}

func TestProjectionResolvesExplicitGlobalSignatureAlias(t *testing.T) {
	projection, graph, facts := buildProjectionWithGlobals(t, `
		local store = ownership.store
		store({}, {})
	`, []string{"require", "ownership"})

	got := onlySignatureName(t, projection, graph, facts)
	if got != "ownership.store" {
		t.Fatalf("SignatureName(explicit global alias) = %q, want ownership.store", got)
	}
}

func TestProjectionResolvesLocalAliasOfExplicitGlobalModuleRoot(t *testing.T) {
	projection, graph, facts := buildProjectionWithGlobals(t, `
		local assert = assert2
		assert.has_error(nil, {})
	`, []string{"require", "assert2"})

	got := onlySignatureName(t, projection, graph, facts)
	if got != "assert2.has_error" {
		t.Fatalf("SignatureName(local alias of explicit global module root) = %q, want assert2.has_error", got)
	}
}

func TestProjectionResolvesCapturedAliasOfExplicitGlobalModuleRoot(t *testing.T) {
	stmts := parseChunk(t, `
		local table = table
		function make()
			return table.create(4, 0)
		end
		make()
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require", "table"}})
	def, ok := stmts[1].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt 1 = %T, want function definition", stmts[1])
	}
	built := cfgbuild.BuildFunction(def.Func, bindings)
	if built == nil || built.Graph == nil {
		t.Fatal("BuildFunction returned nil")
	}
	body := wirlower.LowerFunction("make", def.Func, bindings, built)
	facts := transferfacts.LowerDetailed(built.Graph, transferfacts.Config{Registry: standard.Registry(), WIR: body}).Facts
	projection := moduleidentity.NewFromFacts(bindings, built.Graph, moduleIdentityTestFacts{facts: facts}, def.Func)

	got := onlySignatureName(t, projection, built.Graph, facts)
	if got != "table.create" {
		t.Fatalf("SignatureName(captured explicit global module root alias) = %q, want table.create", got)
	}
}

func TestProjectionKeepsCapturedRequireModuleRootAlias(t *testing.T) {
	stmts := parseChunk(t, `
		local json = require("json")
		function decode()
			return json.decode("{}")
		end
		decode()
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	def, ok := stmts[1].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt 1 = %T, want function definition", stmts[1])
	}
	built := cfgbuild.BuildFunction(def.Func, bindings)
	if built == nil || built.Graph == nil {
		t.Fatal("BuildFunction returned nil")
	}
	body := wirlower.LowerFunction("decode", def.Func, bindings, built)
	facts := transferfacts.LowerDetailed(built.Graph, transferfacts.Config{Registry: standard.Registry(), WIR: body}).Facts
	projection := moduleidentity.NewFromFacts(bindings, built.Graph, moduleIdentityTestFacts{facts: facts}, def.Func)

	got := onlySignatureName(t, projection, built.Graph, facts)
	if got != "json.decode" {
		t.Fatalf("SignatureName(captured require module root alias) = %q, want json.decode", got)
	}
}

func TestRequireAliasesProjectionTracksLocalRequireNamesBeforeSemantics(t *testing.T) {
	stmts := parseChunk(t, `
		local json = require("json")
		local codec = json
		do
			local store = require("store")
		end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	projection := moduleidentity.NewRequireAliases(bindings, stmts, nil)

	aliases := projection.ModuleAliases()
	if aliases["json"] != "json" {
		t.Fatalf("json alias = %q, want json", aliases["json"])
	}
	if aliases["codec"] != "json" {
		t.Fatalf("codec alias = %q, want json", aliases["codec"])
	}
	if aliases["store"] != "store" {
		t.Fatalf("store alias = %q, want store", aliases["store"])
	}
}

func TestRequireAliasesProjectionTracksCapturedRequireNamesBeforeSemantics(t *testing.T) {
	stmts := parseChunk(t, `
		local json = require("json")
		function decode(payload)
			local codec = json
			return codec.decode(payload)
		end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	def, ok := stmts[1].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt 1 = %T, want function definition", stmts[1])
	}
	projection := moduleidentity.NewRequireAliases(bindings, def.Func.Stmts, def.Func)

	aliases := projection.ModuleAliases()
	if aliases["json"] != "json" {
		t.Fatalf("captured json alias = %q, want json", aliases["json"])
	}
	if aliases["codec"] != "json" {
		t.Fatalf("local codec alias = %q, want json", aliases["codec"])
	}
}

func TestRequireAliasesProjectionTracksTypeOnlyOuterRequireRoot(t *testing.T) {
	stmts := parseChunk(t, `
		local protocol = require("protocol")
		local function userID(user: protocol.User): string
			return user.id
		end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	assign, ok := stmts[1].(*ast.LocalAssignStmt)
	if !ok || len(assign.Exprs) != 1 {
		t.Fatalf("stmt 1 = %T, want local function assignment", stmts[1])
	}
	fn, ok := assign.Exprs[0].(*ast.FunctionExpr)
	if !ok || fn == nil {
		t.Fatalf("local function expression = %T, want function", assign.Exprs[0])
	}
	if captures := bindings.DirectCaptures(fn); len(captures) != 0 {
		t.Fatalf("type-only module root became runtime captures: %#v", captures)
	}
	projection := moduleidentity.NewRequireAliases(bindings, fn.Stmts, fn)
	if modulePath, ok := projection.ModulePathForAlias("protocol"); !ok || modulePath != "protocol" {
		t.Fatalf("type-only protocol alias = %q/%v, want protocol/true", modulePath, ok)
	}
}

func TestProjectionDoesNotResolveImplicitGlobalSignatureAlias(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local store = ownership.store
		store({}, {})
	`)

	if got := onlySignatureName(t, projection, graph, facts); got != "" {
		t.Fatalf("SignatureName(implicit global alias) = %q, want none", got)
	}
}

func TestProjectionInvalidatesReassignedRequireRoot(t *testing.T) {
	projection, graph, facts := buildProjection(t, `
		local json = require("json")
		json = {}
		local value = json.decode("{}")
	`)

	for _, point := range graph.RPO() {
		call, ok := facts.CallSiteView(point)
		if !ok || call.CalleePathRef().IsEmpty() {
			continue
		}
		if name, ok := projection.SignatureName(point, call.CalleePathRef()); ok {
			t.Fatalf("SignatureName after reassignment = %q/true, want false", name)
		}
	}
}

func TestProjectionWIRFactsParity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "require member",
			src: `
				local json = require("json")
				local value = json.decode("{}")
			`,
			want: "json.decode",
		},
		{
			name: "root alias static int member",
			src: `
				local runtime = require("runtime")
				local alias = runtime
				local value = alias[1]("payload")
			`,
			want: "runtime[1]",
		},
		{
			name: "local signature alias",
			src: `
				local runtime = require("runtime")
				local store = runtime.store
				store({})
			`,
			want: "runtime.store",
		},
		{
			name: "assigned member signature alias",
			src: `
				local runtime = require("runtime")
				local provider = {}
				provider.store = runtime.store
				provider.store({})
			`,
			want: "runtime.store",
		},
		{
			name: "reassigned alias invalidated",
			src: `
				local runtime = require("runtime")
				local store = runtime.store
				store = function() end
				store({})
			`,
			want: "",
		},
		{
			name: "object literal alias",
			src: `
				local runtime = require("runtime")
				local provider = { store = runtime.store }
				provider.store({})
			`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldProjection, wirProjection, graph, facts := buildProjectionPair(t, tc.src)
			oldName := onlySignatureName(t, oldProjection, graph, facts)
			got := onlySignatureName(t, wirProjection, graph, facts)
			if oldName != tc.want {
				t.Fatalf("factflow oracle = %q, want %q", oldName, tc.want)
			}
			if got != oldName {
				t.Fatalf("WIR projection = %q, want factflow parity %q", got, oldName)
			}
		})
	}
}

func onlySignatureName(t *testing.T, projection moduleidentity.Projection, graph cfg.Graph, facts factflow.Facts) string {
	t.Helper()
	var got string
	for _, point := range graph.RPO() {
		call, ok := facts.CallSiteView(point)
		if !ok || call.CalleePathRef().IsEmpty() {
			continue
		}
		name, ok := projection.SignatureName(point, call.CalleePathRef())
		if ok {
			got = name
		}
	}
	return got
}

func buildProjection(t *testing.T, src string) (moduleidentity.Projection, cfg.Graph, factflow.Facts) {
	t.Helper()
	return buildProjectionWithGlobals(t, src, []string{"require"})
}

func buildProjectionWithGlobals(t *testing.T, src string, globals []string) (moduleidentity.Projection, cfg.Graph, factflow.Facts) {
	t.Helper()
	stmts := parseChunk(t, src)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := transferfacts.LowerDetailed(built.Graph, transferfacts.Config{Registry: standard.Registry(), WIR: body}).Facts
	return moduleidentity.NewFromFacts(bindings, built.Graph, moduleIdentityTestFacts{facts: facts}, nil), built.Graph, facts
}

func buildProjectionPair(t *testing.T, src string) (moduleidentity.Projection, moduleidentity.Projection, cfg.Graph, factflow.Facts) {
	t.Helper()
	stmts := parseChunk(t, src)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := transferfacts.LowerDetailed(built.Graph, transferfacts.Config{Registry: standard.Registry(), WIR: body}).Facts
	return moduleidentity.NewFromFacts(bindings, built.Graph, moduleIdentityTestFacts{facts: facts}, nil),
		moduleidentity.NewFromWIR(bindings, built.Graph, body, nil),
		built.Graph,
		facts
}

type moduleIdentityTestFacts struct {
	facts factflow.Facts
}

func (m moduleIdentityTestFacts) LocalAssignment(point cfg.Point) (moduleidentity.Assignment, bool) {
	fact, ok := m.facts.LocalAssignment(point)
	if !ok {
		return moduleidentity.Assignment{}, false
	}
	return testModuleAssignment(fact), true
}

func (m moduleIdentityTestFacts) OrdinaryAssignment(point cfg.Point) (moduleidentity.Assignment, bool) {
	fact, ok := m.facts.OrdinaryAssignment(point)
	if !ok {
		return moduleidentity.Assignment{}, false
	}
	return testModuleAssignment(fact), true
}

func (m moduleIdentityTestFacts) PathAssignment(point cfg.Point) (moduleidentity.Assignment, bool) {
	fact, ok := m.facts.PathAssignment(point)
	if !ok {
		return moduleidentity.Assignment{}, false
	}
	target := fact.TargetPathRef()
	return moduleidentity.Assignment{
		Target:       target.Clone(),
		TargetSymbol: target.Symbol,
		Source:       testModuleSource(fact.Source()),
	}, true
}

func (m moduleIdentityTestFacts) PathDescendantInvalidation(point cfg.Point) (pathdom.Path, bool) {
	fact, ok := m.facts.PathDescendantInvalidation(point)
	if !ok {
		return pathdom.Path{}, false
	}
	return fact.ContainerPath(), true
}

func (m moduleIdentityTestFacts) CallSite(point cfg.Point) (moduleidentity.CallSite, bool) {
	site, ok := m.facts.CallSiteView(point)
	if !ok {
		return moduleidentity.CallSite{}, false
	}
	outArgs := make([]moduleidentity.Source, 0, site.ArgumentSourceCount())
	site.ForEachArgumentSource(func(_ int, arg factflow.ValueSource) bool {
		outArgs = append(outArgs, testModuleSource(arg))
		return true
	})
	return moduleidentity.CallSite{
		Callee:       site.CalleePath(),
		Args:         outArgs,
		TypeArgCount: site.TypeArgCount(),
		MethodName:   site.MethodName(),
	}, true
}

func (m moduleIdentityTestFacts) ForEachObjectLiteralEntry(expr moduleidentity.SourceRef, fn func(moduleidentity.ObjectEntry) bool) bool {
	lit, ok := m.facts.ObjectLiteralView(factflow.ExprRef(expr))
	if !ok {
		return false
	}
	lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if fn != nil && !fn(moduleidentity.ObjectEntry{
			Suffix: entry.Suffix(),
			Source: testModuleSource(entry.Source()),
		}) {
			return false
		}
		return true
	})
	return true
}

func (m moduleIdentityTestFacts) ExpressionPath(expr moduleidentity.SourceRef) (pathdom.Path, bool) {
	return m.facts.ExpressionPath(factflow.ExprRef(expr))
}

func testModuleAssignment(fact factflow.RootAssignment) moduleidentity.Assignment {
	return moduleidentity.Assignment{
		Target:       fact.TargetPath(),
		TargetSymbol: fact.TargetSymbol(),
		Source:       testModuleSource(fact.Source()),
	}
}

func testModuleSource(source factflow.ValueSource) moduleidentity.Source {
	out := moduleidentity.Source{
		Expr:        moduleidentity.SourceRef(source.ExprRef),
		HasExpr:     source.HasExpr,
		CallPoint:   source.CallPoint,
		ResultIndex: source.ResultIndex,
		PathKey:     source.PathKey,
		String:      source.String,
	}
	switch source.Kind {
	case factflow.ValueSourceExpression:
		out.Kind = moduleidentity.SourceExpression
	case factflow.ValueSourceCall:
		out.Kind = moduleidentity.SourceCall
	case factflow.ValueSourcePath:
		out.Kind = moduleidentity.SourcePath
	case factflow.ValueSourceLiteral:
		if source.LiteralKind == factflow.ValueSourceLiteralString {
			out.Kind = moduleidentity.SourceStringLiteral
		}
	}
	return out
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "moduleidentity_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return stmts
}
