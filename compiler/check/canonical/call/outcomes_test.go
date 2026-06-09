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

func TestCallOutcomeReturnRelationsSummaryBeatsTypeFallback(t *testing.T) {
	t.Parallel()

	summaryRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 2}})
	typeRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})

	got := (CallOutcome{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Summary: summary.Summary{Relations: summaryRel}},
			},
		},
	}).WithTypeFallbackOutcome(TypeFallbackOutcome{returnRelations: typeRel}).ReturnRelations()

	if !flow.ReturnRelationsDomain.Equal(got, summaryRel) {
		t.Fatalf("relations = %#v, want summary relation %#v", got.ErrorReturns(), summaryRel.ErrorReturns())
	}
}

func TestCallOutcomeReturnRelationsClosureAuthoritativeMissBlocksTypeFallback(t *testing.T) {
	t.Parallel()

	typeRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	selection := NewTargetSet(nil, false, nil, true).Select()

	got := (CallOutcome{
		Selection: selection,
	}).WithTypeFallbackOutcome(TypeFallbackOutcome{returnRelations: typeRel}).ReturnRelations()

	if !flow.ReturnRelationsDomain.Equal(got, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("relations = %#v, want Top", got.ErrorReturns())
	}
}

func TestTypeFallbackOutcomeReturnRelationsUsesTypeThenStaticFallback(t *testing.T) {
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

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call:     call,
			Resolver: TypeResolver{Bindings: bindings, ExprType: func(ast.Expr) typ.Type { return signatureWithRelation(typeRel) }, Static: staticLookup},
		},
		UseResolvedSignature: true,
	}).ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, typeRel) {
		t.Fatalf("relations = %#v, want type fallback %#v", got.ErrorReturns(), typeRel.ErrorReturns())
	}

	got = NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call:     call,
			Resolver: TypeResolver{Bindings: bindings, ExprType: func(ast.Expr) typ.Type { return typ.Func().Returns(typ.String, typ.Nil).Build() }, Static: staticLookup},
		},
		UseResolvedSignature: true,
	}).ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, staticRel) {
		t.Fatalf("relations = %#v, want static fallback %#v", got.ErrorReturns(), staticRel.ErrorReturns())
	}

	resolvedUsed := false
	got = NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				Bindings: bindings,
				ExprType: func(ast.Expr) typ.Type {
					resolvedUsed = true
					return signatureWithRelation(typeRel)
				},
				Static: staticLookup,
			},
		},
	}).ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, staticRel) {
		t.Fatalf("relations = %#v, want static fallback %#v", got.ErrorReturns(), staticRel.ErrorReturns())
	}
	if resolvedUsed {
		t.Fatal("resolved signature ran despite UseResolvedSignature=false")
	}
}

func TestTypeFallbackOutcomeBoundaryFactsUsesLengthTypeFallback(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "keys"}
	call := &ast.FuncCallExpr{Func: ident}
	spec := contract.NewSpec().WithEffects(effect.ReturnLength{ReturnIndex: 0, Length: constraint.ParamLen{Index: 0}})

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call:     call,
			Resolver: TypeResolver{ExprType: func(ast.Expr) typ.Type { return typ.Func().Returns(typ.String).Spec(spec).Build() }},
		},
		UseResolvedSignature: true,
	}).BoundaryFacts()

	want := flow.BoundaryLengthRelationFact{
		Target: flow.BoundaryPath{Kind: flow.BoundaryPathReturn, Index: 0},
		Source: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0},
	}
	if !got.HasLengthRelation(want) {
		t.Fatalf("boundary facts = %#v, want length type fallback %#v", got.LengthRelations(), want)
	}
}

func TestCallOutcomeCellEffectsBlocksCallbackFallbackWhenSelectionBlocks(t *testing.T) {
	t.Parallel()

	direct := flow.CaptureMustWrite(valuecfg.SymbolID(10), product.FromType(typ.String))
	callback := flow.CaptureMustWrite(valuecfg.SymbolID(11), product.FromType(typ.Number))
	arg := &ast.IdentExpr{Value: "cb"}
	selection := NewTargetSet(nil, false, nil, true).Select()
	callbackUsed := false

	got := (CallOutcome{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Summary: summary.Summary{CellEffects: direct}},
			},
		},
		Selection: selection,
	}).CellEffects(summary.CellEffectAggregation{
		CallbackSpec: contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}),
		CallbackArgs: []ast.Expr{arg},
		ResolveCallback: func(ast.Expr) ([]summary.FuncRef, bool) {
			callbackUsed = true
			return []summary.FuncRef{{GraphID: 20}}, true
		},
		EffectOf: func(summary.FuncRef, summary.EntryValues) flow.CaptureEffects {
			return callback
		},
	})

	if !flow.CaptureEffectsDomain.Equal(got, direct) {
		t.Fatalf("effects = %s, want direct %s", got.Format(), direct.Format())
	}
	if callbackUsed {
		t.Fatal("callback fallback ran despite closure-authoritative miss")
	}
}

