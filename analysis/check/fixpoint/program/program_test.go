package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestRunBoundChunkUsesSuppliedBindIdentityForLocalCallee(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, `
local f = function()
	return 1
end
return f()
`)
	local := stmts[0].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fTarget := mustBoundLocalAt(t, bindings, local, 0)
	origin := onlyFunctionOrigin(t, bindings)
	if !origin.HasTargetSymbol || origin.TargetSymbol != fTarget {
		t.Fatalf("function origin target = %d/%v, want local symbol %d", origin.TargetSymbol, origin.HasTargetSymbol, fTarget)
	}

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{
			Registry:        reg,
			ExpressionValue: fixedExpressionValue(want),
		},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	targetKey, ok := result.TargetKey(fTarget)
	if !ok {
		t.Fatalf("TargetKey(%d) missing", fTarget)
	}
	if wantKey := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol)); targetKey != wantKey {
		t.Fatalf("TargetKey(%d) = %#v, want %#v", fTarget, targetKey, wantKey)
	}
	assertSummaryReturn(t, reg, result.Snapshot(), result.RootKey(), want)
	assertSummaryReturn(t, reg, result.Snapshot(), targetKey, want)
}

func TestRunChunkReexportsChainedWrapperNormalReturnParam(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local requireValue = function(x: string?)
	assert(x)
end
local requireAgain = function(x: string?)
	requireValue(x)
end
`)
	firstLocal := stmts[0].(*ast.LocalAssignStmt)
	secondLocal := stmts[1].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	requireValue := mustBoundLocalAt(t, bindings, firstLocal, 0)
	requireAgain := mustBoundLocalAt(t, bindings, secondLocal, 0)

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	valueKey, ok := result.TargetKey(requireValue)
	if !ok {
		t.Fatalf("TargetKey(requireValue) missing")
	}
	againKey, ok := result.TargetKey(requireAgain)
	if !ok {
		t.Fatalf("TargetKey(requireAgain) missing")
	}
	assertSummaryNormalReturnParam(t, reg, result.Snapshot(), valueKey, 0, presence.Present(), runtimekind.Singleton(runtimekind.String))
	assertSummaryNormalReturnParam(t, reg, result.Snapshot(), againKey, 0, presence.Present(), runtimekind.Singleton(runtimekind.String))
}

func TestRunChunkSpecializesGenericSummaryReturnAtCallSite(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return ok(fn(result.value))
	end
	return err(result.error)
end

local mapped = map_result(ok("x"), function(item: string): number
	return 1
end)
return mapped
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	got, ok := result.Snapshot().Read(result.RootKey())
	if !ok || len(got.Returns) != 1 {
		t.Fatalf("root summary = %#v/%v, want one return", got, ok)
	}
	witness := product.Get(reg, got.Returns[0], typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || typ.IsAny(gotType) || typ.IsUnknown(gotType) {
		t.Fatalf("mapped return witness = %#v, want concrete Result<number>", witness)
	}
	if refinement.ContainsFreeTypeParam(gotType) {
		t.Fatalf("mapped return type = %v, want no free type params", gotType)
	}
}

func TestRunChunkMaterializesGenericMapBindResultLocals(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }
type Profile = { id: string, count: number }

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function profile(id: string, count: number): Profile
	return { id = id, count = count }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return ok(fn(result.value))
	end
	return err(result.error)
end

local function bind_result<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
	if result.ok then
		return fn(result.value)
	end
	return err(result.error)
end

local mapped = map_result(ok(profile("abc", 41)), function(item: Profile): string
	return item.id
end)

if mapped.ok then
	local x: string = mapped.value
end

local bound = bind_result(ok(profile("def", 41)), function(item: Profile): Result<number>
	return ok(item.count + 1)
end)

if bound.ok then
	local y: number = bound.value
end
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}

	mappedStmt := mustFindLocalAssign(t, stmts, "mapped")
	boundStmt := mustFindLocalAssign(t, stmts, "bound")
	xStmt := mustFindLocalAssign(t, stmts, "x")
	yStmt := mustFindLocalAssign(t, stmts, "y")

	mappedPoint := requireLocalAssignmentPoint(t, root, mappedStmt, 0)
	boundPoint := requireLocalAssignmentPoint(t, root, boundStmt, 0)
	xPoint := requireLocalAssignmentPoint(t, root, xStmt, 0)
	yPoint := requireLocalAssignmentPoint(t, root, yStmt, 0)

	assertBoundarySymbolWitnessClosed(t, reg, root, mappedPoint, mustResultLocalAt(t, root, mappedStmt, 0), "mapped")
	assertBoundarySymbolWitnessClosed(t, reg, root, boundPoint, mustResultLocalAt(t, root, boundStmt, 0), "bound")
	assertBoundaryExprRuntimeKind(t, reg, root, xPoint, xStmt.Exprs[0], runtimekind.Singleton(runtimekind.String), "mapped.value")
	assertBoundaryExprRuntimeKind(t, reg, root, yPoint, yStmt.Exprs[0], runtimekind.Singleton(runtimekind.Number), "bound.value")
}

func TestRunChunkMaterializesGenericPairMultipleReturns(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function pair<A, B>(a: A, b: B): (A, B)
	return a, b
end
local n, s = pair(42, "hello")
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	pairStmt := mustFindLocalAssign(t, stmts, "n")
	nPoint := requireLocalAssignmentPoint(t, root, pairStmt, 0)
	sPoint := requireLocalAssignmentPoint(t, root, pairStmt, 1)

	assertBoundarySymbolType(t, reg, root, nPoint, mustResultLocalAt(t, root, pairStmt, 0), typ.LiteralInt(42), "n")
	assertBoundarySymbolType(t, reg, root, sPoint, mustResultLocalAt(t, root, pairStmt, 1), typ.LiteralString("hello"), "s")
}

func TestRunChunkUsesExactConfiguredRootKey(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, "return 1")
	rootKey := summary.SummaryKey{
		Ref:   ref.FuncRef{Kind: ref.KindRoot, ID: 42},
		Entry: summary.EntryKey{Values: 1, Facts: 2, References: 3},
	}

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:        reg,
			ExpressionValue: fixedExpressionValue(want),
		},
		RootKey: rootKey,
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}

	assertSummaryReturn(t, reg, result.Snapshot(), rootKey, want)
	if got, ok := result.Snapshot().Read(summary.DefaultSummaryKey(ref.Root())); ok {
		t.Fatalf("default root summary = %#v, want missing exact key", got)
	}
}

func fixedExpressionValue(value product.Value) func(cfg.Point, factflow.ExprRef, factflow.ValueSource, state.State) (product.Value, bool) {
	return func(cfg.Point, factflow.ExprRef, factflow.ValueSource, state.State) (product.Value, bool) {
		return value, true
	}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "fixpoint_program_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func onlyFunctionOrigin(t *testing.T, bindings *bind.Result) bind.FunctionOrigin {
	t.Helper()
	origins := bindings.FunctionOrigins()
	if len(origins) != 1 {
		t.Fatalf("FunctionOrigins length = %d, want 1: %#v", len(origins), origins)
	}
	return origins[0]
}

func mustBoundLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := bindings.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("bound local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("bound local symbol at %d is zero", index)
	}
	return locals[index]
}

func mustResultLocalAt(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := result.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("result local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("result local symbol at %d is zero", index)
	}
	return locals[index]
}

func mustFindLocalAssign(t *testing.T, stmts []ast.Stmt, name string) *ast.LocalAssignStmt {
	t.Helper()
	if stmt := findLocalAssign(stmts, name); stmt != nil {
		return stmt
	}
	t.Fatalf("local assignment for %q not found", name)
	return nil
}

func findLocalAssign(stmts []ast.Stmt, name string) *ast.LocalAssignStmt {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.LocalAssignStmt:
			for _, got := range s.Names {
				if got == name {
					return s
				}
			}
		case *ast.IfStmt:
			if found := findLocalAssign(s.Then, name); found != nil {
				return found
			}
			if found := findLocalAssign(s.Else, name); found != nil {
				return found
			}
		case *ast.DoBlockStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.WhileStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.RepeatStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.NumberForStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.GenericForStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		}
	}
	return nil
}

func requireLocalAssignmentPoint(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) cfg.Point {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("result graph missing")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point
		}
	}
	t.Fatalf("local assignment point for %v[%d] not found", stmt.Names, index)
	return 0
}

func assertBoundarySymbolWitnessClosed(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	id symbol.ID,
	name string,
) {
	t.Helper()
	value, ok := result.SymbolValueAtBoundary(point, id)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", name, point)
	}
	gotType, ok := structuralTypeFromBoundaryValue(reg, value)
	if !ok || typ.IsAny(gotType) || typ.IsUnknown(gotType) {
		t.Fatalf("%s boundary value = %#v, want concrete instantiated result evidence", name, value)
	}
	if refinement.ContainsFreeTypeParam(gotType) {
		t.Fatalf("%s structural type = %v, want no free type params", name, gotType)
	}
}

func structuralTypeFromBoundaryValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			return t, true
		}
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return discriminant.TypeFromOrigin(origin.Family(), origin.Cases())
}

func assertBoundaryExprRuntimeKind(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	expr ast.Expr,
	want runtimekind.Value,
	label string,
) {
	t.Helper()
	value, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", label, point)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, want) {
		t.Fatalf("%s runtime kind = %s, want %s (value %#v)", label, got, want, value)
	}
}

func assertBoundarySymbolRuntimeKind(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	id symbol.ID,
	want runtimekind.Value,
	label string,
) {
	t.Helper()
	value, ok := result.SymbolValueAtBoundary(point, id)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", label, point)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, want) {
		t.Fatalf("%s runtime kind = %s, want %s (value %#v)", label, got, want, value)
	}
}

func assertBoundarySymbolType(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	id symbol.ID,
	want typ.Type,
	label string,
) {
	t.Helper()
	value, ok := result.SymbolValueAtBoundary(point, id)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", label, point)
	}
	gotType, typeOK := structuralTypeFromBoundaryValue(reg, value)
	if !typeOK || !typ.TypeEquals(gotType, want) {
		t.Fatalf("%s structural type = %v, want %v (value %#v)", label, gotType, want, value)
	}
}

func assertSummaryReturn(t *testing.T, reg *axis.Registry, snapshot summary.Snapshot, key summary.SummaryKey, want product.Value) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	if len(got.Returns) != 1 {
		t.Fatalf("summary %s returns = %d, want 1: %#v", key.Ref, len(got.Returns), got)
	}
	if !product.Equal(reg, got.Returns[0], want) {
		t.Fatalf("summary %s return = %v, want %v", key.Ref, got.Returns[0], want)
	}
}

func assertSummaryNormalReturnParam(
	t *testing.T,
	reg *axis.Registry,
	snapshot summary.Snapshot,
	key summary.SummaryKey,
	index int,
	wantPresence presence.Value,
	wantKind runtimekind.Value,
) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	if len(got.NormalReturnParams) <= index {
		t.Fatalf("summary %s normal return params = %d, want index %d: %#v", key.Ref, len(got.NormalReturnParams), index, got)
	}
	value := got.NormalReturnParams[index]
	if gotPresence := product.PresenceOf(value); !presence.Equal(gotPresence, wantPresence) {
		t.Fatalf("summary %s param %d presence = %s, want %s", key.Ref, index, gotPresence, wantPresence)
	}
	if gotKind := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(gotKind, wantKind) {
		t.Fatalf("summary %s param %d runtime kind = %s, want %s", key.Ref, index, gotKind, wantKind)
	}
}
