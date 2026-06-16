package effectlowering

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type signatureMap map[string]signature.Function

func (m signatureMap) Lookup(name string) (signature.Function, bool) {
	sig, ok := m[name]
	return sig, ok
}

func staticName(name string) SignatureNameFunc {
	name = strings.TrimSpace(name)
	return func(transfer.NodeContext, factflow.CallProducer) (string, bool) {
		return name, name != ""
	}
}

func testReturnTypeOps() ReturnTypeOps {
	return ReturnTypeOps{
		CallableReturn: testCallableReturn,
		ElementOf:      testElementOf,
		TypeProjection: testTypeProjection,
		InstantiateGenericCall: func(fn *typ.Function, args []typ.Type) (*typ.Function, bool) {
			instantiated, violations := typecall.InstantiateGenericCall(fn, args)
			return instantiated, instantiated != nil && len(violations) == 0
		},
	}
}

func testCallableReturn(t typ.Type) (typ.Type, bool) {
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		return nil, false
	}
	return fn.Returns[0], true
}

func testElementOf(t typ.Type) (typ.Type, bool) {
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return tt.Element, tt.Element != nil
	case *typ.Map:
		return tt.Value, tt.Value != nil
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return nil, false
		}
		return typeexpr.Union(tt.Elements...), true
	default:
		return nil, false
	}
}

func testTypeProjection(source typ.Type, p projection.Projection) (typ.Type, bool) {
	current := source
	for _, step := range p.Steps {
		switch step.Kind {
		case projection.StepField:
			record, ok := unwrap.Alias(current).(*typ.Record)
			if !ok || record == nil {
				return nil, false
			}
			field := record.GetField(step.Field)
			if field == nil || field.Type == nil {
				return nil, false
			}
			current = field.Type
		case projection.StepCallableReturn:
			next, ok := testCallableReturn(current)
			if !ok {
				return nil, false
			}
			current = next
		case projection.StepGenericArg:
			if step.Index < 0 {
				return nil, false
			}
			inst, ok := unwrap.Alias(current).(*typ.Instantiated)
			if !ok || inst == nil || step.Index >= len(inst.TypeArgs) || inst.TypeArgs[step.Index] == nil {
				return nil, false
			}
			current = inst.TypeArgs[step.Index]
		default:
			return nil, false
		}
	}
	return current, current != nil
}

func TestSignatureOutcomeProviderMaterializesDeclaredReturns(t *testing.T) {
	reg := standard.Registry()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: symbol.ID(17),
	}).View(),

		state.State{}, nil).
		Results

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureOutcomeProviderMaterializesOptionalDeclaredReturn(t *testing.T) {
	reg := standard.Registry()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typeexpr.Optional(typ.String)).Build()},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: symbol.ID(18),
	}).View(),

		state.State{}, nil).
		Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertPresence(t, reg, got[0].Value, presence.Maybe())
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	assertTypeWitness(t, reg, got[0].Value, typeexpr.Optional(typ.String))
}

func TestSignatureOutcomeProviderMaterializesInterfaceDeclaredReturnAsPresent(t *testing.T) {
	reg := standard.Registry()
	iface := typ.NewInterface("Resource", []typ.Method{
		{
			Name: "close",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Nil).
				Build(),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(iface).Build()},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: symbol.ID(19),
	}).View(),

		state.State{}, nil).
		Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertPresence(t, reg, got[0].Value, presence.Present())
	assertTypeWitness(t, reg, got[0].Value, iface)
}

func TestSignatureOutcomeProviderLowersErrorReturnToReturnPresenceRelations(t *testing.T) {
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want true for matched signature")
	}
	if len(got.ReturnPresenceRelations) != 2 {
		t.Fatalf("return presence relations = %d, want 2: %#v", len(got.ReturnPresenceRelations), got.ReturnPresenceRelations)
	}
	assertCallReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	assertCallReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
}

func TestSignatureOutcomeProviderWeakAnyReturnIsNotPostReturnAuthority(t *testing.T) {
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"require": {Type: typ.Func().Returns(typ.Any).Build()},
		},
		NameFor: staticName("require"),
	})

	got := provider(transfer.NodeContext{Registry: standard.Registry()}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = true for weak any return, want false")
	}
	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one weak fallback result", got.Results)
	}
}