func TestCallOutcomeCellEffectsComposesCallbackFallbackWhenAllowed(t *testing.T) {
	t.Parallel()

	direct := flow.CaptureMustWrite(valuecfg.SymbolID(10), product.FromType(typ.String))
	callback := flow.CaptureMustWrite(valuecfg.SymbolID(11), product.FromType(typ.Number))
	arg := &ast.IdentExpr{Value: "cb"}

	got := (CallOutcome{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Summary: summary.Summary{CellEffects: direct}},
			},
		},
	}).CellEffects(summary.CellEffectAggregation{
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
	})

	want := flow.CooccurringCaptureEffects(direct, callback)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("effects = %s, want %s", got.Format(), want.Format())
	}
}

func TestCallOutcomeProjectsSelectedBoundaryAxes(t *testing.T) {
	t.Parallel()

	ref := summary.FuncRef{GraphID: 42}
	returnPath := constraint.NewPlaceholder(0).Field("run")
	returnRefs := flow.ReturnRefsOfSlots([]flow.ReturnRefSlot{
		flow.ReturnRefSlotOf(
			flow.WithFunctionRefPath(nil, returnPath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 9})),
			flow.ClosureRefsDomain.Bottom(),
		),
	})
	relations := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	cellEffects := flow.CaptureMustWrite(valuecfg.SymbolID(12), product.FromType(typ.String))
	receiverEffects := flow.ReceiverMustWrite(0, product.FromType(typ.Number))
	table := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0}
	key := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 1}
	boundaryFacts := flow.BoundaryFactsFromParts(flow.BoundaryFactParts{
		KeyPresence: []flow.BoundaryKeyPresenceFact{{Table: table, Key: key}},
	})

	outcome := CallOutcome{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{
				{Ref: ref, Summary: summary.Summary{
					ReturnRefs:      returnRefs,
					Relations:       relations,
					CellEffects:     cellEffects,
					ReceiverEffects: receiverEffects,
					BoundaryFacts:   boundaryFacts,
				}},
			},
		},
		Selection: NewTargetSet([]summary.FuncRef{ref}, true, nil, false).Select(),
	}

	if got := outcome.ReturnRefs(); got.Len() != 1 {
		t.Fatalf("ReturnRefs len = %d, want 1", got.Len())
	}
	if got := outcome.ReturnRelations(); !flow.ReturnRelationsDomain.Equal(got, relations) {
		t.Fatalf("ReturnRelations = %#v, want %#v", got, relations)
	}
	if got := outcome.CellEffects(summary.CellEffectAggregation{}); !flow.CaptureEffectsDomain.Equal(got, cellEffects) {
		t.Fatalf("CellEffects = %s, want %s", got.Format(), cellEffects.Format())
	}
	if got := outcome.ReceiverEffects(); !flow.ReceiverEffectsDomain.Equal(got, receiverEffects) {
		t.Fatalf("ReceiverEffects = %#v, want %#v", got, receiverEffects)
	}
	if got := outcome.BoundaryFacts(); !flow.BoundaryFactsDomain.Equal(got, boundaryFacts) {
		t.Fatalf("BoundaryFacts = %#v, want %#v", got, boundaryFacts)
	}
	if !outcome.NeverReturns(func(got summary.FuncRef) bool { return got == ref }) {
		t.Fatal("NeverReturns = false, want true")
	}
}

