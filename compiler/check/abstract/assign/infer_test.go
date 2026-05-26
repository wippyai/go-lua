package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferLocalTypes_NilGraph(t *testing.T) {
	result := InferLocalTypes(LocalInferenceConfig{})
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestInferLocalTypes_EmptySpecTypes(t *testing.T) {
	specTypes := make(api.SpecTypes)
	result := InferLocalTypes(LocalInferenceConfig{SeedTypes: specTypes})
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestInferLocalTypes_WithAnnotated(t *testing.T) {
	specTypes := make(api.SpecTypes)
	annotated := make(map[cfg.SymbolID]bool)
	annotated[1] = true
	result := InferLocalTypes(LocalInferenceConfig{
		SeedTypes: specTypes,
		Inputs: &flow.Inputs{
			AnnotatedVars: annotated,
		},
	})
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestInferLocalTypes_WithInputs(t *testing.T) {
	specTypes := make(api.SpecTypes)
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
		AnnotatedVars: make(map[cfg.SymbolID]bool),
	}
	result := InferLocalTypes(LocalInferenceConfig{SeedTypes: specTypes, Inputs: inputs})
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestFunctionSignatureSeed_PrefersLiteralProductProjection(t *testing.T) {
	sym := cfg.SymbolID(42)
	fn := &ast.FunctionExpr{}
	preciseOverlay := map[string]typ.Type{
		"up": typ.Func().Param("cb", typ.Func().Param("value", typ.Number).Build()).Build(),
	}
	staleOverlay := map[string]typ.Type{
		"up": typ.Unknown,
	}
	precise := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Spec(contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(preciseOverlay))).
		Build()
	stale := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Spec(contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(staleOverlay))).
		Build()

	got := functionSignatureSeed(signatureSeedInput{
		inputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{sym: stale},
			LiteralTypes:  map[cfg.SymbolID]typ.Type{sym: precise},
		},
		services: abstractcore.FlowServicesFuncs{
			FnSigResolver: func(*ast.FunctionExpr, *scope.State) *typ.Function {
				return stale
			},
		},
		sym: sym,
		fn:  fn,
	})

	if !typ.TypeEquals(got, precise) {
		t.Fatalf("functionSignatureSeed() = %v, want literal FunctionFact projection %v", got, precise)
	}
}

func TestFunctionSignatureSeed_LiteralProjectionCarriesAnnotatedFunctionFacts(t *testing.T) {
	sym := cfg.SymbolID(43)
	declared := typ.Func().Param("value", typ.String).Build()
	literal := typ.Func().Param("value", typ.Number).Build()

	got := functionSignatureSeed(signatureSeedInput{
		inputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{sym: declared},
			LiteralTypes:  map[cfg.SymbolID]typ.Type{sym: literal},
			AnnotatedVars: map[cfg.SymbolID]bool{sym: true},
		},
		services: abstractcore.FlowServicesFuncs{
			FnSigResolver: func(*ast.FunctionExpr, *scope.State) *typ.Function {
				return literal
			},
		},
		sym: sym,
		fn:  &ast.FunctionExpr{},
	})

	if !typ.TypeEquals(got, literal) {
		t.Fatalf("functionSignatureSeed() = %v, want canonical literal projection %v", got, literal)
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

	got := joinInferredType(old, next)
	if !typ.TypeEquals(got, next) {
		t.Fatalf("joinInferredType(unknown, any[]) = %v, want %v", got, next)
	}
}

func TestJoinInferredType_StopsRecursiveNestingGrowth(t *testing.T) {
	old := typ.NewArray(typ.Unknown)
	next := typ.NewArray(old)

	got := joinInferredType(old, next)
	if !typ.TypeEquals(got, old) {
		t.Fatalf("joinInferredType(any[], any[][]) = %v, want %v", got, old)
	}
}

func TestJoinInferredType_TreatsAnyAsTop(t *testing.T) {
	suite := typ.NewRecord().Field("name", typ.String).Build()

	got := joinInferredType(suite, typ.Any)
	if !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("joinInferredType(Suite, any) = %v, want any", got)
	}
}

func TestJoinInferredType_IsMonotoneForUnionCandidate(t *testing.T) {
	candidate := typ.NewUnion(typ.String, typ.Number)
	got := joinInferredType(typ.String, candidate)
	if !typ.TypeEquals(got, candidate) {
		t.Fatalf("joinInferredType(string, string|number) = %v, want %v", got, candidate)
	}
}

func TestLocalInferenceStability_UsesConvergenceEqualityForRecursiveProducts(t *testing.T) {
	left := recursiveBuilderProduct()
	right := recursiveBuilderProduct()
	sym := cfg.SymbolID(77)

	if !localInferenceValueEqual(left, right) {
		t.Fatalf("recursive products from same value-domain family should be stable")
	}
	if !sccTypesStable([]typ.Type{left}, api.SpecTypes{sym: right}, []cfg.SymbolID{sym}) {
		t.Fatalf("SCC stability must use value-domain convergence equality")
	}
}