func TestSignatureOutcomeProviderLowersNormalReturnRefinementToParamPathRefinementAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(801)
	argExpr := factflow.ExprRef(802)
	argSymbol := symbol.ID(803)
	argPath := path.NewPath(argSymbol, "x")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"assertLike": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Present{},
				}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "assertLike", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 {
		t.Fatalf("param path refinements = %d, want 1: %#v", len(got.ParamPathRefinements), got.ParamPathRefinements)
	}
	if !got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("target path = %s, want $0", got.ParamPathRefinements[0].Path.String())
	}
	assertPresence(t, reg, got.ParamPathRefinements[0].Value, presence.Present())

	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: provider,
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Present())
}

func TestSignatureOutcomeProviderLowersAbsentNormalReturnRefinementAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(804)
	argExpr := factflow.ExprRef(805)
	argSymbol := symbol.ID(806)
	argPath := path.NewPath(argSymbol, "err")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"isNil": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Absent{},
				}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "isNil", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 {
		t.Fatalf("param path refinements = %d, want 1: %#v", len(got.ParamPathRefinements), got.ParamPathRefinements)
	}
	if !got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("target path = %s, want $0", got.ParamPathRefinements[0].Path.String())
	}
	assertPresence(t, reg, got.ParamPathRefinements[0].Value, presence.Absent())

	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: provider,
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Absent())
}

func TestSignatureOutcomeProviderNormalReturnRefinementDoesNotApplyWithoutExpressionPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(811)
	argSymbol := symbol.ID(812)
	existing := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(813), HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"assertLike": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(
					postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{}},
					postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 1}, Refinement: postcondition.Present{}},
				),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "assertLike", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 || !got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path refinements = %#v, want one unresolved $0 refinement", got.ParamPathRefinements)
	}
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), existing),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: provider,
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Maybe())
}

func TestSignatureOutcomeProviderLowersTableMutatorToParamPathInvalidationAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	argExpr := factflow.ExprRef(901)
	argSymbol := symbol.ID(902)
	argPath := path.NewPath(argSymbol, "items")
	containerKey := path.PathKey("sym902@1")
	childKey := path.PathKey("sym902@1.child")
	unrelatedKey := path.PathKey("sym903@1.child")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
					mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 {
		t.Fatalf("param path invalidations = %d, want 1: %#v", len(got.ParamPathInvalidations), got.ParamPathInvalidations)
	}
	if !got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("invalidation path = %s, want $0", got.ParamPathInvalidations[0].Path.String())
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(call, argSymbol, "items")
	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WritePathKey(reg, containerKey, present).
			WritePathKey(reg, childKey, present).
			WritePathKey(reg, unrelatedKey, present),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})
	assertPathValue(t, reg, flow[graph.Exit()], containerKey, present)
	assertPathValue(t, reg, flow[graph.Exit()], childKey, product.Bottom(reg))
	assertPathValue(t, reg, flow[graph.Exit()], unrelatedKey, present)
}

func TestSignatureOutcomeProviderLowersStoreIntoContainerArgument(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	containerExpr := factflow.ExprRef(911)
	insertedExpr := factflow.ExprRef(912)
	containerPath := path.NewPath(symbol.ID(913), "container")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: containerExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: insertedExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			containerExpr: containerPath,
			insertedExpr:  path.NewPath(symbol.ID(914), "inserted"),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"store": {
				Effect: effect.Empty.With(ownership.Store{
					Param: effect.ParamRef{Index: -1},
					Into:  effect.ParamRef{Index: 0},
				}),
			},
		},
		NameFor: staticName("store"),
		Facts:   facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 || !got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path invalidations = %#v, want container argument $0", got.ParamPathInvalidations)
	}
}

func TestSignatureOutcomeProviderSkipsStoreWithoutKnownDestination(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	firstExpr := factflow.ExprRef(916)
	lastExpr := factflow.ExprRef(917)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: firstExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: lastExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			firstExpr: path.NewPath(symbol.ID(918), "first"),
			lastExpr:  path.NewPath(symbol.ID(919), "last"),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"store": {
				Effect: effect.Empty.With(ownership.Store{
					Param: effect.ParamRef{Index: 0},
					Into:  effect.ParamRef{Index: -1},
				}),
			},
		},
		NameFor: staticName("store"),
		Facts:   facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathInvalidations) != 0 {
		t.Fatalf("param path invalidations = %#v, want none", got.ParamPathInvalidations)
	}
}