func TestCallOutcomePostconditionsSummaryBeatsTypeFallback(t *testing.T) {
	t.Parallel()

	ref := summary.FuncRef{GraphID: 77}
	summaryPost := paramevidence.ReturnPostconditionsFromParamNarrows([]paramevidence.ParamNarrow{{Param: 0, Check: compilecfg.CheckTruthy, EqParam: -1}})
	fallbackPost := paramevidence.ReturnPostconditionsFromParamNarrows([]paramevidence.ParamNarrow{{Param: 1, Check: compilecfg.CheckNotNil, EqParam: -1}})

	got := (CallOutcome{
		Projection: summary.CallSummaryProjection{
			Targets: []summary.CallSummaryTarget{{Ref: ref, Summary: summary.Summary{Postconditions: summaryPost}}},
		},
		Selection: NewTargetSet([]summary.FuncRef{ref}, true, nil, false).Select(),
	}).WithTypeFallbackOutcome(TypeFallbackOutcome{postconditions: fallbackPost}).Postconditions()

	if !containsConstraint(got.Condition().MustConstraints(), constraint.Truthy{Path: constraint.ParamPath(0)}) {
		t.Fatalf("postconditions = %v, want summary truthy proof", got.Condition())
	}
}

func TestTypeFallbackOutcomeImportedSignaturePostconditions(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "svc"}
	callee := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "run"}}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 100)
	bindings.SetName(100, "svc")
	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
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
		},
	}).Postconditions()

	if !containsConstraint(got.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(1)}) {
		t.Fatalf("postconditions = %v, want imported signature not-nil proof", got.Condition())
	}
}

func TestTypeFallbackOutcomePreservesImportedDNF(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "check"}}
	common := constraint.NotNil{Path: constraint.ParamPath(0)}
	left := constraint.Truthy{Path: constraint.ParamPath(1)}
	right := constraint.Falsy{Path: constraint.ParamPath(1)}

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				Static: StaticTypeLookup{
					GlobalByName: func(name string) (typ.Type, bool) {
						if name != "check" {
							return nil, false
						}
						return typ.Func().
							WithRefinement(&constraint.FunctionRefinement{
								OnReturn: constraint.FromDisjuncts([][]constraint.Constraint{
									{common, left},
									{common, right},
								}),
							}).
							Build(), true
					},
				},
			},
		},
	}).Postconditions()

	if got.Condition().NumDisjuncts() != 2 {
		t.Fatalf("Postconditions disjuncts = %d, want full imported DNF", got.Condition().NumDisjuncts())
	}
	narrows := paramevidence.ParamNarrowsFromReturnPostconditions(got)
	if len(narrows) != 1 || narrows[0].Param != 0 || narrows[0].Check != compilecfg.CheckNotNil {
		t.Fatalf("recovered local ParamNarrows = %#v, want only common not-nil fact", narrows)
	}
}

func TestTypeFallbackOutcomeStaticGlobalFieldPostconditions(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "assert"}
	callee := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "not_nil"}}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, 101)
	bindings.SetName(101, "assert")

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
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
		},
	}).Postconditions()

	if !containsConstraint(got.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(0)}) {
		t.Fatalf("postconditions = %v, want static global-field not-nil proof", got.Condition())
	}
}

func TestTypeFallbackOutcomeStaticIdentPostconditions(t *testing.T) {
	t.Parallel()

	callee := &ast.IdentExpr{Value: "expect_present"}
	call := &ast.FuncCallExpr{Func: callee}
	bindings := bind.NewBindingTable()
	bindings.Bind(callee, 102)
	bindings.SetName(102, "expect_present")

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
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
		},
	}).Postconditions()

	if !containsConstraint(got.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(0)}) {
		t.Fatalf("postconditions = %v, want static identifier not-nil proof", got.Condition())
	}
}

func TestCallOutcomeTypeFallbackCannotInventSummaryAuthority(t *testing.T) {
	t.Parallel()

	effectValue := flow.CaptureMustWrite(valuecfg.SymbolID(22), product.FromType(typ.String))
	fallbackPost := paramevidence.ReturnPostconditionsFromParamNarrows([]paramevidence.ParamNarrow{{Param: 0, Check: compilecfg.CheckTruthy, EqParam: -1}})
	outcome := CallOutcome{}.WithTypeFallbackOutcome(TypeFallbackOutcome{postconditions: fallbackPost})

	if !flow.CaptureEffectsDomain.Equal(outcome.CellEffects(summary.CellEffectAggregation{DirectEffects: effectValue}), flow.CaptureEffectsDomain.Bottom()) {
		t.Fatal("type fallback outcome invented summary capture effects")
	}
	if outcome.ReturnRefs().Len() != 0 {
		t.Fatalf("type fallback ReturnRefs len = %d, want 0", outcome.ReturnRefs().Len())
	}
	if members := outcome.ReturnStaticMembers(); len(members) != 0 {
		t.Fatalf("type fallback ReturnStaticMembers = %#v, want none", members)
	}
	if outcome.NeverReturns(func(summary.FuncRef) bool { return true }) {
		t.Fatal("type fallback outcome invented no-return proof")
	}
	if !outcome.Postconditions().HasConstraints() {
		t.Fatal("type fallback should still expose imported/static postconditions")
	}
}

