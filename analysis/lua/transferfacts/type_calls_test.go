package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type testExternalTypes map[string]typ.Type

func (m testExternalTypes) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) == 2 {
		t, ok := m[path[0]+"."+path[1]]
		return t, ok
	}
	return nil, false
}

func TestLowerTypeCastCallPublishesArgumentAndResultEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local v = Point(data)
`)
	body := wirlower.Lower("type-cast-call", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	dataStmt := mustLocalStmt(t, stmts, 1)
	castStmt := mustLocalStmt(t, stmts, 2)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	refinements := facts.PostconditionRefinements(callPoint)
	if len(refinements) != 1 {
		t.Fatalf("postcondition refinements = %d, want 1: %#v", len(refinements), refinements)
	}
	if !refinements[0].TargetPath().Equal(dataPath) {
		t.Fatalf("postcondition target = %s, want %s", refinements[0].TargetPath(), dataPath)
	}
	assertUntrustedPointNarrowing(t, reg, refinementConstraint(t, refinements[0].Value()))

	results := facts.CallResultValues(callPoint)
	if len(results) != 1 {
		t.Fatalf("call result values = %d, want 1: %#v", len(results), results)
	}
	if results[0].Index() != 0 {
		t.Fatalf("call result index = %d, want 0", results[0].Index())
	}
	assertTypeIsPointProof(t, reg, results[0].Value())
}

func TestLowerPrimitiveTypeCastCallPublishesResultEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data: any = 1
local v = number(data)
`)
	body := wirlower.Lower("type-is-error-nil", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	castStmt := mustLocalStmt(t, stmts, 1)
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	results := facts.CallResultValues(callPoint)
	if len(results) != 1 {
		t.Fatalf("primitive call result values = %d, want 1: %#v", len(results), results)
	}
	assertTypeIsRuntimeProof(t, reg, results[0].Value(), typ.Number)
}

func TestLowerPrimitiveTypeCastCalleeComesFromWIR(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data: any = 1
local v = number(data)
`, "string")
	dataStmt := mustLocalStmt(t, stmts, 0)
	castStmt := mustLocalStmt(t, stmts, 1)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)
	stringSym, ok := bindings.GlobalSymbol("string")
	if !ok {
		t.Fatal("missing string global symbol")
	}
	stringPath := path.NewPath(stringSym, "string")

	body := wir.NewBody("primitive-cast-callee-owner")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(stringPath))}},
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(dataPath))}}),
	})
	body.SetPointRange(callPoint, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	results := facts.CallResultValues(callPoint)
	if len(results) != 1 {
		t.Fatalf("primitive call result values = %d, want 1: %#v", len(results), results)
	}
	assertTypeIsRuntimeProof(t, reg, results[0].Value(), typ.String)
}

func TestLowerPrimitiveTypeCastPostconditionArgumentPathComesFromWIR(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data: any = 1
local other: any = 2
local v = number(data)
`)
	dataStmt := mustLocalStmt(t, stmts, 0)
	otherStmt := mustLocalStmt(t, stmts, 1)
	castStmt := mustLocalStmt(t, stmts, 2)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	otherPath := path.NewPath(mustLocalAt(t, bindings, otherStmt, 0), "other")
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	body := wir.NewBody("primitive-cast-arg-owner")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandTemp, Ref: 99}},
		Type:  body.InternType(typ.Number),
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
	})
	body.SetPointRange(callPoint, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	refinements := facts.PostconditionRefinements(callPoint)
	if len(refinements) != 1 {
		t.Fatalf("postcondition refinements = %d, want 1: %#v", len(refinements), refinements)
	}
	if !refinements[0].TargetPath().Equal(otherPath) || refinements[0].TargetPath().Equal(dataPath) {
		t.Fatalf("postcondition target = %s, want WIR arg %s not semantic arg %s", refinements[0].TargetPath(), otherPath, dataPath)
	}
	got, ok := typevalue.TypeOf(reg, refinementConstraint(t, refinements[0].Value()))
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("postcondition type witness = %v/%v, want number", got, ok)
	}
	results := facts.CallResultValues(callPoint)
	if len(results) != 1 {
		t.Fatalf("primitive call result values = %d, want 1: %#v", len(results), results)
	}
	assertTypeIsRuntimeProof(t, reg, results[0].Value(), typ.Number)
}

