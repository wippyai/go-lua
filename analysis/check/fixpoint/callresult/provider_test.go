package callresult

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestOutcomeProviderReadsSummaryReturnsByCalleeIdentity(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	first := product.Top()
	second := product.Absent(reg)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{first, second}},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}), state.State{}, nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{first, second})
}

func TestOutcomeProviderMapsSummaryReturnsAndNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(37)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 38})
	ret := product.Absent(reg)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{ret},
				NormalReturnFacts: summary.NormalReturnFacts{
					PathRefinements: []summary.PathValueFact{
						{Path: path.NewPlaceholder(0).Field("field"), Value: absent},
					},
					PathStaticMembers: []summary.PathStaticMemberFact{
						{Path: path.NewPlaceholder(0).Field("static"), Value: present},
					},
					DynamicIndexFacts: []summary.DynamicIndexFact{
						{
							Table: path.NewPlaceholder(0).Field("items"),
							Site:  "summary.dynamic",
							Value: dynamicindex.Fact{
								KeyPresence: presence.Present(),
								KeyValue:    present,
								Value:       absent,
								Admission:   dynamicindex.AdmissionAdmitted,
							},
						},
					},
					BranchProofs: []summary.BranchProof{
						{
							Kind:  pathevidence.BranchProofPathEqual,
							Path:  path.NewPlaceholder(0).Field("left"),
							Other: path.NewPlaceholder(1).Field("right"),
						},
					},
					ChannelSelects: []summary.ChannelSelectFact{
						{
							Select: channelselectfact.ID("summary.select"),
							Kind:   channelselectfact.FactReceive,
							Result: path.NewPlaceholder(0).Field("result"),
							Case:   path.NewPlaceholder(1).Field("case"),
							Index:  2,
						},
					},
					EffectDeltas: []summary.EffectDelta{
						{
							Target: path.NewPlaceholder(0).Field("items"),
							Site:   "summary.effect",
							Kind:   effectdelta.Mutation,
							Value:  effectdelta.Value{Before: present, After: absent, Change: effectdelta.ChangeChanged},
						},
					},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}), state.State{}, nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{ret})
	if len(got.PathRefinements) != 1 ||
		!got.PathRefinements[0].Path.Equal(path.NewPlaceholder(0).Field("field")) ||
		!product.Equal(reg, got.PathRefinements[0].Value, absent) {
		t.Fatalf("path refinements = %#v, want mapped summary path refinement", got.PathRefinements)
	}
	if len(got.PathStaticMembers) != 1 ||
		!got.PathStaticMembers[0].Path.Equal(path.NewPlaceholder(0).Field("static")) ||
		!product.Equal(reg, got.PathStaticMembers[0].Value, present) {
		t.Fatalf("path static members = %#v, want mapped summary static member", got.PathStaticMembers)
	}
	if len(got.DynamicIndexFacts) != 1 ||
		!got.DynamicIndexFacts[0].Table.Equal(path.NewPlaceholder(0).Field("items")) ||
		got.DynamicIndexFacts[0].Site != "summary.dynamic" ||
		!presence.Equal(got.DynamicIndexFacts[0].Value.KeyPresence, presence.Present()) ||
		!product.Equal(reg, got.DynamicIndexFacts[0].Value.KeyValue, present) ||
		!product.Equal(reg, got.DynamicIndexFacts[0].Value.Value, absent) ||
		got.DynamicIndexFacts[0].Value.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic-index facts = %#v, want mapped summary dynamic fact", got.DynamicIndexFacts)
	}
	if len(got.BranchProofs) != 1 ||
		got.BranchProofs[0].Kind != pathevidence.BranchProofPathEqual ||
		!got.BranchProofs[0].Path.Equal(path.NewPlaceholder(0).Field("left")) ||
		!got.BranchProofs[0].Other.Equal(path.NewPlaceholder(1).Field("right")) {
		t.Fatalf("branch proofs = %#v, want mapped summary branch proof", got.BranchProofs)
	}
	if len(got.ChannelSelects) != 1 ||
		got.ChannelSelects[0].Kind != channelselectfact.FactReceive ||
		got.ChannelSelects[0].Select != channelselectfact.ID("summary.select") ||
		!got.ChannelSelects[0].Result.Equal(path.NewPlaceholder(0).Field("result")) ||
		!got.ChannelSelects[0].Case.Equal(path.NewPlaceholder(1).Field("case")) ||
		got.ChannelSelects[0].Index != 2 {
		t.Fatalf("channel selects = %#v, want mapped summary channel select", got.ChannelSelects)
	}
	if len(got.EffectDeltas) != 1 ||
		got.EffectDeltas[0].Kind != effectdelta.Mutation ||
		got.EffectDeltas[0].Site != "summary.effect" ||
		!got.EffectDeltas[0].Target.Equal(path.NewPlaceholder(0).Field("items")) ||
		!product.Equal(reg, got.EffectDeltas[0].Value.Before, present) ||
		!product.Equal(reg, got.EffectDeltas[0].Value.After, absent) ||
		got.EffectDeltas[0].Value.Change != effectdelta.ChangeChanged {
		t.Fatalf("effect deltas = %#v, want mapped summary effect delta", got.EffectDeltas)
	}
}

func TestOutcomeProviderMapsNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(137)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 138})
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				NormalReturnParams: []product.Value{
					present,
					product.Top(),
					product.Bottom(reg),
				},
				NormalReturnParamConditions: []summary.ParamCondition{
					summary.ParamConditionTruthy,
					summary.ParamConditionFalsy,
					summary.ParamConditionTop,
				},
				NormalReturnParamEqualities: []summary.ParamEquality{
					{Left: 0, Right: 1},
				},
				ReturnConditionParamRefinements: []summary.ReturnConditionParamRefinement{
					{
						ReturnIndex: 0,
						ReturnValue: true,
						Target:      path.NewPlaceholder(0).Field("value"),
						Value:       present,
					},
				},
				ReturnPresenceRelations: []summary.ReturnPresenceRelation{
					{
						TriggerIndex:    1,
						TriggerPresence: presence.Present(),
						TargetIndex:     0,
						TargetPresence:  presence.Absent(),
					},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}), state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 ||
		!got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) ||
		!product.Equal(reg, got.ParamPathRefinements[0].Value, present) {
		t.Fatalf("param path refinements = %#v, want only useful normal-return param", got.ParamPathRefinements)
	}
	if len(got.ParamConditions) != 2 ||
		got.ParamConditions[0] != (factapply.CallParamCondition{ParamIndex: 0, Value: true}) ||
		got.ParamConditions[1] != (factapply.CallParamCondition{ParamIndex: 1, Value: false}) {
		t.Fatalf("param conditions = %#v, want truthy/falsy conditions only", got.ParamConditions)
	}
	if len(got.ParamPathRelations) != 1 ||
		got.ParamPathRelations[0].Kind != factapply.CallPathRelationEqual ||
		!got.ParamPathRelations[0].Left.Equal(path.NewPlaceholder(0)) ||
		!got.ParamPathRelations[0].Right.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("param path relations = %#v, want mapped equality", got.ParamPathRelations)
	}
	if len(got.ReturnConditionRefinements) != 1 ||
		got.ReturnConditionRefinements[0].ReturnIndex != 0 ||
		!got.ReturnConditionRefinements[0].ReturnValue ||
		!got.ReturnConditionRefinements[0].Target.Equal(path.NewPlaceholder(0).Field("value")) ||
		!product.Equal(reg, got.ReturnConditionRefinements[0].Value, present) {
		t.Fatalf("return condition refinements = %#v, want mapped refinement", got.ReturnConditionRefinements)
	}
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v, want mapped relation", got.ReturnPresenceRelations)
	}
}

