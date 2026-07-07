package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerSymbolTypesKeepsParamAnnotationsWithoutSemanticResult(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function handle(ch: Channel<{kind: "event", id: string}>)
	local selected = channel.select { ch:case_receive() }
end
`, "channel")
	slots := bindings.ParamSlots(fn)
	if len(slots) != 1 {
		t.Fatalf("ParamSlots = %#v, want one typed parameter", slots)
	}

	got := lowerSymbolTypes(bindings, built.Graph, built.Meta, typeresolve.New(bindings), importlookup.Source{}, nil)
	if got == nil {
		t.Fatal("lowerSymbolTypes returned nil without semantic result")
	}
	if gotType, ok := got[slots[0].Symbol]; !ok || gotType == nil {
		t.Fatalf("symbol type for parameter %d = %v/%v, want annotation", slots[0].Symbol, gotType, ok)
	}
}

func TestLowerSymbolTypesReadsCfgMetadataWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
function build(value: string): number
	return 1
end

for i = 1, 3 do
end
`, "build")
	if len(stmts) != 2 {
		t.Fatalf("parsed %d statements, want function and numeric for", len(stmts))
	}
	funcDef, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok {
		t.Fatalf("statement 0 = %T, want function definition", stmts[0])
	}
	numberFor, ok := stmts[1].(*ast.NumberForStmt)
	if !ok {
		t.Fatalf("statement 1 = %T, want numeric for", stmts[1])
	}
	funcSym, ok := bindings.FuncDefTargetSymbol(funcDef)
	if !ok || funcSym == 0 {
		t.Fatalf("FuncDefTargetSymbol = %d/%v, want symbol", funcSym, ok)
	}
	loopSym, ok := bindings.NumForSymbol(numberFor)
	if !ok || loopSym == 0 {
		t.Fatalf("NumForSymbol = %d/%v, want symbol", loopSym, ok)
	}

	got := lowerSymbolTypes(bindings, built.Graph, built.Meta, typeresolve.New(bindings), importlookup.Source{}, nil)
	if got == nil {
		t.Fatal("lowerSymbolTypes returned nil without semantic result")
	}
	fnType, ok := got[funcSym].(*typ.Function)
	if !ok || fnType == nil {
		t.Fatalf("function symbol type = %T %[1]v/%v, want function type", got[funcSym], ok)
	}
	if len(fnType.Params) != 1 || !typ.TypeEquals(fnType.Params[0].Type, typ.String) {
		t.Fatalf("function params = %#v, want one string parameter", fnType.Params)
	}
	if len(fnType.Returns) != 1 || !typ.TypeEquals(fnType.Returns[0], typ.Number) {
		t.Fatalf("function returns = %#v, want number", fnType.Returns)
	}
	if gotType, ok := got[loopSym]; !ok || !typ.TypeEquals(gotType, typ.Integer) {
		t.Fatalf("numeric-for symbol type = %v/%v, want integer", gotType, ok)
	}
}

func TestLowerSymbolTypesFromWIRSeedsLocalCallResultTypes(t *testing.T) {
	body := wir.NewBody("call-result-symbol-types")
	callPoint := cfg.Point(10)
	localPath := path.NewPath(symbol.ID(42), "result")
	resultType := typetable.NewRecord().
		Field("ok", typ.Boolean).
		Field("value", typ.String).
		Build()
	calleeType := typ.Func().Returns(resultType).Build()
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Type:    body.InternType(calleeType),
		Results: body.AppendOperands([]wir.Operand{resultTemp}),
	})
	body.SetPointRange(callPoint, start, body.Len())
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetLocalAssignment,
		Index:       0,
		ResultIndex: 0,
		Path:        localPath,
	})

	got := lowerSymbolTypesFromWIR(body, nil, importlookup.Source{})
	if got == nil {
		t.Fatal("lowerSymbolTypesFromWIR returned nil")
	}
	if gotType, ok := got[localPath.Symbol]; !ok || !typ.TypeEquals(gotType, resultType) {
		t.Fatalf("call result symbol type = %v/%v, want %v", gotType, ok, resultType)
	}
}

func TestLowerSymbolTypesFromWIRUsesRequireExportForMemberCallResultWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
local discovery = require("discovery")
local suite_names = discovery.sorted_keys({})
`)
	body := wirlower.Lower("require-member-call-result-symbol-type", stmts, bindings, built)
	suiteNames, ok := bindings.LocalSymbolAt(stmts[1].(*ast.LocalAssignStmt), 0)
	if !ok || suiteNames == 0 {
		t.Fatalf("suite_names symbol = %d/%v, want local symbol", suiteNames, ok)
	}
	keysType := typ.NewArray(typ.String)
	discoveryManifest := manifest.New("discovery")
	discoveryManifest.SetExport(typetable.NewRecord().
		Field("sorted_keys", typ.Func().
			Param("t", typetable.NewMap(typ.String, typ.Any)).
			Returns(keysType).
			Build()).
		Build())

	got := lowerSymbolTypesFromWIR(body, bindings, importlookup.Source{Manifests: []*manifest.Manifest{discoveryManifest}})
	if got == nil {
		t.Fatal("lowerSymbolTypesFromWIR returned nil")
	}
	if gotType, ok := got[suiteNames]; !ok || !typ.TypeEquals(gotType, keysType) {
		t.Fatalf("suite_names type = %v/%v, want %v", gotType, ok, keysType)
	}
}

func TestLowerSymbolTypesFromWIRDoesNotUseCapturedRequireExportAfterRootWrite(t *testing.T) {
	stmts := parseChunk(t, `
