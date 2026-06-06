package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	compilecfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	valuecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferReturnRelationsSummaryBeatsTypeFallback(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	summaryRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 2}})
	typeRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	fallbackUsed := false

	got := InferReturnRelations(ReturnRelationsInput{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Summary: summary.Summary{Relations: summaryRel}},
			},
		},
		Call: call,
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				fallbackUsed = true
				return signatureWithRelation(typeRel)
			},
		},
		UseResolvedSignature: true,
	})

	if !flow.ReturnRelationsDomain.Equal(got, summaryRel) {
		t.Fatalf("relations = %#v, want summary relation %#v", got.ErrorReturns(), summaryRel.ErrorReturns())
	}
	if fallbackUsed {
		t.Fatal("type fallback ran despite summary relation")
	}
}

func TestInferReturnRelationsClosureAuthoritativeMissBlocksTypeFallback(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	typeRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	selection := SelectTargets(NewTargetSet(nil, false, nil, true))
	fallbackUsed := false

	got := InferReturnRelations(ReturnRelationsInput{
		Selection: selection,
		Call:      call,
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				fallbackUsed = true
				return signatureWithRelation(typeRel)
			},
		},
		UseResolvedSignature: true,
	})

	if !flow.ReturnRelationsDomain.Equal(got, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("relations = %#v, want Top", got.ErrorReturns())
	}
	if fallbackUsed {
		t.Fatal("type fallback ran despite closure-authoritative miss")
	}
}

func TestInferReturnRelationsUsesTypeThenStaticFallback(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "f"}
	call := &ast.FuncCallExpr{Func: ident}
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, 42)
	bindings.SetName(42, "f")
	typeRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	staticRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 2, ErrorIndex: 3}})
	staticLookup := StaticTypeLookup{
		FuncBySymbol: func(sym compilecfg.SymbolID) (typ.Type, bool) {
			if sym != 42 {
				t.Fatalf("static FuncBySymbol sym = %d, want 42", sym)
			}
			return signatureWithRelation(staticRel), true
		},
		FieldFunc: func(compilecfg.SymbolID, fieldkey.Key) (typ.Type, bool) {
			t.Fatal("field fallback should not run for ident callee")
			return nil, false
		},
	}

	got := InferReturnRelations(ReturnRelationsInput{
		Call: call,
		Resolver: TypeResolver{
			Bindings: bindings,
			ExprType: func(ast.Expr) typ.Type {
				return signatureWithRelation(typeRel)
			},
			Static: staticLookup,
		},
		UseResolvedSignature: true,
	})
	if !flow.ReturnRelationsDomain.Equal(got, typeRel) {
		t.Fatalf("relations = %#v, want type fallback %#v", got.ErrorReturns(), typeRel.ErrorReturns())
	}

	got = InferReturnRelations(ReturnRelationsInput{
		Call: call,
		Resolver: TypeResolver{
			Bindings: bindings,
			ExprType: func(ast.Expr) typ.Type {
				return typ.Func().Returns(typ.String, typ.Nil).Build()
			},
			Static: staticLookup,
		},
		UseResolvedSignature: true,
	})
	if !flow.ReturnRelationsDomain.Equal(got, staticRel) {
		t.Fatalf("relations = %#v, want static fallback %#v", got.ErrorReturns(), staticRel.ErrorReturns())
	}

	resolvedUsed := false
	got = InferReturnRelations(ReturnRelationsInput{
		Call: call,
		Resolver: TypeResolver{
			Bindings: bindings,
			ExprType: func(ast.Expr) typ.Type {
				resolvedUsed = true
				return signatureWithRelation(typeRel)
			},
			Static: staticLookup,
		},
		UseResolvedSignature: false,
	})
	if !flow.ReturnRelationsDomain.Equal(got, staticRel) {
		t.Fatalf("relations = %#v, want static fallback %#v", got.ErrorReturns(), staticRel.ErrorReturns())
	}
	if resolvedUsed {
		t.Fatal("resolved signature ran despite UseResolvedSignature=false")
	}
}

func TestInferReturnRelationsUsesLengthOnlyTypeFallback(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "keys"}
	call := &ast.FuncCallExpr{Func: ident}
	lengthRel := flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{{ReturnIndex: 0, ParamIndex: 0}})

	got := InferReturnRelations(ReturnRelationsInput{
		Call: call,
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				return signatureWithRelation(lengthRel)
			},
		},
		UseResolvedSignature: true,
	})

	if !got.HasLengthParam(flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 0}) {
		t.Fatalf("relations = %#v, want length-only type fallback %#v", got.LengthParams(), lengthRel.LengthParams())
	}
}