func TestOutcomeProviderSpecializesGenericReturnFromArgument(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(217)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 218})
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("value", fnParam).
		Returns(typ.Instantiate(result, fnParam)).
		Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParam))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{rawReturn},
				NormalReturnFacts: summary.NormalReturnFacts{
					PathRefinements: []summary.PathValueFact{
						{Path: path.NewPlaceholder(0).Field("ok"), Value: present},
					},
				},
			},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, profile),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
		},
	}), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, profile))
	if len(got.PathRefinements) != 1 ||
		!got.PathRefinements[0].Path.Equal(path.NewPlaceholder(0).Field("ok")) ||
		!product.Equal(reg, got.PathRefinements[0].Value, present) {
		t.Fatalf("path refinements = %#v, want preserved summary fact", got.PathRefinements)
	}
}

func TestOutcomeProviderSpecializesGenericTupleReturnAcrossSlots(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(227)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 228})
	fnParamA := typ.NewTypeParam("A", nil)
	fnParamB := typ.NewTypeParam("B", nil)
	fn := typ.Func().
		TypeParamRef(fnParamA).
		TypeParamRef(fnParamB).
		Param("a", fnParamA).
		Param("b", fnParamB).
		Returns(typ.NewTuple(fnParamA, fnParamB)).
		Build()
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{Returns: []product.Value{
				providerValueForType(reg, fnParamA),
				providerValueForType(reg, fnParamB),
			}},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.LiteralInt(42)),
			2: providerValueForType(reg, typ.LiteralString("hello")),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}), state.State{}, nil)

	assertCallOutcomeResultsTypes(t, reg, got.Results, []typ.Type{
		typ.LiteralInt(42),
		typ.LiteralString("hello"),
	})
}

func TestOutcomeProviderSpecializesGenericReturnFromCallbackReturn(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(237)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 238})
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(fnParamU).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()
	callback := typ.Func().Param("item", profile).Returns(typ.String).Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParamU))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{rawReturn}},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.Instantiate(result, profile)),
			2: providerValueForType(reg, callback),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, typ.String))
}

func TestOutcomeProviderSpecializesGenericReturnFromCallbackResultReturn(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(257)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 258})
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(typ.Instantiate(result, fnParamU)).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()
	callback := typ.Func().Param("item", profile).Returns(typ.Instantiate(result, typ.Number)).Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParamU))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{rawReturn}},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.Instantiate(result, profile)),
			2: providerValueForType(reg, callback),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, typ.Number))
}

func TestOutcomeProviderMissingAndEmptySummaryYieldsZeroOutcome(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	missingKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 19})
	snap := summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{product.Top()}},
	})
	site := factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: callee})
	ctx := transfer.NodeContext{Registry: reg}

	tests := []struct {
		name     string
		provider factapply.CallOutcomeProvider
	}{
		{
			name:     "nil reader",
			provider: OutcomeProvider(ProviderConfig{KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil)}),
		},
		{
			name:     "nil key func",
			provider: OutcomeProvider(ProviderConfig{Summaries: snap}),
		},
		{
			name:     "missing key",
			provider: OutcomeProvider(ProviderConfig{Summaries: snap, KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{}, nil)}),
		},
		{
			name:     "missing summary",
			provider: OutcomeProvider(ProviderConfig{Summaries: snap, KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: missingKey}, nil)}),
		},
		{
			name: "empty returns",
			provider: OutcomeProvider(ProviderConfig{
				Summaries: summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: summary.Summary{Returns: nil}}),
				KeyFor:    ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}, nil),
			}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertEmptyOutcome(t, tc.provider(ctx, site, state.State{}, nil))
		})
	}
}

func TestByCalleeIdentitySymbolKeyMapsAreCloned(t *testing.T) {
	callee := symbol.ID(21)
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 23})

	symbolMap := map[symbol.ID]summary.SummaryKey{callee: symbolKey}
	keyFor := ByCalleeIdentity(symbolMap, nil)
	symbolMap[callee] = summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 25})

	got, ok := keyFor(transfer.NodeContext{}, factflow.NewCallProducer(factflow.CallProducerConfig{CalleeSymbol: callee}))
	if !ok || got != symbolKey {
		t.Fatalf("symbol key = %v, %v; want %v, true", got, ok, symbolKey)
	}
}

