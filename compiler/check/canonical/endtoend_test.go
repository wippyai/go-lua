package canonical_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// These tests drive the WHOLE canonical intraprocedural solver end to end on
// real Lua functions: input.BuildFromFunction assembles the inputs, transfer.New
// builds the real per-node transfer, and equation.NewBuilder runs the single
// generic worklist over the combined point/contract graph. No legacy flow is
// involved.

// solveFn is the end-to-end wiring under test: real inputs + real transfer +
// equation builder over the generic solver.
func solveFn(t *testing.T, params []string, paramTypes []ast.TypeExpr, resolve input.TypeResolver, body string, globals ...string) (state.FunctionState, *cfg.Graph) {
	t.Helper()
	stmts, err := parse.ParseString(body, "canonical.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: params, Types: paramTypes},
		Stmts:   stmts,
	}
	in := input.BuildFromFunction(fn, resolve, nil, globals...)
	if in.Graph == nil {
		t.Fatal("input builder produced no graph")
	}
	tr := transfer.New(in, transfer.Config{})
	fs := equation.NewBuilder(in.Graph, in.Scope.NumParams(), tr).Solve()
	return fs, in.Graph
}

// TestCanonical_DeadlockLoopTerminates is gate (a), the key result: a counting
// loop whose counter grows each iteration deadlocks the legacy runSCC (which has
// no data-axis widening). The canonical solver TERMINATES because the numeric
// domain's Cousot widen fires at the loop-header feedback-vertex set, cutting the
// strictly ascending counter bound to unconstrained. The -timeout on the test
// process is the only backstop; convergence is by design, not a cap.
func TestCanonical_DeadlockLoopTerminates(t *testing.T) {
	// Faithful reduction of the deadlock-dataflow-node counting loops
	// (existing_count = existing_count + 1 inside `for _ in pairs(...)`): a
	// counter that ascends without bound across the back-edge.
	const body = `
local count = 0
while items do
	count = count + 1
end
return count
`
	stmts, err := parse.ParseString(body, "canonical.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"items"}}, Stmts: stmts}
	in := input.BuildFromFunction(fn, nil, nil)
	g := in.Graph
	real := transfer.New(in, transfer.Config{})

	// The loop must have a feedback-vertex set; its header cell is the widening
	// site that makes the ascending counter converge.
	fvs := propagate.FeedbackVertexSet(g)
	if len(fvs) == 0 {
		t.Fatal("while loop must have a non-empty feedback-vertex set")
	}

	countSym, ok := g.SymbolAt(g.Exit(), "count")
	if !ok || countSym == 0 {
		t.Fatal("expected a symbol for count")
	}
	countKey := constraint.PathKey(symbolKey(countSym))

	// Observe the largest counter upper bound arriving at any point. Under pure
	// Join (the widen-free legacy runSCC) this ascends without bound and never
	// converges; the canonical solver converges because WidenAt fires at the
	// loop-header feedback-vertex set. A max above the initial step proves the
	// chain genuinely ascended (the deadlock precondition), not that it
	// converged on its own.
	var maxIncoming int64
	observe := equation.NodeTransferFunc(func(
		g *cfg.Graph, p cfg.Point, incoming flow.PointState,
		entryContracts paramevidence.Contracts, demand func(int, paramevidence.ParamContract),
	) flow.PointState {
		if incoming.Num != nil {
			if _, upper, bounded := incoming.Num.BoundsFor(countKey); bounded && upper > maxIncoming {
				maxIncoming = upper
			}
		}
		return real.Transfer(g, p, incoming, entryContracts, demand)
	})

	// Solve must terminate; the -timeout backstops a genuine non-termination
	// regression.
	fs := equation.NewBuilder(g, in.Scope.NumParams(), observe).Solve()

	if maxIncoming < 2 {
		t.Fatalf("counter did not ascend (max incoming upper=%d); the deadlock precondition is not exercised", maxIncoming)
	}

	// Widening fired: no loop-header FVS cell retains a finite counter upper
	// bound. A finite bound would mean an iteration cap rather than termination
	// by widening — the unsoundness this rebuild removes.
	for p := range fvs {
		ps, ok := fs.Points[p]
		if !ok || ps.Num == nil {
			continue
		}
		// upper == MaxInt64 is the unbounded-above (+inf) sentinel the interval
		// widen produces — that IS widening firing. Only a genuinely finite bound
		// (< MaxInt64) signals an iteration cap, the unsoundness this rebuild removes.
		if _, upper, bounded := ps.Num.BoundsFor(countKey); bounded && upper != math.MaxInt64 {
			t.Fatalf("loop-header FVS point %v retained a finite counter bound (upper=%d); widening did not fire", p, upper)
		}
	}
}

// TestCanonical_StraightLineSoundValue is gate (b), value half: a local assigned
// a literal carries that literal's value at the exit point.
func TestCanonical_StraightLineSoundValue(t *testing.T) {
	fs, g := solveFn(t, nil, nil, nil, `
local x = 5
local y = "hi"
`)

	exit := g.Exit()
	ps, ok := fs.Points[exit]
	if !ok {
		t.Fatalf("exit point %v missing from result", exit)
	}

	symX := mustSymbol(t, g, "x")
	av, ok := ps.Env[symbolKey(symX)]
	if !ok {
		t.Fatalf("local x has no value at exit; env=%v", ps.Env)
	}
	got := av.ProjectValue()
	lit, isLit := got.(*typ.Literal)
	if !isLit || lit.Base != kind.Integer {
		t.Fatalf("local x = 5 must infer an integer literal; got %v", got)
	}
	if v, isInt := lit.Value.(int64); !isInt || v != 5 {
		t.Fatalf("local x = 5 must infer literal 5; got %v", got)
	}

	symY := mustSymbol(t, g, "y")
	avY, ok := ps.Env[symbolKey(symY)]
	if !ok {
		t.Fatalf("local y has no value at exit; env=%v", ps.Env)
	}
	gotY := avY.ProjectValue()
	litY, isLitY := gotY.(*typ.Literal)
	if !isLitY || litY.Base != kind.String {
		t.Fatalf("local y = \"hi\" must infer a string literal; got %v", gotY)
	}
}

// TestCanonical_BodyUseDemandsParameter is gate (b), contract half: a parameter
// read in the body shows up in FunctionState.Contracts. The body use pins the
// parameter; the backward demand component carries the obligation to entry.
func TestCanonical_BodyUseDemandsParameter(t *testing.T) {
	// `local n = p + 1` reads p as an arithmetic operand: a body use that
	// constrains parameter 0.
	fs, _ := solveFn(t, []string{"p"}, nil, nil, `
local n = p + 1
return n
`)

	if _, ok := fs.Contracts[0]; !ok {
		t.Fatalf("parameter 0 read in the body must produce a contract; contracts=%v", fs.Contracts)
	}
}

func TestCanonical_LoopAppendLengthRelationExportsToSummary(t *testing.T) {
	resolve := func(expr ast.TypeExpr, _ *scope.State) typ.Type {
		if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "Map" {
			return typ.NewMap(typ.String, typ.String)
		}
		return nil
	}
	fs, g := solveFn(t, []string{"xs"}, []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "Map"}}, resolve, `
local out = {}
for k, v in pairs(xs) do
	table.insert(out, v)
end
return out
	`, "table", "pairs")

	sum := summary.Project(fs, g)
	rel := flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 0}
	if !sum.Relations.HasLengthParam(rel) {
		t.Fatalf("summary relations = %#v, want loop-derived length relation %#v", sum.Relations, rel)
	}
}