func TestSignatureOutcomeProviderLowersOwnershipSendAndStoreEscapeEvents(t *testing.T) {
	point := cfg.Point(912)
	arg0 := factflow.ExprRef(912)
	arg1 := factflow.ExprRef(913)
	arg2 := factflow.ExprRef(914)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: arg0, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg1, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg2, HasExpr: true},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"send": {
				Effect: effect.Empty.
					With(ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 2}}).
					With(ownership.Send{FromParam: 1}),
			},
		},
		NameFor: staticName("send"),
		Facts:   facts,
	})
	site, ok := facts.CallSite(point)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Point: point}, site.View(), state.State{}, nil)

	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventStore, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(1), callboundary.EscapeEventSend, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(2), callboundary.EscapeEventSend, true)
}

func TestSignatureOutcomeProviderParamPathInvalidationDoesNotApplyWithoutExpressionPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	argSymbol := symbol.ID(921)
	childKey := path.PathKey("sym921@1.child")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(921), HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(mutation.TableMutator{
					Target: effect.ParamRef{Index: 0},
					Value:  effect.ParamRef{Index: -1},
				}),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
	})
	site, ok := facts.CallSite(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site.View(), state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 || !got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path invalidations = %#v, want unresolved $0", got.ParamPathInvalidations)
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(call, argSymbol, "items")
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WritePathKey(reg, childKey, present),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})
	assertPathValue(t, reg, flow[graph.Exit()], childKey, present)
}

func TestWithSignatureNoNormalReturnsMarksNeverReturnCallAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	target := symbol.ID(820)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
			}),
		},
	})

	got := WithSignatureNoNormalReturns(SignatureNoNormalReturnConfig{
		Graph:    graph,
		Registry: reg,
		Signatures: signatureMap{
			"error": {Type: typ.Func().Param("message", typ.Any).Returns(typ.Never).Build()},
		},
		NameFor: staticName("error"),
		Facts:   facts,
	})

	if !got.NoNormalReturn(call) {
		t.Fatalf("NoNormalReturn(%d) = false, want true", call)
	}
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:   got,
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		}),
	})
	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(target), product.Bottom(reg))
}

func TestSignatureOutcomeProviderSameAsReturnsArgumentValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4)
	argRef := factflow.ExprRef(7)
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("value", typ.Any).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	assertCallOutcomeResults(t, reg, got, []product.Value{argValue})
}

func TestSignatureOutcomeProviderSameAsResolvesNegativeParamRef(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(5)
	firstRef := factflow.ExprRef(8)
	lastRef := factflow.ExprRef(9)
	firstValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	lastValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("first", typ.Any).Param("last", typ.Any).Returns(typ.String).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: firstRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: lastRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				firstRef: firstValue,
				lastRef:  lastValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	assertCallOutcomeResults(t, reg, got, []product.Value{lastValue})
}

func TestSignatureOutcomeProviderSameAsUsesDeclaredReturnTypeWhenArgumentProjectionFails(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(10), HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderElementOfArrayReturnsElementRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	assertTypeWitness(t, reg, got[0].Value, typ.String)
}

func TestSignatureOutcomeProviderElementOfMapReturnsValueRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewMap(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderElementOfTupleReturnsElementUnionRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewTuple(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Join(
		runtimekind.Singleton(runtimekind.String),
		runtimekind.Singleton(runtimekind.Number),
	))
}

func TestSignatureOutcomeProviderOptionalElementOfArrayKeepsMaybePresence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	if gotPresence := product.PresenceOf(got[0].Value); !presence.Equal(gotPresence, presence.Top()) {
		t.Fatalf("presence = %s, want maybe/top", gotPresence)
	}
}

func TestSignatureOutcomeProviderElementOfUsesDeclaredReturnTypeWhenParamProjectionFails(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: staticName("f"),
		Facts:   signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderCallbackReturnProjectsFirstReturnRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Integer).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderCallbackReturnResolvesNegativeParamRef(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", typ.String).
					Param("callback", typ.Func().Returns(typ.Boolean).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression},
			{Kind: factflow.ValueSourceExpression},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureOutcomeProviderArrayOfCallbackReturnProjectsTableRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(15)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Table))
}

func TestSignatureOutcomeProviderCallbackReturnUsesDeclaredReturnTypeWhenProjectionFails(t *testing.T) {
	reg := standard.Registry()

	tests := []struct {
		name      string
		point     cfg.Point
		paramType typ.Type
		ref       effect.ParamRef
		args      []factflow.ValueSource
		want      runtimekind.Value
	}{
		{
			name:      "non-callable callback parameter",
			point:     cfg.Point(16),
			paramType: typ.String,
			ref:       effect.ParamRef{Index: 0},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
		{
			name:      "out-of-range callback parameter",
			point:     cfg.Point(17),
			paramType: typ.Func().Returns(typ.Number).Build(),
			ref:       effect.ParamRef{Index: 1},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("callback", tc.paramType).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: tc.ref}}),
					},
				},
				NameFor:       staticName("f"),
				ReturnTypeOps: testReturnTypeOps(),
				Facts:         signatureOutcomeProviderFacts(tc.point, tc.args),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: tc.point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, tc.want)
		})
	}
}