func TestLowerPrimitiveTypeCastFactsFromWIRWithoutSemanticCallView(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data: any = 1
local v = 0
`, "number")
	dataStmt := mustLocalStmt(t, stmts, 0)
	localStmt := mustLocalStmt(t, stmts, 1)
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	if _, ok := result.CallView(point); ok {
		t.Fatalf("test point %d unexpectedly has semantic call view", point)
	}
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	numberSym, ok := bindings.GlobalSymbol("number")
	if !ok {
		t.Fatal("missing number global symbol")
	}
	numberPath := path.NewPath(numberSym, "number")

	body := wir.NewBody("synthetic-primitive-cast-no-call-view")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: point,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(numberPath))}},
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(dataPath))}}),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	refinements := facts.PostconditionRefinements(point)
	if len(refinements) != 1 {
		t.Fatalf("WIR primitive cast postconditions = %d, want 1: %#v", len(refinements), refinements)
	}
	if !refinements[0].TargetPath().Equal(dataPath) {
		t.Fatalf("postcondition target = %s, want %s", refinements[0].TargetPath(), dataPath)
	}
	got, ok := typevalue.TypeOf(reg, refinementConstraint(t, refinements[0].Value()))
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("postcondition type witness = %v/%v, want number", got, ok)
	}
	results := facts.CallResultValues(point)
	if len(results) != 1 {
		t.Fatalf("WIR primitive cast result values = %d, want 1: %#v", len(results), results)
	}
	assertTypeIsRuntimeProof(t, reg, results[0].Value(), typ.Number)
}

func TestLowerTypeValueCastFactsFromWIRTypeWithoutSemanticCallView(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data: any = {}
local v = 0
`)
	dataStmt := mustLocalStmt(t, stmts, 0)
	localStmt := mustLocalStmt(t, stmts, 1)
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	if _, ok := result.CallView(point); ok {
		t.Fatalf("test point %d unexpectedly has semantic call view", point)
	}
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	pointType := typetable.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()

	body := wir.NewBody("synthetic-type-value-cast-no-call-view")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: point,
		Type:  body.InternType(pointType),
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(dataPath))}}),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	refinements := facts.PostconditionRefinements(point)
	if len(refinements) != 1 {
		t.Fatalf("WIR type-value cast postconditions = %d, want 1: %#v", len(refinements), refinements)
	}
	if !refinements[0].TargetPath().Equal(dataPath) {
		t.Fatalf("postcondition target = %s, want %s", refinements[0].TargetPath(), dataPath)
	}
	assertUntrustedPointNarrowing(t, reg, refinementConstraint(t, refinements[0].Value()))
	results := facts.CallResultValues(point)
	if len(results) != 1 {
		t.Fatalf("WIR type-value cast result values = %d, want 1: %#v", len(results), results)
	}
	assertTypeIsPointProof(t, reg, results[0].Value())
}

func TestLowerPrimitiveTypeCastCallRespectsValueShadow(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local number = function(value) return value end
local data: any = 1
local v = number(data)
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	castStmt := mustLocalStmt(t, stmts, 2)
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	if results := facts.CallResultValues(callPoint); len(results) != 0 {
		t.Fatalf("shadowed primitive call results = %#v, want none", results)
	}
}