func TestCanonical_LoopAppendLengthRelationRejectsStaleReturnedTarget(t *testing.T) {
	resolve := func(expr ast.TypeExpr, _ *scope.State) typ.Type {
		if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "Map" {
			return typ.NewMap(typ.String, typ.String)
		}
		return nil
	}
	fs, g := solveFn(t, []string{"xs"}, []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "Map"}}, resolve, `
local out = {}
for k, v in pairs(xs) do
	table.insert(out, v)
end
out = {}
return out
	`, "table", "pairs")

	sum := summary.Project(fs, g)
	rel := flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 0}
	if sum.Relations.HasLengthParam(rel) {
		t.Fatalf("summary relations exported stale loop-derived target: %#v", sum.Relations)
	}
}

func TestCanonical_GuardedFieldDemandExportsGuardImplication(t *testing.T) {
	fs, _ := solveFnWithTransferConfig(t, []string{"page"}, nil, nil, `
if not page or not page.data_func or page.data_func == "" then
	return {}, nil
end
takes_string(page.data_func)
return {}, nil
`, transfer.Config{CallTyper: staticDemandTyper{demand: typ.String}})

	got, ok := fs.Contracts[0]
	if !ok {
		t.Fatalf("guarded field use must produce a parameter contract; contracts=%v", fs.Contracts)
	}
	want := typ.NewRecord().
		ReadonlyField("data_func", typ.NewUnion(typ.String, typ.Nil, typ.False)).
		Build()
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("guarded demand = %v, want %v", got.ProjectValue(), want)
	}
}