func TestTypeFallbackOutcomeCallbackSpecSummarySignatureWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	summarySpec := contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})
	fallbackUsed := false

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				ExprType: func(ast.Expr) typ.Type {
					fallbackUsed = true
					return typ.Func().Spec(contract.NewSpec().WithCallback(1, &contract.CallbackSpec{})).Build()
				},
			},
		},
		SummarySignature: typ.Func().Spec(summarySpec).Build(),
	}).CallbackSpec()

	if got == nil || got.Callbacks[0] == nil {
		t.Fatalf("callback spec = %#v, want summary callback on param 0", got)
	}
	if fallbackUsed {
		t.Fatal("callee fallback ran despite summary signature callback spec")
	}
}

func TestTypeFallbackOutcomeCallbackSpecMethodReceiverFallback(t *testing.T) {
	t.Parallel()

	receiver := &ast.IdentExpr{Value: "obj"}
	call := &ast.FuncCallExpr{Receiver: receiver, Method: "run"}
	methodSpec := contract.NewSpec().WithCallback(1, &contract.CallbackSpec{Cardinality: contract.CardAtMostOnce})
	receiverType := typ.NewRecord().
		Field("run", typ.Func().Spec(methodSpec).Build()).
		Build()

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				ExprType: func(expr ast.Expr) typ.Type {
					if expr != receiver {
						t.Fatalf("ExprType got %#v, want receiver", expr)
					}
					return receiverType
				},
			},
		},
		UseResolvedSignature: true,
	}).CallbackSpec()

	if got == nil || got.Callbacks[1] == nil {
		t.Fatalf("callback spec = %#v, want method callback on param 1", got)
	}
}

func TestTypeFallbackOutcomeExtractsContainerElementUnionLabels(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "send"}}
	spec := contract.NewSpec().WithEffects(effect.Mutate{
		Transform: effect.ContainerElementUnion{
			Container: effect.ParamRef{Index: 0},
			Value:     effect.ParamRef{Index: 1},
		},
	})

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
		},
		SummarySignature: typ.Func().Spec(spec).Build(),
	}).ContainerElementUnions()

	if len(got) != 1 || got[0].Container.Index != 0 || got[0].Value.Index != 1 {
		t.Fatalf("container element unions = %#v, want one 0<-1 effect", got)
	}
}

func TestStaticCallbackOverlayProjectionGlobalIdentWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "schedule"}}

	got := (StaticCallbackOverlayProjection{
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
	}).Overlays()

	if len(got) != 1 || got[0].ParamIndex != 0 {
		t.Fatalf("overlays = %#v, want one global overlay on param 0", got)
	}
	if tpe, ok := got[0].Overlay.Type("migration"); !ok || !typ.TypeEquals(tpe, typ.String) {
		t.Fatalf("overlay migration type = %v, %v; want string", tpe, ok)
	}
}

func TestStaticCallbackOverlayProjectionImportedFallback(t *testing.T) {
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

	got := (StaticCallbackOverlayProjection{
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
	}).Overlays()

	if len(got) != 1 || got[0].ParamIndex != 1 {
		t.Fatalf("overlays = %#v, want one imported overlay on param 1", got)
	}
	if tpe, ok := got[0].Overlay.Type("database"); !ok || !typ.TypeEquals(tpe, typ.Number) {
		t.Fatalf("overlay database type = %v, %v; want number", tpe, ok)
	}
}

func TestStaticCallbackOverlayProjectionGlobalFieldFallback(t *testing.T) {
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

	got := (StaticCallbackOverlayProjection{
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
	}).Overlays()

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
	return typ.Func().Returns(typ.String, typ.Nil).Spec(spec).Build()
}

func containsConstraint(haystack []constraint.Constraint, needle constraint.Constraint) bool {
	for _, c := range haystack {
		if c.Equals(needle) {
			return true
		}
	}
	return false
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