func TestLowerTypeIsErrorNilBranchPublishesArgumentEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local _, err = Point:is(data)
if err == nil then
end
`)
	body := wirlower.Lower("type-is-error-nil", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	dataStmt := mustLocalStmt(t, stmts, 1)
	ifStmt := mustIfStmt(t, stmts, 3)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	branchPoint := requireStmtPoints(t, built, ifStmt, 1)[0]

	var found bool
	refinements := facts.BranchRefinements(branchPoint)
	for _, refinement := range refinements {
		if !refinement.TargetPath().Equal(dataPath) {
			continue
		}
		value, ok := refinement.TrueValue()
		if !ok {
			continue
		}
		assertUntrustedPointNarrowing(t, reg, refinementConstraint(t, value))
		found = true
	}
	if !found {
		t.Fatalf("missing true-edge Point refinement for %s at branch %d", dataPath, branchPoint)
	}
}

func TestLowerTypeIsBranchArgumentPathComesFromWIR(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local other: any = {}
local validated, err = Point:is(data)
if err == nil then
end
`)
	dataStmt := mustLocalStmt(t, stmts, 1)
	otherStmt := mustLocalStmt(t, stmts, 2)
	assign := mustLocalStmt(t, stmts, 3)
	points := requireStmtPoints(t, built, assign, 3)
	callPoint, valueAssignPoint, errAssignPoint := points[0], points[1], points[2]
	valuePath := path.NewPath(mustLocalAt(t, bindings, assign, 0), "validated")
	errPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "err")
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	otherPath := path.NewPath(mustLocalAt(t, bindings, otherStmt, 0), "other")
	typeDecl, ok := bindings.TypeDef(stmts[0].(*ast.TypeDefStmt))
	if !ok {
		t.Fatal("missing Point type declaration")
	}
	pointType, ok := typeresolve.New(bindings).Decl(typeDecl)
	if !ok {
		t.Fatal("failed to resolve Point type")
	}
	ifStmt := mustIfStmt(t, stmts, 4)
	branchPoint := requireStmtPoints(t, built, ifStmt, 1)[0]

	body := wir.NewBody("type-is-arg-owner")
	valueTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	errTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 2}
	callStart := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Type:    body.InternType(pointType),
		Call:    wir.CallInfo{Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(path.NewPath(0, "Point")))}, Method: body.InternConst(wir.Const{Kind: wir.ConstString, Str: "is"})},
		List:    body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
		Results: body.AppendOperands([]wir.Operand{valueTemp, errTemp}),
	})
	body.SetPointRange(callPoint, callStart, callStart+1)
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetLocalAssignment,
		Index:       0,
		ResultIndex: 0,
		Path:        valuePath,
	})
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetLocalAssignment,
		Index:       1,
		ResultIndex: 1,
		Path:        errPath,
	})
	valueAssignStart := body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: valueAssignPoint,
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(valuePath))},
		A:     valueTemp,
	})
	body.SetPointRange(valueAssignPoint, valueAssignStart, valueAssignStart+1)
	errAssignStart := body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: errAssignPoint,
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(errPath))},
		A:     errTemp,
	})
	body.SetPointRange(errAssignPoint, errAssignStart, errAssignStart+1)
	branchStart := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: branchPoint,
		Check: body.InternCheck(wir.Check{Kind: wir.CheckNil, Path: errPath}),
	})
	body.SetPointRange(branchPoint, branchStart, branchStart+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	refinement, ok := branchRefinementAt(facts.BranchRefinements(branchPoint), otherPath)
	if !ok {
		t.Fatalf("missing WIR-arg Point refinement for %s at branch %d; got %#v", otherPath, branchPoint, facts.BranchRefinements(branchPoint))
	}
	value, ok := refinement.TrueValue()
	if !ok {
		t.Fatalf("missing true-edge WIR-arg refinement: %#v", refinement)
	}
	assertUntrustedPointNarrowing(t, reg, refinementConstraint(t, value))
	if _, ok := branchRefinementAt(facts.BranchRefinements(branchPoint), dataPath); ok {
		t.Fatalf("type-is branch refinement used semantic arg path %s instead of WIR arg path %s", dataPath, otherPath)
	}
}