func TestCanonical_GuardedFieldDemandSurvivesTypedLocalRead(t *testing.T) {
	resolve := func(expr ast.TypeExpr, _ *scope.State) typ.Type {
		if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "string" {
			return typ.String
		}
		return nil
	}
	fs, _ := solveFnWithTransferConfig(t, []string{"page"}, nil, resolve, `
if not page or not page.data_func or page.data_func == "" then
	return {}, nil
end
local name: string = page.data_func
takes_string(page.data_func)
return {}, nil
`, transfer.Config{CallTyper: staticDemandTyper{demand: typ.String}})

	got, ok := fs.Contracts[0]
	if !ok {
		t.Fatalf("guarded typed local read must preserve parameter contract; contracts=%v", fs.Contracts)
	}
	want := typ.NewRecord().
		ReadonlyField("data_func", typ.NewUnion(typ.String, typ.Nil, typ.False, typ.LiteralString(""))).
		Build()
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("guarded typed-local demand = %v, want %v", got.ProjectValue(), want)
	}
}

func TestCanonical_GuardedFieldDemandWithContradictoryEntryValueEmitsNoDemand(t *testing.T) {
	resolve := func(expr ast.TypeExpr, _ *scope.State) typ.Type {
		if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "string" {
			return typ.String
		}
		return nil
	}
	stmts, err := parse.ParseString(`
if not page or not page.data_func or page.data_func == "" then
	return {}, nil
end
local name: string = page.data_func
takes_string(page.data_func)
return {}, nil
`, "canonical.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"page"}},
		Stmts:   stmts,
	}
	in := input.BuildFromFunction(fn, resolve, nil, "takes_string")
	tr := transfer.New(in, transfer.Config{CallTyper: staticDemandTyper{demand: typ.String}})
	entryValue := product.FromType(typ.NewRecord().
		Field("data_func", typ.NewUnion(typ.Nil, typ.False, typ.LiteralString(""))).
		Build())
	fs := equation.NewBuilder(in.Graph, in.Scope.NumParams(), tr).
		WithEntryValues(map[int]product.AbstractValue{0: entryValue}).
		Solve()

	if got, ok := fs.Contracts[0]; ok {
		t.Fatalf("contradictory exact entry made guarded call unreachable, but emitted demand %v", got.ProjectValue())
	}
}

func TestCanonical_ShortCircuitAndFalseEdgeUnionsSameValueLiteralRefinements(t *testing.T) {
	input := typ.NewUnion(typ.String, typ.Boolean, typ.Nil)
	resolve := func(expr ast.TypeExpr, _ *scope.State) typ.Type {
		if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "Input" {
			return input
		}
		return nil
	}
	fs, g := solveFn(t, []string{"raw"}, []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "Input"}}, resolve, `
local v = raw
if v and v ~= "" then
	v = "qualified"
end
return v
`)

	ps, ok := fs.Points[g.Exit()]
	if !ok {
		t.Fatalf("no exit state; points=%v", fs.Points)
	}
	vSym := mustSymbol(t, g, "v")
	av, ok := ps.Env[symbolKey(vSym)]
	if !ok {
		t.Fatalf("local v has no value at exit; env=%v", ps.Env)
	}
	want := typ.NewUnion(typ.Nil, typ.False, typ.LiteralString(""), typ.LiteralString("qualified"))
	if got := av.ProjectValue(); !typ.TypeEquals(got, want) {
		t.Fatalf("post short-circuit v = %v, want %v", got, want)
	}
}