func TestInferCellEffectsBlocksCallbackFallbackWhenSelectionBlocks(t *testing.T) {
	t.Parallel()

	direct := flow.CaptureMustWrite(valuecfg.SymbolID(10), product.FromType(typ.String))
	callback := flow.CaptureMustWrite(valuecfg.SymbolID(11), product.FromType(typ.Number))
	arg := &ast.IdentExpr{Value: "cb"}
	selection := SelectTargets(NewTargetSet(nil, false, nil, true))
	callbackUsed := false

	got := InferCellEffects(CellEffectsInput{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Summary: summary.Summary{CellEffects: direct}},
			},
		},
		Selection: selection,
		Aggregation: summary.CellEffectAggregation{
			CallbackSpec: contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}),
			CallbackArgs: []ast.Expr{arg},
			ResolveCallback: func(ast.Expr) ([]summary.FuncRef, bool) {
				callbackUsed = true
				return []summary.FuncRef{{GraphID: 20}}, true
			},
			EffectOf: func(summary.FuncRef, summary.EntryValues) flow.CaptureEffects {
				return callback
			},
		},
	})

	if !flow.CaptureEffectsDomain.Equal(got, direct) {
		t.Fatalf("effects = %s, want direct %s", got.Format(), direct.Format())
	}
	if callbackUsed {
		t.Fatal("callback fallback ran despite closure-authoritative miss")
	}
}

func TestInferCellEffectsComposesCallbackFallbackWhenAllowed(t *testing.T) {
	t.Parallel()

	direct := flow.CaptureMustWrite(valuecfg.SymbolID(10), product.FromType(typ.String))
	callback := flow.CaptureMustWrite(valuecfg.SymbolID(11), product.FromType(typ.Number))
	arg := &ast.IdentExpr{Value: "cb"}

	got := InferCellEffects(CellEffectsInput{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Summary: summary.Summary{CellEffects: direct}},
			},
		},
		Aggregation: summary.CellEffectAggregation{
			CallbackSpec: contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}),
			CallbackArgs: []ast.Expr{arg},
			ResolveCallback: func(expr ast.Expr) ([]summary.FuncRef, bool) {
				if expr != arg {
					t.Fatalf("callback resolver got %#v, want arg", expr)
				}
				return []summary.FuncRef{{GraphID: 20}}, true
			},
			EffectOf: func(summary.FuncRef, summary.EntryValues) flow.CaptureEffects {
				return callback
			},
		},
	})

	want := flow.CooccurringCaptureEffects(direct, callback)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("effects = %s, want %s", got.Format(), want.Format())
	}
}

func TestParamNarrowProjectionSummaryBeatsImportedSignature(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "svc"}
	callee := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "run"}}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 99)
	bindings.SetName(99, "svc")
	summaryNarrow := paramevidence.ParamNarrow{Param: 0, Check: compilecfg.CheckTruthy}
	importedUsed := false

	got := (ParamNarrowProjection{
		Call: call,
		SummaryNarrows: func(*ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			return []paramevidence.ParamNarrow{summaryNarrow}, true
		},
		Resolver: TypeResolver{
			Bindings: bindings,
			Static: StaticTypeLookup{
				ImportedBase: func(compilecfg.SymbolID) (typ.Type, bool) {
					importedUsed = true
					return typ.NewRecord().
						Field("run", signatureWithParamNarrow(1, compilecfg.CheckNotNil)).
						Build(), true
				},
			},
		},
	}).Narrows()

	if len(got) != 1 || got[0].Param != 0 || got[0].Check != compilecfg.CheckTruthy {
		t.Fatalf("param narrows = %#v, want summary narrow", got)
	}
	if importedUsed {
		t.Fatal("imported signature fallback ran despite summary narrows")
	}
}

func TestParamNarrowProjectionImportedSignatureFallback(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "svc"}
	callee := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "run"}}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 100)
	bindings.SetName(100, "svc")
	got := (ParamNarrowProjection{
		Call: call,
		SummaryNarrows: func(*ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			return nil, false
		},
		Resolver: TypeResolver{
			Bindings: bindings,
			Static: StaticTypeLookup{
				ImportedBase: func(sym compilecfg.SymbolID) (typ.Type, bool) {
					if sym != 100 {
						t.Fatalf("ImportedBase sym = %d, want 100", sym)
					}
					return typ.NewRecord().
						Field("run", signatureWithParamNarrow(1, compilecfg.CheckNotNil)).
						Build(), true
				},
			},
		},
	}).Narrows()

	if len(got) != 1 || got[0].Param != 1 || got[0].Check != compilecfg.CheckNotNil {
		t.Fatalf("param narrows = %#v, want imported signature narrow", got)
	}
}