func TestLowerImportedTypeIsMemberCalleePublishesResultSlots(t *testing.T) {
	reg := standard.Registry()
	appError := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local errors = require("errors")
local raw: any = {}
local validated, err = errors.AppError:is(raw)
`, "require")
	resolver := typeresolve.NewWithExternal(bindings, testExternalTypes{"errors.AppError": appError})
	body := wirlower.Lower("imported-type-is", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, TypeResolver: resolver, WIR: body})
	castStmt := mustLocalStmt(t, stmts, 2)
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	values := facts.CallResultValues(callPoint)
	if len(values) != 2 {
		t.Fatalf("imported Type:is call result values = %d, want 2: %#v", len(values), values)
	}
	gotType, ok := typevalue.TypeOf(reg, values[0].Value())
	wantType := typ.MaterializeOptional(appError)
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("type witness = %v/%v, want %v", gotType, ok, wantType)
	}
	if got := product.PresenceOf(values[0].Value()); !presence.Equal(got, presence.Maybe()) {
		t.Fatalf("value presence = %s, want maybe before the success branch proves it present", got)
	}
	if got := product.Get(reg, values[0].Value(), assertion.Key); !got.Has(assertion.RuntimeClaim) {
		t.Fatalf("assertion = %s, want runtime validation proof", got)
	}
}

func TestLowerImportedTypeIsReceiverComesFromWIR(t *testing.T) {
	reg := standard.Registry()
	appError := typetable.NewRecord().
		Field("code", typ.String).
		Build()
	otherError := typetable.NewRecord().
		Field("status", typ.Integer).
		Build()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local errors = require("errors")
local raw: any = {}
local validated, err = errors.AppError:is(raw)
`, "require")
	errorsStmt := mustLocalStmt(t, stmts, 0)
	rawStmt := mustLocalStmt(t, stmts, 1)
	assign := mustLocalStmt(t, stmts, 2)
	castCall := assign.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)
	errorsPath := path.NewPath(mustLocalAt(t, bindings, errorsStmt, 0), "errors")
	rawPath := path.NewPath(mustLocalAt(t, bindings, rawStmt, 0), "raw")
	otherReceiver := errorsPath.Field("OtherError")

	body := wir.NewBody("type-is-receiver-owner")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call: wir.CallInfo{
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherReceiver))},
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "is"}),
		},
		List: body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(rawPath))}}),
	})
	body.SetPointRange(callPoint, start, start+1)

	resolver := typeresolve.NewWithExternal(bindings, testExternalTypes{
		"errors.AppError":   appError,
		"errors.OtherError": otherError,
	})
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, TypeResolver: resolver, WIR: body})
	values := facts.CallResultValues(callPoint)
	if len(values) != 2 {
		t.Fatalf("imported Type:is call result values = %d, want 2: %#v", len(values), values)
	}
	gotType, ok := typevalue.TypeOf(reg, values[0].Value())
	wantType := typ.MaterializeOptional(otherError)
	if !ok || !typ.TypeEquals(gotType, wantType) || typ.TypeEquals(gotType, typ.MaterializeOptional(appError)) {
		t.Fatalf("type witness = %v/%v, want WIR receiver %v not semantic receiver %v", gotType, ok, wantType, typ.MaterializeOptional(appError))
	}
}