local discovery = require("discovery")

function run(): ()
    discovery = {}
    local suite_names = discovery.sorted_keys({})
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	def := stmts[1].(*ast.FuncDefStmt)
	built := cfgbuild.BuildFunction(def.Func, bindings)
	body := wirlower.Lower("captured-require-root-write", def.Func.Stmts, bindings, built)
	suiteNames, ok := bindings.LocalSymbolAt(def.Func.Stmts[1].(*ast.LocalAssignStmt), 0)
	if !ok || suiteNames == 0 {
		t.Fatalf("suite_names symbol = %d/%v, want local symbol", suiteNames, ok)
	}
	discoveryManifest := manifest.New("discovery")
	discoveryManifest.SetExport(typetable.NewRecord().
		Field("sorted_keys", typ.Func().
			Param("t", typetable.NewMap(typ.String, typ.Any)).
			Returns(typ.NewArray(typ.String)).
			Build()).
		Build())

	got := lowerSymbolTypesFromWIR(body, bindings, importlookup.Source{Manifests: []*manifest.Manifest{discoveryManifest}})
	if gotType, ok := got[suiteNames]; ok {
		t.Fatalf("suite_names type = %v, want no manifest-derived call result after discovery root write", gotType)
	}
}

func TestLowerSymbolTypesFromWIRCopiesStaticSourcePathTypeWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
local source: { retry: { max_attempts: number }? } = { retry = { max_attempts = 3 } }
local options = source
`)
	body := wirlower.Lower("static-source-path-local-copy", stmts, bindings, built)
	sourceSym, ok := bindings.LocalSymbolAt(stmts[0].(*ast.LocalAssignStmt), 0)
	if !ok || sourceSym == 0 {
		t.Fatalf("source symbol = %d/%v, want local symbol", sourceSym, ok)
	}
	optionsSym, ok := bindings.LocalSymbolAt(stmts[1].(*ast.LocalAssignStmt), 0)
	if !ok || optionsSym == 0 {
		t.Fatalf("options symbol = %d/%v, want local symbol", optionsSym, ok)
	}

	got := lowerSymbolTypesFromWIR(body, bindings, importlookup.Source{})
	sourceType, hasSource := got[sourceSym]
	optionsType, hasOptions := got[optionsSym]
	if !hasSource || sourceType == nil {
		t.Fatalf("source type = %v/%v, want annotated source type", sourceType, hasSource)
	}
	if !hasOptions || !typ.TypeEquals(optionsType, sourceType) {
		t.Fatalf("options type = %v/%v, want copied source type %v", optionsType, hasOptions, sourceType)
	}
}

func TestAccessChainTypeProjectsStaticStringAndIntIndexes(t *testing.T) {
	element := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	rootType := typetable.NewRecord().
		StaticStringIndex("payload", element).
		Field("items", typ.NewArray(element)).
		Build()

	t.Run("static string index", func(t *testing.T) {
		root := &ast.IdentExpr{Value: "root"}
		expr := attrGet(
			attrGet(root, &ast.StringExpr{Value: "payload"}, ast.AttrKeyIndex),
			&ast.StringExpr{Value: "name"},
			ast.AttrKeyDot,
		)
		assertAccessChainType(t, root, expr, rootType, typ.String)
	})

	t.Run("static integer index", func(t *testing.T) {
		root := &ast.IdentExpr{Value: "root"}
		expr := attrGet(
			attrGet(
				attrGet(root, &ast.StringExpr{Value: "items"}, ast.AttrKeyDot),
				&ast.NumberExpr{Value: "1"},
				ast.AttrKeyIndex,
			),
			&ast.StringExpr{Value: "name"},
			ast.AttrKeyDot,
		)
		assertAccessChainType(t, root, expr, rootType, typeexpr.Optional(typ.String))
	})
}

func attrGet(obj ast.Expr, key ast.Expr, syntax ast.AttrKeySyntax) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       key,
		KeySyntax: syntax,
	}
}

func assertAccessChainType(t *testing.T, root *ast.IdentExpr, expr ast.Expr, rootType typ.Type, want typ.Type) {
	t.Helper()
	bindings := bind.BindChunk([]ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{expr}}}, bind.Options{})
	rootSym, ok := bindings.SymbolOf(root)
	if !ok || rootSym == 0 {
		t.Fatalf("SymbolOf(%q) = %d/%v, want non-zero symbol", root.Value, rootSym, ok)
	}
	got, ok := accessChainType(map[symbol.ID]typ.Type{rootSym: rootType}, bindings, expr)
	if !ok {
		t.Fatal("accessChainType rejected static path")
	}
	if !typ.TypeEquals(got, want) {
		t.Fatalf("accessChainType type = %v, want %v", got, want)
	}
}