func recursiveBuilderProduct() typ.Type {
	return typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.Number).
			Field("add", typ.Func().
				Param("self", self).
				Param("n", typ.Number).
				Returns(self).
				Build()).
			Field("result", typ.Func().
				Param("self", self).
				Returns(typ.Number).
				Build()).
			Build()
	})
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

func TestInferLocalTypes_UsesModuleCalleeCandidatesForExpectedArgs(t *testing.T) {
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

	evidence := trace.GraphEvidence(graph, graph.Bindings())
	inferred := InferLocalTypes(LocalInferenceConfig{
		Context: &abstractcore.FlowContext{
			Graph:          graph,
			Evidence:       evidence,
			ModuleBindings: moduleBindings,
			CallCtx:        db.NewQueryContext(db.New()),
			TypeOps:        querycore.NewEngine(),
			Derived: &abstractcore.Derived{
				Synth: func(ast.Expr, cfg.Point) typ.Type { return typ.Unknown },
				SymResolver: func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
					if sym == globalCalleeSym {
						return nil, false
					}
					if sym == moduleCalleeSym {
						return typ.Func().Param("n", typ.Number).Returns(typ.Nil).Build(), true
					}
					return nil, false
				},
			},
		},
	})

	got := inferred[xSym]
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("inferred param type = %v, want number", got)
	}
	if len(evidence.Calls) != 1 {
		t.Fatalf("expected one call evidence entry, got %d", len(evidence.Calls))
	}
	if expected := evidence.Calls[0].ExpectedArgType(0); !typ.TypeEquals(expected, typ.Number) {
		t.Fatalf("materialized expected arg = %v, want number", expected)
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

func TestCallBoundarySymbols_CollectsAndDeduplicates(t *testing.T) {
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

	refs := callBoundarySymbols(info, bindings)
	if !hasSymbol(refs, calleeSym) || !hasSymbol(refs, recvSym) || !hasSymbol(refs, argSym) || !hasSymbol(refs, objSym) || !hasSymbol(refs, fieldSym) {
		t.Fatalf("expected refs to include callee/receiver/arg/object/field symbols, got %v", refs)
	}
}

func TestCallBoundarySymbols_TreatsNestedCallsAsSolvedEvents(t *testing.T) {
	bindings := bind.NewBindingTable()
	obj := &ast.IdentExpr{Value: "obj"}
	arg := &ast.IdentExpr{Value: "x"}
	objSym := cfg.SymbolID(111)
	argSym := cfg.SymbolID(112)
	bindings.Bind(obj, objSym)
	bindings.Bind(arg, argSym)

	nested := &ast.FuncCallExpr{
		Receiver: obj,
		Method:   "make",
	}
	info := &cfg.CallInfo{
		Receiver: nested,
		Method:   "use",
		Args:     []ast.Expr{arg},
	}

	refs := callBoundarySymbols(info, bindings)
	if hasSymbol(refs, objSym) {
		t.Fatalf("nested receiver call internals must not be outer call dependencies, got %v", refs)
	}
	if !hasSymbol(refs, argSym) {
		t.Fatalf("direct argument symbol should remain a call boundary dependency, got %v", refs)
	}
}

func TestDeferredCallbackExpectationProjection_IsCallbackOnly(t *testing.T) {
	bindings := bind.NewBindingTable()
	solver := &localInferenceSolver{
		ctx:            &abstractcore.FlowContext{},
		bindings:       bindings,
		moduleBindings: bindings,
	}

	plain := localCallEntry{info: &cfg.CallInfo{
		Receiver: &ast.FuncCallExpr{Method: "builder"},
		Method:   "add",
		Args:     []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}}
	if solver.callNeedsDeferredCallbackExpectation(plain) {
		t.Fatalf("plain method chains should not request deferred callback expectation projection")
	}

	direct := localCallEntry{info: &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "register"},
		Args:   []ast.Expr{&ast.FunctionExpr{}},
	}}
	if !solver.callNeedsDeferredCallbackExpectation(direct) {
		t.Fatalf("direct callback literal should request deferred expectation projection")
	}

	cbIdent := &ast.IdentExpr{Value: "cb"}
	cbSym := cfg.SymbolID(2201)
	cbFn := &ast.FunctionExpr{}
	bindings.Bind(cbIdent, cbSym)
	bindings.SetFuncLitSymbol(cbFn, cbSym)
	named := localCallEntry{info: &cfg.CallInfo{
		Callee:     &ast.IdentExpr{Value: "register"},
		Args:       []ast.Expr{cbIdent},
		ArgSymbols: []cfg.SymbolID{cbSym},
	}}
	if !solver.callNeedsDeferredCallbackExpectation(named) {
		t.Fatalf("named callback literal should request deferred expectation projection")
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
		nil,
		map[cfg.SymbolID]typ.Type{aSym: typ.Number},
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
	if got := synth(ident, 0); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected overlay type string, got %v", got)
	}

	synth = synthWithInferenceOverlay(
		nil,
		nil,
		nil,
		map[cfg.SymbolID]typ.Type{aSym: typ.Number},
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
	if got := synth(ident, 0); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("expected function signature type number, got %v", got)
	}

	synth = synthWithInferenceOverlay(
		nil,
		nil,
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