func TestLowerTypeIsDirectConditionPublishesArgumentEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
if Point:is(data) then
end
`)
	body := wirlower.Lower("type-is-condition", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	dataStmt := mustLocalStmt(t, stmts, 1)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	branchPoint := requireWIRBranchPoint(t, built.Graph, body)

	refinements := facts.BranchRefinements(branchPoint)
	var found bool
	for _, refinement := range refinements {
		if !refinement.TargetPath().Equal(dataPath) {
			continue
		}
		value, ok := refinement.TrueValue()
		if !ok {
			continue
		}
		assertUntrustedPointNarrowing(t, reg, refinementConstraint(t, value))
		found = true
	}
	if !found {
		t.Fatalf("missing true-edge Point refinement for %s at branch %d", dataPath, branchPoint)
	}
}

func TestLowerTypeIsNegatedConditionPublishesInvertedArgumentEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
if not Point:is(data) then
end
`)
	body := wirlower.Lower("type-is-negated-condition", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	dataStmt := mustLocalStmt(t, stmts, 1)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	branchPoint := requireWIRBranchPoint(t, built.Graph, body)

	refinements := facts.BranchRefinements(branchPoint)
	var found bool
	for _, refinement := range refinements {
		if !refinement.TargetPath().Equal(dataPath) {
			continue
		}
		if _, ok := refinement.TrueValue(); ok {
			t.Fatalf("unexpected true-edge Point refinement for negated Type:is")
		}
		value, ok := refinement.FalseValue()
		if !ok {
			continue
		}
		assertUntrustedPointNarrowing(t, reg, refinementConstraint(t, value))
		found = true
	}
	if !found {
		t.Fatalf("missing false-edge Point refinement for %s at branch %d: %#v", dataPath, branchPoint, refinements)
	}
}

func TestLowerTypeIsOpenTailReturnPublishesSlotsAndPresenceRelation(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
return Point:is(data)
`)
	body := wirlower.Lower("type-is-open-tail", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	ret := stmts[2].(*ast.ReturnStmt)
	points := requireStmtPoints(t, built, ret, 2)
	callPoint := points[0]
	returnPoint := points[1]

	values := facts.CallResultValues(callPoint)
	if len(values) != 2 {
		t.Fatalf("Type:is call result values = %d, want 2: %#v", len(values), values)
	}
	returnFact, ok := facts.Return(returnPoint)
	if !ok {
		t.Fatalf("missing lowered return fact")
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceCall || !sources[0].OpenTail || !sources[0].Expanded || sources[0].TargetIndex != 0 || sources[0].ResultIndex != 0 {
		t.Fatalf("return sources = %#v, want one expanded open-tail Type:is call source", sources)
	}
	relations := facts.ReturnPresenceRelations(returnPoint)
	assertReturnPresenceRelation(t, relations, 1, presence.Present(), 0, presence.Absent())
	assertReturnPresenceRelation(t, relations, 1, presence.Absent(), 0, presence.Present())
}

func TestLowerWithWIRTypeIsReturnPresenceUsesWIRReturnSources(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
return Point:is(data)
`)
	ret := stmts[2].(*ast.ReturnStmt)
	points := requireStmtPoints(t, built, ret, 2)
	returnPoint := points[1]
	body := wir.NewBody("synthetic-type-is-empty-return")
	start := body.Emit(wir.Instruction{Op: wir.OpReturn, Point: returnPoint})
	body.SetPointRange(returnPoint, start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	returnFact, ok := facts.Return(returnPoint)
	if !ok {
		t.Fatalf("missing WIR return fact")
	}
	if sources := returnFact.Sources(); len(sources) != 0 {
		t.Fatalf("WIR return sources = %#v, want empty synthetic return", sources)
	}
	if relations := facts.ReturnPresenceRelations(returnPoint); len(relations) != 0 {
		t.Fatalf("WIR empty return inherited semantic Type:is presence relations: %#v", relations)
	}
}