func TestByCalleeIdentityPrefersSymbolAndUsesPathWhenSymbolMissing(t *testing.T) {
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 31})
	pathKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 32})
	calleeSymbol := symbol.ID(33)
	calleePath := path.NewPath(symbol.ID(34), "module").Field("call")
	keyFor := ByCalleeIdentity(
		map[symbol.ID]summary.SummaryKey{calleeSymbol: symbolKey},
		map[path.PathKey]summary.SummaryKey{calleePath.Key(): pathKey},
	)

	got, ok := keyFor(transfer.NodeContext{}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: calleeSymbol,
		CalleePath:   calleePath,
	}))
	if !ok || got != symbolKey {
		t.Fatalf("symbol key = %v, %v; want %v, true", got, ok, symbolKey)
	}

	got, ok = keyFor(transfer.NodeContext{}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleePath: calleePath,
	}))
	if !ok || got != pathKey {
		t.Fatalf("path key = %v, %v; want %v, true", got, ok, pathKey)
	}
}

func TestOutcomeProviderIntegratesWithFactflowCallRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	calleeKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 26})
	callValue := product.Top()
	existingTargetValue := product.Absent(reg)
	target := symbol.ID(27)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), existingTargetValue),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{
					call: factflow.NewCallSite(factflow.CallSiteConfig{
						Context:      factflow.CallSiteContextAssignmentSource,
						CalleeSymbol: symbol.ID(28),
						ResultTargets: []factflow.CallResultTarget{
							factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "x")),
						},
					}),
				},
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "x"), factflow.ValueSource{
						Kind:         factflow.ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  0,
					}),
				},
			}),
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: OutcomeProvider(ProviderConfig{
				Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
					Key:     calleeKey,
					Summary: summary.Summary{Returns: []product.Value{callValue}},
				}),
				KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{symbol.ID(28): calleeKey}, nil),
			}),
		}),
	})

	assertValue(t, reg, got[call], key.SymbolValue(target), existingTargetValue)
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), callValue)
}

func TestProductionImportsAreBounded(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list imports . error = %v", err)
	}
	allowed := map[string]bool{
		"github.com/wippyai/go-lua/analysis/check/fixpoint/summary":          true,
		"github.com/wippyai/go-lua/analysis/domain/path":                     true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis":               true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness":   true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin": true,
		"github.com/wippyai/go-lua/analysis/domain/value/product":            true,
		"github.com/wippyai/go-lua/analysis/domain/value/typevalue":          true,
		"github.com/wippyai/go-lua/analysis/engine/factapply":                true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":                 true,
		"github.com/wippyai/go-lua/analysis/engine/sourcevalue":              true,
		"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact":  true,
		"github.com/wippyai/go-lua/analysis/engine/state/pathevidence":       true,
		"github.com/wippyai/go-lua/analysis/engine/state":                    true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":                 true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                          true,
		"github.com/wippyai/go-lua/analysis/lua/typecall":                    true,
		"github.com/wippyai/go-lua/analysis/symbol":                          true,
		"github.com/wippyai/go-lua/analysis/type/discriminant":               true,
		"github.com/wippyai/go-lua/analysis/type/refinement":                 true,
		"github.com/wippyai/go-lua/analysis/type/typ":                        true,
	}
	for _, imp := range strings.Fields(string(out)) {
		if !allowed[imp] {
			t.Fatalf("unexpected production import %q", imp)
		}
	}
}

func TestFactapplyDoesNotImportSummary(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", "../../../engine/factapply").Output()
	if err != nil {
		t.Fatalf("go list imports factapply error = %v", err)
	}
	for _, imp := range strings.Fields(string(out)) {
		if imp == "github.com/wippyai/go-lua/analysis/check/fixpoint/summary" {
			t.Fatalf("factapply imports summary")
		}
	}
}

