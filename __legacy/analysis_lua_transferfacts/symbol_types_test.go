package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/domain/path"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerSymbolTypesKeepsParamAnnotationsWithoutSemanticResult(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function handle(ch: Channel<{kind: "event", id: string}>)
	local selected = channel.select { ch:case_receive() }
end
`, "channel")
	slots := bindings.ParamSlots(fn)
	if len(slots) != 1 {
		t.Fatalf("ParamSlots = %#v, want one typed parameter", slots)
	}

	body := wirlower.LowerFunction("handle", fn, bindings, built)
	got := lowerSymbolTypesFromWIR(body, importlookup.Source{})
	if got == nil {
		t.Fatal("lowerSymbolTypesFromWIR returned nil without semantic result")
	}
	if gotType, ok := got[slots[0].Symbol]; !ok || gotType == nil {
		t.Fatalf("symbol type for parameter %d = %v/%v, want annotation", slots[0].Symbol, gotType, ok)
	}
}

func TestLowerSymbolTypesReadsFunctionAndNumericForFromWIRWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
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

	wirBody := wirlower.Lower("function-and-numeric-for-symbol-types", stmts, bindings, built)
	wirTypes := lowerSymbolTypesFromWIR(wirBody, importlookup.Source{})
	if wirTypes == nil {
		t.Fatal("lowerSymbolTypesFromWIR returned nil")
	}
	fnType, ok := wirTypes[funcSym].(*typ.Function)
	if !ok || fnType == nil {
		t.Fatalf("WIR function symbol type = %T %[1]v/%v, want function type", wirTypes[funcSym], ok)
	}
	if len(fnType.Params) != 1 || !typ.TypeEquals(fnType.Params[0].Type, typ.String) {
		t.Fatalf("function params = %#v, want one string parameter", fnType.Params)
	}
	if len(fnType.Returns) != 1 || !typ.TypeEquals(fnType.Returns[0], typ.Number) {
		t.Fatalf("function returns = %#v, want number", fnType.Returns)
	}
	if gotType, ok := wirTypes[loopSym]; !ok || !typ.TypeEquals(gotType, typ.Integer) {
		t.Fatalf("numeric-for symbol type = %v/%v, want integer", gotType, ok)
	}
}

func TestLowerSymbolTypesFromWIRSeedsCapturedFunctionRootTypes(t *testing.T) {
	stmts, bindings, _ := parseSemanticChunk(t, `
local function make_status(): number
	return 1
end

local function run(): number
	return make_status()
end
`)
	if len(stmts) != 2 {
		t.Fatalf("parsed %d statements, want two local functions", len(stmts))
	}
	makeStmt := mustLocalStmt(t, stmts, 0)
	runStmt := mustLocalStmt(t, stmts, 1)
	makePath := path.NewPath(mustLocalAt(t, bindings, makeStmt, 0), "make_status")
	runFn, ok := runStmt.Exprs[0].(*ast.FunctionExpr)
	if !ok || runFn == nil {
		t.Fatalf("run expression = %T, want function", runStmt.Exprs[0])
	}
	runBuilt := cfgbuild.BuildFunction(runFn, bindings)
	body := wirlower.LowerFunction("run", runFn, bindings, runBuilt)
	got := lowerSymbolTypesFromWIR(body, importlookup.Source{})
	fnType, ok := got[makePath.Symbol].(*typ.Function)
	if !ok || fnType == nil || len(fnType.Returns) != 1 || !typ.TypeEquals(fnType.Returns[0], typ.Number) {
		t.Fatalf("captured function root type = %T %[1]v/%v, want () -> number", got[makePath.Symbol], ok)
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

	got := lowerSymbolTypesFromWIR(body, importlookup.Source{})
	if got == nil {
		t.Fatal("lowerSymbolTypesFromWIR returned nil")
	}
	if gotType, ok := got[localPath.Symbol]; !ok || !typ.TypeEquals(gotType, resultType) {
		t.Fatalf("call result symbol type = %v/%v, want %v", gotType, ok, resultType)
	}
}

func TestLowerSymbolTypesFromWIRUsesRequireExportForMemberCallResultWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
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

	got := lowerSymbolTypesFromWIR(body, importlookup.Source{Manifests: []*manifest.Manifest{discoveryManifest}})
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
	body := wirlower.LowerFunction("captured-require-root-write", def.Func, bindings, built)
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

	got := lowerSymbolTypesFromWIR(body, importlookup.Source{Manifests: []*manifest.Manifest{discoveryManifest}})
	if gotType, ok := got[suiteNames]; ok {
		t.Fatalf("suite_names type = %v, want no manifest-derived call result after discovery root write", gotType)
	}
}

func TestLowerSymbolTypesFromWIRCopiesStaticSourcePathTypeWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
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

	got := lowerSymbolTypesFromWIR(body, importlookup.Source{})
	sourceType, hasSource := got[sourceSym]
	optionsType, hasOptions := got[optionsSym]
	if !hasSource || sourceType == nil {
		t.Fatalf("source type = %v/%v, want annotated source type", sourceType, hasSource)
	}
	if !hasOptions || !typ.TypeEquals(optionsType, sourceType) {
		t.Fatalf("options type = %v/%v, want copied source type %v", optionsType, hasOptions, sourceType)
	}
}