func TestSignatureOutcomeProviderTypeProjectionFieldReturnsFieldRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	record := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", record).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.Field("name")}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureOutcomeProviderTypeProjectionCallableReturnReturnsFirstReturnRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Boolean, typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.CallableReturn()}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureOutcomeProviderTypeProjectionGenericArgReturnsArgRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)
	stringBox := typ.NewAlias("StringBox", typ.Instantiate(box, typ.String))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", stringBox).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.GenericArg(0)}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureOutcomeProviderInstantiatesGenericDeclaredReturnFromArgumentWitnesses(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	resultParam := typ.NewTypeParam("T", nil)
	resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{resultParam},
		typetable.NewRecord().Field("value", resultParam).Build())
	tParam := typ.NewTypeParam("T", nil)
	uParam := typ.NewTypeParam("U", nil)
	mapType := typ.Func().
		TypeParamRef(tParam).
		TypeParamRef(uParam).
		Param("result", typ.Instantiate(resultGeneric, tParam)).
		Param("fn", typ.Func().Param("value", tParam).Returns(uParam).Build()).
		Returns(typ.Instantiate(resultGeneric, uParam)).
		Build()
	decodedRef := factflow.ExprRef(22)
	callbackRef := factflow.ExprRef(23)
	decodedType := typ.Instantiate(resultGeneric, typ.String)
	callbackType := typ.Func().Param("value", typ.String).Returns(typ.Number).Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"map": {Type: mapType},
		},
		NameFor:       staticName("map"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: decodedRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: callbackRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				decodedRef:  typevalue.WithWitness(reg, typevalue.FromType(reg, decodedType), decodedType),
				callbackRef: typevalue.WithWitness(reg, typevalue.FromType(reg, callbackType), callbackType),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	gotType, ok := typevalue.TypeOf(reg, got[0].Value)
	if !ok {
		t.Fatalf("result type missing from value: %#v", got[0].Value)
	}
	wantType := typ.Instantiate(resultGeneric, typ.Number)
	if !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("result type = %v, want %v", gotType, wantType)
	}
}

func TestSignatureOutcomeProviderTypeProjectionUsesDeclaredReturnTypeWhenProjectionFails(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(24)
	record := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", record).
					Returns(typ.Number).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.Field("missing")}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestActiveReturnTransformClassificationMatrix(t *testing.T) {
	tests := []struct {
		name       string
		label      effect.Label
		wantActive bool
	}{
		{
			name:       "same as",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}},
			wantActive: true,
		},
		{
			name:       "element of",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}},
			wantActive: true,
		},
		{
			name:       "optional element of",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}},
			wantActive: true,
		},
		{
			name:       "callback return",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}},
			wantActive: true,
		},
		{
			name:       "array of callback return",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}},
			wantActive: true,
		},
		{
			name: "type projection",
			label: returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
				Source: effect.ParamRef{Index: 0},
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("payload"),
					projection.CallableReturn(),
				}},
			}},
			wantActive: true,
		},
		{
			name:       "deep element",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.DeepElementOf{Source: effect.ParamRef{Index: 0}}},
			wantActive: false,
		},
		{
			name:       "string unpack",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}}},
			wantActive: false,
		},
		{
			name:       "select case",
			label:      returns.Return{ReturnIndex: 0, Transform: returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}}},
			wantActive: false,
		},
		{
			name: "select result",
			label: returns.Return{ReturnIndex: 0, Transform: returns.SelectResultOfCases{
				Cases:   effect.ParamRef{Index: 0},
				Default: effect.ParamRef{Index: 1},
			}},
			wantActive: false,
		},
		{
			name:       "return length",
			label:      returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
			wantActive: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transform, ok := activeReturnTransform(signature.Function{Effect: effect.Empty.With(tc.label)}, 0)
			if ok != tc.wantActive {
				t.Fatalf("active = %v, want %v", ok, tc.wantActive)
			}
			if tc.wantActive && transform == nil {
				t.Fatal("active transform = nil, want concrete return transform")
			}
			if !tc.wantActive && transform != nil {
				t.Fatalf("active transform = %#v, want none", transform)
			}
		})
	}
}