func TestLowerWithWIRTypeIsReturnPresenceUsesCallSiteWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
return Point:is(data)
`)
	ret := stmts[2].(*ast.ReturnStmt)
	points := requireStmtPoints(t, built, ret, 2)
	callPoint := points[0]
	returnPoint := points[1]
	body := wirlower.Lower("chunk", stmts, bindings, built)
	seed := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	site, ok := seed.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing lowered Type:is callsite at point %d", callPoint)
	}
	lowered := lowerer{
		registry:     reg,
		bindings:     bindings,
		wir:          body,
		typeResolver: typeresolve.New(bindings),
	}
	sources, ok := lowered.returnValueSourcesFromWIR(returnPoint)
	if !ok {
		t.Fatalf("missing WIR return sources at point %d", returnPoint)
	}

	relations := lowered.typeIsReturnPresenceRelationsFromSources(
		sources,
		map[cfg.Point]factflow.CallSite{callPoint: site},
	)

	assertReturnPresenceRelation(t, relations, 1, presence.Present(), 0, presence.Absent())
	assertReturnPresenceRelation(t, relations, 1, presence.Absent(), 0, presence.Present())
}

func TestLowerWithWIRTypeIsCallResultValuesWithoutSemanticCallView(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local value = nil
`)
	assign := mustLocalStmt(t, stmts, 2)
	callPoint := requireStmtPoints(t, built, assign, 1)[0]
	if _, ok := result.CallView(callPoint); ok {
		t.Fatalf("fixture unexpectedly has semantic call view at point %d", callPoint)
	}
	typeDecl, ok := bindings.TypeDef(stmts[0].(*ast.TypeDefStmt))
	if !ok {
		t.Fatal("missing Point type declaration")
	}
	pointType, ok := typeresolve.New(bindings).Decl(typeDecl)
	if !ok {
		t.Fatal("failed to resolve Point type")
	}
	dataPath := path.NewPath(mustLocalAt(t, bindings, stmts[1].(*ast.LocalAssignStmt), 0), "data")
	body := wir.NewBody("synthetic-type-is-call")
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Type:    body.InternType(pointType),
		Call:    wir.CallInfo{Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(path.NewPath(0, "Point")))}, Method: body.InternConst(wir.Const{Kind: wir.ConstString, Str: "is"})},
		List:    body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(dataPath))}}),
		Results: body.AppendOperands([]wir.Operand{{Kind: wir.OperandTemp, Ref: 1}, {Kind: wir.OperandTemp, Ref: 2}}),
	})
	body.SetPointRange(callPoint, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	values := facts.CallResultValues(callPoint)
	if len(values) != 2 {
		t.Fatalf("WIR Type:is call result values = %d, want 2: %#v", len(values), values)
	}
	first, ok := callResultValueAt(values, 0)
	if !ok {
		t.Fatalf("missing WIR Type:is value result: %#v", values)
	}
	witness := product.Get(reg, first.Value(), typewitness.Key)
	got, ok := witness.Type()
	if !ok || !typ.TypeEquals(got, pointType) {
		t.Fatalf("WIR Type:is value result witness = %v/%v, want %v", got, ok, pointType)
	}
	if _, ok := callResultValueAt(values, 1); !ok {
		t.Fatalf("missing WIR Type:is error result: %#v", values)
	}
}

func TestLowerExplicitErrorReturnsPublishPresenceRelation(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function process(x: number): (number?, string?)
	if x < 0 then
		return nil, "negative"
	end
	return x * 2, nil
end`)
	body := wirlower.LowerFunction("process", fn, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	relations := allReturnPresenceRelations(built.Graph, facts)
	assertReturnPresenceRelation(t, relations, 1, presence.Present(), 0, presence.Absent())
	assertReturnPresenceRelation(t, relations, 1, presence.Absent(), 0, presence.Present())
}

func TestLowerReturnPresenceUnknownSourceBlocksMustRelation(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function process(value: number?): (number?, string?)
	return value, nil
end`)
	body := wirlower.LowerFunction("process", fn, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	relations := allReturnPresenceRelations(built.Graph, facts)
	assertNoReturnPresenceRelation(t, relations, 1, presence.Absent(), 0, presence.Present())
}

func TestLowerTypeCastCallIgnoresValueShadow(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local Point = function(value) return value end
local data: any = {}
local v = Point(data)
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	castStmt := mustLocalStmt(t, stmts, 3)
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	if refinements := facts.PostconditionRefinements(callPoint); len(refinements) != 0 {
		t.Fatalf("shadowed value call postconditions = %#v, want none", refinements)
	}
	if results := facts.CallResultValues(callPoint); len(results) != 0 {
		t.Fatalf("shadowed value call results = %#v, want none", results)
	}
}

