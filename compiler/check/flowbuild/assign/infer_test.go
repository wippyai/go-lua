package assign

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCollectInferredTypes_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	result := CollectInferredTypes(fc, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectInferredTypes_EmptySpecTypes(t *testing.T) {
	fc := &core.FlowContext{}
	specTypes := make(api.SpecTypes)
	result := CollectInferredTypes(fc, specTypes, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectInferredTypes_WithAnnotated(t *testing.T) {
	fc := &core.FlowContext{}
	specTypes := make(api.SpecTypes)
	annotated := make(map[cfg.SymbolID]bool)
	annotated[1] = true
	result := CollectInferredTypes(fc, specTypes, annotated, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectInferredTypes_WithInputs(t *testing.T) {
	fc := &core.FlowContext{}
	specTypes := make(api.SpecTypes)
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
		AnnotatedVars: make(map[cfg.SymbolID]bool),
	}
	result := CollectInferredTypes(fc, specTypes, nil, inputs)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectExprSymbols_NilExpr(t *testing.T) {
	var refs []cfg.SymbolID
	collectExprSymbols(nil, nil, &refs)
	if len(refs) != 0 {
		t.Errorf("expected no refs for nil expr, got %d", len(refs))
	}
}

func TestCollectExprSymbols_NilBindings(t *testing.T) {
	var refs []cfg.SymbolID
	expr := &ast.IdentExpr{Value: "x"}
	collectExprSymbols(expr, nil, &refs)
	if len(refs) != 0 {
		t.Errorf("expected no refs for nil bindings, got %d", len(refs))
	}
}

func TestCollectExprSymbols_IdentExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.IdentExpr{Value: "x"}
	collectExprSymbols(expr, bindings, &refs)
	// BindingTable is empty, so no symbol should be found
}

func TestCollectExprSymbols_AttrGetExpr(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(301)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "obj")
	fieldSym := bindings.GetOrCreateFieldSymbol(baseSym, "field")

	var refs []cfg.SymbolID
	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "field"},
	}
	collectExprSymbols(expr, bindings, &refs)
	if !hasSymbol(refs, fieldSym) {
		t.Fatalf("expected refs to include field symbol %d, got %v", fieldSym, refs)
	}
	if !hasSymbol(refs, baseSym) {
		t.Fatalf("expected refs to include base symbol %d, got %v", baseSym, refs)
	}
}

func TestCollectExprSymbols_FuncCallExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.FuncCallExpr{
		Func:     &ast.IdentExpr{Value: "fn"},
		Receiver: &ast.IdentExpr{Value: "recv"},
		Args:     []ast.Expr{&ast.IdentExpr{Value: "arg"}},
	}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_TableExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "key"},
				Value: &ast.IdentExpr{Value: "val"},
			},
		},
	}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_UnaryExpressions(t *testing.T) {
	bindings := &bind.BindingTable{}

	tests := []struct {
		name string
		expr ast.Expr
	}{
		{"UnaryMinusOpExpr", &ast.UnaryMinusOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
		{"UnaryNotOpExpr", &ast.UnaryNotOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
		{"UnaryLenOpExpr", &ast.UnaryLenOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
		{"UnaryBNotOpExpr", &ast.UnaryBNotOpExpr{Expr: &ast.IdentExpr{Value: "x"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var refs []cfg.SymbolID
			collectExprSymbols(tt.expr, bindings, &refs)
		})
	}
}

func TestCollectExprSymbols_BinaryExpressions(t *testing.T) {
	bindings := &bind.BindingTable{}

	tests := []struct {
		name string
		expr ast.Expr
	}{
		{"ArithmeticOpExpr", &ast.ArithmeticOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
		{"RelationalOpExpr", &ast.RelationalOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
		{"LogicalOpExpr", &ast.LogicalOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
		{"StringConcatOpExpr", &ast.StringConcatOpExpr{Lhs: &ast.IdentExpr{Value: "a"}, Rhs: &ast.IdentExpr{Value: "b"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var refs []cfg.SymbolID
			collectExprSymbols(tt.expr, bindings, &refs)
		})
	}
}

func TestCollectExprSymbols_CastExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.CastExpr{Expr: &ast.IdentExpr{Value: "x"}}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_NonNilAssertExpr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.NonNilAssertExpr{Expr: &ast.IdentExpr{Value: "x"}}
	collectExprSymbols(expr, bindings, &refs)
}

func TestCollectExprSymbols_Comma3Expr(t *testing.T) {
	bindings := &bind.BindingTable{}
	var refs []cfg.SymbolID
	expr := &ast.Comma3Expr{}
	collectExprSymbols(expr, bindings, &refs)
	if len(refs) != 0 {
		t.Errorf("expected no refs for Comma3Expr, got %d", len(refs))
	}
}

func TestJoinInferredType_StabilizesSelfEmbeddingFromUnknown(t *testing.T) {
	old := typ.Unknown
	next := typ.NewArray(typ.Unknown)

	got := joinInferredType(old, next, old)
	if !typ.TypeEquals(got, next) {
		t.Fatalf("joinInferredType(unknown, any[]) = %v, want %v", got, next)
	}
}

func TestJoinInferredType_StopsRecursiveNestingGrowth(t *testing.T) {
	old := typ.NewArray(typ.Unknown)
	next := typ.NewArray(old)

	got := joinInferredType(old, next, old)
	if !typ.TypeEquals(got, old) {
		t.Fatalf("joinInferredType(any[], any[][]) = %v, want %v", got, old)
	}
}

// sharedLadder builds a type DAG of the given depth in which every level is a
// union of two records that both point at the previous level. The DAG holds
// 3*depth nodes while its tree expansion holds 2^depth paths, the shape that
// SCC inference produces for a local reassigned as `v = { key = v }` in
// several branches.
func sharedLadder(depth int, leaf typ.Type) typ.Type {
	level := leaf
	for i := 0; i < depth; i++ {
		level = typ.NewUnion(
			typ.NewRecord().Field("left", level).Build(),
			typ.NewRecord().Field("right", level).Build(),
		)
	}
	return level
}

func typeContainsWithin(t *testing.T, bound time.Duration, haystack, needle typ.Type) bool {
	t.Helper()
	done := make(chan bool, 1)
	go func() { done <- typeContains(haystack, needle) }()
	select {
	case got := <-done:
		return got
	case <-time.After(bound):
		t.Fatalf("typeContains did not finish within %v on a %T haystack", bound, haystack)
		return false
	}
}

func TestTypeContains_VisitsSharedSubstructureOnce(t *testing.T) {
	leaf := typ.NewRecord().Field("leaf", typ.String).Build()
	haystack := sharedLadder(60, leaf)

	if typeContainsWithin(t, 10*time.Second, haystack, typ.Boolean) {
		t.Fatal("expected boolean to be absent from the ladder")
	}
	if !typeContainsWithin(t, 10*time.Second, haystack, leaf) {
		t.Fatal("expected the leaf record to be found at the bottom of the ladder")
	}
}

func TestTypeContains_FindsNeedleBelowDefaultRecursionDepth(t *testing.T) {
	leaf := typ.NewRecord().Field("leaf", typ.String).Build()
	var nested typ.Type = leaf
	for i := 0; i < 2*typ.DefaultRecursionDepth; i++ {
		nested = typ.NewArray(nested)
	}
	if !typeContainsWithin(t, 10*time.Second, nested, leaf) {
		t.Fatal("expected the leaf record to be found under deep array nesting")
	}
}

// inferenceRound applies one SCC round of `if value then value = { key = value } end`
// for every key: each assignment embeds the truthy narrowing of the round's
// starting type, and the joins accumulate into the symbol's type.
func inferenceRound(start typ.Type, keys ...string) typ.Type {
	embedded := narrow.ToTruthy(start)
	out := start
	for _, key := range keys {
		out = joinInferredType(out, typ.NewRecord().OptField(key, embedded).Build(), start)
	}
	return out
}

func TestJoinInferredType_FoldsTruthyNarrowedSelfEmbedding(t *testing.T) {
	brief := typ.NewRecord().Field("brief", typ.String).Build()
	tasks := typ.NewRecord().Field("tasks", typ.Number).Build()
	start := typ.NewUnion(typ.Nil, brief, tasks)

	got := inferenceRound(start, "brief", "tasks")

	want := typ.NewRecursive(inferredSelfName, func(self typ.Type) typ.Type {
		return typ.NewUnion(
			typ.Nil,
			brief,
			tasks,
			typ.NewRecord().OptField("brief", self).Build(),
			typ.NewRecord().OptField("tasks", self).Build(),
		)
	})
	if !typ.TypeEquals(got, want) {
		t.Fatalf("round 1 = %v, want %v", got, want)
	}

	again := inferenceRound(got, "brief", "tasks")
	if !typ.TypeEquals(again, got) {
		t.Fatalf("round 2 = %v, want the round 1 fixpoint %v", again, got)
	}
}

// A scalar is never an approximation of the symbol: joining it keeps the
// record, and the chain folds once the symbol's type holds a table.
func TestJoinInferredType_JoinsSelfEmbeddedScalar(t *testing.T) {
	next := typ.NewRecord().Field("count", typ.Number).Build()

	got := joinInferredType(typ.Number, next, typ.Number)

	want := typ.NewUnion(typ.Number, next)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("joinInferredType(number, {count: number}) = %v, want %v", got, want)
	}
}

func TestTypeContains(t *testing.T) {
	base := typ.NewArray(typ.Unknown)
	outer := typ.NewArray(base)
	if !typeContains(outer, base) {
		t.Fatal("expected typeContains(any[][], any[]) to be true")
	}
	if typeContains(typ.Number, base) {
		t.Fatal("expected typeContains(number, any[]) to be false")
	}
}

func TestMergeSpecTypesSoft_IgnoresUnknownAndNilOverrides(t *testing.T) {
	sym := cfg.SymbolID(1)
	base := api.SpecTypes{
		sym: typ.NewOptional(typ.LuaError),
	}
	override := api.SpecTypes{
		sym: typ.Nil,
	}
	merged := mergeSpecTypesSoft(base, override)
	got, ok := merged[sym]
	if !ok || got == nil {
		t.Fatalf("expected merged type for symbol %d", sym)
	}
	if !typ.TypeEquals(got, base[sym]) {
		t.Fatalf("merged type = %v, want %v", got, base[sym])
	}
}

func TestCollectInferredTypes_UsesModuleCalleeCandidatesForExpectedArgs(t *testing.T) {
	body, err := parse.ParseString(`external_fn(x)`, "infer_module_candidates.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts:   body,
	}
	bindings := bind.Bind(fn, []string{"external_fn"})
	graph := cfg.BuildWithBindings(fn, bindings)
	if graph == nil {
		t.Fatal("expected graph")
	}

	paramSyms := graph.ParamSymbols()
	if len(paramSyms) != 1 || paramSyms[0] == 0 {
		t.Fatalf("expected one parameter symbol, got %v", paramSyms)
	}
	xSym := paramSyms[0]

	var globalCalleeSym cfg.SymbolID
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if globalCalleeSym == 0 && info != nil && info.CalleeName == "external_fn" {
			globalCalleeSym = info.CalleeSymbol
		}
	})
	if globalCalleeSym == 0 {
		t.Fatal("expected callsite callee symbol for external_fn")
	}

	moduleBindings := bind.NewBindingTable()
	const moduleCalleeSym cfg.SymbolID = 9001
	moduleBindings.SetName(moduleCalleeSym, "external_fn")

	inferred := collectInferredTypes(
		graph,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return typ.Unknown },
		nil,
		func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
			if sym == globalCalleeSym {
				return nil, false
			}
			if sym == moduleCalleeSym {
				return typ.Func().Param("n", typ.Number).Returns(typ.Nil).Build(), true
			}
			return nil, false
		},
		nil,
		nil,
		nil,
		moduleBindings,
		db.NewQueryContext(db.New()),
		querycore.NewEngine(),
		nil,
		nil,
	)

	got := inferred[xSym]
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("inferred param type = %v, want number", got)
	}
}

func TestNormalizedCallArgSymbols_UsesBindingsFallback(t *testing.T) {
	bindings := bind.NewBindingTable()
	x := &ast.IdentExpr{Value: "x"}
	y := &ast.IdentExpr{Value: "y"}
	xSym := cfg.SymbolID(11)
	ySym := cfg.SymbolID(12)
	bindings.Bind(x, xSym)
	bindings.Bind(y, ySym)

	info := &cfg.CallInfo{
		Args:       []ast.Expr{x, y},
		ArgSymbols: []cfg.SymbolID{0, 42},
	}
	got := normalizedCallArgSymbols(info, bindings)
	if len(got) != 2 {
		t.Fatalf("expected 2 arg symbols, got %d", len(got))
	}
	if got[0] != xSym {
		t.Fatalf("expected first arg symbol %d from bindings, got %d", xSym, got[0])
	}
	if got[1] != 42 {
		t.Fatalf("expected second arg symbol to preserve explicit symbol 42, got %d", got[1])
	}
}

func TestCallRefSymbols_CollectsAndDeduplicates(t *testing.T) {
	bindings := bind.NewBindingTable()
	callee := &ast.IdentExpr{Value: "f"}
	recv := &ast.IdentExpr{Value: "recv"}
	arg := &ast.IdentExpr{Value: "x"}
	obj := &ast.IdentExpr{Value: "obj"}

	calleeSym := cfg.SymbolID(101)
	recvSym := cfg.SymbolID(102)
	argSym := cfg.SymbolID(103)
	objSym := cfg.SymbolID(104)
	bindings.Bind(callee, calleeSym)
	bindings.Bind(recv, recvSym)
	bindings.Bind(arg, argSym)
	bindings.Bind(obj, objSym)
	fieldSym := bindings.GetOrCreateFieldSymbol(objSym, "k")

	attr := &ast.AttrGetExpr{
		Object: obj,
		Key:    &ast.StringExpr{Value: "k"},
	}
	info := &cfg.CallInfo{
		Callee:   callee,
		Receiver: recv,
		Args:     []ast.Expr{arg, attr, arg},
	}

	refs := callRefSymbols(info, bindings)
	if !hasSymbol(refs, calleeSym) || !hasSymbol(refs, recvSym) || !hasSymbol(refs, argSym) || !hasSymbol(refs, objSym) || !hasSymbol(refs, fieldSym) {
		t.Fatalf("expected refs to include callee/receiver/arg/object/field symbols, got %v", refs)
	}
}

func TestSynthWithInferenceOverlay_PriorityAndParamFallback(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "a"}
	aSym := cfg.SymbolID(201)
	bindings.Bind(ident, aSym)

	base := func(ast.Expr, cfg.Point) typ.Type { return typ.Boolean }
	paramSet := map[cfg.SymbolID]bool{aSym: true}

	synth := synthWithInferenceOverlay(
		nil,
		map[cfg.SymbolID]typ.Type{aSym: typ.String},
		map[cfg.SymbolID]typ.Type{aSym: typ.Number},
		paramSet,
		nil,
		bindings,
		nil,
		nil,
		nil,
		nil,
		base,
	)
	if got := synth(ident, 0); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected overlay type string, got %v", got)
	}

	synth = synthWithInferenceOverlay(
		nil,
		nil,
		map[cfg.SymbolID]typ.Type{aSym: typ.Number},
		paramSet,
		nil,
		bindings,
		nil,
		nil,
		nil,
		nil,
		base,
	)
	if got := synth(ident, 0); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("expected function signature type number, got %v", got)
	}

	synth = synthWithInferenceOverlay(
		nil,
		nil,
		nil,
		paramSet,
		nil,
		bindings,
		nil,
		nil,
		nil,
		nil,
		base,
	)
	if got := synth(ident, 0); !typ.TypeEquals(got, typ.Unknown) {
		t.Fatalf("expected unknown fallback for unannotated param, got %v", got)
	}

	synth = synthWithInferenceOverlay(
		nil,
		nil,
		nil,
		paramSet,
		map[cfg.SymbolID]bool{aSym: true},
		bindings,
		nil,
		nil,
		nil,
		nil,
		base,
	)
	if got := synth(ident, 0); !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("expected base type for annotated param, got %v", got)
	}
}