func solveFnWithTransferConfig(t *testing.T, params []string, paramTypes []ast.TypeExpr, resolve input.TypeResolver, body string, config transfer.Config, globals ...string) (state.FunctionState, *cfg.Graph) {
	t.Helper()
	stmts, err := parse.ParseString(body, "canonical.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: params, Types: paramTypes},
		Stmts:   stmts,
	}
	in := input.BuildFromFunction(fn, resolve, nil, globals...)
	if in.Graph == nil {
		t.Fatal("input builder produced no graph")
	}
	tr := transfer.New(in, config)
	fs := equation.NewBuilder(in.Graph, in.Scope.NumParams(), tr).Solve()
	return fs, in.Graph
}

type staticDemandTyper struct {
	demand typ.Type
}

func (s staticDemandTyper) CallReturns(*ast.FuncCallExpr, []typ.Type, func(ast.Expr) typ.Type, flow.CaptureCells, flow.FunctionRefs) ([]typ.Type, bool) {
	return nil, false
}

func (s staticDemandTyper) IterVars(*ast.FuncCallExpr, int, func(ast.Expr) typ.Type) ([]typ.Type, bool) {
	return nil, false
}

func (s staticDemandTyper) KeyedIterSource(*ast.FuncCallExpr) (ast.Expr, bool) {
	return nil, false
}

func (s staticDemandTyper) IndexedIterSource(*ast.FuncCallExpr) (constraint.Path, bool) {
	return constraint.Path{}, false
}

func (s staticDemandTyper) KeysCollectorContainer(*cfg.CallInfo, int) (constraint.Path, bool) {
	return constraint.Path{}, false
}

func (s staticDemandTyper) ParamNarrows(*ast.FuncCallExpr) []transfer.ParamNarrow {
	return nil
}

func (s staticDemandTyper) IsNoReturn(*ast.FuncCallExpr, transfer.ProductCallContext) bool {
	return false
}

func (s staticDemandTyper) TypeCastTarget(*ast.FuncCallExpr, func(ast.Expr) typ.Type) (typ.Type, bool) {
	return nil, false
}

func (s staticDemandTyper) CallArgDemands(call *ast.FuncCallExpr, _ transfer.ProductCallContext) []callobligation.Obligation {
	if call == nil || len(call.Args) == 0 || s.demand == nil {
		return nil
	}
	out := make([]callobligation.Obligation, len(call.Args))
	for i := range out {
		out[i] = callobligation.Body(s.demand)
	}
	return out
}

// TestCanonical_DeclaredParamSeedsEntry confirms a declared parameter type seeds
// the entry env, so a body read of the parameter recovers its declared type. A
// plain read is not a body-proven caller obligation; the annotation is already
// the public contract.
func TestCanonical_DeclaredParamSeedsEntry(t *testing.T) {
	resolve := func(expr ast.TypeExpr, _ *scope.State) typ.Type {
		if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "number" {
			return typ.Number
		}
		return nil
	}
	fs, g := solveFn(t, []string{"p"}, []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}}, resolve, `
local n = p
return n
`)

	if _, ok := fs.Contracts[0]; ok {
		t.Fatalf("plain read of declared parameter must not create body contract; contracts=%v", fs.Contracts)
	}
	ps, ok := fs.Points[g.Exit()]
	if !ok {
		t.Fatalf("no exit state; points=%v", fs.Points)
	}
	symN := mustSymbol(t, g, "n")
	av, ok := ps.Env[symbolKey(symN)]
	if !ok {
		t.Fatalf("local n has no value at exit; env=%v", ps.Env)
	}
	if got := av.ProjectValue(); got != typ.Number {
		t.Fatalf("local n = p projected as %v, want number", got)
	}
}

func mustSymbol(t *testing.T, g *cfg.Graph, name string) cfg.SymbolID {
	t.Helper()
	sym, ok := g.SymbolAt(g.Exit(), name)
	if !ok || sym == 0 {
		t.Fatalf("expected symbol for %s", name)
	}
	return sym
}

func symbolKey(sym cfg.SymbolID) flow.ValueKey {
	return flow.SymbolValueKey(sym)
}