func requireCallPoint(t *testing.T, graph cfg.Graph, result *semantics.Result, call *ast.FuncCallExpr) cfg.Point {
	t.Helper()
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if ok && fact.Call == call {
			return point
		}
	}
	t.Fatalf("missing call point for %#v", call)
	return 0
}

func refinementConstraint(t *testing.T, refinement factflow.ValueRefinement) product.Value {
	t.Helper()
	value, ok := refinement.Constraint()
	if !ok {
		t.Fatalf("missing refinement constraint")
	}
	return value
}

func assertProductPointLike(t *testing.T, reg *axis.Registry, value product.Value) {
	t.Helper()
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("presence = %s, want present", got)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtime kind = %s, want table", got)
	}
}

func assertTypeIsPointProof(t *testing.T, reg *axis.Registry, value product.Value) {
	t.Helper()
	assertProductPointLike(t, reg, value)
	if got := product.Get(reg, value, assertion.Key); !got.Has(assertion.RuntimeClaim) {
		t.Fatalf("assertion = %s, want runtime validation proof", got)
	}
}

func assertTypeIsRuntimeProof(t *testing.T, reg *axis.Registry, value product.Value, want typ.Type) {
	t.Helper()
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("presence = %s, want present", got)
	}
	gotType, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("type witness = %v/%v, want %v", gotType, ok, want)
	}
	if got := product.Get(reg, value, assertion.Key); !got.Has(assertion.RuntimeClaim) {
		t.Fatalf("assertion = %s, want runtime validation proof", got)
	}
}

func assertUntrustedPointNarrowing(t *testing.T, reg *axis.Registry, value product.Value) {
	t.Helper()
	assertProductPointLike(t, reg, value)
	if got := product.Get(reg, value, evidence.Key); !got.IsExplicitTop() {
		t.Fatalf("evidence = %s, want explicit top origin preserved", got)
	}
	if got := product.Get(reg, value, assertion.Key); got.Has(assertion.RuntimeClaim) {
		t.Fatalf("assertion = %s, argument narrowing must not be runtime validation proof", got)
	}
}

func assertReturnPresenceRelation(
	t *testing.T,
	relations []factflow.ReturnPresenceRelation,
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex() == triggerIndex &&
			presence.Equal(relation.TriggerPresence(), triggerPresence) &&
			relation.TargetIndex() == targetIndex &&
			presence.Equal(relation.TargetPresence(), targetPresence) {
			return
		}
	}
	t.Fatalf("missing return presence relation %d/%s -> %d/%s in %#v",
		triggerIndex, triggerPresence, targetIndex, targetPresence, relations)
}

func assertNoReturnPresenceRelation(
	t *testing.T,
	relations []factflow.ReturnPresenceRelation,
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex() == triggerIndex &&
			presence.Equal(relation.TriggerPresence(), triggerPresence) &&
			relation.TargetIndex() == targetIndex &&
			presence.Equal(relation.TargetPresence(), targetPresence) {
			t.Fatalf("return presence relation %d/%s -> %d/%s unexpectedly present in %#v",
				triggerIndex, triggerPresence, targetIndex, targetPresence, relations)
		}
	}
}

func allReturnPresenceRelations(graph cfg.Graph, facts factflow.Facts) []factflow.ReturnPresenceRelation {
	var out []factflow.ReturnPresenceRelation
	for _, point := range graph.RPO() {
		out = append(out, facts.ReturnPresenceRelations(point)...)
	}
	return out
}

func callResultValueAt(values []factflow.CallResultValue, index int) (factflow.CallResultValue, bool) {
	for _, value := range values {
		if value.Index() == index {
			return value, true
		}
	}
	return factflow.CallResultValue{}, false
}