func TestParamNarrowProjectionStaticGlobalFieldFallback(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "assert"}
	callee := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "not_nil"}}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 101)
	bindings.SetName(101, "assert")

	got := (ParamNarrowProjection{
		Call: call,
		SummaryNarrows: func(*ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			return nil, false
		},
		Resolver: TypeResolver{
			Bindings: bindings,
			Static: StaticTypeLookup{
				GlobalBySymbol: func(sym compilecfg.SymbolID) (typ.Type, bool) {
					if sym != 101 {
						t.Fatalf("GlobalBySymbol sym = %d, want 101", sym)
					}
					return typ.NewRecord().
						Field("not_nil", signatureWithParamNarrow(0, compilecfg.CheckNotNil)).
						Build(), true
				},
			},
		},
	}).Narrows()

	if len(got) != 1 || got[0].Param != 0 || got[0].Check != compilecfg.CheckNotNil {
		t.Fatalf("param narrows = %#v, want static global-field signature narrow", got)
	}
}

func TestParamNarrowProjectionStaticIdentFallback(t *testing.T) {
	t.Parallel()

	callee := &ast.IdentExpr{Value: "expect_present"}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(callee, 102)
	bindings.SetName(102, "expect_present")

	got := (ParamNarrowProjection{
		Call: call,
		SummaryNarrows: func(*ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			return nil, false
		},
		Resolver: TypeResolver{
			Bindings: bindings,
			Static: StaticTypeLookup{
				FuncBySymbol: func(sym compilecfg.SymbolID) (typ.Type, bool) {
					if sym != 102 {
						t.Fatalf("FuncBySymbol sym = %d, want 102", sym)
					}
					return signatureWithParamNarrow(0, compilecfg.CheckNotNil), true
				},
			},
		},
	}).Narrows()

	if len(got) != 1 || got[0].Param != 0 || got[0].Check != compilecfg.CheckNotNil {
		t.Fatalf("param narrows = %#v, want static identifier signature narrow", got)
	}
}

func TestCallbackSpecForCallSummarySignatureWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	summarySpec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	fallbackUsed := false

	got := CallbackSpecForCall(CallbackSpecInput{
		Call: call,
		SummarySignature: func(*ast.FuncCallExpr) typ.Type {
			return typ.Func().Spec(summarySpec).Build()
		},
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				fallbackUsed = true
				return typ.Func().Spec(contract.NewSpec().WithCallback(1, &contract.CallbackSpec{})).Build()
			},
		},
	})

	if got == nil || got.Callbacks[0] == nil {
		t.Fatalf("callback spec = %#v, want summary callback on param 0", got)
	}
	if fallbackUsed {
		t.Fatal("callee fallback ran despite summary signature callback spec")
	}
}

func TestCallbackSpecForCallMethodReceiverFallback(t *testing.T) {
	t.Parallel()

	receiver := &ast.IdentExpr{Value: "obj"}
	call := &ast.FuncCallExpr{Receiver: receiver, Method: "run"}
	methodSpec := contract.NewSpec().WithCallback(1, &contract.CallbackSpec{Cardinality: contract.CardAtMostOnce})
	receiverType := typ.NewRecord().
		Field("run", typ.Func().Spec(methodSpec).Build()).
		Build()

	got := CallbackSpecForCall(CallbackSpecInput{
		Call: call,
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr != receiver {
					t.Fatalf("ExprType got %#v, want receiver", expr)
				}
				return receiverType
			},
		},
	})

	if got == nil || got.Callbacks[1] == nil {
		t.Fatalf("callback spec = %#v, want method callback on param 1", got)
	}
}

func TestContainerElementUnionProjectionExtractsMutateLabels(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "send"}}
	spec := contract.NewSpec().WithEffects(effect.Mutate{
		Transform: effect.ContainerElementUnion{
			Container: effect.ParamRef{Index: 0},
			Value:     effect.ParamRef{Index: 1},
		},
	})

	got := (ContainerElementUnionProjection{
		Call: call,
		SummarySignature: func(*ast.FuncCallExpr) typ.Type {
			return typ.Func().Spec(spec).Build()
		},
		Resolver: TypeResolver{},
	}).Effects()

	if len(got) != 1 || got[0].Container.Index != 0 || got[0].Value.Index != 1 {
		t.Fatalf("container element unions = %#v, want one 0<-1 effect", got)
	}
}