func TestSynthWithInferenceOverlay_PreservesNilOverlayEntries(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "a"}
	aSym := cfg.SymbolID(301)
	bindings.Bind(ident, aSym)

	baseCalled := false
	synth := synthWithInferenceOverlay(
		nil,
		map[cfg.SymbolID]typ.Type{aSym: nil},
		nil,
		nil,
		nil,
		bindings,
		nil,
		nil,
		nil,
		nil,
		func(ast.Expr, cfg.Point) typ.Type {
			baseCalled = true
			return typ.Boolean
		},
	)
	if got := synth(ident, 0); got != nil {
		t.Fatalf("expected nil type from explicit nil overlay, got %v", got)
	}
	if baseCalled {
		t.Fatalf("expected base synth not to be called when overlay entry exists")
	}
}

func hasSymbol(refs []cfg.SymbolID, sym cfg.SymbolID) bool {
	for _, r := range refs {
		if r == sym {
			return true
		}
	}
	return false
}

func TestJoinMembers_RecursiveUnionListsItselfOnce(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("R")
	leaf := typ.NewRecord().Field("leaf", typ.String).Build()
	rec.SetBody(typ.NewUnion(leaf, typ.NewRecord().Field("next", rec).Build(), rec))

	members := joinMembers(rec)
	if len(members) != 3 {
		t.Fatalf("expected leaf, node and the recursive self reference, got %v", members)
	}
}