func providerResultGeneric() *typ.Generic {
	param := typ.NewTypeParam("T", nil)
	okRecord := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", param).
		Build()
	errRecord := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	return typ.NewGeneric("Result", []*typ.TypeParam{param}, typ.NewUnion(okRecord, errRecord))
}

func providerProfileType() typ.Type {
	return typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()
}

func providerValueForType(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}

func providerSourceValues(reg *axis.Registry, values map[factflow.ExprRef]product.Value) sourcevalue.SourceValues {
	return sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:         reg,
		ExpressionValues: values,
	})
}

func providerExpressionSource(t *testing.T, ref factflow.ExprRef, index int) factflow.ValueSource {
	t.Helper()
	shape, ok := factflow.NewValueSourceShape(false, false, false, false)
	if !ok {
		t.Fatal("expression source shape invalid")
	}
	source, ok := factflow.NewExpressionValueSource(ref, index, index, factflow.NoValueSourceIndex, shape)
	if !ok {
		t.Fatalf("expression source for ref %d invalid", ref)
	}
	return source
}

func assertCallOutcomeResultType(t *testing.T, reg *axis.Registry, got []factapply.CallResult, want typ.Type) {
	t.Helper()
	if len(got) != 1 || got[0].Index != 0 {
		t.Fatalf("results = %#v, want one result at index 0", got)
	}
	gotType, ok := typeFromValue(reg, got[0].Value)
	if !ok {
		t.Fatalf("result value has no structural type: %#v", got[0].Value)
	}
	gotType = subst.ExpandInstantiated(gotType)
	want = subst.ExpandInstantiated(want)
	if !typ.TypeEquals(gotType, want) {
		t.Fatalf("result type = %v, want %v", gotType, want)
	}
}

func assertCallOutcomeResultsTypes(t *testing.T, reg *axis.Registry, got []factapply.CallResult, want []typ.Type) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %#v", len(got), len(want), got)
	}
	for i, wantType := range want {
		if got[i].Index != i {
			t.Fatalf("result %d index = %d, want %d", i, got[i].Index, i)
		}
		gotType, ok := typeFromValue(reg, got[i].Value)
		if !ok {
			t.Fatalf("result %d value has no structural type: %#v", i, got[i].Value)
		}
		gotType = subst.ExpandInstantiated(gotType)
		wantType = subst.ExpandInstantiated(wantType)
		if !typ.TypeEquals(gotType, wantType) {
			t.Fatalf("result %d type = %v, want %v", i, gotType, wantType)
		}
	}
}

func assertCallOutcomeResults(t *testing.T, reg *axis.Registry, got []factapply.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %#v", len(got), len(want), got)
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("result %d index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("result %d value = %#v, want %#v", i, got[i].Value, value)
		}
	}
}

func assertEmptyOutcome(t *testing.T, got factapply.CallOutcome) {
	t.Helper()
	if len(got.Results) != 0 ||
		len(got.PathRefinements) != 0 ||
		len(got.ParamPathRefinements) != 0 ||
		len(got.ParamConditions) != 0 ||
		len(got.ParamPathRelations) != 0 ||
		len(got.PathStaticMembers) != 0 ||
		len(got.DynamicIndexFacts) != 0 ||
		len(got.BranchProofs) != 0 ||
		len(got.ChannelSelects) != 0 ||
		len(got.EffectDeltas) != 0 ||
		len(got.ReturnConditionRefinements) != 0 ||
		len(got.ReturnPresenceRelations) != 0 {
		t.Fatalf("provider returned non-empty outcome: %#v", got)
	}
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	got := st.ReadValue(reg, slot)
	if !product.Equal(reg, got, want) {
		t.Fatalf("state[%v] = %#v, want %#v", slot, got, want)
	}
}