func TestResolveCallbackArgRefsFunctionLiteralBeatsStaticExpr(t *testing.T) {
	t.Parallel()

	arg := &ast.FunctionExpr{}
	literalRef := summary.FuncRef{GraphID: 7}
	staticUsed := false

	got, ok := ResolveCallbackArgRefs(CallbackArgInput{
		Arg: arg,
		FunctionLiteral: func(fn *ast.FunctionExpr) (summary.FuncRef, bool) {
			if fn != arg {
				t.Fatalf("function literal resolver got %#v, want arg", fn)
			}
			return literalRef, true
		},
		StaticExpr: func(ast.Expr) (summary.FuncRef, bool) {
			staticUsed = true
			return summary.FuncRef{GraphID: 8}, true
		},
	})

	if !ok || len(got) != 1 || got[0] != literalRef {
		t.Fatalf("callback arg refs = %+v/%v; want literal ref", got, ok)
	}
	if staticUsed {
		t.Fatal("static expr fallback ran despite function literal ref")
	}
}

func TestResolveCallbackArgRefsStaticExprFallback(t *testing.T) {
	t.Parallel()

	arg := &ast.IdentExpr{Value: "cb"}
	staticRef := summary.FuncRef{GraphID: 9}

	got, ok := ResolveCallbackArgRefs(CallbackArgInput{
		Arg: arg,
		FunctionLiteral: func(*ast.FunctionExpr) (summary.FuncRef, bool) {
			t.Fatal("function literal resolver ran for ident")
			return summary.FuncRef{}, false
		},
		StaticExpr: func(expr ast.Expr) (summary.FuncRef, bool) {
			if expr != arg {
				t.Fatalf("static resolver got %#v, want arg", expr)
			}
			return staticRef, true
		},
	})

	if !ok || len(got) != 1 || got[0] != staticRef {
		t.Fatalf("callback arg refs = %+v/%v; want static ref", got, ok)
	}
}

func TestResolveCallbackArgRefsLiveFunctionRefsBeatStaticExpr(t *testing.T) {
	t.Parallel()

	arg := &ast.IdentExpr{Value: "cb"}
	live := []summary.FuncRef{{GraphID: 20}, {GraphID: 10}}
	staticUsed := false

	got, ok := ResolveCallbackArgRefs(CallbackArgInput{
		Arg: arg,
		FunctionRefs: func(expr ast.Expr) ([]summary.FuncRef, bool) {
			if expr != arg {
				t.Fatalf("live resolver got %#v, want arg", expr)
			}
			return live, true
		},
		StaticExpr: func(ast.Expr) (summary.FuncRef, bool) {
			staticUsed = true
			return summary.FuncRef{GraphID: 30}, true
		},
	})

	if !ok || len(got) != 2 || got[0].GraphID != 10 || got[1].GraphID != 20 {
		t.Fatalf("callback arg refs = %+v/%v, want sorted live refs 10,20", got, ok)
	}
	if staticUsed {
		t.Fatal("static fallback ran despite authoritative live FunctionRefs")
	}
}

func TestResolveCallbackArgRefsLiveTopBlocksStaticExpr(t *testing.T) {
	t.Parallel()

	arg := &ast.IdentExpr{Value: "cb"}
	staticUsed := false

	got, ok := ResolveCallbackArgRefs(CallbackArgInput{
		Arg: arg,
		FunctionRefs: func(ast.Expr) ([]summary.FuncRef, bool) {
			return nil, true
		},
		StaticExpr: func(ast.Expr) (summary.FuncRef, bool) {
			staticUsed = true
			return summary.FuncRef{GraphID: 30}, true
		},
	})

	if !ok || len(got) != 0 {
		t.Fatalf("callback arg refs = %+v/%v, want authoritative unknown", got, ok)
	}
	if staticUsed {
		t.Fatal("static fallback ran despite authoritative unknown FunctionRefs")
	}
}

func TestStaticCallbackOverlaysForCallGlobalIdentWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "schedule"}}

	got := StaticCallbackOverlaysForCall(StaticCallbackOverlayInput{
		Call: call,
		Resolver: TypeResolver{
			Static: StaticTypeLookup{
				GlobalByName: func(name string) (typ.Type, bool) {
					if name != "schedule" {
						t.Fatalf("GlobalByName name = %q, want schedule", name)
					}
					return signatureWithCallbackOverlay(0, "migration", typ.String), true
				},
			},
		},
	})

	if len(got) != 1 || got[0].ParamIndex != 0 {
		t.Fatalf("overlays = %#v, want one global overlay on param 0", got)
	}
	if tpe, ok := got[0].Overlay.Type("migration"); !ok || !typ.TypeEquals(tpe, typ.String) {
		t.Fatalf("overlay migration type = %v, %v; want string", tpe, ok)
	}
}