func TestSignatureOutcomeProviderReservedReturnTransformsUseOnlyDeclaredReturnType(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name: "deep element",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "string unpack",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select case",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select result",
			label: returns.Return{
				ReturnIndex: 0,
				Transform: returns.SelectResultOfCases{
					Cases:   effect.ParamRef{Index: 0},
					Default: effect.ParamRef{Index: 1},
				},
			},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("items", typ.NewArray(typ.String)).
							Param("default", typ.Number).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(tc.label),
					},
				},
				NameFor: staticName("f"),
				Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression},
					{Kind: factflow.ValueSourceExpression},
				}),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1 declared result: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
		})
	}
}

func TestActiveReturnTransformIgnoresReservedReturnTransforms(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name: "deep element",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "string unpack",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select case",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select result",
			label: returns.Return{
				ReturnIndex: 0,
				Transform: returns.SelectResultOfCases{
					Cases:   effect.ParamRef{Index: 0},
					Default: effect.ParamRef{Index: 1},
				},
			},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if transform, ok := activeReturnTransform(signature.Function{Effect: effect.Empty.With(tc.label)}, 0); ok {
				t.Fatalf("active transform = %#v, want none", transform)
			}
		})
	}
}

func TestSupplementalResultsKeepsPrimarySlotsAndFillsMissingSignatureSlots(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{Results: []factapply.CallResult{{Index: 0, Value: primaryValue}}}
	}
	signatures := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: staticName("f"),
	})

	got := calloutcome.WithSupplemental(primary, signatures)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	if got[0].Index != 0 || !product.Equal(reg, got[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got[0])
	}
	if got[1].Index != 1 {
		t.Fatalf("supplemental slot index = %d, want 1", got[1].Index)
	}
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSupplementalResultsKeepsPrimarySlotOverSignatureSameAs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	argRef := factflow.ExprRef(11)
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{Results: []factapply.CallResult{{Index: 0, Value: primaryValue}}}
	}
	signatures := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := calloutcome.WithSupplemental(primary, signatures)(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	assertCallOutcomeResults(t, reg, got, []product.Value{primaryValue})
}

func assertCallOutcomeResults(t *testing.T, reg *axis.Registry, got []factapply.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("got result[%d].Index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("got result[%d].Value = %v, want %v", i, got[i].Value, value)
		}
	}
}

func assertEscapeEvent(
	t *testing.T,
	events []callboundary.EscapeEventFact,
	target path.Path,
	kind callboundary.EscapeEventKind,
	recursive bool,
) {
	t.Helper()
	for _, event := range events {
		if event.Target.Equal(target) && event.Kind == kind && event.Recursive == recursive {
			return
		}
	}
	t.Fatalf("escape events = %#v, want target %s kind %d recursive=%v", events, target, kind, recursive)
}

func signatureOutcomeProviderFacts(point cfg.Point, args []factflow.ValueSource) factflow.Facts {
	return factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{ArgumentSources: args}),
		},
	})
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := st.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("state[%s] = %v, want %v", slot, got, want)
	}
}

func assertStatePresence(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want presence.Value) {
	t.Helper()
	if got := product.PresenceOf(st.ReadValue(reg, slot)); !presence.Equal(got, want) {
		t.Fatalf("state[%s] presence = %s, want %s", slot, got, want)
	}
}

func assertPathValue(t *testing.T, reg *axis.Registry, st state.State, pathKey path.PathKey, want product.Value) {
	t.Helper()
	if got := st.ReadPathKey(reg, pathKey); !product.Equal(reg, got, want) {
		t.Fatalf("state path[%s] = %v, want %v", pathKey, got, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtimekind = %s, want %s", kind, want)
	}
}

func assertPresence(t *testing.T, _ *axis.Registry, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertTypeWitness(t *testing.T, reg *axis.Registry, got product.Value, want typ.Type) {
	t.Helper()
	witness := product.Get(reg, got, typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("type witness = %v/%v, want %v", gotType, ok, want)
	}
}

func assertCallReturnPresenceRelation(
	t *testing.T,
	relations []factapply.CallReturnPresenceRelation,
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex == triggerIndex &&
			presence.Equal(relation.TriggerPresence, triggerPresence) &&
			relation.TargetIndex == targetIndex &&
			presence.Equal(relation.TargetPresence, targetPresence) {
			return
		}
	}
	t.Fatalf("missing relation %d/%s -> %d/%s in %#v", triggerIndex, triggerPresence, targetIndex, targetPresence, relations)
}