func TestStaticCallbackOverlaysForCallImportedFallback(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "jobs"}
	callee := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "run"},
	}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 101)
	bindings.SetName(101, "jobs")

	got := StaticCallbackOverlaysForCall(StaticCallbackOverlayInput{
		Call: call,
		Resolver: TypeResolver{
			Bindings: bindings,
			Static: StaticTypeLookup{
				GlobalByName: func(name string) (typ.Type, bool) {
					t.Fatalf("GlobalByName %q should not run for non-ident callee", name)
					return nil, false
				},
				ImportedBase: func(sym compilecfg.SymbolID) (typ.Type, bool) {
					if sym != 101 {
						t.Fatalf("ImportedBase sym = %d, want 101", sym)
					}
					return typ.NewRecord().
						Field("run", signatureWithCallbackOverlay(1, "database", typ.Number)).
						Build(), true
				},
			},
		},
	})

	if len(got) != 1 || got[0].ParamIndex != 1 {
		t.Fatalf("overlays = %#v, want one imported overlay on param 1", got)
	}
	if tpe, ok := got[0].Overlay.Type("database"); !ok || !typ.TypeEquals(tpe, typ.Number) {
		t.Fatalf("overlay database type = %v, %v; want number", tpe, ok)
	}
}

func TestStaticCallbackOverlaysForCallGlobalFieldFallback(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "migration"}
	callee := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "define"},
	}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 102)
	bindings.SetName(102, "migration")
	bindings.SetKind(102, compilecfg.SymbolGlobal)

	got := StaticCallbackOverlaysForCall(StaticCallbackOverlayInput{
		Call: call,
		Resolver: TypeResolver{
			Bindings: bindings,
			Static: StaticTypeLookup{
				GlobalBySymbol: func(sym compilecfg.SymbolID) (typ.Type, bool) {
					if sym != 102 {
						t.Fatalf("GlobalBySymbol sym = %d, want 102", sym)
					}
					return typ.NewRecord().
						Field("define", signatureWithCallbackOverlay(1, "up", typ.Boolean)).
						Build(), true
				},
				ImportedBase: func(sym compilecfg.SymbolID) (typ.Type, bool) {
					t.Fatalf("ImportedBase %d should not run for global-base field", sym)
					return nil, false
				},
			},
		},
	})

	if len(got) != 1 || got[0].ParamIndex != 1 {
		t.Fatalf("overlays = %#v, want one global-field overlay on param 1", got)
	}
	if tpe, ok := got[0].Overlay.Type("up"); !ok || !typ.TypeEquals(tpe, typ.Boolean) {
		t.Fatalf("overlay up type = %v, %v; want boolean", tpe, ok)
	}
}

func signatureWithRelation(rels flow.ReturnRelations) typ.Type {
	spec := contract.NewSpec()
	for _, rel := range rels.ErrorReturns() {
		spec = spec.WithEffects(effect.ErrorReturn{
			ValueIndex: rel.ValueIndex,
			ErrorIndex: rel.ErrorIndex,
		})
	}
	for _, rel := range rels.LengthParams() {
		spec = spec.WithEffects(effect.ReturnLength{
			ReturnIndex: rel.ReturnIndex,
			Length:      constraint.ParamLen{Index: rel.ParamIndex},
		})
	}
	return typ.Func().Returns(typ.String, typ.Nil).Spec(spec).Build()
}

func signatureWithParamNarrow(param int, check compilecfg.CondCheckKind) typ.Type {
	c, ok := paramevidence.ParamNarrowConstraint(paramevidence.ParamNarrow{
		Param:    param,
		Check:    check,
		EqParam:  -1,
		Segments: []constraint.Segment{},
	})
	if !ok {
		return typ.Func().Build()
	}
	return typ.Func().
		Param("x", typ.Any).
		Param("y", typ.Any).
		WithRefinement(&constraint.FunctionRefinement{
			Terminates: true,
			OnReturn:   constraint.FromConstraints(c),
		}).
		Build()
}

func signatureWithCallbackOverlay(param int, name string, t typ.Type) typ.Type {
	return typ.Func().
		Spec(contract.NewSpec().WithCallback(param, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
			name: t,
		}))).
		Build()
}
